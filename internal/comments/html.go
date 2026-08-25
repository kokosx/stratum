package comments

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var tagStrip = regexp.MustCompile(`(?i)<[^>]*>`)

// HTMLToText converts WordPress comment HTML to plain text, preserving paragraphs.
func HTMLToText(s string) string {
	// Remove script/style
	s = regexp.MustCompile(`(?is)<script.*?</script>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?is)<style.*?</style>`).ReplaceAllString(s, "")

	// Use html parser for proper handling
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		// fallback: strip tags
		t := tagStrip.ReplaceAllString(s, "")
		t = strings.ReplaceAll(t, "\r\n", "\n")
		return strings.TrimSpace(t)
	}
	var b strings.Builder
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, isBlock bool) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			return
		}
		if node.Type == html.CommentNode {
			return
		}
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			switch tag {
			case "p", "div", "br", "blockquote", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr":
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				for c := node.FirstChild; c != nil; c = c.NextSibling {
					walk(c, true)
				}
				if !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				return
			case "script", "style", "iframe", "object", "embed":
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c, false)
		}
	}
	walk(n, false)
	text := strings.TrimSpace(b.String())
	// Collapse multiple newlines
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return text
}
