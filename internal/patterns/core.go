package patterns

import "github.com/kokosx/stratum/internal/document"

// corePatterns returns the bundled core patterns. IDs are stable and unique.
// Node IDs inside are temporary and regenerated on insertion.
func corePatterns() []Pattern {
	return []Pattern{
		heroCentered(),
		heroSplit(),
		featuresThreeColumns(),
		ctaCentered(),
		testimonialsGrid(),
		logoWall(),
		pricingThreeTiers(),
		faqAccordion(),
		contentImageText(),
		latestPosts(),
		archiveCollectionGrid(),
		singleArticle(),
	}
}

func heroCentered() Pattern {
	nodes := mustNodes(`[
		{"id":"a1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"xl","horizontalPadding":"md","align":"center","background":"muted","minHeight":"auto","anchorID":""},"children":[
			{"id":"a2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"lg","align":"center","justify":"center","wrap":false,"width":"auto"},"children":[
				{"id":"a3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Build something remarkable","marks":[]}] },"level":1},"settings":{"align":"center","visualSize":"display","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"a4","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"A modern CMS that stays out of your way. Semantic content, thoughtful defaults, and a polished design system.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"lg","maxWidth":"normal"},"children":[]},
				{"id":"a5","block":"core/button-group","version":1,"props":{},"settings":{"direction":"horizontal","gap":"sm","align":"center","wrap":true},"children":[
					{"id":"a6","block":"core/button","version":1,"props":{"label":"Get started","url":"#"},"settings":{"variant":"primary","size":"lg","width":"auto","align":"center","openInNewTab":false},"children":[]},
					{"id":"a7","block":"core/button","version":1,"props":{"label":"Learn more","url":"#"},"settings":{"variant":"outline","size":"lg","width":"auto","align":"center","openInNewTab":false},"children":[]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "hero-centered",
		Name:        "Hero — centered",
		Description: "Centered headline, supporting text and two calls to action.",
		Category:    "Hero",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func heroSplit() Pattern {
	nodes := mustNodes(`[
		{"id":"b1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"xl","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"b2","block":"core/columns","version":1,"props":{},"settings":{"columns":2,"ratio":"equal","gap":"lg","mobileStack":true},"children":[
				{"id":"b3","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[
					{"id":"b4","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Build something remarkable","marks":[]}] },"level":1},"settings":{"align":"left","visualSize":"xl","tone":"default","maxWidth":"none"},"children":[]},
					{"id":"b5","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Stratum helps you publish quickly without sacrificing quality. Every block is semantic and every layout is responsive.","marks":[]}] }},"settings":{"align":"left","tone":"muted","size":"lg","maxWidth":"none"},"children":[]},
					{"id":"b6","block":"core/button-group","version":1,"props":{},"settings":{"direction":"horizontal","gap":"sm","align":"start","wrap":true},"children":[
						{"id":"b7","block":"core/button","version":1,"props":{"label":"Get started","url":"#"},"settings":{"variant":"primary","size":"md","width":"auto","align":"left","openInNewTab":false},"children":[]},
						{"id":"b8","block":"core/button","version":1,"props":{"label":"View features","url":"#"},"settings":{"variant":"ghost","size":"md","width":"auto","align":"left","openInNewTab":false},"children":[]}
					]}
				]},
				{"id":"b9","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[
					{"id":"b10","block":"core/image","version":1,"props":{"mediaId":"","alt":"","caption":""},"settings":{"align":"none","decorative":true,"sizes":"","priority":"normal","radius":"lg","aspect":"4:3","fit":"cover"},"children":[]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "hero-split",
		Name:        "Hero — split",
		Description: "Two-column hero with headline and image.",
		Category:    "Hero",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func featuresThreeColumns() Pattern {
	nodes := mustNodes(`[
		{"id":"c1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"center","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"c2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"lg","align":"center","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"c3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Everything you need","marks":[]}] },"level":2},"settings":{"align":"center","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"c4","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Thoughtful defaults let you ship a polished site without endless tweaking.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"normal"},"children":[]},
				{"id":"c5","block":"core/grid","version":1,"props":{},"settings":{"columns":3,"gap":"lg","align":"stretch","equalHeight":true},"children":[
					{"id":"c6","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[
						{"id":"c7","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Fast","marks":[]}] },"level":3},"settings":{"align":"left","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"c8","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Single-binary deployment and instant server renders keep your site snappy.","marks":[]}] }},"settings":{"align":"left","tone":"muted","size":"md","maxWidth":"none"},"children":[]}
					]},
					{"id":"c9","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[
						{"id":"c10","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Composable","marks":[]}] },"level":3},"settings":{"align":"left","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"c11","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Sections, Grids and Stacks combine to cover most business layouts.","marks":[]}] }},"settings":{"align":"left","tone":"muted","size":"md","maxWidth":"none"},"children":[]}
					]},
					{"id":"c12","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[
						{"id":"c13","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Accessible","marks":[]}] },"level":3},"settings":{"align":"left","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"c14","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Semantic markup, keyboard-friendly components and no JS for core layouts.","marks":[]}] }},"settings":{"align":"left","tone":"muted","size":"md","maxWidth":"none"},"children":[]}
					]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "features-3-col",
		Name:        "Features — 3 columns",
		Description: "Three feature cards in a responsive grid.",
		Category:    "Features",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func ctaCentered() Pattern {
	nodes := mustNodes(`[
		{"id":"d1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"lg","horizontalPadding":"md","align":"center","background":"primary","minHeight":"auto","anchorID":""},"children":[
			{"id":"d2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"center","justify":"center","wrap":false,"width":"auto"},"children":[
				{"id":"d3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Ready to get started?","marks":[]}] },"level":2},"settings":{"align":"center","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"d4","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Launch your site today and publish your first page in minutes.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"normal"},"children":[]},
				{"id":"d5","block":"core/button","version":1,"props":{"label":"Create your site","url":"#"},"settings":{"variant":"primary","size":"lg","width":"auto","align":"center","openInNewTab":false},"children":[]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "cta-centered",
		Name:        "CTA — centered",
		Description: "Centered call to action with heading and button.",
		Category:    "CTA",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func testimonialsGrid() Pattern {
	nodes := mustNodes(`[
		{"id":"e1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"center","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"e2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"lg","align":"center","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"e3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Loved by teams","marks":[]}] },"level":2},"settings":{"align":"center","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"e4","block":"core/grid","version":1,"props":{},"settings":{"columns":3,"gap":"md","align":"stretch","equalHeight":true},"children":[
					{"id":"e5","block":"core/quote","version":1,"props":{"text":"Stratum made our launch painless.","citation":"Alex — Studio North"},"settings":{"style":"simple","align":"left"},"children":[]},
					{"id":"e6","block":"core/quote","version":1,"props":{"text":"The design system is exactly the right size.","citation":"Maya — Bloom Co"},"settings":{"style":"simple","align":"left"},"children":[]},
					{"id":"e7","block":"core/quote","version":1,"props":{"text":"Finally a CMS that values semantics.","citation":"Jon — Craft & Co"},"settings":{"style":"simple","align":"left"},"children":[]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "testimonials-grid",
		Name:        "Testimonials — grid",
		Description: "Three quotes in a grid.",
		Category:    "Testimonials",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func logoWall() Pattern {
	nodes := mustNodes(`[
		{"id":"f1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"md","horizontalPadding":"md","align":"center","background":"muted","minHeight":"auto","anchorID":""},"children":[
			{"id":"f2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"center","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"f3","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Trusted by teams worldwide","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"sm","maxWidth":"none"},"children":[]},
				{"id":"f4","block":"core/grid","version":1,"props":{},"settings":{"columns":4,"gap":"lg","align":"center","equalHeight":false},"children":[
					{"id":"f5","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"ACME","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"muted","size":"lg","maxWidth":"none"},"children":[]},
					{"id":"f6","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"NOVA","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"muted","size":"lg","maxWidth":"none"},"children":[]},
					{"id":"f7","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"ATLAS","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"muted","size":"lg","maxWidth":"none"},"children":[]},
					{"id":"f8","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"LUMEN","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"muted","size":"lg","maxWidth":"none"},"children":[]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "logo-wall",
		Name:        "Logo wall",
		Description: "Row of logos or brand names.",
		Category:    "Logos",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func pricingThreeTiers() Pattern {
	nodes := mustNodes(`[
		{"id":"g1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"center","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"g2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"lg","align":"center","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"g3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Simple pricing","marks":[]}] },"level":2},"settings":{"align":"center","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"g4","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Pick the plan that fits your team. No hidden fees.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"normal"},"children":[]},
				{"id":"g5","block":"core/grid","version":1,"props":{},"settings":{"columns":3,"gap":"lg","align":"stretch","equalHeight":true},"children":[
					{"id":"g6","block":"core/card","version":1,"props":{},"settings":{"variant":"outlined","padding":"md","radius":"md","align":"center"},"children":[
						{"id":"g7","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Starter","marks":[]}] },"level":3},"settings":{"align":"center","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"g8","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"$19 / month","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"default","size":"lg","maxWidth":"none"},"children":[]},
						{"id":"g9","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Perfect for side projects and small teams.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"none"},"children":[]},
						{"id":"g10","block":"core/button","version":1,"props":{"label":"Choose Starter","url":"#"},"settings":{"variant":"outline","size":"md","width":"full","align":"center","openInNewTab":false},"children":[]}
					]},
					{"id":"g11","block":"core/card","version":1,"props":{},"settings":{"variant":"elevated","padding":"md","radius":"md","align":"center"},"children":[
						{"id":"g12","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Growth","marks":[]}] },"level":3},"settings":{"align":"center","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"g13","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"$49 / month","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"default","size":"lg","maxWidth":"none"},"children":[]},
						{"id":"g14","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Most popular. Everything you need to scale.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"none"},"children":[]},
						{"id":"g15","block":"core/button","version":1,"props":{"label":"Choose Growth","url":"#"},"settings":{"variant":"primary","size":"md","width":"full","align":"center","openInNewTab":false},"children":[]}
					]},
					{"id":"g16","block":"core/card","version":1,"props":{},"settings":{"variant":"outlined","padding":"md","radius":"md","align":"center"},"children":[
						{"id":"g17","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Scale","marks":[]}] },"level":3},"settings":{"align":"center","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"g18","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"$99 / month","marks":[{"type":"bold"}]}] }},"settings":{"align":"center","tone":"default","size":"lg","maxWidth":"none"},"children":[]},
						{"id":"g19","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"For large teams with advanced needs.","marks":[]}] }},"settings":{"align":"center","tone":"muted","size":"md","maxWidth":"none"},"children":[]},
						{"id":"g20","block":"core/button","version":1,"props":{"label":"Choose Scale","url":"#"},"settings":{"variant":"outline","size":"md","width":"full","align":"center","openInNewTab":false},"children":[]}
					]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "pricing-3-tiers",
		Name:        "Pricing — 3 tiers",
		Description: "Three pricing cards with headings, prices and buttons.",
		Category:    "Pricing",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func faqAccordion() Pattern {
	nodes := mustNodes(`[
		{"id":"h1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"lg","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"h2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"h3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Frequently asked questions","marks":[]}] },"level":2},"settings":{"align":"left","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"h4","block":"core/accordion","version":1,"props":{},"settings":{"variant":"bordered"},"children":[
					{"id":"h5","block":"core/accordion-item","version":1,"props":{"title":"What is Stratum?"},"settings":{},"children":[
						{"id":"h6","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Stratum is a modern self-hosted CMS written in Go, focused on semantic content and composable blocks.","marks":[]}] }},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"},"children":[]}
					]},
					{"id":"h7","block":"core/accordion-item","version":1,"props":{"title":"Do I need to write code?"},"settings":{},"children":[
						{"id":"h8","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"No. You can build pages, templates and site parts using only blocks and patterns. Custom code is optional.","marks":[]}] }},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"},"children":[]}
					]},
					{"id":"h9","block":"core/accordion-item","version":1,"props":{"title":"Can I use my own theme?"},"settings":{},"children":[
						{"id":"h10","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Yes. Themes control markup and CSS. Content stays presentation-independent.","marks":[]}] }},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"},"children":[]}
					]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "faq-accordion",
		Name:        "FAQ — accordion",
		Description: "Section with heading and FAQ accordion.",
		Category:    "FAQ",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func contentImageText() Pattern {
	nodes := mustNodes(`[
		{"id":"i1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"i2","block":"core/columns","version":1,"props":{},"settings":{"columns":2,"ratio":"equal","gap":"lg","mobileStack":true},"children":[
				{"id":"i3","block":"core/image","version":1,"props":{"mediaId":"","alt":"","caption":""},"settings":{"align":"none","decorative":true,"sizes":"","priority":"normal","radius":"md","aspect":"4:3","fit":"cover"},"children":[]},
				{"id":"i4","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"center","wrap":false,"width":"auto"},"children":[
					{"id":"i5","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Crafted for clarity","marks":[]}] },"level":2},"settings":{"align":"left","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
					{"id":"i6","block":"core/text","version":2,"props":{"text":{"version":1,"content":[{"text":"Our editor is HTML-first and server-driven. You shape content, Stratum handles the rest.","marks":[]}] }},"settings":{"align":"left","tone":"muted","size":"md","maxWidth":"none"},"children":[]},
					{"id":"i7","block":"core/button","version":1,"props":{"label":"Learn more","url":"#"},"settings":{"variant":"outline","size":"md","width":"auto","align":"left","openInNewTab":false},"children":[]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "content-image-text",
		Name:        "Content — image + text",
		Description: "Two-column image and text.",
		Category:    "Content",
		Contexts:    []string{"entry", "single-template", "site-part"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func latestPosts() Pattern {
	nodes := mustNodes(`[
		{"id":"j1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"j2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"j3","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Latest posts","marks":[]}] },"level":2},"settings":{"align":"left","visualSize":"lg","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"j4","block":"core/collection","version":2,"props":{},"settings":{"contentType":"post","limit":3,"offset":0,"source":"query","excludeCurrent":false,"orderBy":"entry.published_at","direction":"desc","termId":"","filters":[]},"children":[
					{"id":"j5","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[
						{"id":"j6","block":"core/entry-title","version":1,"props":{},"settings":{"level":3,"align":"left","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"j7","block":"core/entry-excerpt","version":1,"props":{},"settings":{"align":"left","tone":"muted","size":"md"},"children":[]},
						{"id":"j8","block":"core/entry-publish-date","version":1,"props":{},"settings":{"format":"long","align":"left"},"children":[]}
					]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "latest-posts",
		Name:        "Blog — latest posts",
		Description: "Collection of latest posts with card layout.",
		Category:    "Blog",
		Contexts:    []string{"entry", "single-template"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func archiveCollectionGrid() Pattern {
	nodes := mustNodes(`[
		{"id":"k1","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"k2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"k3","block":"core/archive-title","version":1,"props":{},"settings":{"level":1,"align":"left"},"children":[]},
				{"id":"k4","block":"core/archive-description","version":1,"props":{},"settings":{"align":"left"},"children":[]},
				{"id":"k5","block":"core/collection","version":2,"props":{},"settings":{"contentType":"post","limit":9,"offset":0,"source":"context","excludeCurrent":false,"orderBy":"entry.published_at","direction":"desc","termId":"","filters":[]},"children":[
					{"id":"k6","block":"core/card","version":1,"props":{},"settings":{"variant":"default","padding":"md","radius":"md","align":"start"},"children":[
						{"id":"k7","block":"core/entry-media","version":1,"props":{"source":"entry.featured_media"},"settings":{"alt":"","sizes":"100vw"},"children":[]},
						{"id":"k8","block":"core/entry-title","version":1,"props":{},"settings":{"level":3,"align":"left","visualSize":"md","tone":"default","maxWidth":"none"},"children":[]},
						{"id":"k9","block":"core/entry-excerpt","version":1,"props":{},"settings":{"align":"left","tone":"muted","size":"md"},"children":[]}
					]}
				]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "archive-collection-grid",
		Name:        "Archive — collection grid",
		Description: "Archive title with collection grid.",
		Category:    "Blog",
		Contexts:    []string{"archive-template"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

func singleArticle() Pattern {
	nodes := mustNodes(`[
		{"id":"l1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"lg","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[
			{"id":"l2","block":"core/stack","version":1,"props":{},"settings":{"direction":"vertical","gap":"md","align":"stretch","justify":"start","wrap":false,"width":"auto"},"children":[
				{"id":"l3","block":"core/entry-title","version":1,"props":{},"settings":{"level":1,"align":"left","visualSize":"xl","tone":"default","maxWidth":"none"},"children":[]},
				{"id":"l4","block":"core/entry-publish-date","version":1,"props":{},"settings":{"format":"long","align":"left"},"children":[]},
				{"id":"l5","block":"core/entry-media","version":1,"props":{"source":"entry.featured_media"},"settings":{"alt":"","sizes":"100vw"},"children":[]}
			]}
		]}
	]`)
	return Pattern{
		ID:          "single-article",
		Name:        "Single — article",
		Description: "Entry title, date and featured media for single templates.",
		Category:    "Content",
		Contexts:    []string{"single-template"},
		Source:      "core",
		Document:    doc(nodes),
	}
}

// Ensure document package is referenced to avoid import error
var _ = document.Document{}
