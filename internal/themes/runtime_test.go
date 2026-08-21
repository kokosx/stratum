package themes_test

import (
	"context"
	"database/sql"
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

func TestSharedRuntimeIsSameInstance(t *testing.T) {
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
	if err := runtime.Save(ctx, map[string]any{"header.layout": "centered"}, ""); err != nil {
		t.Fatal(err)
	}
	after := runtime.Current()
	if after.Settings["header.layout"] != "centered" {
		t.Fatalf("save did not publish: %#v", after)
	}
	if before.Settings["header.layout"] == after.Settings["header.layout"] {
		t.Fatal("settings did not change")
	}
}

func TestVersionMismatchStartsWithDefaultsWhenNoMigration(t *testing.T) {
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

	if err := queries.UpsertThemeCustomization(ctx, db.UpsertThemeCustomizationParams{
		ThemeID:      "stratum/default",
		ThemeVersion: 999,
		SettingsJson: `{"header.layout":"stacked","colors.primary":"#ff0000"}`,
		CustomCss:    ".old{}",
	}); err != nil {
		t.Fatal(err)
	}

	runtime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatalf("runtime failed to start with old version: %v", err)
	}

	current := runtime.Current()
	if current.Version != 1 {
		t.Fatalf("expected version 1, got %d", current.Version)
	}
	defaults := current.Schema.Defaults()
	if current.Settings["header.layout"] != defaults["header.layout"] {
		t.Fatalf("expected default header.layout %v, got %v", defaults["header.layout"], current.Settings["header.layout"])
	}

	stored, err := queries.GetThemeCustomization(ctx, "stratum/default")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThemeVersion != 999 {
		t.Fatalf("old customization was overwritten: version=%d", stored.ThemeVersion)
	}
}

func TestVersionMismatchRunsRegisteredMigration(t *testing.T) {
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

	if err := queries.UpsertThemeCustomization(ctx, db.UpsertThemeCustomizationParams{
		ThemeID:      "stratum/default",
		ThemeVersion: 0,
		SettingsJson: `{"header.layout":"stacked","colors.primary":"#ff0000"}`,
		CustomCss:    ".migrated{}",
	}); err != nil {
		t.Fatal(err)
	}

	runtime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	runtime.RegisterMigration(themes.Migration{
		ThemeID:     "stratum/default",
		FromVersion: 0,
		ToVersion:   1,
		Migrate: func(settings map[string]any) (map[string]any, error) {
			settings["colors.primary"] = "#00ff00"
			return settings, nil
		},
	})

	if err := runtime.Reload(ctx); err != nil {
		t.Fatalf("reload after migration registration failed: %v", err)
	}

	current := runtime.Current()
	if current.Settings["colors.primary"] != "#00ff00" {
		t.Fatalf("migration did not run: colors.primary=%v", current.Settings["colors.primary"])
	}
	if current.CustomCSS != ".migrated{}" {
		t.Fatalf("custom CSS was lost after migration: %q", current.CustomCSS)
	}

	stored, err := queries.GetThemeCustomization(ctx, "stratum/default")
	if err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if stored.ThemeVersion != 1 {
		t.Fatalf("migrated customization not saved: version=%d", stored.ThemeVersion)
	}
}
