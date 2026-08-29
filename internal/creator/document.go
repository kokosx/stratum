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
	return b.node("core/text", 2, map[string]any{"text": richText(text)}, map[string]any{"align": "left", "tone": "default", "size": "md", "maxWidth": "normal"})
}

func (b *docBuilder) lead(text string) document.Node {
	return b.node("core/text", 2, map[string]any{"text": richText(text)}, map[string]any{"align": "left", "tone": "muted", "size": "lg", "maxWidth": "narrow"})
}

func (b *docBuilder) heading(text string, level int) document.Node {
	return b.node("core/heading", 2, map[string]any{"text": richText(text), "level": level}, map[string]any{"align": "left", "visualSize": "auto", "tone": "default", "maxWidth": "none"})
}

func (b *docBuilder) section(width, spacing, background string, children ...document.Node) document.Node {
	return b.node("core/section", 1, nil, map[string]any{"width": width, "verticalSpacing": spacing, "horizontalPadding": "md", "align": "left", "background": background, "minHeight": "auto"}, children...)
}

func (b *docBuilder) sectionAnchor(width, spacing, background, anchor string, children ...document.Node) document.Node {
	settings := map[string]any{"width": width, "verticalSpacing": spacing, "horizontalPadding": "md", "align": "left", "background": background, "minHeight": "auto", "anchorID": anchor}
	if anchor != "" {
		settings["anchorID"] = anchor
	}
	return b.node("core/section", 1, nil, settings, children...)
}

func (b *docBuilder) stack(direction, gap, align, justify string, children ...document.Node) document.Node {
	return b.node("core/stack", 1, nil, map[string]any{"direction": direction, "gap": gap, "align": align, "justify": justify, "wrap": true, "fullWidth": true}, children...)
}

func (b *docBuilder) grid(columns int, gap string, children ...document.Node) document.Node {
	return b.node("core/grid", 1, nil, map[string]any{"columns": columns, "gap": gap, "align": "stretch", "equalHeight": false}, children...)
}

func (b *docBuilder) entryTitle(level int, size string) document.Node {
	return b.node("core/entry-title", 1, nil, map[string]any{"level": level, "visualSize": size, "align": "left", "tone": "default", "maxWidth": "none"})
}

func (b *docBuilder) entryField(source, tag string) document.Node {
	return b.node("core/entry-field", 1, map[string]any{"source": source}, map[string]any{"tag": tag})
}

func (b *docBuilder) entryMedia(sizes string) document.Node {
	return b.node("core/entry-media", 1, map[string]any{"source": "entry.featured_media"}, map[string]any{"sizes": sizes})
}

func (b *docBuilder) collection(contentType, source, layout string, columns int, gap string, limit int, children ...document.Node) document.Node {
	settings := map[string]any{"source": source, "contentType": contentType, "limit": limit, "orderBy": "entry.published_at", "direction": "desc", "layout": layout, "columns": columns, "gap": gap}
	return b.node("core/collection", 3, nil, settings, children...)
}

func (b *docBuilder) button(label, url, variant string) document.Node {
	return b.node("core/button", 1, map[string]any{"label": label, "url": url}, map[string]any{"variant": variant, "size": "lg", "width": "auto", "align": "left", "openInNewTab": false})
}

func emptyDocument(prefix string) *document.Document {
	return &document.Document{Version: 1, Nodes: []document.Node{}}
}

func bodyDocument(prefix, body, formID string) *document.Document {
	b := &docBuilder{prefix: prefix}
	nodes := []document.Node{}
	if body != "" {
		nodes = append(nodes, b.section("content", "sm", "default", b.text(body)))
	}
	if formID != "" {
		nodes = append(nodes, b.section("content", "sm", "default", b.heading("Get in touch", 2), b.node("core/form", 2, nil, map[string]any{"formId": formID})))
	}
	return &document.Document{Version: 1, Nodes: nodes}
}

func pageTemplate(prefix string) *document.Document {
	b := &docBuilder{prefix: prefix}
	return &document.Document{Version: 1, Nodes: []document.Node{
		b.section("content", "md", "default", b.entryTitle(1, "xl"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "lg"})),
		b.node("core/content-slot", 1, nil, nil),
	}}
}

