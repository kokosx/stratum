package wordpress

import (
	"path/filepath"
	"testing"
)

func TestParseWXRStreaming(t *testing.T) {
	var items []item
	var terms []term
	var authors []author
	err := parse(filepath.Join("testdata", "basic.xml"), func(i item) error { items = append(items, i); return nil }, func(v term) error { terms = append(terms, v); return nil }, func(v author) error { authors = append(authors, v); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "1" || items[0].Comments != 1 {
		t.Fatalf("unexpected items: %#v", items)
	}
	if len(terms) != 1 || terms[0].Slug != "news" {
		t.Fatalf("unexpected terms: %#v", terms)
	}
	if len(authors) != 1 || authors[0].Email != "author@example.test" {
		t.Fatalf("unexpected authors: %#v", authors)
	}
}

func TestHTMLToSDTHeadingAndRichText(t *testing.T) {
	doc, err := htmlDocument("<h2>Heading</h2><p>One <strong>two</strong> <a href=\"https://example.test\">three</a></p><script>alert(1)</script>", nil, new([]string))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 2 || doc.Nodes[0].Block != "core/heading" || doc.Nodes[1].Block != "core/text" {
		t.Fatalf("unexpected nodes: %#v", doc.Nodes)
	}
}

func TestMediaSSRFRejected(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/a", "http://[::1]/a", "http://192.168.1.2/a"} {
		if _, _, err := download(t.Context(), raw); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

func TestShortcodeNotExecuted(t *testing.T) {
	warnings := []string{}
	got := stripShortcodes("before [gallery id=1] after", &warnings)
	if got != "before  after" || len(warnings) != 1 {
		t.Fatalf("got %q %#v", got, warnings)
	}
}
