package layouts

import (
	"encoding/json"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func mustDoc(t *testing.T, s string) *document.Document {
	t.Helper()
	var d document.Document
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &d
}

func TestCompose_RootSlot(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"a","block":"core/text","version":1,"props":{"text":"Hello"},"settings":{}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != 1 || out.Nodes[0].ID != "a" {
		t.Fatalf("expected entry node, got %+v", out.Nodes)
	}
	// not mutated
	if len(layout.Nodes) != 1 || layout.Nodes[0].Block != "core/content-slot" {
		t.Fatal("layout mutated")
	}
}

func TestCompose_Nested(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Hi","level":1},"settings":{}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != 1 || len(out.Nodes[0].Children) != 1 || out.Nodes[0].Children[0].ID != "h" {
		t.Fatalf("unexpected %+v", out)
	}
}

func TestCompose_DeepNested(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"grid","block":"core/grid","version":1,"props":{},"settings":{},"children":[{"id":"stack","block":"core/stack","version":1,"props":{},"settings":{},"children":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"a"},"settings":{}},{"id":"t2","block":"core/text","version":1,"props":{"text":"b"},"settings":{}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	// depth check
	got := out.Nodes[0].Children[0].Children[0].Children
	if len(got) != 2 || got[0].ID != "t1" || got[1].ID != "t2" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestCompose_MultipleRootNodes(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"s","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"title","block":"core/entry-title","version":1,"props":{},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}},{"id":"btn","block":"core/button","version":1,"props":{"label":"x","url":"/"},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"a","block":"core/text","version":1,"props":{"text":"a"},"settings":{}},{"id":"b","block":"core/heading","version":1,"props":{"text":"b","level":2},"settings":{}},{"id":"c","block":"core/image","version":1,"props":{"mediaId":"m1"},"settings":{}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	children := out.Nodes[0].Children
	if len(children) != 5 {
		t.Fatalf("expected 5 children, got %d", len(children))
	}
	if children[0].ID != "title" || children[1].ID != "a" || children[2].ID != "b" || children[3].ID != "c" || children[4].ID != "btn" {
		t.Fatalf("order wrong %+v", children)
	}
}

func TestCompose_EmptyEntry(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"title","block":"core/entry-title","version":1,"props":{},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}},{"id":"btn","block":"core/button","version":1,"props":{"label":"x","url":"/"},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	children := out.Nodes[0].Children
	if len(children) != 2 || children[0].ID != "title" || children[1].ID != "btn" {
		t.Fatalf("expected slot removed, got %+v", children)
	}
}

func TestCompose_OrderPreserved(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"a","block":"core/text","version":1,"props":{"text":"before"},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}},{"id":"b","block":"core/text","version":1,"props":{"text":"after"},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"e1","block":"core/text","version":1,"props":{"text":"e"},"settings":{}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	c := out.Nodes[0].Children
	if c[0].ID != "a" || c[1].ID != "e1" || c[2].ID != "b" {
		t.Fatalf("order wrong %v", c)
	}
}

func TestCompose_PreserveIDsProps(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{"width":"wide"},"children":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"keep","block":"core/text","version":1,"props":{"text":"hi"},"settings":{"align":"center"}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	if out.Nodes[0].ID != "sec" {
		t.Fatal("layout ID not preserved")
	}
	if out.Nodes[0].Children[0].ID != "keep" {
		t.Fatal("entry ID not preserved")
	}
}

func TestCompose_ZeroSlotRejected(t *testing.T) {
	// EPIC 2: Single templates may have zero Content Slot (fields-only). Compose should succeed and return layout doc.
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"h","block":"core/heading","version":1,"props":{"text":"hi","level":1},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[]}`)
	composed, err := Compose(layout, entry)
	if err != nil {
		t.Fatalf("unexpected error for zero slot: %v", err)
	}
	if len(composed.Nodes) != 1 || composed.Nodes[0].ID != "sec" {
		t.Fatalf("expected layout doc returned for zero slot, got %+v", composed)
	}
}

func TestCompose_TwoSlotsRejected(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}},{"id":"slot2","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[]}`)
	_, err := Compose(layout, entry)
	if err == nil {
		t.Fatal("expected error for two slots")
	}
}

func TestCompose_SlotInsideEntryRejected(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"slot2","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	_, err := Compose(layout, entry)
	if err == nil {
		t.Fatal("expected error for slot in entry")
	}
}

func TestCompose_VersionMismatch(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`)
	entry := &document.Document{Version: 2, Nodes: []document.Node{}}
	_, err := Compose(layout, entry)
	if err == nil {
		t.Fatal("expected version mismatch")
	}
}

func TestCompose_DuplicateIDs(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"dup","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"dup","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	_, err := Compose(layout, entry)
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestCompose_SlotIDReuseAllowed(t *testing.T) {
	// entry reusing slot's ID should be allowed because slot disappears
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"slot","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	out, err := Compose(layout, entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Nodes[0].Children) != 1 || out.Nodes[0].Children[0].ID != "slot" {
		t.Fatalf("unexpected %+v", out)
	}
}

func TestCompose_NotMutate(t *testing.T) {
	layout := mustDoc(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`)
	entry := mustDoc(t, `{"version":1,"nodes":[{"id":"e1","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	origLayoutJSON, _ := json.Marshal(layout)
	origEntryJSON, _ := json.Marshal(entry)
	_, err := Compose(layout, entry)
	if err != nil {
		t.Fatal(err)
	}
	afterLayout, _ := json.Marshal(layout)
	afterEntry, _ := json.Marshal(entry)
	if string(origLayoutJSON) != string(afterLayout) {
		t.Fatal("layout mutated")
	}
	if string(origEntryJSON) != string(afterEntry) {
		t.Fatal("entry mutated")
	}
}
