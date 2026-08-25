package wordpress

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/richtext"
)

var gutenbergOpen = regexp.MustCompile(`(?s)<!--\s*wp:[^>]*-->`)
var gutenbergClose = regexp.MustCompile(`(?s)<!--\s*/wp:[^>]*-->`)

// openShortcode matches an opening/self-closing tag: [name] or [name attrs].
var openShortcode = regexp.MustCompile(`\[([a-zA-Z_][a-zA-Z0-9_-]{1,})([^\]\n]*)\]`)

// strayCloser matches leftover closing tags whose opener was not recognized.
var strayCloser = regexp.MustCompile(`\[/[a-zA-Z_][a-zA-Z0-9_]{1,}\]`)

// knownShortcodes are common WordPress core/plugin shortcodes safe to drop even
// without attributes. Unknown bare words in brackets stay untouched prose.
var knownShortcodes = map[string]bool{
	"gallery": true, "caption": true, "wp_caption": true, "audio": true,
	"video": true, "playlist": true, "embed": true, "contact-form-7": true,
	"elementor-template": true, "vc_row": true, "vc_column": true,
	"products": true, "woocommerce_cart": true,
}

func isLikelyShortcodeBare(name string) bool {
	return knownShortcodes[strings.ToLower(name)]
}

// stripGutenberg removes editor serialization comments.
func stripGutenberg(s string) string {
	s = gutenbergOpen.ReplaceAllString(s, "")
	s = gutenbergClose.ReplaceAllString(s, "")
	return s
}

// stripShortcodes removes likely WordPress shortcodes WITHOUT executing them:
//   - paired   [foo]Hello[/foo]      -> Hello          (text preserved)
//   - attribs  [contact-form-7 id="1"] -> removed
//   - bare     [gallery]             -> removed (known list)
//   - prose    Value [optional]… / Array index [0]  -> unchanged
//
// Every actual removal appends one warning.
func stripShortcodes(s string, w *[]string) string {
	if !strings.Contains(s, "[") {
		return s
	}
	var removed bool

	// PASS 1: paired forms [name ...]inner[/name] -> inner (RE2 lacks backrefs,
	// so pairs are resolved by scanning for the matching closer manually).
	for {
		loc := openShortcode.FindStringSubmatchIndex(s)
		if loc == nil {
			break
		}
		name := s[loc[2]:loc[3]]
		closer := "[/" + name + "]"
		ci := strings.Index(s[loc[1]:], closer)
		if ci < 0 {
			// Unpaired opener: fall through to bare/attribution rules below.
			break
		}
		start, innerEnd := loc[0], loc[1]+ci
		end := innerEnd + len(closer)
		s = s[:start] + s[loc[1]:innerEnd] + s[end:]
		removed = true
	}

	// PASS 2: self-closing tags WITH attributes, e.g. [contact-form-7 id="123"].
	out := openShortcode.ReplaceAllStringFunc(s, func(m string) string {
		parts := openShortcode.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		name, attrs := parts[1], strings.TrimSpace(parts[2])
		switch {
		case attrs != "":
			removed = true // attribute-carrying tag: machine syntax, drop it
			return ""
		case isLikelyShortcodeBare(name):
			removed = true // known bare shortcode, e.g. [gallery]
			return ""
		default:
			return m // plain bracketed prose like [optional]; preserve
		}
	})

	// PASS 3: stray closers whose opener was not recognized.
	out = strayCloser.ReplaceAllStringFunc(out, func(m string) string {
		removed = true
		return ""
	})

	if removed && w != nil {
		*w = append(*w, "WordPress shortcodes removed")
	}
	return out
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

func newNodeID() (string, error) {
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("generate node id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(id), nil
}

func richNode(block string, props map[string]any) document.Node {
	b, _ := json.Marshal(props)
	nodeID, err := newNodeID()
	if err != nil {
		panic(err) // unreachable in practice; entropy failure is not recoverable
	}
	version := 2
	if block == "core/image" || block == "core/list" || block == "core/quote" || block == "core/divider" {
		version = 1
	}
	return document.Node{ID: nodeID, Block: block, Version: version, Props: b, Settings: json.RawMessage(`{}`)}
}

func listNode(items string, ordered bool) document.Node {
	props := map[string]any{"items": items}
	settings := map[string]any{"ordered": ordered}
	pb, _ := json.Marshal(props)
	sb, _ := json.Marshal(settings)
	nodeID, err := newNodeID()
	if err != nil {
		panic(err)
	}
	return document.Node{ID: nodeID, Block: "core/list", Version: 1, Props: pb, Settings: sb}
}

func quoteNode(text, citation string) document.Node {
	props := map[string]any{"text": text, "citation": citation}
	b, _ := json.Marshal(props)
	nodeID, err := newNodeID()
	if err != nil {
		panic(err)
	}
	return document.Node{ID: nodeID, Block: "core/quote", Version: 1, Props: b, Settings: json.RawMessage(`{}`)}
}

func dividerNode() document.Node {
	nodeID, err := newNodeID()
	if err != nil {
		panic(err)
	}
	return document.Node{ID: nodeID, Block: "core/divider", Version: 1, Props: json.RawMessage(`{}`), Settings: json.RawMessage(`{}`)}
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
