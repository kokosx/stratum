package blocks

import (
	"context"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/rendering"
)

// fakeMedia resolves a single known id so image rendering can be tested.
type fakeMedia struct{}

func (fakeMedia) MediaView(_ context.Context, id string) (rendering.MediaView, bool) {
	if id == "m1" {
		return rendering.MediaView{Src: "/img.png", Alt: "Stored alt", Width: 100, Height: 100, SrcSet: "/img.png 1x"}, true
	}
	return rendering.MediaView{}, false
}

// ============================================================
// Schema definitions for testing
// ============================================================

const headingSchema = `{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""},"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":2}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"visualSize":{"type":"string","enum":["auto","sm","md","lg","xl"],"default":"auto"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"maxWidth":{"type":"string","enum":["normal","wide","none"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"heading","fields":{"props.text":{"label":"Text","control":"textarea"},"props.level":{"label":"Level","control":"select"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.visualSize":{"label":"Visual size","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.maxWidth":{"label":"Max width","control":"segmented","group":"Layout"}}}}`

const textSchema = `{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"maxWidth":{"type":"string","enum":["narrow","normal","wide","none"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"text","fields":{"props.text":{"label":"Text","control":"textarea"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.maxWidth":{"label":"Max width","control":"segmented","group":"Layout"}}}}`

const buttonSchema = `{"schemaVersion":1,"props":{"type":"object","required":["label","url"],"properties":{"label":{"type":"string","default":"Button"},"url":{"type":"string","default":"#"}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["primary","secondary","outline","ghost"],"default":"primary"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"},"width":{"type":"string","enum":["auto","full"],"default":"auto"},"align":{"type":"string","enum":["left","center","right"],"default":"left"},"openInNewTab":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"button","fields":{"props.label":{"label":"Label","control":"text"},"props.url":{"label":"URL","control":"text"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.openInNewTab":{"label":"Open in new tab","control":"checkbox","group":"Link"}}}}`

const sectionSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"width":{"type":"string","enum":["content","wide","full"],"default":"content"},"verticalSpacing":{"type":"string","enum":["none","sm","md","lg","xl"],"default":"md"},"horizontalPadding":{"type":"string","enum":["none","sm","md","lg"],"default":"md"},"align":{"type":"string","enum":["left","center"],"default":"left"},"background":{"type":"string","enum":["default","surface","muted","primary"],"default":"default"},"minHeight":{"type":"string","enum":["auto","small","medium","screen"],"default":"auto"},"anchorID":{"type":"string","default":""}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"section","fields":{"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.verticalSpacing":{"label":"Vertical spacing","control":"select","group":"Layout"},"settings.horizontalPadding":{"label":"Horizontal padding","control":"select","group":"Layout"},"settings.align":{"label":"Content alignment","control":"segmented","group":"Style"},"settings.background":{"label":"Background","control":"select","group":"Style"},"settings.minHeight":{"label":"Min height","control":"select","group":"Layout"},"settings.anchorID":{"label":"Anchor ID","control":"text","group":"Advanced"}}}}`

const stackSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"direction":{"type":"string","enum":["vertical","horizontal"],"default":"vertical"},"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"md"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"stretch"},"justify":{"type":"string","enum":["start","center","end","between"],"default":"start"},"wrap":{"type":"boolean","default":false},"width":{"type":"string","enum":["auto","full"],"default":"auto"}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"stack","fields":{"settings.direction":{"label":"Direction","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Cross axis","control":"segmented","group":"Layout"},"settings.justify":{"label":"Main axis","control":"segmented","group":"Layout"},"settings.wrap":{"label":"Wrap","control":"checkbox","group":"Layout"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"}}}}`

const gridSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"columns":{"type":"integer","enum":[1,2,3,4],"default":2},"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"md"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"stretch"},"equalHeight":{"type":"boolean","default":false}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"grid","fields":{"settings.columns":{"label":"Columns","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Align","control":"segmented","group":"Layout"},"settings.equalHeight":{"label":"Equal height","control":"checkbox","group":"Layout"}}}}`

const coreCardSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["default","outlined","elevated","muted"],"default":"default"},"padding":{"type":"string","enum":["sm","md","lg"],"default":"md"},"radius":{"type":"string","enum":["sm","md","lg"],"default":"md"},"align":{"type":"string","enum":["start","center","end"],"default":"start"}}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"card","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.padding":{"label":"Padding","control":"segmented","group":"Style"},"settings.radius":{"label":"Radius","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}`

const accordionSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["minimal","bordered","cards"],"default":"minimal"}}},"children":{"mode":"allowed","blocks":["core/accordion-item"],"min":1},"editor":{"category":"design","icon":"accordion","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}`

const accordionItemSchema = `{"schemaVersion":1,"props":{"type":"object","required":["title"],"properties":{"title":{"type":"string","default":"Item"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"accordion-item","fields":{"props.title":{"label":"Title","control":"text"}}}}`

const dividerSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"style":{"type":"string","enum":["solid","dashed"],"default":"solid"},"width":{"type":"string","enum":["content","full"],"default":"full"},"spacing":{"type":"string","enum":["sm","md","lg"],"default":"md"}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"divider","fields":{"settings.style":{"label":"Style","control":"segmented","group":"Style"},"settings.width":{"label":"Width","control":"segmented","group":"Layout"},"settings.spacing":{"label":"Spacing","control":"select","group":"Layout"}}}}`

const quoteSchema = `{"schemaVersion":1,"props":{"type":"object","required":["text"],"properties":{"text":{"type":"string","default":""},"citation":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"style":{"type":"string","enum":["simple","bordered","large"],"default":"simple"},"align":{"type":"string","enum":["left","center"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"quote","fields":{"props.text":{"label":"Quote text","control":"textarea"},"props.citation":{"label":"Citation","control":"text"},"settings.style":{"label":"Style","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"}}}}`

const rowSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"md"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"stretch"},"justify":{"type":"string","enum":["start","center","end","between"],"default":"start"},"wrap":{"type":"boolean","default":false},"reverse":{"type":"boolean","default":false}}},"children":{"mode":"any","min":0},"editor":{"category":"layout","icon":"row","fields":{"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Cross axis","control":"segmented","group":"Layout"},"settings.justify":{"label":"Main axis","control":"segmented","group":"Layout"},"settings.wrap":{"label":"Wrap","control":"checkbox","group":"Layout"},"settings.reverse":{"label":"Reverse","control":"checkbox","group":"Layout"}}}}`

const listSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{"items":{"type":"string","default":"First item\nSecond item"}}},"settings":{"type":"object","properties":{"ordered":{"type":"boolean","default":false},"marker":{"type":"string","enum":["disc","circle","square","check","none"],"default":"disc"},"start":{"type":"integer","default":1}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"list","fields":{"props.items":{"label":"Items (one per line)","control":"textarea","group":"Content"},"settings.ordered":{"label":"Ordered","control":"checkbox","group":"Style"},"settings.marker":{"label":"Marker","control":"select","group":"Style"},"settings.start":{"label":"Start number","control":"number","group":"Style"}}}}`

const imageSchema = `{"schemaVersion":1,"props":{"type":"object","required":["mediaId"],"properties":{"mediaId":{"type":"string","default":""},"alt":{"type":"string","default":""},"caption":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["none","left","center"],"default":"none"},"decorative":{"type":"boolean","default":false},"sizes":{"type":"string","default":""},"eager":{"type":"boolean","default":false}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"image","fields":{"props.mediaId":{"label":"Image","control":"media","group":"Content"},"props.alt":{"label":"Alt text","control":"text","group":"Accessibility"},"settings.decorative":{"label":"Decorative (no alt)","control":"checkbox","group":"Accessibility"},"props.caption":{"label":"Caption","control":"text","group":"Content"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.sizes":{"label":"Sizes (responsive)","control":"text","group":"Advanced"},"settings.eager":{"label":"Load eagerly (LCP)","control":"checkbox","group":"Advanced"}}}}`

const buttonGroupSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"direction":{"type":"string","enum":["horizontal","vertical"],"default":"horizontal"},"gap":{"type":"string","enum":["none","xs","sm","md","lg","xl"],"default":"sm"},"align":{"type":"string","enum":["start","center","end","stretch"],"default":"start"},"wrap":{"type":"boolean","default":true}}},"children":{"mode":"allowed","blocks":["core/button"],"min":1},"editor":{"category":"design","icon":"button-group","fields":{"settings.direction":{"label":"Direction","control":"segmented","group":"Layout"},"settings.gap":{"label":"Gap","control":"select","group":"Layout"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"},"settings.wrap":{"label":"Wrap","control":"checkbox","group":"Layout"}}}}`

const iconSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{"name":{"type":"string","enum":["arrow-right","check","x","info","warning","star","menu","search","plus","chevron-down","phone","mail","location","link","external","heart"],"default":"check"},"label":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"size":{"type":"string","enum":["sm","md","lg","xl"],"default":"md"},"color":{"type":"string","enum":["default","muted","primary","inherit"],"default":"default"}}},"children":{"mode":"none"},"editor":{"category":"media","icon":"icon","fields":{"props.name":{"label":"Icon","control":"select","group":"Content"},"props.label":{"label":"Accessibility label","control":"text","group":"Content"},"settings.size":{"label":"Size","control":"segmented","group":"Style"},"settings.color":{"label":"Color","control":"select","group":"Style"}}}}`

const calloutSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{"title":{"type":"string","default":""},"text":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["info","success","warning","error"],"default":"info"},"icon":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"design","icon":"callout","fields":{"props.title":{"label":"Title","control":"text","group":"Content"},"props.text":{"label":"Message","control":"textarea","group":"Content"},"settings.variant":{"label":"Variant","control":"segmented","group":"Style"},"settings.icon":{"label":"Show icon","control":"checkbox","group":"Style"}}}}`

const codeSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{"code":{"type":"string","default":""},"filename":{"type":"string","default":""},"language":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"showLineNumbers":{"type":"boolean","default":false},"copyButton":{"type":"boolean","default":true}}},"children":{"mode":"none"},"editor":{"category":"text","icon":"code","fields":{"props.code":{"label":"Code","control":"textarea","group":"Content"},"props.filename":{"label":"Filename","control":"text","group":"Content"},"props.language":{"label":"Language","control":"text","group":"Content"},"settings.showLineNumbers":{"label":"Line numbers","control":"checkbox","group":"Style"},"settings.copyButton":{"label":"Copy button","control":"checkbox","group":"Style"}}}}`

const tabsSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["underline","boxed"],"default":"underline"}}},"children":{"mode":"allowed","blocks":["core/tab"],"min":1},"editor":{"category":"design","icon":"tabs","fields":{"settings.variant":{"label":"Variant","control":"segmented","group":"Style"}}}}`

const tabSchema = `{"schemaVersion":1,"props":{"type":"object","required":["label"],"properties":{"label":{"type":"string","default":"Tab"}}},"settings":{"type":"object","properties":{}},"children":{"mode":"any","min":0},"editor":{"category":"design","icon":"tab","fields":{"props.label":{"label":"Tab label","control":"text"}}}}`

const entryTitleSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"level":{"type":"integer","enum":[1,2,3,4,5,6],"default":1},"align":{"type":"string","enum":["left","center","right"],"default":"left"},"visualSize":{"type":"string","enum":["auto","sm","md","lg","xl"],"default":"auto"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"maxWidth":{"type":"string","enum":["normal","wide","none"],"default":"none"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"heading","fields":{"settings.level":{"label":"Level","control":"select","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.visualSize":{"label":"Visual size","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.maxWidth":{"label":"Max width","control":"segmented","group":"Layout"}}}}`

const entryExcerptSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"},"tone":{"type":"string","enum":["default","muted","primary"],"default":"default"},"size":{"type":"string","enum":["sm","md","lg"],"default":"md"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"text","fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Style"},"settings.tone":{"label":"Tone","control":"select","group":"Style"},"settings.size":{"label":"Size","control":"segmented","group":"Style"}}}}`

const entryPublishDateSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"format":{"type":"string","enum":["long","iso"],"default":"long"},"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"calendar","fields":{"settings.format":{"label":"Format","control":"segmented","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}`

const siteNameSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"link":{"type":"boolean","default":true},"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"site","fields":{"settings.link":{"label":"Link to home","control":"checkbox","group":"Style"},"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}`

const siteTaglineSchema = `{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"align":{"type":"string","enum":["left","center","right"],"default":"left"}}},"children":{"mode":"none"},"editor":{"category":"dynamic","icon":"text","fields":{"settings.align":{"label":"Alignment","control":"segmented","group":"Layout"}}}}`

// ============================================================
// Schema parsing tests
// ============================================================

func TestCoreBlockSchemasParse(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{"heading", headingSchema},
		{"text", textSchema},
		{"button", buttonSchema},
		{"section", sectionSchema},
		{"stack", stackSchema},
		{"grid", gridSchema},
		{"card", coreCardSchema},
		{"accordion", accordionSchema},
		{"accordion-item", accordionItemSchema},
		{"divider", dividerSchema},
		{"quote", quoteSchema},
		{"row", rowSchema},
		{"list", listSchema},
		{"image", imageSchema},
		{"button-group", buttonGroupSchema},
		{"icon", iconSchema},
		{"callout", calloutSchema},
		{"code", codeSchema},
		{"tabs", tabsSchema},
		{"tab", tabSchema},
		{"entry-title", entryTitleSchema},
		{"entry-excerpt", entryExcerptSchema},
		{"entry-publish-date", entryPublishDateSchema},
		{"site-name", siteNameSchema},
		{"site-tagline", siteTaglineSchema},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := ParseSchema(test.schema)
			if err != nil {
				t.Fatalf("ParseSchema(%s) error: %v", test.name, err)
			}
			if schema.SchemaVersion != 1 {
				t.Fatalf("schemaVersion = %d, want 1", schema.SchemaVersion)
			}
		})
	}
}

func TestHeadingSchemaDefaults(t *testing.T) {
	schema, err := ParseSchema(headingSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := schema.Props.Properties["level"].Default; got != float64(2) {
		t.Fatalf("level default = %v, want 2", got)
	}
	if got := schema.Settings.Properties["align"].Default; got != "left" {
		t.Fatalf("align default = %v, want left", got)
	}
	if got := schema.Settings.Properties["visualSize"].Default; got != "auto" {
		t.Fatalf("visualSize default = %v, want auto", got)
	}
	if got := schema.Settings.Properties["tone"].Default; got != "default" {
		t.Fatalf("tone default = %v, want default", got)
	}
	if got := schema.Settings.Properties["maxWidth"].Default; got != "none" {
		t.Fatalf("maxWidth default = %v, want none", got)
	}
}

func TestButtonSchemaDefaults(t *testing.T) {
	schema, err := ParseSchema(buttonSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := schema.Settings.Properties["variant"].Default; got != "primary" {
		t.Fatalf("variant default = %v, want primary", got)
	}
	if got := schema.Settings.Properties["size"].Default; got != "md" {
		t.Fatalf("size default = %v, want md", got)
	}
	if got := schema.Settings.Properties["openInNewTab"].Default; got != false {
		t.Fatalf("openInNewTab default = %v, want false", got)
	}
}

func TestSectionSchemaDefaults(t *testing.T) {
	schema, err := ParseSchema(sectionSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := schema.Settings.Properties["width"].Default; got != "content" {
		t.Fatalf("width default = %v, want content", got)
	}
	if got := schema.Settings.Properties["verticalSpacing"].Default; got != "md" {
		t.Fatalf("verticalSpacing default = %v, want md", got)
	}
	if got := schema.Settings.Properties["background"].Default; got != "default" {
		t.Fatalf("background default = %v, want default", got)
	}
}

func TestAccordionChildrenMode(t *testing.T) {
	schema, err := ParseSchema(accordionSchema)
	if err != nil {
		t.Fatal(err)
	}
	if schema.Children.Mode != "allowed" {
		t.Fatalf("children.mode = %s, want allowed", schema.Children.Mode)
	}
	if len(schema.Children.Blocks) != 1 || schema.Children.Blocks[0] != "core/accordion-item" {
		t.Fatalf("children.blocks = %v, want [core/accordion-item]", schema.Children.Blocks)
	}
	if schema.Children.Min == nil || *schema.Children.Min != 1 {
		t.Fatalf("children.min = %v, want 1", schema.Children.Min)
	}
}

// ============================================================
// Validation tests
// ============================================================

func TestHeadingValidation(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "heading", 1, true, headingSchema, `<h{{ .Props.level }}>{{ .Props.text }}</h{{ .Props.level }}>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"valid", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Hello","level":2},"settings":{"align":"left","visualSize":"auto","tone":"default","maxWidth":"none"}}]}`, ""},
		{"missing text", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"level":2},"settings":{}}]}`, "field is required"},
		{"invalid level", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Hello","level":7},"settings":{}}]}`, "expected one of"},
		{"invalid align", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Hello"},"settings":{"align":"justify"}}]}`, "expected one of"},
		{"invalid visualSize", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Hello"},"settings":{"visualSize":"xxl"}}]}`, "expected one of"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := decodeTestDocument(t, test.input)
			err := registry.ValidateDocument(doc)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
			}
		})
	}
}

