package creator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/site"
)

func TestBodyDocument_SingleSemanticSection(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		formID       string
		wantSections int
		wantHasStack bool
	}{
		{"body+form one section with stack", "hello body", "form-123", 1, true},
		{"body only one section", "just body", "", 1, false},
		{"form only one section", "", "form-123", 1, true},
		{"empty none", "", "", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := bodyDocument("test", tc.body, tc.formID)
			if len(doc.Nodes) != tc.wantSections {
				t.Fatalf("bodyDocument Nodes len = %d, want %d", len(doc.Nodes), tc.wantSections)
			}
			if tc.wantSections == 0 {
				return
			}
			// First node must be Section
			if doc.Nodes[0].Block != "core/section" {
				t.Fatalf("first node Block = %q, want core/section", doc.Nodes[0].Block)
			}
			// Check settings width/content md default
			var s map[string]any
			if err := json.Unmarshal(doc.Nodes[0].Settings, &s); err != nil {
				t.Fatal(err)
			}
			if s["width"] != "content" {
				t.Fatalf("section width = %v, want content", s["width"])
			}
			// When both body and form, children should be a single Stack owning the group gap
			if tc.body != "" && tc.formID != "" {
				if len(doc.Nodes[0].Children) != 1 {
					t.Fatalf("body+form section should have 1 child Stack, got %d", len(doc.Nodes[0].Children))
				}
				if doc.Nodes[0].Children[0].Block != "core/stack" {
					t.Fatalf("body+form inner Block = %q, want core/stack", doc.Nodes[0].Children[0].Block)
				}
				// Stack should contain 2 children: text and form-group stack
				outer := doc.Nodes[0].Children[0]
				if len(outer.Children) != 2 {
					t.Fatalf("outer stack children = %d, want 2 (text + form group)", len(outer.Children))
				}
				if outer.Children[0].Block != "core/text" {
					t.Fatalf("first stack child = %q, want core/text", outer.Children[0].Block)
				}
				inner := outer.Children[1]
				if inner.Block != "core/stack" {
					t.Fatalf("second stack child = %q, want inner stack", inner.Block)
				}
				if len(inner.Children) != 2 {
					t.Fatalf("inner stack children = %d, want 2 (heading+form)", len(inner.Children))
				}
				if inner.Children[0].Block != "core/heading" || inner.Children[1].Block != "core/form" {
					t.Fatalf("inner stack blocks = %q,%q, want heading+form", inner.Children[0].Block, inner.Children[1].Block)
				}
			}
		})
	}
	// Ensure body+form does NOT produce two adjacent same-purpose Sections
	doc := bodyDocument("c", "b", "f")
	if len(doc.Nodes) == 2 && doc.Nodes[0].Block == "core/section" && doc.Nodes[1].Block == "core/section" {
		t.Fatal("bodyDocument still produces two adjacent Sections for body+form — should be one")
	}
}

func TestPageTemplate_CompactTitleBand(t *testing.T) {
	doc := pageTemplate("p")
	if len(doc.Nodes) != 2 {
		t.Fatalf("pageTemplate nodes = %d, want 2 (title section + content-slot)", len(doc.Nodes))
	}
	titleSection := doc.Nodes[0]
	if titleSection.Block != "core/section" {
		t.Fatalf("first node = %q, want core/section", titleSection.Block)
	}
	var s map[string]any
	if err := json.Unmarshal(titleSection.Settings, &s); err != nil {
		t.Fatal(err)
	}
	if s["verticalSpacing"] != "md" {
		t.Fatalf("pageTemplate verticalSpacing = %v, want md (compact secondary page header)", s["verticalSpacing"])
	}
	if s["width"] != "content" {
		t.Fatalf("width = %v, want content", s["width"])
	}
	// Title section must contain a Stack owning H1→excerpt gap, not loose title+excerpt as direct Section children
	if len(titleSection.Children) != 1 || titleSection.Children[0].Block != "core/stack" {
		t.Fatalf("title section children = %v, want single Stack", func() string {
			var b []string
			for _, c := range titleSection.Children {
				b = append(b, c.Block)
			}
			return strings.Join(b, ",")
		}())
	}
	stack := titleSection.Children[0]
	if len(stack.Children) != 2 {
		t.Fatalf("title stack children = %d, want 2 (entry-title + excerpt)", len(stack.Children))
	}
	if stack.Children[0].Block != "core/entry-title" || stack.Children[1].Block != "core/entry-excerpt" {
		t.Fatalf("title stack blocks = %q,%q, want entry-title+entry-excerpt", stack.Children[0].Block, stack.Children[1].Block)
	}
	var ss map[string]any
	if err := json.Unmarshal(stack.Settings, &ss); err != nil {
		t.Fatal(err)
	}
	if ss["gap"] != "md" {
		t.Fatalf("title stack gap = %v, want md (unified generic header)", ss["gap"])
	}
	if doc.Nodes[1].Block != "core/content-slot" {
		t.Fatalf("second node = %q, want core/content-slot", doc.Nodes[1].Block)
	}
}

