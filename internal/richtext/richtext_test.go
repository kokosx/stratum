package richtext

import "testing"

func TestNormalizeAndRender(t *testing.T) {
	text, err := Normalize(RichText{Version: Version, Content: []Run{
		{Text: "Hello", Marks: []Mark{{Type: "italic"}, {Type: "bold"}, {Type: "bold"}}},
		{Text: " world", Marks: []Mark{{Type: "bold"}, {Type: "italic"}}},
		{Text: ""},
		{Text: "!", Marks: []Mark{{Type: "link", Href: "/about"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(text.Content) != 2 || text.PlainText() != "Hello world!" {
		t.Fatalf("normalized text = %#v", text)
	}
	links := text.Links()
	if len(links) != 1 || links[0] != "/about" {
		t.Fatalf("links = %#v", links)
	}
	rendered, err := Render(text)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(rendered), "<em><strong>Hello world</strong></em><a href=\"/about\">!</a>"; got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestRejectsUnsafeLinksAndEscapesText(t *testing.T) {
	if _, err := Normalize(RichText{Version: Version, Content: []Run{{Text: "bad", Marks: []Mark{{Type: "link", Href: "javascript:alert(1)"}}}}}); err == nil {
		t.Fatal("unsafe link was accepted")
	}
	rendered, err := Render(RichText{Version: Version, Content: []Run{{Text: "<script>"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(rendered), "&lt;script&gt;"; got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}
