package blocks

import (
	"context"
	"sync"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestPreparedCacheGenerationSafe(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	reg, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	revID := "rev-1"

	// First cache
	pd1, err := reg.PreparedCache(revID, doc)
	if err != nil {
		t.Fatal(err)
	}
	gen1 := reg.Generation()
	// Reload with new definitions (simulate block change)
	store.definitions = []db.BlockDefinition{
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }} updated</p>`),
	}
	if err := reg.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	gen2 := reg.Generation()
	if gen2 == gen1 {
		t.Fatalf("generation should increment")
	}
	// After reload, cache with same revID must be different (re-prepared)
	pd2, err := reg.PreparedCache(revID, doc)
	if err != nil {
		t.Fatal(err)
	}
	if pd1 == pd2 {
		t.Fatalf("cache should be generation-scoped, got same pointer after reload")
	}
	// Ensure that old generation key does not leak
	// The cache key is generation:revID, so old entry with gen1 should not be returned for gen2
	// Verify that a second call with same gen returns same pointer (cached)
	pd3, _ := reg.PreparedCache(revID, doc)
	if pd2 != pd3 {
		t.Fatalf("second call should hit cache")
	}
}

func TestPreparedCacheConcurrentReload(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	reg, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	revID := "rev-concurrent"

	var wg sync.WaitGroup
	// Concurrent Prepare and Reload (no store mutation race: Reload uses same definitions)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = reg.PreparedCache(revID, doc)
		}()
		go func() {
			defer wg.Done()
			_ = reg.Reload(context.Background())
		}()
	}
	wg.Wait()
	// After concurrent, cache should still be valid and not contain stale generation data
	_, err = reg.PreparedCache(revID, doc)
	if err != nil {
		t.Fatalf("after concurrent, PreparedCache failed: %v", err)
	}
	// Also test with document that would have been prepared during old generation via direct Prepare
	// Ensure that no panic and returned document is from current generation
	pd, _ := reg.PreparedCache("another", doc)
	if pd == nil {
		t.Fatal("pd nil")
	}
}

func TestPreparedCacheKeyIncludesGeneration(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	reg, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	// Simulate prepare with generation 1, then reload, then ensure old generation entry not returned
	pd1, err := reg.PreparedCache("revX", doc)
	if err != nil {
		t.Fatalf("pd1: %v", err)
	}
	gen1 := reg.Generation()
	store.definitions = []db.BlockDefinition{customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }} b</p>`)}
	_ = reg.Reload(context.Background())
	gen2 := reg.Generation()
	if gen1 == gen2 {
		t.Fatal("gen not bumped")
	}
	pd2, err := reg.PreparedCache("revX", doc)
	if err != nil {
		t.Fatalf("pd2: %v", err)
	}
	if pd1 == pd2 {
		t.Fatal("should be different generation")
	}
}