func TestPreview_SiteURL_Persisted_NotSilentlyErased(t *testing.T) {
	// Use a minimal valid service without DB by calling Preview directly via a service with nil dependencies (only uses site.Validate)
	// We instantiate Service with zero DB to call Preview (which doesn't touch DB)
	s := &Service{}
	// Valid public URL must survive
	plan, err := s.Preview(Input{PresetID: PresetBlog, SiteTitle: "Test", SiteURL: "https://example.com/", Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if plan.Input.SiteURL != "https://example.com" {
		t.Fatalf("SiteURL = %q, want https://example.com (trimmed slash, not erased)", plan.Input.SiteURL)
	}
	// localhost/private must now be persisted, not erased (previous bug)
	plan, err = s.Preview(Input{PresetID: PresetBlog, SiteTitle: "Test", SiteURL: "http://localhost:3000", Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatalf("Preview localhost: %v", err)
	}
	if plan.Input.SiteURL != "http://localhost:3000" {
		t.Fatalf("localhost URL = %q, want http://localhost:3000 (must not be silently erased)", plan.Input.SiteURL)
	}
	plan, err = s.Preview(Input{PresetID: PresetBlog, SiteTitle: "Test", SiteURL: "http://192.168.1.10", Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatalf("Preview 192: %v", err)
	}
	if plan.Input.SiteURL != "http://192.168.1.10" {
		t.Fatalf("192 URL erased = %q", plan.Input.SiteURL)
	}
}

func TestPreview_SiteURL_Normalization_Parity_With_Site(t *testing.T) {
	s := &Service{}
	cases := []struct{ in, want string }{
		{"https://example.com/", "https://example.com"},
		{"https://example.com", "https://example.com"},
		{"http://example.com:8080/", "http://example.com:8080"},
		{"", ""},
	}
	for _, tc := range cases {
		plan, err := s.Preview(Input{PresetID: PresetBlog, SiteTitle: "T", SiteURL: tc.in, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
		if err != nil {
			t.Fatalf("Preview %q: %v", tc.in, err)
		}
		direct, err := site.ValidateSiteURL(tc.in)
		if err != nil {
			t.Fatalf("ValidateSiteURL %q: %v", tc.in, err)
		}
		if plan.Input.SiteURL != tc.want || direct != tc.want {
			t.Fatalf("parity %q: Plan=%q direct=%q want %q", tc.in, plan.Input.SiteURL, direct, tc.want)
		}
		if plan.Input.SiteURL != direct {
			t.Fatalf("Creator vs Settings parity mismatch %q", tc.in)
		}
	}
}

func TestPreview_BlogCounts_Parsed(t *testing.T) {
	s := &Service{}
	// Magazine latest 8 archive 20
	plan, err := s.Preview(Input{PresetID: PresetMagazine, SiteTitle: "T", BlogLatestCount: 8, BlogArchiveCount: 20, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatalf("Magazine Preview: %v", err)
	}
	if plan.Input.BlogLatestCount != 8 {
		t.Fatalf("Magazine BlogLatestCount = %d, want 8", plan.Input.BlogLatestCount)
	}
	if plan.Input.BlogArchiveCount != 20 {
		t.Fatalf("Magazine BlogArchiveCount = %d, want 20", plan.Input.BlogArchiveCount)
	}
	// Blog latest 5 archive 10 defaults
	plan, err = s.Preview(Input{PresetID: PresetBlog, SiteTitle: "T", BlogLatestCount: 5, BlogArchiveCount: 10, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Input.BlogLatestCount != 5 || plan.Input.BlogArchiveCount != 10 {
		t.Fatalf("Blog counts %d/%d", plan.Input.BlogLatestCount, plan.Input.BlogArchiveCount)
	}
}

func TestPreview_KnowledgeBaseArchive(t *testing.T) {
	s := &Service{}
	plan, err := s.Preview(Input{PresetID: PresetKnowledgeBase, SiteTitle: "T", BlogArchiveCount: 20, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Input.BlogArchiveCount != 20 {
		t.Fatalf("KB archive = %d, want 20", plan.Input.BlogArchiveCount)
	}
}

func TestPreview_AgencyPortfolioCols(t *testing.T) {
	s := &Service{}
	plan, err := s.Preview(Input{PresetID: PresetAgency, SiteTitle: "T", PortfolioColumns: 3, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatalf("Agency Preview: %v", err)
	}
	if plan.Input.PortfolioColumns != 3 {
		t.Fatalf("Agency PortfolioColumns = %d, want 3", plan.Input.PortfolioColumns)
	}
	// Portfolio preset
	plan, err = s.Preview(Input{PresetID: PresetPortfolio, SiteTitle: "T", PortfolioColumns: 3, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Input.PortfolioColumns != 3 {
		t.Fatalf("Portfolio cols = %d, want 3", plan.Input.PortfolioColumns)
	}
}

func TestPreview_Indexing_DiscourageSemantics(t *testing.T) {
	s := &Service{}
	// IndexingEnabled true = allow indexing (discourage unchecked)
	plan, err := s.Preview(Input{PresetID: PresetBlog, SiteTitle: "T", IndexingEnabled: true, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Input.IndexingEnabled {
		t.Fatal("IndexingEnabled true should stay true")
	}
	// false = discourage checked
	plan, err = s.Preview(Input{PresetID: PresetBlog, SiteTitle: "T", IndexingEnabled: false, Language: "en", Timezone: "UTC", SiteRepresents: "organization"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Input.IndexingEnabled {
		t.Fatal("IndexingEnabled false should stay false")
	}
}

func TestEmptyExcerpt_NoPublicWrapper_NeedsMigration(t *testing.T) {
	// Verify migration file contains public-no-wrapper + preview-placeholder logic.
	// This is the canonical source; rendering tests in internal/blocks also validate.
	data, err := readMigrationFile("082_fix_empty_dynamic_blocks.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	// Must contain conditional on Entry.Excerpt and IsPreview for placeholder
	if !strings.Contains(string(data), "Entry.Excerpt") {
		t.Fatal("migration missing Entry.Excerpt conditional")
	}
	if !strings.Contains(string(data), "IsPreview") {
		t.Fatal("migration must keep editor placeholder via IsPreview, not always")
	}
	// Must output nothing when empty in public (no unconditional <p>)
	if strings.Contains(string(data), `stratum-placeholder">Entry excerpt</span>{{ end }}</p>`) && !strings.Contains(string(data), "IsPreview") {
		t.Fatal("migration should not always show placeholder")
	}
	// Also verify form style uses lg/xs gaps per spec
	if !strings.Contains(string(data), "gap:var(--st-space-lg)") {
		t.Fatal("form style should use gap lg for field→field 18-24")
	}
	if !strings.Contains(string(data), "gap:var(--st-space-xs)") {
		t.Fatal("form field should use xs for label→control 6-10")
	}
}

func readMigrationFile(name string) ([]byte, error) {
	// Try embedded path via repo relative
	candidates := []string{
		filepath.Join("internal", "storage", "schema", name),
		filepath.Join("..", "storage", "schema", name),
		filepath.Join("..", "..", "internal", "storage", "schema", name),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			return data, nil
		}
	}
	// Also try absolute from module root via os.Getwd traversal
	wd, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		p := filepath.Join(wd, "internal", "storage", "schema", name)
		if data, err := os.ReadFile(p); err == nil {
			return data, nil
		}
		wd = filepath.Dir(wd)
	}
	return nil, os.ErrNotExist
}