func homepageTemplate(prefix string, preset PresetID, tagline, formID string) *document.Document {
	b := &docBuilder{prefix: prefix}
	switch preset {
	case PresetBlog:
		hero := []document.Node{b.entryTitle(1, "xl")}
		if tagline != "" {
			hero = append(hero, b.lead(tagline))
		}
		// Compact editorial hero: md when tagline exists, sm when title only.
		// Avoids the previous lg whitespace bug around a single H1.
		heroSpacing := "sm"
		if tagline != "" {
			heroSpacing = "md"
		}
		post := b.stack("vertical", "sm", "start", "start", b.node("core/entry-publish-date", 1, nil, map[string]any{"format": "long", "align": "left"}), b.entryTitle(2, "md"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "md"}), b.node("core/entry-link", 1, map[string]any{"text": "Read article"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("content", heroSpacing, "default", hero...), b.section("content", "sm", "default", b.heading("Latest posts", 2), b.collection("post", "query", "list", 1, "lg", 5, post)), b.node("core/content-slot", 1, nil, nil)}}
	case PresetPortfolio:
		hero := []document.Node{b.entryTitle(1, "xl")}
		if tagline != "" {
			hero = append(hero, b.lead(tagline))
		}
		project := b.stack("vertical", "sm", "start", "start", b.entryMedia("(min-width: 900px) 45vw, 100vw"), b.entryTitle(2, "md"), b.stack("horizontal", "md", "center", "start", b.entryField("fields.client", "span"), b.entryField("fields.year", "span")), b.node("core/entry-link", 1, map[string]any{"text": "View project"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("wide", "lg", "default", hero...), b.section("wide", "md", "default", b.heading("Selected Work", 2), b.collection("project", "query", "grid", 2, "xl", 6, project)), b.node("core/content-slot", 1, nil, nil)}}
	case PresetLanding:
		hero := []document.Node{b.entryTitle(1, "xl")}
		if tagline != "" {
			hero = append(hero, b.lead(tagline))
		}
		hero = append(hero, b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.button("Start a conversation", "#contact", "primary")))
		testimonial := b.stack("vertical", "sm", "start", "start", b.entryField("fields.quote", "p"), b.entryField("fields.person", "strong"), b.entryField("fields.role", "span"), b.entryField("fields.company", "span"))
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("content", "lg", "default", hero...), b.section("wide", "md", "muted", b.heading("What clients say", 2), b.collection("testimonial", "query", "grid", 2, "lg", 4, testimonial)), b.sectionAnchor("content", "md", "default", "contact", b.heading("Make the next step clear", 2), b.lead("Share what you are trying to achieve. We will respond with a practical next step."), b.node("core/form", 2, nil, map[string]any{"formId": formID})), b.node("core/content-slot", 1, nil, nil)}}
	case PresetProducts:
		hero := []document.Node{b.entryTitle(1, "xl")}
		if tagline != "" {
			hero = append(hero, b.lead(tagline))
		}
		product := b.stack("vertical", "sm", "start", "start", b.entryMedia("(min-width: 1100px) 30vw, (min-width: 640px) 50vw, 100vw"), b.entryTitle(2, "md"), b.entryField("fields.price_display", "strong"), b.entryField("fields.short_description", "p"), b.node("core/entry-link", 1, map[string]any{"text": "View product"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("wide", "md", "muted", hero...), b.section("wide", "md", "default", b.heading("Featured Products", 2), b.collection("product", "query", "grid", 3, "lg", 6, product)), b.node("core/content-slot", 1, nil, nil)}}
	default:
		hero := []document.Node{b.entryTitle(1, "xl")}
		if tagline != "" {
			hero = append(hero, b.lead(tagline))
		}
		hero = append(hero, b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.button("Request a consultation", "/contact", "primary")))
		service := b.stack("vertical", "sm", "start", "start", b.entryTitle(2, "md"), b.entryField("fields.short_summary", "p"), b.entryField("fields.service_area", "span"), b.node("core/entry-link", 1, map[string]any{"text": "Learn more"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("content", "md", "muted", hero...), b.section("wide", "md", "default", b.heading("Services", 2), b.collection("service", "query", "grid", 3, "lg", 5, service)), b.section("content", "md", "primary", b.heading("Need a practical next step?", 2), b.text("Tell us what you need and we will explain how we can help."), b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.button("Contact us", "/contact", "primary"))), b.node("core/content-slot", 1, nil, nil)}}
	}
}

func singleTemplate(prefix string, preset PresetID) *document.Document {
	b := &docBuilder{prefix: prefix}
	switch preset {
	case PresetBlog:
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("content", "md", "default", b.node("core/entry-publish-date", 1, nil, map[string]any{"format": "long", "align": "left"}), b.entryTitle(1, "xl"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "lg"})), b.node("core/content-slot", 1, nil, nil)}}
	case PresetPortfolio:
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("wide", "md", "default", b.entryTitle(1, "xl"), b.stack("horizontal", "lg", "center", "start", b.entryField("fields.client", "strong"), b.entryField("fields.year", "span"), b.entryField("fields.services", "span")), b.entryMedia("(min-width: 1200px) 80vw, 100vw")), b.node("core/content-slot", 1, nil, nil)}}
	case PresetProducts:
		details := b.stack("vertical", "md", "start", "start", b.entryTitle(1, "xl"), b.entryField("fields.price_display", "strong"), b.entryField("fields.short_description", "p"), b.entryField("fields.sku", "span"))
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("wide", "md", "default", b.grid(2, "xl", b.entryMedia("(min-width: 900px) 50vw, 100vw"), details)), b.node("core/content-slot", 1, nil, nil)}}
	default:
		return &document.Document{Version: 1, Nodes: []document.Node{b.section("content", "md", "default", b.entryTitle(1, "xl"), b.entryField("fields.short_summary", "p"), b.entryField("fields.service_area", "strong")), b.node("core/content-slot", 1, nil, nil), b.section("content", "sm", "muted", b.heading("Ready to talk?", 2), b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.button("Contact us", "/contact", "primary")))}}
	}
}