func TestButtonValidation(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "button", 1, true, buttonSchema, `<a>{{ .Props.label }}</a>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"valid", `{"version":1,"nodes":[{"id":"b","block":"core/button","version":1,"props":{"label":"Click","url":"/go"},"settings":{"variant":"primary","size":"md","width":"auto","align":"left","openInNewTab":false}}]}`, ""},
		{"missing label", `{"version":1,"nodes":[{"id":"b","block":"core/button","version":1,"props":{"url":"/go"},"settings":{}}]}`, "field is required"},
		{"missing url", `{"version":1,"nodes":[{"id":"b","block":"core/button","version":1,"props":{"label":"Click"},"settings":{}}]}`, "field is required"},
		{"invalid variant", `{"version":1,"nodes":[{"id":"b","block":"core/button","version":1,"props":{"label":"Click","url":"/go"},"settings":{"variant":"danger"}}]}`, "expected one of"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := decodeTestDocument(t, test.input)
			err := registry.ValidateDocument(doc)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
			}
		})
	}
}

func TestAccordionNestingRules(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "accordion", 1, true, accordionSchema, `<div>{{ .Children }}</div>`),
		customDefinition("core", "accordion-item", 1, true, accordionItemSchema, `<details>{{ .Children }}</details>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("accordion accepts accordion-item", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"acc","block":"core/accordion","version":1,"props":{},"settings":{"variant":"minimal"},"children":[{"id":"item","block":"core/accordion-item","version":1,"props":{"title":"Q1"},"settings":{},"children":[{"id":"txt","block":"core/text","version":1,"props":{"text":"Answer"},"settings":{}}]}]}]}`)
		if err := registry.ValidateDocument(doc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("accordion rejects non-accordion-item", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"acc","block":"core/accordion","version":1,"props":{},"settings":{"variant":"minimal"},"children":[{"id":"txt","block":"core/text","version":1,"props":{"text":"Bad"},"settings":{}}]}]}`)
		if err := registry.ValidateDocument(doc); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("error = %v, want 'not allowed'", err)
		}
	})

	t.Run("accordion requires at least 1 child", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"acc","block":"core/accordion","version":1,"props":{},"settings":{"variant":"minimal"}}]}`)
		if err := registry.ValidateDocument(doc); err == nil || !strings.Contains(err.Error(), "requires at least 1") {
			t.Fatalf("error = %v, want 'requires at least 1'", err)
		}
	})
}

func TestSectionAcceptsAnyChildren(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "section", 1, true, sectionSchema, `<section>{{ .Children }}</section>`),
		customDefinition("core", "heading", 1, true, headingSchema, `<h2>{{ .Props.text }}</h2>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Title","level":1},"settings":{}},{"id":"p","block":"core/text","version":1,"props":{"text":"Body"},"settings":{}}]}]}`)
	if err := registry.ValidateDocument(doc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDividerRejectsChildren(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "divider", 1, true, dividerSchema, `<hr>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"div","block":"core/divider","version":1,"props":{},"settings":{"style":"solid","width":"full","spacing":"md"},"children":[{"id":"txt","block":"core/text","version":1,"props":{"text":"Bad"},"settings":{}}]}]}`)
	if err := registry.ValidateDocument(doc); err == nil || !strings.Contains(err.Error(), "does not allow child blocks") {
		t.Fatalf("error = %v, want 'does not allow child blocks'", err)
	}
}

// ============================================================
// Rendering tests
// ============================================================

func TestHeadingRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "heading", 1, true, headingSchema, `{{ if integerEquals .Props.level 1 }}<h1 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h1>{{ else if integerEquals .Props.level 3 }}<h3 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h3>{{ else }}<h2 class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h2>{{ end }}`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		input string
		want  string
	}{
		{"h1", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Title","level":1},"settings":{"align":"center","visualSize":"lg","tone":"primary","maxWidth":"wide"}}]}`, `<h1 class="stratum-heading stratum-align-center stratum-heading-size-lg stratum-tone-primary stratum-maxw-wide">Title</h1>`},
		{"h2 default", `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Sub","level":2},"settings":{"align":"left","visualSize":"auto","tone":"default","maxWidth":"none"}}]}`, `<h2 class="stratum-heading stratum-align-left stratum-heading-size-auto stratum-tone-default stratum-maxw-none">Sub</h2>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := decodeTestDocument(t, test.input)
			rendered, err := registry.RenderDocument(doc)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(rendered); got != test.want {
				t.Fatalf("rendered = %q, want %q", got, test.want)
			}
		})
	}
}

func TestButtonRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "button", 1, true, buttonSchema, `<div class="stratum-btn-wrap stratum-align-{{ .Settings.align }}"><a class="stratum-button stratum-button-{{ .Settings.variant }} stratum-button-size-{{ .Settings.size }}" href="{{ .Props.url }}"{{ if .Settings.openInNewTab }} target="_blank" rel="noopener noreferrer"{{ end }}>{{ .Props.label }}</a></div>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("internal link", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"b","block":"core/button","version":1,"props":{"label":"Go","url":"/about"},"settings":{"variant":"primary","size":"md","width":"auto","align":"left","openInNewTab":false}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if strings.Contains(got, "target=") || strings.Contains(got, "rel=") {
			t.Fatalf("internal link should not have target/rel: %q", got)
		}
		if !strings.Contains(got, `href="/about"`) {
			t.Fatalf("missing href: %q", got)
		}
	})

	t.Run("external new tab", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"b","block":"core/button","version":1,"props":{"label":"Go","url":"https://example.com"},"settings":{"variant":"outline","size":"lg","width":"auto","align":"center","openInNewTab":true}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, `target="_blank"`) {
			t.Fatalf("missing target=_blank: %q", got)
		}
		if !strings.Contains(got, `rel="noopener noreferrer"`) {
			t.Fatalf("missing rel=noopener noreferrer: %q", got)
		}
	})
}

func TestSectionRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "section", 1, true, sectionSchema, `<section{{ if .Settings.anchorID }} id="{{ .Settings.anchorID }}"{{ end }} class="stratum-section stratum-section-width-{{ .Settings.width }} stratum-section-vspace-{{ .Settings.verticalSpacing }} stratum-section-hpad-{{ .Settings.horizontalPadding }} stratum-section-align-{{ .Settings.align }} stratum-section-bg-{{ .Settings.background }} stratum-section-minh-{{ .Settings.minHeight }}">{{ .Children }}</section>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("with anchor", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{"width":"wide","verticalSpacing":"lg","horizontalPadding":"md","align":"center","background":"muted","minHeight":"medium","anchorID":"features"},"children":[{"id":"txt","block":"core/text","version":1,"props":{"text":"Content"},"settings":{}}]}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, `id="features"`) {
			t.Fatalf("missing anchor id: %q", got)
		}
		if !strings.Contains(got, "stratum-section-width-wide") {
			t.Fatalf("missing width class: %q", got)
		}
		if !strings.Contains(got, "stratum-section-bg-muted") {
			t.Fatalf("missing background class: %q", got)
		}
	})

	t.Run("without anchor", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"txt","block":"core/text","version":1,"props":{"text":"Content"},"settings":{}}]}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if strings.Contains(got, "id=") {
			t.Fatalf("should not have id attribute: %q", got)
		}
	})
}

func TestAccordionRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "accordion", 1, true, accordionSchema, `<div class="stratum-accordion stratum-accordion-{{ .Settings.variant }}">{{ .Children }}</div>`),
		customDefinition("core", "accordion-item", 1, true, accordionItemSchema, `<details class="stratum-accordion-item"><summary class="stratum-accordion-trigger">{{ .Props.title }}</summary><div class="stratum-accordion-content">{{ .Children }}</div></details>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"acc","block":"core/accordion","version":1,"props":{},"settings":{"variant":"bordered"},"children":[{"id":"item1","block":"core/accordion-item","version":1,"props":{"title":"What is Stratum?"},"settings":{},"children":[{"id":"a1","block":"core/text","version":1,"props":{"text":"A modern CMS."},"settings":{}}]},{"id":"item2","block":"core/accordion-item","version":1,"props":{"title":"How does it work?"},"settings":{},"children":[{"id":"a2","block":"core/text","version":1,"props":{"text":"Structured documents."},"settings":{}}]}]}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	if !strings.Contains(got, "stratum-accordion-bordered") {
		t.Fatalf("missing bordered class: %q", got)
	}
	if !strings.Contains(got, "<details") {
		t.Fatalf("missing details element: %q", got)
	}
	if !strings.Contains(got, "What is Stratum?") {
		t.Fatalf("missing accordion title: %q", got)
	}
	if !strings.Contains(got, "A modern CMS.") {
		t.Fatalf("missing accordion content: %q", got)
	}
}

func TestQuoteRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "quote", 1, true, quoteSchema, `<blockquote class="stratum-quote stratum-quote-{{ .Settings.style }} stratum-quote-align-{{ .Settings.align }}"><p class="stratum-quote-text">{{ .Props.text }}</p>{{ if .Props.citation }}<cite class="stratum-quote-citation">{{ .Props.citation }}</cite>{{ end }}</blockquote>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("with citation", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"q","block":"core/quote","version":1,"props":{"text":"Simplicity is the ultimate sophistication.","citation":"Leonardo da Vinci"},"settings":{"style":"bordered","align":"left"}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "stratum-quote-bordered") {
			t.Fatalf("missing bordered class: %q", got)
		}
		if !strings.Contains(got, "Leonardo da Vinci") {
			t.Fatalf("missing citation: %q", got)
		}
	})

	t.Run("without citation", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"q","block":"core/quote","version":1,"props":{"text":"Hello world","citation":""},"settings":{"style":"simple","align":"left"}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if strings.Contains(got, "<cite") {
			t.Fatalf("should not have cite element: %q", got)
		}
	})
}

func TestDividerRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "divider", 1, true, dividerSchema, `<hr class="stratum-divider stratum-divider-{{ .Settings.style }} stratum-divider-width-{{ .Settings.width }} stratum-divider-space-{{ .Settings.spacing }}">`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"d","block":"core/divider","version":1,"props":{},"settings":{"style":"dashed","width":"content","spacing":"lg"}}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	if !strings.Contains(got, "stratum-divider-dashed") {
		t.Fatalf("missing dashed class: %q", got)
	}
	if !strings.Contains(got, "stratum-divider-width-content") {
		t.Fatalf("missing width-content class: %q", got)
	}
}

// ============================================================
// Default application tests
// ============================================================

func TestDefaultsAppliedOnRender(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "heading", 1, true, headingSchema, `<h{{ .Props.level }} class="stratum-heading stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ .Props.text }}</h{{ .Props.level }}>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"h","block":"core/heading","version":1,"props":{"text":"Hello"}}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	if !strings.Contains(got, `stratum-align-left`) {
		t.Fatalf("default align not applied: %q", got)
	}
	if !strings.Contains(got, `stratum-heading-size-auto`) {
		t.Fatalf("default visualSize not applied: %q", got)
	}
	if !strings.Contains(got, `stratum-tone-default`) {
		t.Fatalf("default tone not applied: %q", got)
	}
}

