package wordpress

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/richtext"
)

var gutenbergOpen = regexp.MustCompile(`(?s)<!--\s*wp:[^>]*-->`)
var gutenbergClose = regexp.MustCompile(`(?s)<!--\s*/wp:[^>]*-->`)

func stripGutenberg(s string) string {
	s = gutenbergOpen.ReplaceAllString(s, "")
	s = gutenbergClose.ReplaceAllString(s, "")
	return s
}

func stripShortcodes(s string, w *[]string) string {
	if strings.Contains(s, "[") {
		*w = append(*w, "WordPress shortcodes removed")
	}
	for {
		a := strings.Index(s, "[")
		if a < 0 {
			return s
		}
		b := strings.Index(s[a:], "]")
		if b < 0 {
			return s[:a]
		}
		s = s[:a] + s[a+b+1:]
	}
}

func htmlDocument(source string, imageIDs map[string]string, warnings *[]string) (*document.Document, error) {
	source = stripGutenberg(source)
	source = stripShortcodes(source, warnings)
	n, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil, err
	}
	doc := &document.Document{Version: 1}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.CommentNode {
			return
		}
		if node.Type != html.ElementNode {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				visit(c)
			}
			return
		}
		tag := strings.ToLower(node.Data)
		switch tag {
		case "script", "style", "iframe", "object", "embed":
			*warnings = append(*warnings, "dangerous HTML removed: "+tag)
			return
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(tag[1] - '0')
			doc.Nodes = append(doc.Nodes, richNode("core/heading", map[string]any{"text": inline(node), "level": level}))
		case "p":
			if text := inline(node); len(text.Content) > 0 {
				doc.Nodes = append(doc.Nodes, richNode("core/text", map[string]any{"text": text}))
			} else if strings.TrimSpace(textContent(node)) != "" {
				// fallback plain text if inline normalization removed content
				plain := strings.TrimSpace(textContent(node))
				doc.Nodes = append(doc.Nodes, richNode("core/text", map[string]any{"text": richtext.RichText{Version: richtext.Version, Content: []richtext.Run{{Text: plain}}}}))
			}
		case "ul", "ol":
			var items []string
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && strings.ToLower(c.Data) == "li" {
					t := strings.TrimSpace(textContent(c))
					if t != "" {
						items = append(items, t)
					}
				}
			}
			if len(items) > 0 {
				ordered := tag == "ol"
				doc.Nodes = append(doc.Nodes, listNode(strings.Join(items, "\n"), ordered))
			} else {
				// empty list, recurse
				for c := node.FirstChild; c != nil; c = c.NextSibling {
					visit(c)
				}
			}
		case "blockquote":
			text, citation := extractQuote(node)
			if strings.TrimSpace(text) != "" {
				doc.Nodes = append(doc.Nodes, quoteNode(text, citation))
			}
		case "hr":
			doc.Nodes = append(doc.Nodes, dividerNode())
		case "img":
			src, alt := attr(node, "src"), attr(node, "alt")
			if id := imageIDs[src]; id != "" {
				doc.Nodes = append(doc.Nodes, richNode("core/image", map[string]any{"mediaId": id, "alt": alt}))
			} else if src != "" {
				*warnings = append(*warnings, "image omitted because its attachment was not imported: "+src)
			}
		case "br":
			// treat as paragraph break - handled by parent text extraction
			// create a text node with newline if inside p? For now ignore, will be flattened.
			return
		case "html", "head", "body", "div", "section", "article", "main", "span", "a", "strong", "b", "em", "i", "code", "s", "del", "li":
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				visit(c)
			}
		default:
			// For unsupported safe semantic content, flatten to text if it contains text
			// To avoid losing content, recurse; script etc already handled
			// If tag contains meaningful text like <pre>, <code>, etc, fallback to text
			hasElementChild := false
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode {
					hasElementChild = true
					break
				}
			}
			if !hasElementChild {
				txt := strings.TrimSpace(textContent(node))
				if txt != "" {
					doc.Nodes = append(doc.Nodes, richNode("core/text", map[string]any{"text": richtext.RichText{Version: richtext.Version, Content: []richtext.Run{{Text: txt}}}}))
					return
				}
			}
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				visit(c)
			}
		}
	}
	visit(n)
	if len(doc.Nodes) == 0 {
		text := strings.TrimSpace(textContent(n))
		if text != "" {
			doc.Nodes = append(doc.Nodes, richNode("core/text", map[string]any{"text": richtext.RichText{Version: richtext.Version, Content: []richtext.Run{{Text: text}}}}))
		}
	}
	if err := document.Validate(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func richNode(block string, props map[string]any) document.Node {
	b, _ := json.Marshal(props)
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	version := 2
	if block == "core/image" || block == "core/list" || block == "core/quote" || block == "core/divider" {
		version = 1
	}
	return document.Node{ID: base64.RawURLEncoding.EncodeToString(id), Block: block, Version: version, Props: b, Settings: json.RawMessage(`{}`)}
}

func listNode(items string, ordered bool) document.Node {
	props := map[string]any{"items": items}
	settings := map[string]any{"ordered": ordered}
	pb, _ := json.Marshal(props)
	sb, _ := json.Marshal(settings)
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	return document.Node{ID: base64.RawURLEncoding.EncodeToString(id), Block: "core/list", Version: 1, Props: pb, Settings: sb}
}

func quoteNode(text, citation string) document.Node {
	props := map[string]any{"text": text, "citation": citation}
	b, _ := json.Marshal(props)
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	return document.Node{ID: base64.RawURLEncoding.EncodeToString(id), Block: "core/quote", Version: 1, Props: b, Settings: json.RawMessage(`{}`)}
}

func dividerNode() document.Node {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	return document.Node{ID: base64.RawURLEncoding.EncodeToString(id), Block: "core/divider", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`)}
}

func extractQuote(n *html.Node) (string, string) {
	var textParts []string
	var citation string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (strings.ToLower(c.Data) == "cite" || strings.ToLower(c.Data) == "footer") {
			citation = strings.TrimSpace(textContent(c))
			continue
		}
		if c.Type == html.TextNode {
			textParts = append(textParts, c.Data)
		} else if c.Type == html.ElementNode {
			textParts = append(textParts, textContent(c))
		}
	}
	text := strings.TrimSpace(strings.Join(textParts, " "))
	if text == "" {
		text = strings.TrimSpace(textContent(n))
		if citation != "" {
			// remove citation from text if it was included
			text = strings.TrimSpace(strings.Replace(text, citation, "", 1))
		}
	}
	return text, citation
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func inline(n *html.Node) richtext.RichText {
	out := richtext.RichText{Version: richtext.Version}
	var walk func(*html.Node, []richtext.Mark)
	walk = func(x *html.Node, marks []richtext.Mark) {
		if x.Type == html.TextNode {
			if x.Data != "" {
				out.Content = append(out.Content, richtext.Run{Text: x.Data, Marks: append([]richtext.Mark(nil), marks...)})
			}
			return
		}
		if x.Type == html.CommentNode {
			return
		}
		next := marks
		switch strings.ToLower(x.Data) {
		case "strong", "b":
			next = append(next, richtext.Mark{Type: "bold"})
		case "em", "i":
			next = append(next, richtext.Mark{Type: "italic"})
		case "s", "del", "strike":
			next = append(next, richtext.Mark{Type: "strike"})
		case "code":
			next = append(next, richtext.Mark{Type: "code"})
		case "a":
			if href := attr(x, "href"); href != "" {
				next = append(next, richtext.Mark{Type: "link", Href: href})
			}
		case "br":
			out.Content = append(out.Content, richtext.Run{Text: "\n", Marks: append([]richtext.Mark(nil), marks...)})
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c, next)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, nil)
	}
	normalized, _ := richtext.Normalize(out)
	return normalized
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		if x.Type == html.ElementNode && strings.ToLower(x.Data) == "br" {
			b.WriteString("\n")
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
