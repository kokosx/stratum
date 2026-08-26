package comments

import "testing"

func TestRateLimiterHardCapAndCleanupInAllow(t *testing.T) {
	r := newRateLimiter()
	for i := 0; i < maxBuckets; i++ {
		if !r.Allow(string(rune(i)), 1) {
			t.Fatalf("bucket %d unexpectedly rejected", i)
		}
	}
	if r.Allow("overflow", 1) {
		t.Fatal("expected hard-cap rejection")
	}
	if got := len(r.buckets); got != maxBuckets {
		t.Fatalf("bucket count = %d, want %d", got, maxBuckets)
	}
	if !r.Allow("after-cleanup", 700) {
		t.Fatal("expected cleanup to make room")
	}
	if got := len(r.buckets); got > maxBuckets {
		t.Fatalf("bucket count = %d exceeds cap", got)
	}
}