// ============================================================
// Editor catalog tests
// ============================================================

func TestAllCoreBlocksInCatalog(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "heading", 1, true, headingSchema, `<h2>{{ .Props.text }}</h2>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
		customDefinition("core", "button", 1, true, buttonSchema, `<a>{{ .Props.label }}</a>`),
		customDefinition("core", "section", 1, true, sectionSchema, `<section>{{ .Children }}</section>`),
		customDefinition("core", "stack", 1, true, stackSchema, `<div>{{ .Children }}</div>`),
		customDefinition("core", "grid", 1, true, gridSchema, `<div>{{ .Children }}</div>`),
		customDefinition("core", "card", 1, true, coreCardSchema, `<div>{{ .Children }}</div>`),
		customDefinition("core", "accordion", 1, true, accordionSchema, `<div>{{ .Children }}</div>`),
		customDefinition("core", "accordion-item", 1, true, accordionItemSchema, `<details>{{ .Children }}</details>`),
		customDefinition("core", "divider", 1, true, dividerSchema, `<hr>`),
		customDefinition("core", "quote", 1, true, quoteSchema, `<blockquote>{{ .Props.text }}</blockquote>`),
		customDefinition("core", "row", 1, true, rowSchema, `<div class="stratum-row stratum-row-gap-{{ .Settings.gap }} stratum-row-align-{{ .Settings.align }} stratum-row-justify-{{ .Settings.justify }}{{ if .Settings.wrap }} stratum-row-wrap{{ end }}{{ if .Settings.reverse }} stratum-row-reverse{{ end }}">{{ .Children }}</div>`),
		customDefinition("core", "list", 1, true, listSchema, `{{ $items := lines .Props.items }}{{ if .Settings.ordered }}<ol class="stratum-list stratum-list-ordered stratum-list-marker-{{ .Settings.marker }}"{{ if ne .Settings.start 1.0 }} start="{{ .Settings.start }}"{{ end }}>{{ else }}<ul class="stratum-list stratum-list-marker-{{ .Settings.marker }}">{{ end }}{{ range $items }}<li>{{ . }}</li>{{ end }}{{ if .Settings.ordered }}</ol>{{ else }}</ul>{{ end }}`),
		customDefinition("core", "image", 1, true, imageSchema, `{{ $m := media .Props.mediaId }}{{ if $m.Src }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}"{{ if .Settings.sizes }} sizes="{{ .Settings.sizes }}"{{ end }}{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}" decoding="async" fetchpriority="{{ if .Settings.eager }}high{{ end }}">{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing">Image unavailable</div>{{ end }}`),
		customDefinition("core", "button-group", 1, true, buttonGroupSchema, `<div class="stratum-btn-group stratum-btn-group-dir-{{ .Settings.direction }} stratum-btn-group-gap-{{ .Settings.gap }} stratum-btn-group-align-{{ .Settings.align }}{{ if .Settings.wrap }} stratum-btn-group-wrap{{ end }}">{{ .Children }}</div>`),
		customDefinition("core", "icon", 1, true, iconSchema, `<span class="stratum-icon stratum-icon-size-{{ .Settings.size }} stratum-icon-color-{{ .Settings.color }}"{{ if .Props.label }} role="img" aria-label="{{ .Props.label }}"{{ else }} aria-hidden="true"{{ end }}>{{ icon .Props.name }}</span>`),
		customDefinition("core", "callout", 1, true, calloutSchema, `<aside class="stratum-callout stratum-callout-{{ .Settings.variant }}">{{ if .Settings.icon }}<span class="stratum-callout-mark">{{ if eq .Settings.variant "info" }}{{ icon "info" }}{{ else if eq .Settings.variant "success" }}{{ icon "check" }}{{ else if eq .Settings.variant "warning" }}{{ icon "warning" }}{{ else }}{{ icon "x" }}{{ end }}</span>{{ end }}<div class="stratum-callout-content">{{ if .Props.title }}<p class="stratum-callout-title">{{ .Props.title }}</p>{{ end }}<div class="stratum-callout-body">{{ .Props.text }}</div></div></aside>`),
		customDefinition("core", "code", 1, true, codeSchema, `<figure class="stratum-code{{ if .Settings.showLineNumbers }} stratum-code-lines{{ end }}">{{ if .Props.filename }}<figcaption class="stratum-code-filename">{{ .Props.filename }}</figcaption>{{ end }}<pre class="stratum-code-pre"><code{{ if .Props.language }} data-lang="{{ .Props.language }}"{{ end }}>{{ .Props.code }}</code></pre>{{ if .Settings.copyButton }}<button type="button" class="stratum-code-copy" data-copy-code aria-label="Copy code">Copy</button>{{ end }}</figure>`),
		customDefinition("core", "tabs", 1, true, tabsSchema, `<div class="stratum-tabs stratum-tabs-{{ .Settings.variant }}" data-tabs><div class="stratum-tabs-nav" role="tablist" data-tabs-nav></div>{{ .Children }}</div>`),
		customDefinition("core", "tab", 1, true, tabSchema, `<section class="stratum-tab" data-label="{{ .Props.label }}">{{ .Children }}</section>`),
		customDefinition("core", "entry-title", 1, true, entryTitleSchema, `{{ $tag := tagFor .Settings.level }}{{ $o := tagOpen $tag }}{{ $c := tagClose $tag }}{{ $o }} class="stratum-entry-title stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ if .Context.Entry.Title }}{{ .Context.Entry.Title }}{{ else }}<span class="stratum-placeholder">Entry title</span>{{ end }}{{ $c }}`),
		customDefinition("core", "entry-excerpt", 1, true, entryExcerptSchema, `<p class="stratum-entry-excerpt stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }} stratum-text-size-{{ .Settings.size }}">{{ if .Context.Entry.Excerpt }}{{ .Context.Entry.Excerpt }}{{ else }}<span class="stratum-placeholder">Entry excerpt</span>{{ end }}</p>`),
		customDefinition("core", "entry-publish-date", 1, true, entryPublishDateSchema, `{{ $d := .Context.Entry.PublishDate }}{{ if eq .Settings.format "iso" }}{{ $d = .Context.Entry.PublishISO }}{{ end }}<time class="stratum-entry-date stratum-align-{{ .Settings.align }}"{{ if .Context.Entry.PublishISO }} datetime="{{ .Context.Entry.PublishISO }}"{{ end }}>{{ if $d }}{{ $d }}{{ else }}<span class="stratum-placeholder">Publish date</span>{{ end }}</time>`),
		customDefinition("core", "site-name", 1, true, siteNameSchema, `{{ if .Settings.link }}<a class="stratum-site-name-link" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}<span class="stratum-site-name stratum-align-{{ .Settings.align }}">{{ if .Context.Site.Name }}{{ .Context.Site.Name }}{{ else }}<span class="stratum-placeholder">Site name</span>{{ end }}</span>{{ if .Settings.link }}</a>{{ end }}`),
		customDefinition("core", "site-tagline", 1, true, siteTaglineSchema, `<p class="stratum-site-tagline stratum-align-{{ .Settings.align }}">{{ if .Context.Site.Tagline }}{{ .Context.Site.Tagline }}{{ else }}<span class="stratum-placeholder">Site tagline</span>{{ end }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	catalog := registry.EditorCatalog()
	if len(catalog) != 25 {
		t.Fatalf("catalog length = %d, want 25", len(catalog))
	}

	expected := map[string]bool{
		"core/heading":        false,
		"core/text":           false,
		"core/button":         false,
		"core/section":        false,
		"core/stack":          false,
		"core/grid":           false,
		"core/card":           false,
		"core/accordion":      false,
		"core/accordion-item": false,
		"core/divider":        false,
		"core/quote":          false,
		"core/row":            false,
		"core/list":           false,
		"core/image":          false,
		"core/button-group":   false,
		"core/icon":           false,
		"core/callout":        false,
		"core/code":           false,
		"core/tabs":           false,
		"core/tab":            false,
		"core/entry-title":    false,
		"core/entry-excerpt":  false,
		"core/entry-publish-date": false,
		"core/site-name":      false,
		"core/site-tagline":   false,
	}
	for _, def := range catalog {
		expected[def.Block] = true
	}
	for block, found := range expected {
		if !found {
			t.Fatalf("missing block %s in catalog", block)
		}
	}
}

// ============================================================
// Stage 1 block rendering tests
// ============================================================

func TestRowRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "row", 1, true, rowSchema, `<div class="stratum-row stratum-row-gap-{{ .Settings.gap }} stratum-row-align-{{ .Settings.align }} stratum-row-justify-{{ .Settings.justify }}{{ if .Settings.wrap }} stratum-row-wrap{{ end }}{{ if .Settings.reverse }} stratum-row-reverse{{ end }}">{{ .Children }}</div>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"r","block":"core/row","version":1,"props":{},"settings":{"gap":"lg","align":"center","justify":"between","wrap":true,"reverse":false},"children":[{"id":"t","block":"core/text","version":1,"props":{"text":"Hi"},"settings":{}}]}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	if !strings.Contains(got, "stratum-row-gap-lg") || !strings.Contains(got, "stratum-row-align-center") || !strings.Contains(got, "stratum-row-justify-between") || !strings.Contains(got, "stratum-row-wrap") {
		t.Fatalf("missing row classes: %q", got)
	}
	if !strings.Contains(got, "<p>Hi</p>") {
		t.Fatalf("child not rendered: %q", got)
	}
}

func TestListRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "list", 1, true, listSchema, `{{ $items := lines .Props.items }}{{ if .Settings.ordered }}<ol class="stratum-list stratum-list-ordered stratum-list-marker-{{ .Settings.marker }}"{{ if ne .Settings.start 1.0 }} start="{{ .Settings.start }}"{{ end }}>{{ else }}<ul class="stratum-list stratum-list-marker-{{ .Settings.marker }}">{{ end }}{{ range $items }}<li>{{ . }}</li>{{ end }}{{ if .Settings.ordered }}</ol>{{ else }}</ul>{{ end }}`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("unordered", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"l","block":"core/list","version":1,"props":{"items":"Alpha\nBeta\nGamma"},"settings":{"ordered":false,"marker":"check","start":1}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "<ul") || !strings.Contains(got, "stratum-list-marker-check") {
			t.Fatalf("missing ul/marker: %q", got)
		}
		if !strings.Contains(got, "<li>Alpha</li>") || !strings.Contains(got, "<li>Beta</li>") || !strings.Contains(got, "<li>Gamma</li>") {
			t.Fatalf("missing list items: %q", got)
		}
	})

	t.Run("ordered with start", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"l","block":"core/list","version":1,"props":{"items":"One\nTwo"},"settings":{"ordered":true,"marker":"disc","start":3}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "<ol") || !strings.Contains(got, "start=\"3\"") {
			t.Fatalf("missing ol/start: %q", got)
		}
	})
}

func TestImageRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "image", 1, true, imageSchema, `{{ $m := media .Props.mediaId }}{{ if $m.Src }}<figure class="stratum-image stratum-image-align-{{ .Settings.align }}"><img src="{{ $m.Src }}"{{ if $m.SrcSet }} srcset="{{ $m.SrcSet }}"{{ if .Settings.sizes }} sizes="{{ .Settings.sizes }}"{{ end }}{{ end }}{{ if $m.Width }} width="{{ $m.Width }}"{{ end }}{{ if $m.Height }} height="{{ $m.Height }}"{{ end }} alt="{{ if not .Settings.decorative }}{{ if .Props.alt }}{{ .Props.alt }}{{ else }}{{ $m.Alt }}{{ end }}{{ end }}" decoding="async" fetchpriority="{{ if .Settings.eager }}high{{ end }}">{{ if .Props.caption }}<figcaption class="stratum-image-caption">{{ .Props.caption }}</figcaption>{{ end }}</figure>{{ else }}<div class="stratum-image stratum-image-missing">Image unavailable</div>{{ end }}`),
	}}
	registry, err := NewRegistry(context.Background(), store, fakeMedia{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing media", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"i","block":"core/image","version":1,"props":{"mediaId":""},"settings":{"align":"none","decorative":false,"sizes":"","eager":false}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(rendered), "stratum-image-missing") {
			t.Fatalf("expected missing fallback: %q", rendered)
		}
	})

	t.Run("populated", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"i","block":"core/image","version":1,"props":{"mediaId":"m1","alt":"My alt","caption":"A caption"},"settings":{"align":"center","decorative":false,"sizes":"","eager":true}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, `<img src="/img.png"`) || !strings.Contains(got, `srcset="/img.png 1x"`) || !strings.Contains(got, `width="100"`) || !strings.Contains(got, `height="100"`) {
			t.Fatalf("missing img attributes: %q", got)
		}
		if !strings.Contains(got, `alt="My alt"`) {
			t.Fatalf("missing explicit alt: %q", got)
		}
		if !strings.Contains(got, "stratum-image-align-center") || !strings.Contains(got, "fetchpriority=\"high\"") {
			t.Fatalf("missing settings classes: %q", got)
		}
		if !strings.Contains(got, "<figcaption") || !strings.Contains(got, "A caption") {
			t.Fatalf("missing caption: %q", got)
		}
	})
}

func TestButtonGroupRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "button-group", 1, true, buttonGroupSchema, `<div class="stratum-btn-group stratum-btn-group-dir-{{ .Settings.direction }} stratum-btn-group-gap-{{ .Settings.gap }} stratum-btn-group-align-{{ .Settings.align }}{{ if .Settings.wrap }} stratum-btn-group-wrap{{ end }}">{{ .Children }}</div>`),
		customDefinition("core", "button", 1, true, buttonSchema, `<div class="stratum-btn-wrap"><a class="stratum-button stratum-button-{{ .Settings.variant }}">{{ .Props.label }}</a></div>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"bg","block":"core/button-group","version":1,"props":{},"settings":{"direction":"vertical","gap":"lg","align":"center","wrap":false},"children":[{"id":"b","block":"core/button","version":1,"props":{"label":"Go","url":"/"},"settings":{"variant":"primary","size":"md","width":"auto","align":"left","openInNewTab":false}}]}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	if !strings.Contains(got, "stratum-btn-group-dir-vertical") || !strings.Contains(got, "stratum-btn-group-gap-lg") || !strings.Contains(got, "stratum-btn-group-align-center") {
		t.Fatalf("missing button-group classes: %q", got)
	}
	if !strings.Contains(got, "Go") {
		t.Fatalf("child button not rendered: %q", got)
	}
}

func TestIconRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "icon", 1, true, iconSchema, `<span class="stratum-icon stratum-icon-size-{{ .Settings.size }} stratum-icon-color-{{ .Settings.color }}"{{ if .Props.label }} role="img" aria-label="{{ .Props.label }}"{{ else }} aria-hidden="true"{{ end }}>{{ icon .Props.name }}</span>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("decorative", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"ic","block":"core/icon","version":1,"props":{"name":"check","label":""},"settings":{"size":"lg","color":"primary"}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "stratum-icon-size-lg") || !strings.Contains(got, "stratum-icon-color-primary") {
			t.Fatalf("missing icon classes: %q", got)
		}
		if !strings.Contains(got, "<svg") || !strings.Contains(got, "aria-hidden=\"true\"") {
			t.Fatalf("missing svg/aria: %q", got)
		}
	})

	t.Run("labeled", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"ic","block":"core/icon","version":1,"props":{"name":"star","label":"Featured"},"settings":{"size":"md","color":"default"}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, `role="img"`) || !strings.Contains(got, `aria-label="Featured"`) {
			t.Fatalf("missing accessible label: %q", got)
		}
	})
}

func TestButtonGroupRejectsNonButton(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "button-group", 1, true, buttonGroupSchema, `<div>{{ .Children }}</div>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"bg","block":"core/button-group","version":1,"props":{},"settings":{"direction":"horizontal","gap":"sm","align":"start","wrap":true},"children":[{"id":"t","block":"core/text","version":1,"props":{"text":"nope"},"settings":{}}]}]}`)
	if err := registry.ValidateDocument(doc); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error = %v, want 'not allowed'", err)
	}
}

// ============================================================
// Stage 2 block rendering tests
// ============================================================

func TestCalloutRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "callout", 1, true, calloutSchema, `<aside class="stratum-callout stratum-callout-{{ .Settings.variant }}">{{ if .Settings.icon }}<span class="stratum-callout-mark">{{ if eq .Settings.variant "info" }}{{ icon "info" }}{{ else if eq .Settings.variant "success" }}{{ icon "check" }}{{ else if eq .Settings.variant "warning" }}{{ icon "warning" }}{{ else }}{{ icon "x" }}{{ end }}</span>{{ end }}<div class="stratum-callout-content">{{ if .Props.title }}<p class="stratum-callout-title">{{ .Props.title }}</p>{{ end }}<div class="stratum-callout-body">{{ .Props.text }}</div></div></aside>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("success with title", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c","block":"core/callout","version":1,"props":{"title":"Done","text":"It worked."},"settings":{"variant":"success","icon":true}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "stratum-callout-success") || !strings.Contains(got, "stratum-callout-title") {
			t.Fatalf("missing success classes: %q", got)
		}
		if !strings.Contains(got, "<svg") {
			t.Fatalf("missing icon svg: %q", got)
		}
	})

	t.Run("no icon", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c","block":"core/callout","version":1,"props":{"title":"","text":"Plain."},"settings":{"variant":"info","icon":false}}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(rendered), "<svg") {
			t.Fatalf("should not render icon: %q", rendered)
		}
	})
}

func TestCodeRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "code", 1, true, codeSchema, `<figure class="stratum-code{{ if .Settings.showLineNumbers }} stratum-code-lines{{ end }}">{{ if .Props.filename }}<figcaption class="stratum-code-filename">{{ .Props.filename }}</figcaption>{{ end }}<pre class="stratum-code-pre"><code{{ if .Props.language }} data-lang="{{ .Props.language }}"{{ end }}>{{ .Props.code }}</code></pre>{{ if .Settings.copyButton }}<button type="button" class="stratum-code-copy" data-copy-code aria-label="Copy code">Copy</button>{{ end }}</figure>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"c","block":"core/code","version":1,"props":{"code":"if a < b { return }","filename":"main.go","language":"go"},"settings":{"showLineNumbers":false,"copyButton":true}}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	if !strings.Contains(got, "stratum-code-filename") || !strings.Contains(got, "main.go") {
		t.Fatalf("missing filename: %q", got)
	}
	if !strings.Contains(got, `data-lang="go"`) {
		t.Fatalf("missing language: %q", got)
	}
	if !strings.Contains(got, "data-copy-code") {
		t.Fatalf("missing copy button: %q", got)
	}
	// Angle brackets in code must be escaped, not interpreted as HTML.
	if !strings.Contains(got, "&lt;") {
		t.Fatalf("code not escaped: %q", got)
	}
	if strings.Contains(got, "<b>") {
		t.Fatalf("code interpreted as HTML: %q", got)
	}
}

