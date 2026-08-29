package creator

import (
	"encoding/json"
	"fmt"

	"github.com/kokosx/stratum/internal/document"
)

type docBuilder struct {
	prefix string
	next   int
}

func (b *docBuilder) node(block string, version int, props, settings map[string]any, children ...document.Node) document.Node {
	b.next++
	return document.Node{ID: fmt.Sprintf("%s-%d", b.prefix, b.next), Block: block, Version: version, Props: raw(props), Settings: raw(settings), Children: children}
}

func raw(value map[string]any) json.RawMessage {
	if value == nil {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func richText(text string) map[string]any {
	return map[string]any{"version": 1, "content": []any{map[string]any{"text": text}}}
}

func (b *docBuilder) text(text string) document.Node {
	return b.node("core/text", 2, map[string]any{"text": richText(text)}, nil)
}

func (b *docBuilder) heading(text string, level int) document.Node {
	return b.node("core/heading", 2, map[string]any{"text": richText(text), "level": level}, nil)
}

func (b *docBuilder) section(children ...document.Node) document.Node {
	return b.node("core/section", 1, nil, map[string]any{"width": "wide", "verticalSpacing": "md", "horizontalPadding": "md", "align": "left", "background": "default", "minHeight": "auto"}, children...)
}

func emptyDocument(prefix string) *document.Document {
	return &document.Document{Version: 1, Nodes: []document.Node{}}
}

func bodyDocument(prefix, body, formID string) *document.Document {
	b := &docBuilder{prefix: prefix}
	nodes := []document.Node{}
	if body != "" {
		nodes = append(nodes, b.section(b.text(body)))
	}
	if formID != "" {
		nodes = append(nodes, b.section(b.heading("Get in touch", 2), b.node("core/form", 1, nil, map[string]any{"formId": formID})))
	}
	return &document.Document{Version: 1, Nodes: nodes}
}

func pageTemplate(prefix string) *document.Document {
	b := &docBuilder{prefix: prefix}
	return &document.Document{Version: 1, Nodes: []document.Node{
		b.section(b.node("core/entry-title", 1, nil, map[string]any{"level": 1, "visualSize": "xl", "align": "left", "tone": "default", "maxWidth": "wide"})),
		b.node("core/content-slot", 1, nil, nil),
	}}
}

func homepageTemplate(prefix, tagline, contentType string, fields []string, formID string) *document.Document {
	b := &docBuilder{prefix: prefix}
	hero := []document.Node{b.node("core/entry-title", 1, nil, map[string]any{"level": 1, "visualSize": "xl", "align": "left", "tone": "default", "maxWidth": "wide"})}
	if tagline != "" {
		hero = append(hero, b.text(tagline))
	}
	cardChildren := []document.Node{b.node("core/entry-media", 1, map[string]any{"source": "entry.featured_media"}, map[string]any{"sizes": "(min-width: 768px) 33vw, 100vw"}), b.node("core/entry-title", 1, nil, map[string]any{"level": 2, "visualSize": "md", "align": "left", "tone": "default", "maxWidth": "none"})}
	for _, field := range fields {
		cardChildren = append(cardChildren, b.node("core/entry-field", 1, map[string]any{"source": field}, map[string]any{"tag": "p"}))
	}
	if contentType != "testimonial" {
		cardChildren = append(cardChildren, b.node("core/entry-link", 1, map[string]any{"text": "View details"}, nil))
	}
	collection := b.node("core/collection", 2, nil, map[string]any{"source": "query", "contentType": contentType, "limit": 6, "orderBy": "entry.published_at", "direction": "desc"}, b.node("core/card", 1, nil, map[string]any{"variant": "default", "padding": "md", "radius": "md", "align": "start"}, cardChildren...))
	nodes := []document.Node{b.section(hero...), b.node("core/content-slot", 1, nil, nil), b.section(b.heading(collectionHeading(contentType), 2), collection)}
	if formID != "" {
		nodes = append(nodes, b.section(b.heading("Start a conversation", 2), b.node("core/form", 1, nil, map[string]any{"formId": formID})))
	}
	return &document.Document{Version: 1, Nodes: nodes}
}

func collectionHeading(contentType string) string {
	switch contentType {
	case "post":
		return "Latest posts"
	case "project":
		return "Selected work"
	case "product":
		return "Featured products"
	case "service":
		return "Services"
	default:
		return "What people say"
	}
}

func singleTemplate(prefix string, fields []string) *document.Document {
	b := &docBuilder{prefix: prefix}
	meta := []document.Node{b.node("core/entry-media", 1, map[string]any{"source": "entry.featured_media"}, nil), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "lg"})}
	for _, field := range fields {
		meta = append(meta, b.node("core/entry-field", 1, map[string]any{"source": field}, map[string]any{"tag": "p"}))
	}
	return &document.Document{Version: 1, Nodes: []document.Node{b.section(append([]document.Node{b.node("core/entry-title", 1, nil, map[string]any{"level": 1, "visualSize": "xl", "align": "left", "tone": "default", "maxWidth": "wide"})}, meta...)...), b.node("core/content-slot", 1, nil, nil)}}
}

func archiveTemplate(prefix string, fields []string) *document.Document {
	b := &docBuilder{prefix: prefix}
	children := []document.Node{b.node("core/entry-media", 1, map[string]any{"source": "entry.featured_media"}, nil), b.node("core/entry-title", 1, nil, map[string]any{"level": 2, "visualSize": "md", "align": "left", "tone": "default", "maxWidth": "none"})}
	for _, field := range fields {
		children = append(children, b.node("core/entry-field", 1, map[string]any{"source": field}, map[string]any{"tag": "p"}))
	}
	children = append(children, b.node("core/entry-link", 1, map[string]any{"text": "View details"}, nil))
	collection := b.node("core/collection", 2, nil, map[string]any{"source": "context", "limit": 20}, b.node("core/card", 1, nil, map[string]any{"variant": "default", "padding": "md", "radius": "md", "align": "start"}, children...))
	return &document.Document{Version: 1, Nodes: []document.Node{b.section(b.node("core/archive-title", 1, nil, map[string]any{"level": 1, "align": "left"}), b.node("core/archive-description", 1, nil, map[string]any{"align": "left"})), b.section(collection)}}
}

func sitePartDocument(prefix, location string) *document.Document {
	b := &docBuilder{prefix: prefix}
	menu := "primary"
	if location == "footer" {
		menu = "footer"
	}
	return &document.Document{Version: 1, Nodes: []document.Node{b.section(b.node("core/stack", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "center", "justify": "between", "wrap": true, "fullWidth": true}, b.node("core/site-name", 1, nil, map[string]any{"level": 2, "link": true}), b.node("core/navigation", 1, nil, map[string]any{"location": menu, "style": "horizontal"})))}}
}
