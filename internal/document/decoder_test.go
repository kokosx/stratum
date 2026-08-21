package document

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestDecodeRejectsUnsupportedDocumentVersion(t *testing.T) {
	_, err := Decode([]byte(`{"version":2,"nodes":[]}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported document version") {
		t.Fatalf("Decode() error = %v, want unsupported version", err)
	}
}

func TestDecodeRejectsDuplicateNodeIDs(t *testing.T) {
	_, err := Decode([]byte(`{"version":1,"nodes":[{"id":"same","block":"core/text","version":1},{"id":"parent","block":"core/text","version":1,"children":[{"id":"same","block":"core/text","version":1}]}]}`))
	if err == nil || !strings.Contains(err.Error(), `duplicate node id "same"`) {
		t.Fatalf("Decode() error = %v, want duplicate node ID", err)
	}
}

func TestDecodeRejectsExcessiveDepth(t *testing.T) {
	node := Node{ID: "leaf", Block: "core/text", Version: 1}
	for depth := 0; depth < maxDocumentDepth; depth++ {
		node = Node{ID: "node-" + strconv.Itoa(depth), Block: "core/text", Version: 1, Children: []Node{node}}
	}
	data, err := json.Marshal(Document{Version: 1, Nodes: []Node{node}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decode(data)
	if err == nil || !strings.Contains(err.Error(), "maximum document depth") {
		t.Fatalf("Decode() error = %v, want maximum depth", err)
	}
}
