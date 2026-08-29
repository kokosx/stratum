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
	return b.node("core/section", 2, nil, map[string]any{"width": width, "verticalSpacing": spacing, "horizontalPadding": "md", "align": "left", "background": background, "minHeight": "auto"}, children...)
}

func (b *docBuilder) sectionAlign(width, spacing, background, align string, children ...document.Node) document.Node {
	return b.node("core/section", 2, nil, map[string]any{"width": width, "verticalSpacing": spacing, "horizontalPadding": "md", "align": align, "background": background, "minHeight": "auto"}, children...)
}

func (b *docBuilder) sectionAnchor(width, spacing, background, anchor string, children ...document.Node) document.Node {
	settings := map[string]any{"width": width, "verticalSpacing": spacing, "horizontalPadding": "md", "align": "left", "background": background, "minHeight": "auto", "anchorID": anchor}
	if anchor != "" {
		settings["anchorID"] = anchor
	}
	return b.node("core/section", 2, nil, settings, children...)
}

func (b *docBuilder) sectionAnchorAlign(width, spacing, background, anchor, align string, children ...document.Node) document.Node {
	settings := map[string]any{"width": width, "verticalSpacing": spacing, "horizontalPadding": "md", "align": align, "background": background, "minHeight": "auto", "anchorID": anchor}
	if anchor != "" {
		settings["anchorID"] = anchor
	}
	return b.node("core/section", 2, nil, settings, children...)
}

func (b *docBuilder) stack(direction, gap, align, justify string, children ...document.Node) document.Node {
	return b.node("core/stack", 1, nil, map[string]any{"direction": direction, "gap": gap, "align": align, "justify": justify, "wrap": true, "width": "full"}, children...)
}

func (b *docBuilder) stackWidth(direction, gap, align, justify, width string, children ...document.Node) document.Node {
	return b.node("core/stack", 1, nil, map[string]any{"direction": direction, "gap": gap, "align": align, "justify": justify, "wrap": true, "width": width}, children...)
}

func (b *docBuilder) grid(columns int, gap string, children ...document.Node) document.Node {
	return b.node("core/grid", 1, nil, map[string]any{"columns": columns, "gap": gap, "align": "stretch", "equalHeight": false}, children...)
}

func (b *docBuilder) entryTitle(level int, size string) document.Node {
	return b.node("core/entry-title", 1, nil, map[string]any{"level": level, "visualSize": size, "align": "left", "tone": "default", "maxWidth": "none"})
}

func (b *docBuilder) entryTitleMaxWidth(level int, size, maxWidth string) document.Node {
	return b.node("core/entry-title", 1, nil, map[string]any{"level": level, "visualSize": size, "align": "left", "tone": "default", "maxWidth": maxWidth})
}

func (b *docBuilder) entryTitleAlign(level int, size, align string) document.Node {
	return b.node("core/entry-title", 1, nil, map[string]any{"level": level, "visualSize": size, "align": align, "tone": "default", "maxWidth": "none"})
}

func (b *docBuilder) entryTitleAlignMaxWidth(level int, size, align, maxWidth string) document.Node {
	return b.node("core/entry-title", 1, nil, map[string]any{"level": level, "visualSize": size, "align": align, "tone": "default", "maxWidth": maxWidth})
}

func (b *docBuilder) entryField(source, tag string) document.Node {
	return b.node("core/entry-field", 1, map[string]any{"source": source}, map[string]any{"tag": tag})
}

func (b *docBuilder) entryMedia(sizes string) document.Node {
	return b.node("core/entry-media", 2, map[string]any{"source": "entry.featured_media"}, map[string]any{"sizes": sizes, "aspect": "natural", "fit": "cover"})
}

func (b *docBuilder) entryMediaAspect(sizes, aspect, fit string) document.Node {
	if aspect == "" {
		aspect = "natural"
	}
	if fit == "" {
		fit = "cover"
	}
	return b.node("core/entry-media", 2, map[string]any{"source": "entry.featured_media"}, map[string]any{"sizes": sizes, "aspect": aspect, "fit": fit})
}

func (b *docBuilder) collection(contentType, source, layout string, columns int, gap string, limit int, children ...document.Node) document.Node {
	settings := map[string]any{"source": source, "contentType": contentType, "limit": limit, "orderBy": "entry.published_at", "direction": "desc", "layout": layout, "columns": columns, "gap": gap}
	return b.node("core/collection", 3, nil, settings, children...)
}

func (b *docBuilder) button(label, url, variant string) document.Node {
	return b.node("core/button", 1, map[string]any{"label": label, "url": url}, map[string]any{"variant": variant, "size": "md", "width": "auto", "align": "left", "openInNewTab": false})
}

