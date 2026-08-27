package public

import "testing"

func TestResolveArchivePresentation(t *testing.T) {
	tests := []struct {
		name                                    string
		term                                    bool
		termTitle, termDescription, title, desc string
		wantTitle, wantDescription              string
	}{
		{name: "taxonomy is canonical", term: true, termTitle: "News", termDescription: "Latest news", title: "Products", desc: "fallback", wantTitle: "News", wantDescription: "Latest news"},
		{name: "ordinary archive uses resolved compatibility fallback", title: "Products", desc: "Catalog", wantTitle: "Products", wantDescription: "Catalog"},
		{name: "empty remains empty", wantTitle: "", wantDescription: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			title, description := resolveArchivePresentation(test.term, test.termTitle, test.termDescription, test.title, test.desc)
			if title != test.wantTitle || description != test.wantDescription {
				t.Fatalf("got (%q, %q), want (%q, %q)", title, description, test.wantTitle, test.wantDescription)
			}
		})
	}
}
