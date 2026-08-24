package pagecache

import (
	"sync"
	"testing"
)

func TestCacheSetGet(t *testing.T) {
	c := New()
	e := Entry{HTML: []byte("hello"), ETag: `"a"`}
	c.Set("k1", e, "site", "entry:1")
	if got, ok := c.Get("k1"); !ok || string(got.HTML) != "hello" {
		t.Fatalf("Get after Set")
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss")
	}
}

func TestCacheInvalidateTag(t *testing.T) {
	c := New()
	c.Set("k1", Entry{HTML: []byte("a")}, "site", "entry:1")
	c.Set("k2", Entry{HTML: []byte("b")}, "site", "entry:2")
	c.Set("k3", Entry{HTML: []byte("c")}, "site", "content-type:post")
	c.InvalidateTag("entry:1")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("k1 should be invalidated")
	}
	if _, ok := c.Get("k2"); !ok {
		t.Fatal("k2 should remain")
	}
	if _, ok := c.Get("k3"); !ok {
		t.Fatal("k3 should remain")
	}
}

func TestCacheInvalidateTags(t *testing.T) {
	c := New()
	c.Set("k1", Entry{HTML: []byte("a")}, "site")
	c.Set("k2", Entry{HTML: []byte("b")}, "navigation")
	c.InvalidateTags("site", "navigation")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("k1 should be invalidated")
	}
	if _, ok := c.Get("k2"); ok {
		t.Fatal("k2 should be invalidated")
	}
}

func TestCacheInvalidateAll(t *testing.T) {
	c := New()
	c.Set("k1", Entry{HTML: []byte("a")}, "site")
	c.Set("k2", Entry{HTML: []byte("b")}, "site")
	c.InvalidateAll()
	if _, ok := c.Get("k1"); ok {
		t.Fatal("should be cleared")
	}
	if c.Entries() != 0 {
		t.Fatal("entries should be 0")
	}
}

func TestCacheSetSameKeyTwiceUpdatesTags(t *testing.T) {
	c := New()
	c.Set("k1", Entry{HTML: []byte("a")}, "site", "layout:A")
	// Verify layout:A has k1
	c.InvalidateTag("layout:A")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("k1 should be invalidated via layout:A")
	}
	// Re-set with different layout
	c.Set("k1", Entry{HTML: []byte("b")}, "site", "layout:B")
	// layout:A should no longer have k1
	c.InvalidateTag("layout:A")
	if _, ok := c.Get("k1"); !ok {
		t.Fatal("k1 should remain after invalidating old tag layout:A")
	}
	// layout:B should invalidate
	c.InvalidateTag("layout:B")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("k1 should be invalidated via layout:B")
	}
	// Also verify reverse index cleanup: set again with B, then InvalidateTag site should clear
	c.Set("k1", Entry{HTML: []byte("c")}, "site", "layout:B")
	c.InvalidateTag("site")
	if _, ok := c.Get("k1"); ok {
		t.Fatal("k1 should be gone after site")
	}
	// After site invalidation, layout:B index should be empty (no leak)
	// Set new key with layout:B and ensure it works
	c.Set("k2", Entry{HTML: []byte("d")}, "layout:B")
	c.InvalidateTag("layout:B")
	if _, ok := c.Get("k2"); ok {
		t.Fatal("k2 should be invalidated")
	}
}

func TestCacheConcurrentGetSetInvalidate(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			c.Set("k", Entry{HTML: []byte("v")}, "site", "entry:1")
			c.InvalidateTag("entry:1")
		}
	}()
	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_, _ = c.Get("k")
			}
		}()
	}
	wg.Wait()
}

func TestCacheGenerationPreventsStaleResurrection(t *testing.T) {
	c := New()
	// Start render for R1 and block
	block := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_, _ = c.Do("k", func() (Entry, error) {
			close(started)
			<-block
			return Entry{HTML: []byte("R1"), Tags: []string{"entry:x"}}, nil
		})
	}()
	<-started
	// Invalidate while R1 is inflight (no cached entry yet)
	c.InvalidateTag("entry:x")
	// Start second render for R2 after invalidation
	doneB := make(chan struct{})
	go func() {
		_, _ = c.Do("k", func() (Entry, error) {
			return Entry{HTML: []byte("R2"), Tags: []string{"entry:x"}}, nil
		})
		close(doneB)
	}()
	<-doneB
	// Allow R1 to finish – it must not repopulate cache with stale R1
	close(block)
	// Give scheduler time (deterministic via second Get)
	// Poll until cache has R2 or timeout
	for i := 0; i < 100; i++ {
		if e, ok := c.Get("k"); ok {
			if string(e.HTML) == "R2" {
				return
			}
			if string(e.HTML) == "R1" {
				t.Fatalf("cache resurrected stale R1 after invalidation, got R1")
			}
		}
	}
	t.Fatalf("cache should contain R2 after race")
}

func TestCachePostInvalidationDoesNotJoinStaleInflight(t *testing.T) {
	c := New()
	blockA := make(chan struct{})
	startedA := make(chan struct{})
	// Request A starts R1
	go func() {
		_, _ = c.Do("k", func() (Entry, error) {
			close(startedA)
			<-blockA
			return Entry{HTML: []byte("R1"), Tags: []string{"entry:x"}}, nil
		})
	}()
	<-startedA
	// Invalidate
	c.InvalidateTag("entry:x")
	// Request B after invalidation should not join A
	resultB := make(chan string, 1)
	go func() {
		e, _ := c.Do("k", func() (Entry, error) {
			return Entry{HTML: []byte("R2"), Tags: []string{"entry:x"}}, nil
		})
		resultB <- string(e.HTML)
	}()
	gotB := <-resultB
	if gotB != "R2" {
		t.Fatalf("B should get R2, got %s (joined stale A)", gotB)
	}
	close(blockA)
	// Ensure cache has R2, not R1
	if e, ok := c.Get("k"); ok && string(e.HTML) != "R2" {
		t.Fatalf("cache should be R2, got %s", string(e.HTML))
	}
}

func TestCacheInvalidationDuringInflightNotCached(t *testing.T) {
	c := New()
	block := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_, _ = c.Do("k", func() (Entry, error) {
			close(started)
			<-block
			return Entry{HTML: []byte("R1")}, nil
		})
	}()
	<-started
	c.InvalidateTag("site")
	close(block)
	// Give time for Do to finish
	for i := 0; i < 50; i++ {
		if _, ok := c.Get("k"); ok {
			t.Fatalf("stale result should not be cached after invalidation")
		}
	}
}
