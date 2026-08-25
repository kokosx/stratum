package comments

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	count        int
	windowStart  int64
	blockedUntil int64
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*bucket)}
}

func (r *rateLimiter) Allow(key string, now int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[key]
	if !ok {
		b = &bucket{windowStart: now}
		r.buckets[key] = b
	}
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
		b = &bucket{windowStart: now}
		r.buckets[key] = b
	}
	if now-b.windowStart > 60 {
		b.windowStart = now
		b.count = 1
		return
	}
	b.count++
	if b.count >= 5 {
		b.blockedUntil = now + 300
	}
	if len(r.buckets) > 1000 {
		n := 0
		for k := range r.buckets {
			delete(r.buckets, k)
			n++
			if n >= 100 {
				break
			}
		}
	}
}

func clientIPKey(ip, entryID string) string {
	if ip == "" {
		ip = "unknown"
	}
	// Use RemoteAddr directly, not X-Forwarded-For, unless trusted proxy mechanism exists (we don't)
	// Strip port if present
	if idx := lastColon(ip); idx != -1 {
		// naive strip port, but keep IPv6 bracket handling simple
		if ip[0] == '[' {
			if end := lastBracket(ip); end != -1 {
				ip = ip[:end+1]
			}
		} else {
			ip = ip[:idx]
		}
	}
	return entryID + "|" + ip
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

func lastBracket(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ']' {
			return i
		}
	}
	return -1
}

var _ = time.Now
