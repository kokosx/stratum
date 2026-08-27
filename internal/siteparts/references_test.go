package siteparts

import (
	"encoding/json"
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func TestCollectSitePartRefsUsesExactSDTTraversal(t *testing.T) {
	doc := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "text", Block: "core/text", Version: 1, Props: json.RawMessage(`{"text":"part-a appears in ordinary text"}`)},
		{ID: "one", Block: "core/site-part", Version: 1, Settings: json.RawMessage(`{"sitePartId":"part-a"}`)},
		{ID: "container", Block: "core/section", Version: 1, Children: []document.Node{
			{ID: "duplicate", Block: "core/site-part", Version: 1, Settings: json.RawMessage(`{"sitePartId":"part-a"}`)},
			{ID: "other", Block: "core/site-part", Version: 1, Settings: json.RawMessage(`{"sitePartId":"part-ab"}`)},
		}},
	}}
	refs := CollectSitePartRefs(doc)
	if len(refs) != 2 || refs[0] != "part-a" || refs[1] != "part-ab" {
		t.Fatalf("refs = %#v", refs)
	}
	if !ReferencesSitePart(doc, "part-a") || ReferencesSitePart(doc, "part") {
		t.Fatal("reference matching must be exact")
	}
}