func (b *docBuilder) buttonSize(label, url, variant, size string) document.Node {
	return b.node("core/button", 1, map[string]any{"label": label, "url": url}, map[string]any{"variant": variant, "size": size, "width": "auto", "align": "left", "openInNewTab": false})
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
		nodes = append(nodes, b.section("content", "md", "default", b.heading("Get in touch", 2), b.node("core/form", 2, nil, map[string]any{"formId": formID})))
	}
	return &document.Document{Version: 1, Nodes: nodes}
}

func pageTemplate(prefix string) *document.Document {
	b := &docBuilder{prefix: prefix}
	return &document.Document{Version: 1, Nodes: []document.Node{
		b.section("content", "md", "default", b.entryTitle(1, "lg"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "lg"})),
		b.node("core/content-slot", 1, nil, nil),
	}}
}

func homepageTemplate(prefix string, preset PresetID, tagline, formID string) *document.Document {
	// Minimal shell: only content-slot. The homepage-specific layout now lives
	// in the Home Entry SDT (see homepageEntryDocument). This keeps tags like
	// core/entry-title dynamic via EntryContext while allowing the user to edit
	// the homepage directly in Pages → Home.
	_ = preset
	_ = tagline
	_ = formID
	b := &docBuilder{prefix: prefix}
	return &document.Document{Version: 1, Nodes: []document.Node{
		b.node("core/content-slot", 1, nil, nil),
	}}
}

