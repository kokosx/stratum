package themes_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func TestRuntimePreviewDoesNotMutateAndSavePersists(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	runtime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}

	before := runtime.Current()
	preview := map[string]any{"header.layout": "stacked", "colors.primary": "#123456"}
	previewPage, err := runtime.Preview(themes.PageView{Site: themes.SiteView{Title: "Test", Language: "en"}, Entry: themes.EntryView{Title: "Page"}}, preview, ".draft-only{}")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Current().Settings["header.layout"] != before.Settings["header.layout"] {
		t.Fatal("temporary preview mutated persisted snapshot")
	}
	if !strings.Contains(string(previewPage), ".draft-only{}") || strings.Contains(runtime.Styles(), ".draft-only{}") {
		t.Fatal("temporary preview CSS leaked into the published stylesheet")
	}

	if err := runtime.Save(ctx, preview, ".custom-theme-rule{display:block}"); err != nil {
		t.Fatal(err)
	}
	after := runtime.Current()
	if after.Settings["header.layout"] != "stacked" || !strings.Contains(runtime.Styles(), "--st-color-primary: #123456;") || !strings.Contains(runtime.Styles(), ".custom-theme-rule") {
		t.Fatalf("saved customization not published: %#v\n%s", after, runtime.Styles())
	}

	reloaded, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Current().Settings["header.layout"] != "stacked" {
		t.Fatal("saved customization was not persisted")
	}
}
