package publishing

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/kokosx/stratum/internal/runtimehub"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Scheduler runs durable scheduled publications as part of stratum serve.
type Scheduler struct {
	db       *sql.DB
	queries  *db.Queries
	hub      *runtimehub.Runtime
	interval time.Duration
	stopCh   chan struct{}
}

func NewScheduler(database *sql.DB, queries *db.Queries) *Scheduler {
	return &Scheduler{
		db: database, queries: queries,
		interval: 15 * time.Second,
		stopCh:   make(chan struct{}),
	}
}

func NewSchedulerWithHub(database *sql.DB, queries *db.Queries, hub *runtimehub.Runtime) *Scheduler {
	return &Scheduler{
		db: database, queries: queries, hub: hub,
		interval: 15 * time.Second,
		stopCh:   make(chan struct{}),
	}
}

func (s *Scheduler) SetHub(hub *runtimehub.Runtime) { s.hub = hub }

// SetInterval overrides ticker interval (for tests).
func (s *Scheduler) SetInterval(d time.Duration) { s.interval = d }

// Start runs the scheduler: startup catch-up then ticker until ctx done.
func (s *Scheduler) Start(ctx context.Context) {
	if err := s.RunDue(ctx, time.Now().Unix()); err != nil {
		log.Printf("publishing scheduler startup catch-up: %v", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.RunDue(ctx, time.Now().Unix()); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Printf("publishing scheduler tick: %v", err)
				}
			}
		}
	}
}

// Stop signals the scheduler to stop (for graceful shutdown without context).
func (s *Scheduler) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

// RunDue publishes all due scheduled jobs whose scheduled_at <= now.
// It is synchronous and deterministic for tests. It uses per-job transactions for idempotency and crash safety.
func (s *Scheduler) RunDue(ctx context.Context, now int64) error {
	jobs, err := s.queries.ListDuePublicationJobs(ctx, now)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	for _, job := range jobs {
		if err := s.runOne(ctx, job, now); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			log.Printf("publishing scheduler job %s (entry %s rev %s) failed: %v", job.ID, job.EntryID, job.RevisionID, err)
		}
	}
	return nil
}

func (s *Scheduler) runOne(ctx context.Context, job db.PublicationJob, now int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	current, err := qtx.GetPublicationJob(ctx, job.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if current.Status != "scheduled" {
		return nil
	}
	if current.ScheduledAt > now {
		return nil
	}
	entry, err := qtx.GetEntry(ctx, current.EntryID)
	if err != nil {
		origErr := errors.New("entry not found")
		if markErr := s.markFailedWithTx(ctx, qtx, current.ID, now, "entry not found"); markErr != nil {
			log.Printf("publishing scheduler: failed to mark job %s as failed after %v: %v", current.ID, origErr, markErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			log.Printf("publishing scheduler: commit after marking job %s failed (original %v) failed: %v", current.ID, origErr, commitErr)
			return commitErr
		}
		return origErr
	}
	if entry.Status == "trash" {
		origErr := errors.New("entry is in trash")
		if markErr := s.markFailedWithTx(ctx, qtx, current.ID, now, "entry is in trash"); markErr != nil {
			log.Printf("publishing scheduler: failed to mark job %s as failed after %v: %v", current.ID, origErr, markErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			log.Printf("publishing scheduler: commit after marking job %s failed (original %v) failed: %v", current.ID, origErr, commitErr)
			return commitErr
		}
		return origErr
	}
	rev, err := qtx.GetEntryRevision(ctx, current.RevisionID)
	if err != nil {
		origErr := errors.New("revision not found")
		if markErr := s.markFailedWithTx(ctx, qtx, current.ID, now, "revision not found"); markErr != nil {
			log.Printf("publishing scheduler: failed to mark job %s as failed after %v: %v", current.ID, origErr, markErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			log.Printf("publishing scheduler: commit after marking job %s failed (original %v) failed: %v", current.ID, origErr, commitErr)
			return commitErr
		}
		return origErr
	}
	if rev.EntryID != current.EntryID {
		origErr := errors.New("revision does not belong to entry")
		if markErr := s.markFailedWithTx(ctx, qtx, current.ID, now, "revision does not belong to entry"); markErr != nil {
			log.Printf("publishing scheduler: failed to mark job %s as failed after %v: %v", current.ID, origErr, markErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			log.Printf("publishing scheduler: commit after marking job %s failed (original %v) failed: %v", current.ID, origErr, commitErr)
			return commitErr
		}
		return origErr
	}
	if err := s.publishWithQueries(ctx, qtx, entry, rev, now); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if markErr := s.markFailedWithTx(ctx, qtx, current.ID, now, err.Error()); markErr != nil {
			log.Printf("publishing scheduler: failed to mark job %s as failed after publish error %v: %v", current.ID, err, markErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			log.Printf("publishing scheduler: commit after marking job %s failed (publish error %v) failed: %v", current.ID, err, commitErr)
			return commitErr
		}
		return err
	}
	if err := qtx.UpdatePublicationJobStatus(ctx, db.UpdatePublicationJobStatusParams{
		Status: "completed", UpdatedAt: now, LastError: sql.NullString{}, ID: current.ID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		log.Printf("publishing scheduler: commit for job %s after publish failed: %v", current.ID, err)
		return err
	}
	if s.hub != nil {
		if err := s.hub.Routes.Reload(context.Background()); err != nil {
			log.Printf("publishing scheduler: post-publish route reload failed for job %s (entry %s rev %s): %v (DB remains source of truth)", current.ID, entry.ID, rev.ID, err)
		}
		s.hub.Pages.InvalidateAll()
		s.hub.Sitemap.Invalidate()
		s.hub.Feed.Invalidate()
	}
	log.Printf("publishing scheduler: published entry %s revision %s (job %s)", entry.ID, rev.ID, job.ID)
	return nil
}

func (s *Scheduler) markFailedWithTx(ctx context.Context, qtx *db.Queries, jobID string, now int64, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return qtx.UpdatePublicationJobStatus(ctx, db.UpdatePublicationJobStatusParams{
		Status: "failed", UpdatedAt: now, LastError: sql.NullString{String: reason, Valid: true}, ID: jobID,
	})
}

func (s *Scheduler) publishWithQueries(ctx context.Context, qtx *db.Queries, entry db.Entry, rev db.EntryRevision, now int64) error {
	return PublishWithQueries(ctx, qtx, entry, rev, now)
}