func homepageEntryDocument(prefix string, preset PresetID, formID string, plan Plan) *document.Document {
	b := &docBuilder{prefix: prefix}
	lang := plan.Input.Language
	if lang == "" {
		lang = "en"
	}
	// Use dynamic site-tagline block instead of duplicating tagline string.
	// This keeps Settings → Site tagline as single source of truth.
	siteTaglineLeft := b.node("core/site-tagline", 1, nil, map[string]any{"align": "left"})
	siteTaglineCenter := b.node("core/site-tagline", 1, nil, map[string]any{"align": "center"})
	switch preset {
	case PresetBlog:
		latestLimit := 5
		if plan.Input.BlogLatestCount == 8 {
			latestLimit = 8
		}
		heroStack := b.stack("vertical", "md", "start", "start", b.entryTitle(1, "lg"), siteTaglineLeft)
		linkText := "Read article"
		if lang == "pl" {
			linkText = "Czytaj artykuł"
		}
		post := b.stack("vertical", "sm", "start", "start", b.node("core/entry-publish-date", 1, nil, map[string]any{"format": "long", "align": "left"}), b.entryTitle(2, "md"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "md"}), b.node("core/entry-link", 1, map[string]any{"text": linkText}, nil))
		latestStack := b.stack("vertical", "lg", "stretch", "start", b.heading(copyFor(lang, "heading.latest_posts"), 2), b.collection("post", "query", "list", 1, "lg", latestLimit, post))
		editorial := b.stack("vertical", "2xl", "stretch", "start", heroStack, latestStack)
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("content", "lg", "default", editorial),
		}}
	case PresetPortfolio:
		cols := 2
		if plan.Input.PortfolioColumns == 3 {
			cols = 3
		}
		heroStack := b.stack("vertical", "md", "start", "start", b.entryTitle(1, "lg"), siteTaglineLeft)
		linkText := "View project"
		if lang == "pl" {
			linkText = "Zobacz projekt"
		}
		project := b.stack("vertical", "sm", "start", "start", b.entryMediaAspect("(min-width: 900px) 45vw, 100vw", "landscape", "cover"), b.entryTitle(2, "md"), b.stack("horizontal", "md", "center", "start", b.entryField("fields.client", "span"), b.entryField("fields.year", "span")), b.node("core/entry-link", 1, map[string]any{"text": linkText}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("wide", "lg", "default", heroStack),
			b.section("wide", "md", "default", b.stack("vertical", "lg", "stretch", "start", b.heading(copyFor(lang, "heading.selected_work"), 2), b.collection("project", "query", "grid", cols, "xl", 6, project))),
		}}
	case PresetLanding:
		testCols := 2
		if plan.Input.LandingTestimonialsColumns == 1 {
			testCols = 1
		}
		cta := copyFor(lang, "cta.start_conversation")
		heroStack := b.stack("vertical", "md", "center", "center", b.entryTitleAlign(1, "lg", "center"), siteTaglineCenter, b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "center", "wrap": true}, b.buttonSize(cta, "#contact", "primary", "lg")))
		testimonial := b.stack("vertical", "sm", "start", "start", b.entryField("fields.quote", "p"), b.entryField("fields.person", "strong"), b.entryField("fields.role", "span"), b.entryField("fields.company", "span"))
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.sectionAlign("content", "xl", "muted", "center", heroStack),
			b.section("wide", "lg", "default", b.stack("vertical", "lg", "stretch", "start", b.heading(copyFor(lang, "heading.testimonials"), 2), b.collection("testimonial", "query", "grid", testCols, "lg", 4, testimonial))),
			b.sectionAnchor("content", "lg", "default", "contact", b.stack("vertical", "md", "start", "start", b.heading(copyFor(lang, "cta.next_step"), 2), b.lead(copyFor(lang, "lead.testimonial_next")), b.node("core/form", 2, nil, map[string]any{"formId": formID}))),
		}}
	case PresetProducts:
		cols := 3
		if plan.Input.ProductColumns == 4 {
			cols = 4
		}
		// Use standard aspect by default; Product Showcase prefers standard/square image presentation.
		heroStack := b.stack("vertical", "sm", "start", "start", b.entryTitle(1, "lg"), siteTaglineLeft)
		linkText := "View product"
		if lang == "pl" {
			linkText = "Zobacz produkt"
		}
		product := b.stack("vertical", "sm", "start", "start", b.entryMediaAspect("(min-width: 1100px) 30vw, (min-width: 640px) 50vw, 100vw", "standard", "cover"), b.entryTitle(2, "md"), b.entryField("fields.price_display", "strong"), b.entryField("fields.short_description", "p"), b.node("core/entry-link", 1, map[string]any{"text": linkText}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("wide", "md", "muted", heroStack),
			b.section("wide", "md", "default", b.stack("vertical", "lg", "stretch", "start", b.heading(copyFor(lang, "heading.featured"), 2), b.collection("product", "query", "grid", cols, "lg", 6, product))),
		}}
	default:
		svcCols := 3
		if plan.Input.ServiceColumns == 2 {
			svcCols = 2
		}
		cta := copyFor(lang, "cta.request_consult")
		linkText := "Learn more"
		if lang == "pl" {
			linkText = "Dowiedz się więcej"
		}
		heroStack := b.stack("vertical", "md", "start", "start", b.entryTitle(1, "lg"), siteTaglineLeft, b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.button(cta, "/contact", "primary")))
		serviceInner := b.stack("vertical", "sm", "start", "start", b.entryTitle(2, "md"), b.entryField("fields.short_summary", "p"), b.entryField("fields.service_area", "span"), b.node("core/entry-link", 1, map[string]any{"text": linkText}, nil))
		service := b.node("core/card", 1, nil, map[string]any{"variant": "default", "padding": "md", "radius": "md", "align": "start"}, serviceInner)
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("content", "lg", "muted", heroStack),
			b.section("wide", "md", "default", b.stack("vertical", "lg", "stretch", "start", b.heading(copyFor(lang, "heading.services"), 2), b.collection("service", "query", "grid", svcCols, "lg", 5, service))),
			b.section("content", "lg", "primary", b.stack("vertical", "md", "start", "start", b.heading(copyFor(lang, "cta.need_next_step"), 2), b.text(copyFor(lang, "lead.local_next")), b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.node("core/button", 1, map[string]any{"label": copyFor(lang, "cta.contact_us"), "url": "/contact"}, map[string]any{"variant": "primary", "size": "md", "width": "auto", "align": "left", "openInNewTab": false})))),
		}}
	}
}

func singleTemplate(prefix string, preset PresetID) *document.Document {
	return singleTemplateForPlan(prefix, preset, Plan{})
}

