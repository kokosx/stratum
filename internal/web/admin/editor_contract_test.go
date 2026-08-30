package admin

import (
	"encoding/json"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/content"
	webassets "github.com/kokosx/stratum/internal/web"
)

var (
	editorNamedImportPattern = regexp.MustCompile(`(?m)import\s*\{([^}]+)\}\s*from\s*["'](\./[^"']+)["']`)
	editorNamedExportPattern = regexp.MustCompile(`(?m)^export\s+(?:async\s+)?(?:function|class|const|let|var)\s+([A-Za-z_$][\w$]*)`)
	editorExportListPattern  = regexp.MustCompile(`(?m)^export\s*\{([^}]+)\}`)
)

func TestEditorModuleNamedImportsAreExported(t *testing.T) {
	files, err := fs.Glob(webassets.Assets, "static/editor/*.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no editor modules found")
	}

	sources := make(map[string]string, len(files))
	exports := make(map[string]map[string]bool, len(files))
	for _, filename := range files {
		data, readErr := fs.ReadFile(webassets.Assets, filename)
		if readErr != nil {
			t.Fatalf("read %s: %v", filename, readErr)
		}
		source := string(data)
		sources[filename] = source
		exports[filename] = editorModuleExports(source)
	}

	for filename, source := range sources {
		for _, match := range editorNamedImportPattern.FindAllStringSubmatch(source, -1) {
			dependency := path.Clean(path.Join(path.Dir(filename), match[2]))
			dependencyExports, ok := exports[dependency]
			if !ok {
				t.Errorf("%s imports missing editor module %s", filename, dependency)
				continue
			}
			for _, imported := range strings.Split(match[1], ",") {
				name := strings.TrimSpace(strings.SplitN(strings.TrimSpace(imported), " as ", 2)[0])
				if name != "" && !dependencyExports[name] {
					t.Errorf("%s imports %q from %s, but that module does not export it", filename, name, dependency)
				}
			}
		}
	}
}

func editorModuleExports(source string) map[string]bool {
	exports := make(map[string]bool)
	for _, match := range editorNamedExportPattern.FindAllStringSubmatch(source, -1) {
		exports[match[1]] = true
	}
	for _, match := range editorExportListPattern.FindAllStringSubmatch(source, -1) {
		for _, exported := range strings.Split(match[1], ",") {
			parts := strings.SplitN(strings.TrimSpace(exported), " as ", 2)
			name := strings.TrimSpace(parts[len(parts)-1])
			if name != "" {
				exports[name] = true
			}
		}
	}
	return exports
}

func TestEditorCapabilitiesForEntry(t *testing.T) {
	def := content.ContentTypeDefinition{
		ID:           "post",
		Capabilities: content.Capabilities{HasContent: true, HasSEO: true, HasExcerpt: true, HasFeatured: true},
	}
	def.Routing.Single = true
	def.Routing.Archive = true
	caps := editorCapabilitiesForEntry(def)
	if !caps.SaveDraft || !caps.Publish || !caps.Preview {
		t.Fatalf("basic caps should be true")
	}
	if !caps.SEO || !caps.Slug || !caps.FeaturedMedia || !caps.TemplateAssignment {
		t.Fatalf("SEO/Slug/Featured/Template should be true for post single")
	}
	// archive-only type: HasSEO but Single false -> SEO false
	def2 := content.ContentTypeDefinition{ID: "custom", Capabilities: content.Capabilities{HasSEO: true}}
	def2.Routing.Single = false
	def2.Routing.Archive = true
	caps2 := editorCapabilitiesForEntry(def2)
	if caps2.Slug || caps2.SEO || caps2.TemplateAssignment {
		t.Fatalf("route-less type should have Slug/SEO/Template false, got %+v", caps2)
	}
}

func TestEditorBootstrapContainsResource(t *testing.T) {
	// Verify that editorBootstrap marshals resource and capabilities
	bootstrap := editorBootstrap{
		Document:     json.RawMessage(`{"version":1,"nodes":[]}`),
		PreviewURL:   "/admin/editor/preview",
		ContextKind:  "entry",
		Resource:     EditorResource{Type: "entry", ID: "entry123", Kind: "post", Label: "Hello", ContentTypeID: "post"},
		Capabilities: EditorCapabilities{SaveDraft: true, Publish: true, SEO: true},
		Actions:      EditorActions{PreviewURL: "/admin/editor/preview", SaveURL: "/admin/pages/entry123", BackURL: "/admin/pages"},
	}
	data, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res, ok := decoded["resource"].(map[string]any)
	if !ok {
		t.Fatalf("resource missing")
	}
	if res["type"] != "entry" || res["id"] != "entry123" {
		t.Fatalf("resource mismatch: %v", res)
	}
	caps, ok := decoded["capabilities"].(map[string]any)
	if !ok || caps["saveDraft"] != true {
		t.Fatalf("capabilities missing")
	}
	actions, ok := decoded["actions"].(map[string]any)
	if !ok || actions["previewUrl"] != "/admin/editor/preview" {
		t.Fatalf("actions missing")
	}
}

func TestEditorCapabilitiesForLayoutTemplate(t *testing.T) {
	caps := editorCapabilitiesForLayoutTemplate("single")
	if !caps.DynamicContent || caps.SEO {
		t.Fatalf("single template caps incorrect: %+v", caps)
	}
	capsArch := editorCapabilitiesForLayoutTemplate("archive")
	if !capsArch.DynamicContent {
		t.Fatalf("archive caps incorrect")
	}
}

func TestEditorCapabilitiesForSitePart(t *testing.T) {
	caps := editorCapabilitiesForSitePart()
	if !caps.SitePartLocation || caps.SEO {
		t.Fatalf("site part caps incorrect: %+v", caps)
	}
}