func TestTabsRendering(t *testing.T) {
	store := &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "tabs", 1, true, tabsSchema, `<div class="stratum-tabs stratum-tabs-{{ .Settings.variant }}" data-tabs><div class="stratum-tabs-nav" role="tablist" data-tabs-nav></div>{{ .Children }}</div>`),
		customDefinition("core", "tab", 1, true, tabSchema, `<section class="stratum-tab" data-label="{{ .Props.label }}">{{ .Children }}</section>`),
		customDefinition("core", "text", 1, true, textSchema, `<p>{{ .Props.text }}</p>`),
	}}
	registry, err := NewRegistry(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("renders panels and nav slot", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"tabs","block":"core/tabs","version":1,"props":{},"settings":{"variant":"boxed"},"children":[{"id":"t1","block":"core/tab","version":1,"props":{"label":"First"},"settings":{},"children":[{"id":"p1","block":"core/text","version":1,"props":{"text":"One"},"settings":{}}]},{"id":"t2","block":"core/tab","version":1,"props":{"label":"Second"},"settings":{},"children":[{"id":"p2","block":"core/text","version":1,"props":{"text":"Two"},"settings":{}}]}]}]}`)
		rendered, err := registry.RenderDocument(doc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "stratum-tabs-boxed") || !strings.Contains(got, "data-tabs-nav") {
			t.Fatalf("missing tabs wrapper: %q", got)
		}
		if !strings.Contains(got, `data-label="First"`) || !strings.Contains(got, `data-label="Second"`) {
			t.Fatalf("missing tab labels: %q", got)
		}
		if !strings.Contains(got, "<p>One</p>") || !strings.Contains(got, "<p>Two</p>") {
			t.Fatalf("missing tab content: %q", got)
		}
	})

	t.Run("rejects non-tab child", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"tabs","block":"core/tabs","version":1,"props":{},"settings":{"variant":"underline"},"children":[{"id":"p","block":"core/text","version":1,"props":{"text":"nope"},"settings":{}}]}]}`)
		if err := registry.ValidateDocument(doc); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("error = %v, want 'not allowed'", err)
		}
	})
}

// ============================================================
// Dynamic block rendering (RenderContext bound)
// ============================================================

func dynamicStore() *definitionStore {
	return &definitionStore{definitions: []db.BlockDefinition{
		customDefinition("core", "entry-title", 1, true, entryTitleSchema, `{{ $tag := tagFor .Settings.level }}{{ $o := tagOpen $tag }}{{ $c := tagClose $tag }}{{ $o }} class="stratum-entry-title stratum-align-{{ .Settings.align }} stratum-heading-size-{{ .Settings.visualSize }} stratum-tone-{{ .Settings.tone }} stratum-maxw-{{ .Settings.maxWidth }}">{{ if .Context.Entry.Title }}{{ .Context.Entry.Title }}{{ else }}<span class="stratum-placeholder">Entry title</span>{{ end }}{{ $c }}`),
		customDefinition("core", "entry-excerpt", 1, true, entryExcerptSchema, `<p class="stratum-entry-excerpt stratum-align-{{ .Settings.align }} stratum-tone-{{ .Settings.tone }} stratum-text-size-{{ .Settings.size }}">{{ if .Context.Entry.Excerpt }}{{ .Context.Entry.Excerpt }}{{ else }}<span class="stratum-placeholder">Entry excerpt</span>{{ end }}</p>`),
		customDefinition("core", "entry-publish-date", 1, true, entryPublishDateSchema, `{{ $d := .Context.Entry.PublishDate }}{{ if eq .Settings.format "iso" }}{{ $d = .Context.Entry.PublishISO }}{{ end }}<time class="stratum-entry-date stratum-align-{{ .Settings.align }}"{{ if .Context.Entry.PublishISO }} datetime="{{ .Context.Entry.PublishISO }}"{{ end }}>{{ if $d }}{{ $d }}{{ else }}<span class="stratum-placeholder">Publish date</span>{{ end }}</time>`),
		customDefinition("core", "site-name", 1, true, siteNameSchema, `{{ if .Settings.link }}<a class="stratum-site-name-link" href="{{ if .Context.Site.URL }}{{ .Context.Site.URL }}{{ else }}/{{ end }}">{{ end }}<span class="stratum-site-name stratum-align-{{ .Settings.align }}">{{ if .Context.Site.Name }}{{ .Context.Site.Name }}{{ else }}<span class="stratum-placeholder">Site name</span>{{ end }}</span>{{ if .Settings.link }}</a>{{ end }}`),
		customDefinition("core", "site-tagline", 1, true, siteTaglineSchema, `<p class="stratum-site-tagline stratum-align-{{ .Settings.align }}">{{ if .Context.Site.Tagline }}{{ .Context.Site.Tagline }}{{ else }}<span class="stratum-placeholder">Site tagline</span>{{ end }}</p>`),
	}}
}

func TestDynamicBlocksWithContext(t *testing.T) {
	registry, err := NewRegistry(context.Background(), dynamicStore())
	if err != nil {
		t.Fatal(err)
	}
	rc := rendering.RenderContext{
		Site:  rendering.SiteContext{Name: "Acme", Tagline: "We build things", URL: "https://acme.example"},
		Entry: rendering.EntryContext{Title: "Hello World", Excerpt: "An intro.", PublishDate: "January 2, 2024", PublishISO: "2024-01-02T00:00:00Z"},
	}

	t.Run("entry title renders bound value", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"t","block":"core/entry-title","version":1,"props":{},"settings":{"level":1,"align":"left","visualSize":"auto","tone":"default","maxWidth":"none"}}]}`)
		rendered, err := registry.RenderDocumentContext(doc, rc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, "<h1") || !strings.Contains(got, "Hello World") {
			t.Fatalf("expected bound title: %q", got)
		}
	})

	t.Run("entry date iso format", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"d","block":"core/entry-publish-date","version":1,"props":{},"settings":{"format":"iso","align":"left"}}]}`)
		rendered, err := registry.RenderDocumentContext(doc, rc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, `datetime="2024-01-02T00:00:00Z"`) || !strings.Contains(got, "2024-01-02T00:00:00Z") {
			t.Fatalf("expected iso date: %q", got)
		}
	})

	t.Run("site name links to url", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"s","block":"core/site-name","version":1,"props":{},"settings":{"link":true,"align":"left"}}]}`)
		rendered, err := registry.RenderDocumentContext(doc, rc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(rendered)
		if !strings.Contains(got, `href="https://acme.example"`) || !strings.Contains(got, "Acme") {
			t.Fatalf("expected linked site name: %q", got)
		}
	})

	t.Run("site tagline renders", func(t *testing.T) {
		doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"tg","block":"core/site-tagline","version":1,"props":{},"settings":{"align":"left"}}]}`)
		rendered, err := registry.RenderDocumentContext(doc, rc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(rendered), "We build things") {
			t.Fatalf("expected tagline: %q", rendered)
		}
	})
}

func TestDynamicBlocksPlaceholderWithoutContext(t *testing.T) {
	registry, err := NewRegistry(context.Background(), dynamicStore())
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeTestDocument(t, `{"version":1,"nodes":[{"id":"t","block":"core/entry-title","version":1,"props":{},"settings":{"level":1,"align":"left","visualSize":"auto","tone":"default","maxWidth":"none"}},{"id":"e","block":"core/entry-excerpt","version":1,"props":{},"settings":{"align":"left","tone":"default","size":"md"}},{"id":"s","block":"core/site-name","version":1,"props":{},"settings":{"link":true,"align":"left"}}]}`)
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	for _, placeholder := range []string{"Entry title", "Entry excerpt", "Site name"} {
		if !strings.Contains(got, placeholder) {
			t.Fatalf("expected placeholder %q in %q", placeholder, got)
		}
	}
}
