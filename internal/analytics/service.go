package analytics

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kokosx/stratum/internal/site"
)

// Service is the analytics core service. It holds a bounded queue and a single worker
// that aggregates observations in memory and periodically flushes to SQLite.
type Service struct {
	db    *sql.DB
	store *Store
	site  *site.Runtime
	queue chan Observation
	aggs  *aggregates

	accepted  atomic.Uint64
	dropped   atomic.Uint64
	flushErr  atomic.Uint64
	lastFlush atomic.Value // time.Time

	mu sync.Mutex // protects aggs for fallback but worker is primary

	stopCh  chan struct{}
	done    chan struct{}
	clearCh chan clearReq

	wg sync.WaitGroup

	enabledCache atomic.Bool // cached from site runtime for hot path? but we check site snapshot directly for correctness with immediate reload

}

type clearReq struct {
	done chan error
}

func New(database *sql.DB, siteRuntime *site.Runtime) *Service {
	s := &Service{
		db:      database,
		store:   NewStore(database),
		site:    siteRuntime,
		queue:   make(chan Observation, QueueSize),
		aggs:    newAggregates(),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		clearCh: make(chan clearReq),
	}
	if siteRuntime != nil {
		if snap := siteRuntime.Current(); snap != nil {
			// initial sync
			s.enabledCache.Store(snap.AnalyticsEnabled)
		}
	}
	s.wg.Add(1)
	go s.loop()
	return s
}

// Enabled returns whether analytics is enabled per current runtime snapshot.
// Hot path should call this first to avoid classification.
func (s *Service) Enabled() bool {
	if s.site == nil {
		return false
	}
	snap := s.site.Current()
	if snap == nil {
		return true // default enabled
	}
	return snap.AnalyticsEnabled
}

// Record enqueues an observation non-blocking. Returns true if accepted, false if dropped/ disabled.
// Must be extremely cheap when disabled.
func (s *Service) Record(obs Observation) bool {
	if !s.Enabled() {
		return false
	}
	// Sanitize already done by caller? Ensure.
	SanitizeObservation(&obs)
	select {
	case s.queue <- obs:
		s.accepted.Add(1)
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

// Health snapshots
type Health struct {
	Accepted     uint64
	Dropped      uint64
	FlushErrors  uint64
	LastFlush    time.Time
	QueueLen     int
	QueueCap     int
	PendingCount int
}

func (s *Service) Health() Health {
	var last time.Time
	if v := s.lastFlush.Load(); v != nil {
		last = v.(time.Time)
	}
	s.aggs.mu.Lock()
	pending := s.aggs.count()
	s.aggs.mu.Unlock()
	return Health{
		Accepted:     s.accepted.Load(),
		Dropped:      s.dropped.Load(),
		FlushErrors:  s.flushErr.Load(),
		LastFlush:    last,
		QueueLen:     len(s.queue),
		QueueCap:     cap(s.queue),
		PendingCount: pending,
	}
}

func (s *Service) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()
	retentionTicker := time.NewTicker(1 * time.Hour)
	defer retentionTicker.Stop()

	for {
		select {
		case obs := <-s.queue:
			s.aggs.mu.Lock()
			s.aggs.add(obs)
			shouldFlush := s.aggs.count() >= FlushThreshold
			s.aggs.mu.Unlock()
			if shouldFlush {
				s.flush()
			}
		case <-ticker.C:
			s.flush()
		case <-retentionTicker.C:
			s.runRetention()
		case req := <-s.clearCh:
			// discard pending aggregates
			s.aggs.mu.Lock()
			s.aggs.clear()
			// drain queue: discard any queued observations
			drained := 0
			for {
				select {
				case <-s.queue:
					drained++
				default:
					goto afterDrain
				}
			}
		afterDrain:
			s.aggs.mu.Unlock()
			// Clear DB
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := s.store.Clear(ctx)
			cancel()
			if err != nil {
				log.Printf("analytics clear: %v", err)
			}
			// also clear queue counts? not needed
			_ = drained
			req.done <- err
			close(req.done)
		case <-s.stopCh:
			// drain queue and flush
			for {
				select {
				case obs := <-s.queue:
					s.aggs.mu.Lock()
					s.aggs.add(obs)
					s.aggs.mu.Unlock()
				default:
					s.flush()
					close(s.done)
					return
				}
			}
		}
	}
}

func (s *Service) flush() {
	s.aggs.mu.Lock()
	site, page, dim, trans := s.aggs.snapshotAndReset()
	s.aggs.mu.Unlock()
	if len(site) == 0 && len(page) == 0 && len(dim) == 0 && len(trans) == 0 {
		s.lastFlush.Store(time.Now())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.Flush(ctx, site, page, dim, trans); err != nil {
		s.flushErr.Add(1)
		log.Printf("analytics flush: %v", err)
		// We drop data on flush error (bounded memory, do not requeue unbounded)
		return
	}
	s.lastFlush.Store(time.Now())
}

func (s *Service) runRetention() {
	if s.site == nil {
		return
	}
	snap := s.site.Current()
	if snap == nil {
		return
	}
	// Defaults already handled in runtime
	ret := snap.AnalyticsRetentionDays
	hourly := snap.AnalyticsHourlyRetentionDays
	if ret == 0 {
		ret = 730
	}
	if hourly == 0 {
		hourly = 90
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.store.Retention(ctx, ret, hourly)
}

// Clear synchronously clears pending state and DB tables. Coordinates with worker.
func (s *Service) Clear(ctx context.Context) error {
	req := clearReq{done: make(chan error, 1)}
	select {
	case s.clearCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops worker, flushes remaining, with timeout.
func (s *Service) Close() error {
	// idempotent
	select {
	case <-s.stopCh:
		// already closed
		return nil
	default:
		close(s.stopCh)
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return nil // bounded shutdown, don't keep process alive
	}
}

// For tests: FlushSync forces immediate flush synchronously.
func (s *Service) FlushSync(ctx context.Context) error {
	// Drain queue synchronously before flush
	for {
		select {
		case obs := <-s.queue:
			s.aggs.mu.Lock()
			s.aggs.add(obs)
			s.aggs.mu.Unlock()
		default:
			goto doFlush
		}
	}
doFlush:
	s.flush()
	return nil
}

// For tests: Pending returns pending count.
func (s *Service) Pending() int {
	s.aggs.mu.Lock()
	defer s.aggs.mu.Unlock()
	return s.aggs.count()
}
