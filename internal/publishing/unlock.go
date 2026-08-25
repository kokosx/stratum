package publishing

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const unlockExpiry = 24 * time.Hour

type UnlockStore struct {
	mu     sync.RWMutex
	tokens map[string]unlockEntry
}

type unlockEntry struct {
	EntryID    string
	RevisionID string
	ExpiresAt  int64
}

func NewUnlockStore() *UnlockStore {
	return &UnlockStore{tokens: make(map[string]unlockEntry)}
}

func (s *UnlockStore) Create(entryID, revisionID string, now int64) (string, int64) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	expires := now + int64(unlockExpiry.Seconds())
	s.mu.Lock()
	defer s.mu.Unlock()
	// Clean expired opportunistically
	for k, v := range s.tokens {
		if v.ExpiresAt < now {
			delete(s.tokens, k)
		}
	}
	s.tokens[token] = unlockEntry{EntryID: entryID, RevisionID: revisionID, ExpiresAt: expires}
	return token, expires
}

func (s *UnlockStore) Valid(token, entryID, revisionID string, now int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.tokens[token]
	if !ok {
		return false
	}
	if e.ExpiresAt < now {
		return false
	}
	if e.EntryID != entryID {
		return false
	}
	// revisionID must match published revision exactly; publishing new revision invalidates old token.
	if e.RevisionID != revisionID {
		return false
	}
	return true
}

func (s *UnlockStore) CookieName(entryID string) string {
	// Scope per entry so unlocking one doesn't revoke another.
	// Use truncated hash to keep cookie name reasonable and avoid special chars.
	// entryID is base64 URL, safe for cookie name? Replace non-alphanum.
	// Keep simple: "stratum_unlock_" + entryID prefix
	// Ensure cookie name valid chars (RFC6265 token)
	// Sanitize: keep alphanum
	sanitized := ""
	for _, r := range entryID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sanitized += string(r)
		} else if r == '-' || r == '_' {
			sanitized += string(r)
		}
		if len(sanitized) >= 32 {
			break
		}
	}
	if sanitized == "" {
		sanitized = "default"
	}
	return "stratum_unlock_" + sanitized
}

// Rate limiter for password attempts: in-memory, bounded, keyed by entry + client identity.
type UnlockLimiter struct {
	mu      sync.Mutex
	buckets map[string]*limiterBucket
}

type limiterBucket struct {
	count        int
	windowStart  int64
	blockedUntil int64
}

func NewUnlockLimiter() *UnlockLimiter {
	return &UnlockLimiter{buckets: make(map[string]*limiterBucket)}
}

// Allow reports whether attempt should be allowed. If blocked, returns false.
// It records the attempt when allow is true but caller must call RecordResult for failure/success?
// Simplified: Allow checks block, then caller does password check, then Record.
func (l *UnlockLimiter) Allow(key string, now int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &limiterBucket{windowStart: now}
		l.buckets[key] = b
	}
	// If blocked
	if b.blockedUntil > now {
		return false
	}
	// Reset window if expired (1 minute window)
	if now-b.windowStart > 60 {
		b.windowStart = now
		b.count = 0
	}
	// Allow up to 5 attempts per minute, then block for 5 minutes
	if b.count >= 5 {
		b.blockedUntil = now + 300
		return false
	}
	return true
}

func (l *UnlockLimiter) Record(key string, success bool, now int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &limiterBucket{windowStart: now}
		l.buckets[key] = b
	}
	if success {
		// Reset on success
		b.count = 0
		b.blockedUntil = 0
		b.windowStart = now
		return
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
	// Bound map size: if > 1000 entries, prune oldest
	if len(l.buckets) > 1000 {
		// simple prune: delete 100 random
		n := 0
		for k := range l.buckets {
			delete(l.buckets, k)
			n++
			if n >= 100 {
				break
			}
		}
	}
}

func ClientKey(entryID, ip string) string {
	return entryID + "|" + ip
}