func singleTemplateForPlan(prefix string, preset PresetID, plan Plan) *document.Document {
	b := &docBuilder{prefix: prefix}
	switch preset {
	case PresetBlog:
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("content", "lg", "default",
				b.stack("vertical", "md", "start", "start",
					b.node("core/entry-publish-date", 1, nil, map[string]any{"format": "long", "align": "left"}),
					b.entryTitle(1, "lg"),
					b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "lg"}),
				),
			),
			b.node("core/content-slot", 1, nil, nil),
		}}
	case PresetPortfolio:
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("wide", "lg", "default",
				b.stack("vertical", "md", "start", "start",
					b.entryTitle(1, "lg"),
					b.stack("horizontal", "md", "center", "start", b.entryField("fields.client", "strong"), b.entryField("fields.year", "span"), b.entryField("fields.services", "span")),
					b.entryMediaAspect("(min-width: 1200px) 80vw, 100vw", "landscape", "cover"),
				),
			),
			b.node("core/content-slot", 1, nil, nil),
		}}
	case PresetProducts:
		details := b.stack("vertical", "md", "start", "start", b.entryTitle(1, "lg"), b.entryField("fields.price_display", "strong"), b.entryField("fields.short_description", "p"), b.entryField("fields.sku", "span"))
		media := b.entryMediaAspect("(min-width: 900px) 50vw, 100vw", "standard", "cover")
		var gridNode document.Node
		if plan.Input.ProductMediaPosition == "right" {
			gridNode = b.grid(2, "xl", details, media)
		} else {
			gridNode = b.grid(2, "xl", media, details)
		}
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("wide", "lg", "default", gridNode),
			b.node("core/content-slot", 1, nil, nil),
		}}
	default:
		return &document.Document{Version: 1, Nodes: []document.Node{
			b.section("content", "lg", "default", b.stack("vertical", "md", "start", "start", b.entryTitle(1, "lg"), b.entryField("fields.short_summary", "p"), b.entryField("fields.service_area", "strong"))),
			b.node("core/content-slot", 1, nil, nil),
			b.section("content", "md", "muted", b.stack("vertical", "md", "start", "start", b.heading("Ready to talk?", 2), b.node("core/button-group", 1, nil, map[string]any{"direction": "horizontal", "gap": "md", "align": "start", "wrap": true}, b.button("Contact us", "/contact", "primary")))),
		}}
	}
}

func archiveTemplate(prefix string, preset PresetID) *document.Document {
	return archiveTemplateForPlan(prefix, preset, Plan{})
}

func archiveTemplateForPlan(prefix string, preset PresetID, plan Plan) *document.Document {
	b := &docBuilder{prefix: prefix}
	header := b.section("content", "lg", "default", b.stack("vertical", "sm", "start", "start", b.node("core/archive-title", 1, nil, map[string]any{"level": 1, "align": "left"}), b.node("core/archive-description", 1, nil, map[string]any{"align": "left"})))
	switch preset {
	case PresetBlog:
		limit := 20
		if plan.Input.BlogArchiveCount == 10 {
			limit = 10
		}
		item := b.stack("vertical", "sm", "start", "start", b.node("core/entry-publish-date", 1, nil, map[string]any{"format": "long", "align": "left"}), b.entryTitle(2, "md"), b.node("core/entry-excerpt", 1, nil, map[string]any{"align": "left", "tone": "muted", "size": "md"}), b.node("core/entry-link", 1, map[string]any{"text": "Read article"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("content", "md", "default", b.collection("post", "context", "list", 1, "lg", limit, item))}}
	case PresetPortfolio:
		cols := 2
		if plan.Input.PortfolioColumns == 3 {
			cols = 3
		}
		item := b.stack("vertical", "sm", "start", "start", b.entryMediaAspect("(min-width: 900px) 45vw, 100vw", "landscape", "cover"), b.entryTitle(2, "md"), b.stack("horizontal", "md", "center", "start", b.entryField("fields.client", "span"), b.entryField("fields.year", "span")), b.node("core/entry-link", 1, map[string]any{"text": "View project"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("wide", "md", "default", b.collection("project", "context", "grid", cols, "xl", 20, item))}}
	case PresetProducts:
		cols := 3
		if plan.Input.ProductColumns == 4 {
			cols = 4
		}
		item := b.stack("vertical", "sm", "start", "start", b.entryMediaAspect("(min-width: 1100px) 30vw, 100vw", "standard", "cover"), b.entryTitle(2, "md"), b.entryField("fields.price_display", "strong"), b.entryField("fields.short_description", "p"), b.node("core/entry-link", 1, map[string]any{"text": "View product"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("wide", "md", "default", b.collection("product", "context", "grid", cols, "lg", 20, item))}}
	default:
		cols := 3
		if plan.Input.ServiceColumns == 2 {
			cols = 2
		}
		item := b.stack("vertical", "sm", "start", "start", b.entryTitle(2, "md"), b.entryField("fields.short_summary", "p"), b.entryField("fields.service_area", "span"), b.node("core/entry-link", 1, map[string]any{"text": "Learn more"}, nil))
		return &document.Document{Version: 1, Nodes: []document.Node{header, b.section("wide", "md", "default", b.collection("service", "context", "grid", cols, "lg", 20, item))}}
	}
}

func sitePartDocument(prefix, location string) *document.Document {
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
	default:
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
	default:
		return &document.Document{Version: 1, Nodes: []document.Node{b.stack("horizontal", "lg", "center", "between", b.stack("vertical", "xs", "start", "start", b.node("core/site-name", 1, nil, nil), b.node("core/site-tagline", 1, nil, nil)), b.node("core/navigation", 1, nil, map[string]any{"location": "footer", "style": "horizontal"}))}}
	}
}