func archiveTemplate(prefix string, preset PresetID) *document.Document {
	b := &docBuilder{prefix: prefix}
	header := b.section("content", "md", "default", b.node("core/archive-title", 1, nil, map[string]any{"level": 1, "align": "left"}), b.node("core/archive-description", 1, nil, map[string]any{"align": "left"}))
	switch preset {
	case PresetBlog:
		item := b.stack("vertical", "sm", "start", "start", b.node("core/entry-publish-date", 1, nil, map[string]any{"format": "long", "align": "left"}), b.entryTitle(2, "md"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "md"}), b.node("core/entry-link", 1, map[string]any{"text": "Read article"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("content", "sm", "default", b.collection("post", "context", "list", 1, "lg", 20, item))}}
	case PresetPortfolio:
		item := b.stack("vertical", "sm", "start", "start", b.entryMedia("(min-width: 900px) 45vw, 100vw"), b.entryTitle(2, "md"), b.stack("horizontal", "md", "center", "start", b.entryField("fields.client", "span"), b.entryField("fields.year", "span")), b.node("core/entry-link", 1, map[string]any{"text": "View project"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("wide", "sm", "default", b.collection("project", "context", "grid", 2, "xl", 20, item))}}
	case PresetProducts:
		item := b.stack("vertical", "sm", "start", "start", b.entryMedia("(min-width: 1100px) 30vw, 100vw"), b.entryTitle(2, "md"), b.entryField("fields.price_display", "strong"), b.entryField("fields.short_description", "p"), b.node("core/entry-link", 1, map[string]any{"text": "View product"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("wide", "sm", "default", b.collection("product", "context", "grid", 3, "lg", 20, item))}}
	default:
		item := b.stack("vertical", "sm", "start", "start", b.entryTitle(2, "md"), b.entryField("fields.short_summary", "p"), b.entryField("fields.service_area", "span"), b.node("core/entry-link", 1, map[string]any{"text": "Learn more"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("wide", "sm", "default", b.collection("service", "context", "grid", 3, "lg", 20, item))}}
	}
}

func sitePartDocument(prefix, location string) *document.Document {
	// Legacy entry point kept for tests that call it directly; uses minimal/split defaults.
	if location == "header" {
		return sitePartDocumentForHeader(prefix, HeaderMinimal)
	}
	return sitePartDocumentForFooter(prefix, FooterSplit)
}

func sitePartDocumentForHeader(prefix string, style HeaderStyleID) *document.Document {
	b := &docBuilder{prefix: prefix}
	switch style {
	case HeaderCentered:
		return &document.Document{Version: 1, Nodes: []document.Node{b.stack("vertical", "md", "center", "center", b.node("core/site-name", 1, nil, map[string]any{"align": "center"}), b.node("core/navigation", 1, nil, map[string]any{"location": "primary", "style": "horizontal"}))}}
	case HeaderClassic:
		return &document.Document{Version: 1, Nodes: []document.Node{b.stack("horizontal", "md", "center", "between", b.stack("vertical", "xs", "start", "start", b.node("core/site-name", 1, nil, nil), b.node("core/site-tagline", 1, nil, nil)), b.node("core/navigation", 1, nil, map[string]any{"location": "primary", "style": "horizontal"}))}}
	default: // minimal
		return &document.Document{Version: 1, Nodes: []document.Node{b.stack("horizontal", "md", "center", "between", b.node("core/site-name", 1, nil, nil), b.node("core/navigation", 1, nil, map[string]any{"location": "primary", "style": "horizontal"}))}}
	}
}

func sitePartDocumentForFooter(prefix string, style FooterStyleID) *document.Document {
	b := &docBuilder{prefix: prefix}
	switch style {
	case FooterSimple:
		return &document.Document{Version: 1, Nodes: []document.Node{b.node("core/site-name", 1, nil, nil)}}
	case FooterCentered:
		return &document.Document{Version: 1, Nodes: []document.Node{b.stack("vertical", "md", "center", "center", b.node("core/site-name", 1, nil, map[string]any{"align": "center"}), b.node("core/site-tagline", 1, nil, map[string]any{"align": "center"}), b.node("core/navigation", 1, nil, map[string]any{"location": "footer", "style": "horizontal"}))}}
	default: // split
		return &document.Document{Version: 1, Nodes: []document.Node{b.stack("horizontal", "lg", "center", "between", b.stack("vertical", "xs", "start", "start", b.node("core/site-name", 1, nil, nil), b.node("core/site-tagline", 1, nil, nil)), b.node("core/navigation", 1, nil, map[string]any{"location": "footer", "style": "horizontal"}))}}
	}
}
