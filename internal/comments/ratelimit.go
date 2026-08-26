package comments

import (
	"net"
	"sync"
)

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	count        int
	windowStart  int64
	blockedUntil int64
	lastSeen     int64
}

const maxBuckets = 1000

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*bucket)}
}

func (r *rateLimiter) Allow(key string, now int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evictInactive(now)
	b, ok := r.buckets[key]
	if !ok {
		if len(r.buckets) >= maxBuckets {
			// A rejected/invalid request must not be able to grow this map.
			return false
		}
		b = &bucket{windowStart: now, lastSeen: now}
		r.buckets[key] = b
	}
	b.lastSeen = now
	if b.blockedUntil > now {
		return false
	}
	if now-b.windowStart > 60 {
		b.windowStart = now
		b.count = 0
	}
	if b.count >= 5 {
		b.blockedUntil = now + 300
		return false
	}
	return true
}

func (r *rateLimiter) Record(key string, now int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[key]
	if !ok {
		r.evictInactive(now)
		if len(r.buckets) >= maxBuckets {
			return
		}
		b = &bucket{windowStart: now, lastSeen: now}
		r.buckets[key] = b
	}
	b.lastSeen = now
	if now-b.windowStart > 60 {
		b.windowStart = now
		b.count = 1
		return
	}
	b.count++
	if b.count >= 5 {
		b.blockedUntil = now + 300
	}
	r.evictInactive(now)
}

func (r *rateLimiter) evictInactive(now int64) {
	if len(r.buckets) < maxBuckets {
		return
	}
	for key, b := range r.buckets {
		if now-b.lastSeen > 600 && b.blockedUntil <= now {
			delete(r.buckets, key)
		}
	}
}

func clientIPKey(ip, entryID string) string {
	if ip == "" {
		ip = "unknown"
	}
	// RemoteAddr is authoritative until explicit trusted-proxy support exists.
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return entryID + "|" + ip
}
