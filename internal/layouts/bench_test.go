package layouts

import (
	"testing"

	"github.com/kokosx/stratum/internal/document"
)

func BenchmarkCompose(b *testing.B) {
	layout := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "sec", Block: "core/section", Version: 1, Children: []document.Node{{ID: "slot", Block: "core/content-slot", Version: 1}}},
	}}
	entry := &document.Document{Version: 1, Nodes: []document.Node{
		{ID: "t1", Block: "core/text", Version: 1, Props: []byte(`{"text":"hello"}`)},
		{ID: "t2", Block: "core/text", Version: 1, Props: []byte(`{"text":"world"}`)},
	}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Compose(layout, entry); err != nil {
			b.Fatal(err)
		}
	}
}
