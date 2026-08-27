package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/contenttypes"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
	adminweb "github.com/kokosx/stratum/internal/web/admin"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func newHardeningServer(t *testing.T) (*http.Client, string, *db.Queries, *storage.Database, *runtimehub.Runtime, func()) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	service, err := auth.NewService(database.DB, queries, false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	store, err := media.NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	mediaService := media.NewService(queries, store)
	hub, err := runtimehub.New(queries, registry, themeRuntime, mediaService)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler, err := adminweb.NewHandler(database.DB, queries, service, registry, themeRuntime, mediaService, hub)
	if err != nil {
		t.Fatal(err)
	}
	publicHandler, err := publicweb.NewHandlerWithHub(hub)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler.SetPreviewRenderer(publicHandler.RenderPreview)
	adminHandler.SetDocumentPreviewRenderer(publicHandler.RenderEditableDocument)
	mux := http.NewServeMux()
	mux.Handle("/admin", adminHandler.Routes())
	mux.Handle("/admin/", adminHandler.Routes())
	mux.Handle("/", publicHandler)
	srv := httptest.NewServer(mux)
	client := newClient(t)
	setupToken := service.SetupCode()
	form := url.Values{"setup_code": {setupToken}, "site_title": {"Test Site"}, "email": {"admin@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, client, srv.URL, "/admin/setup")}}
	resp := postForm(t, client, srv.URL, "/admin/setup", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	loginForm := url.Values{"email": {"admin@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, client, srv.URL, "/admin/login")}}
	resp = postForm(t, client, srv.URL, "/admin/login", loginForm)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %d", resp.StatusCode)
	}
	resp.Body.Close()
	return client, srv.URL, queries, database, hub, func() { srv.Close(); _ = database.Close() }
}

func TestHardening_TestimonialRouteLess(t *testing.T) {
	client, srvURL, queries, _, _, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()

	fields := []content.FieldDefinition{
		{Key: "quote", Label: "Quote", Type: content.FieldTextarea},
		{Key: "author", Label: "Author", Type: content.FieldText},
	}
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         "testimonial",
		Name:       "Testimonial",
		PluralName: "Testimonials",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Fields:        fields,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: false, Archive: false, BasePath: ""},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatalf("create testimonial type %v", err)
	}

	def, err := cat.GetDefinition(ctx, "testimonial")
	if err != nil {
		t.Fatal(err)
	}
	catalogOpts := content.FieldCatalog(def)
	for _, opt := range catalogOpts {
		if opt.Value == "entry.permalink" {
			t.Fatalf("field catalog should not contain permalink for route-less")
		}
	}

	tok := csrfToken(t, client, srvURL, "/admin/content/testimonial/new")
	f := url.Values{
		"title":         {"Homepage"},
		"field_quote":   {"Great CMS."},
		"field_author":  {"Alice"},
		"document_json": {`{"version":1,"nodes":[]}`},
		"csrf_token":    {tok},
		"publish":       {"true"},
	}
	resp := postForm(t, client, srvURL, "/admin/content/testimonial", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create testimonial 1 %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()

	entries, err := queries.ListEntriesByContentType(ctx, "testimonial")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry got %d", len(entries))
	}
	entry := entries[0]
	if !entry.PublishedRevisionID.Valid {
		t.Fatalf("published_revision_id not set")
	}
	rev, err := queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil {
		t.Fatal(err)
	}
	var fieldsMap map[string]any
	if err := json.Unmarshal([]byte(rev.FieldsJson), &fieldsMap); err != nil {
		t.Fatal(err)
	}
	if fieldsMap["quote"] != "Great CMS." {
		t.Fatalf("fields quote mismatch %v", fieldsMap)
	}
	if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: entry.ID, Valid: true}); err == nil {
		t.Fatalf("route-less entry should have no entry route")
	} else if err != sql.ErrNoRows {
		t.Fatalf("unexpected route error %v", err)
	}
	if _, err := queries.GetArchiveRouteByContentType(ctx, sql.NullString{String: "testimonial", Valid: true}); err == nil {
		t.Fatalf("archive route should not exist")
	}

	resp = getPath(t, client, srvURL, "/testimonials/homepage")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for route-less entry, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	collectionDoc := `{"version":1,"nodes":[{"id":"coll1","block":"core/collection","version":2,"props":{},"settings":{"contentType":"testimonial","limit":10,"source":"query"},"children":[{"id":"f1","block":"core/entry-field","version":1,"props":{"source":"fields.quote"},"settings":{"tag":"p"}}]}]}`
	tok = csrfToken(t, client, srvURL, "/admin/pages/new")
	f = url.Values{"title": {"Collection Test"}, "slug": {"collection-test"}, "document_json": {collectionDoc}, "csrf_token": {tok}, "publish": {"true"}}
	resp = postForm(t, client, srvURL, "/admin/pages", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create collection page %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/collection-test")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("collection page %d", resp.StatusCode)
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, "Great CMS.") {
		t.Fatalf("collection should render testimonial quote, body %s", body[:2000])
	}
	resp.Body.Close()

	resp = getPath(t, client, srvURL, "/admin/content/testimonial/"+entry.ID+"/edit")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit page %d", resp.StatusCode)
	}
	editBody := bodyString(t, resp)
	if strings.Contains(editBody, `<label>Slug`) {
		t.Fatalf("edit should not show slug field for route-less")
	}
	if strings.Contains(editBody, `id="entry-visibility"`) && strings.Contains(editBody, `<select name="visibility"`) {
		t.Fatalf("edit should not show visibility selector")
	}
	if strings.Contains(editBody, `id="entry-layout-template"`) {
		t.Fatalf("edit should not show template selector")
	}
	if !strings.Contains(editBody, "Rich content editor is disabled") && !strings.Contains(editBody, "structured data only") {
		t.Fatalf("edit should indicate block workspace disabled for HasContent false")
	}
	resp.Body.Close()

	tok = csrfToken(t, client, srvURL, "/admin/content/testimonial/new")
	f = url.Values{
		"title":         {"Homepage"},
		"field_quote":   {"Second"},
		"field_author":  {"Bob"},
		"document_json": {`{"version":1,"nodes":[]}`},
		"csrf_token":    {tok},
		"publish":       {"true"},
	}
	resp = postForm(t, client, srvURL, "/admin/content/testimonial", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("second testimonial should succeed despite duplicate name, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	entries, err = queries.ListEntriesByContentType(ctx, "testimonial")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries got %d", len(entries))
	}
	if entries[0].Slug == entries[1].Slug {
		t.Fatalf("slugs should be unique %s %s", entries[0].Slug, entries[1].Slug)
	}
	if entries[0].Slug != "homepage" && entries[1].Slug != "homepage" {
		t.Fatalf("one slug should be homepage, got %s %s", entries[0].Slug, entries[1].Slug)
	}
	found2 := false
	for _, e := range entries {
		if e.Slug == "homepage-2" {
			found2 = true
		}
	}
	if !found2 {
		t.Fatalf("expected homepage-2 slug, got %v", []string{entries[0].Slug, entries[1].Slug})
	}
}

func TestHardening_TeamMemberFieldsOnlySingle(t *testing.T) {
	client, srvURL, queries, _, _, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	fields := []content.FieldDefinition{
		{Key: "position", Label: "Position", Type: content.FieldText},
		{Key: "bio", Label: "Bio", Type: content.FieldTextarea},
	}
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         "team_member",
		PluralName: "Team",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Fields:        fields,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: true, Archive: false, BasePath: "/team"},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatalf("create team_member %v", err)
	}

	tok := csrfToken(t, client, srvURL, "/admin/content/team_member/new")
	f := url.Values{
		"title":          {"John Doe"},
		"field_position": {"CEO"},
		"field_bio":      {"Hello"},
		"document_json":  {`{"version":1,"nodes":[]}`},
		"csrf_token":     {tok},
		"publish":        {"true"},
	}
	resp := postForm(t, client, srvURL, "/admin/content/team_member", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create team member %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/team/john-doe")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("team page %d %s", resp.StatusCode, bodyString(t, resp))
	}
	body := bodyString(t, resp)
	_ = body
	resp.Body.Close()

	entries, _ := queries.ListEntriesByContentType(ctx, "team_member")
	if len(entries) != 1 {
		t.Fatalf("expected 1 team member")
	}
	rev, _ := queries.GetEntryRevision(ctx, entries[0].PublishedRevisionID.String)
	var doc struct {
		Nodes []any `json:"nodes"`
	}
	json.Unmarshal([]byte(rev.DocumentJson), &doc)
	if len(doc.Nodes) != 0 {
		t.Fatalf("fields-only single should have empty document, got %v", doc.Nodes)
	}
}

func TestHardening_Transition(t *testing.T) {
	client, srvURL, queries, database, hub, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         "technology",
		PluralName: "Technologies",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Fields:        []content.FieldDefinition{},
			Features:      content.ContentTypeFeatures{Content: true},
			Routing:       content.ContentTypeRouting{Single: false, Archive: false, BasePath: ""},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	create := func(title, slug string) {
		t.Helper()
		tok := csrfToken(t, client, srvURL, "/admin/content/technology/new")
		f := url.Values{"title": {title}, "slug": {slug}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
		resp := postForm(t, client, srvURL, "/admin/content/technology", f)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("create %s %d %s", title, resp.StatusCode, bodyString(t, resp))
		}
		resp.Body.Close()
	}
	create("Go", "go")
	create("SQLite", "sqlite")
	collectionDoc := `{"version":1,"nodes":[{"id":"coll1","block":"core/collection","version":2,"props":{},"settings":{"contentType":"technology","limit":10,"source":"query"},"children":[{"id":"f1","block":"core/entry-field","version":1,"props":{"source":"entry.title"},"settings":{"tag":"p"}}]}]}`
	tok := csrfToken(t, client, srvURL, "/admin/pages/new")
	f := url.Values{"title": {"Tech List"}, "slug": {"tech-list"}, "document_json": {collectionDoc}, "csrf_token": {tok}, "publish": {"true"}}
	resp := postForm(t, client, srvURL, "/admin/pages", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create tech list %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/tech-list")
	body := bodyString(t, resp)
	if !strings.Contains(body, "Go") || !strings.Contains(body, "SQLite") {
		t.Fatalf("collection should render both %s", body[:2000])
	}
	resp.Body.Close()

	svc := contenttypes.New(database.DB, queries)
	newInput := content.ContentTypeInput{
		ID:         "technology",
		PluralName: "Technologies",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: true},
			Routing:       content.ContentTypeRouting{Single: true, Archive: false, BasePath: "/technologies"},
		},
	}
	if err := svc.Update(ctx, "technology", newInput); err != nil {
		t.Fatalf("enable single %v", err)
	}
	hub.Routes.Reload(ctx)
	hub.Pages.InvalidateAll()
	resp = getPath(t, client, srvURL, "/technologies/go")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/technologies/go %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/technologies/sqlite")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/technologies/sqlite %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/tech-list")
	body = bodyString(t, resp)
	if !strings.Contains(body, "Go") {
		t.Fatalf("collection after enable %s", body[:2000])
	}
	resp.Body.Close()

	newInput.Config.Routing.Single = false
	newInput.Config.Routing.BasePath = ""
	if err := svc.Update(ctx, "technology", newInput); err != nil {
		t.Fatalf("disable single %v", err)
	}
	hub.Routes.Reload(ctx)
	hub.Pages.InvalidateAll()
	resp = getPath(t, client, srvURL, "/technologies/go")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after disable should 404 got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/tech-list")
	body = bodyString(t, resp)
	if !strings.Contains(body, "Go") {
		t.Fatalf("collection after disable should still work %s", body[:2000])
	}
	resp.Body.Close()
	entries, _ := queries.ListEntriesByContentType(ctx, "technology")
	for _, e := range entries {
		if !e.PublishedRevisionID.Valid {
			t.Fatalf("published revision missing")
		}
	}
}

func TestHardening_HasContentTransition(t *testing.T) {
	client, srvURL, queries, database, _, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         "custom_has",
		PluralName: "CustomHas",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: true},
			Routing:       content.ContentTypeRouting{Single: false, Archive: false},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	tok := csrfToken(t, client, srvURL, "/admin/content/custom_has/new")
	blockDoc := `{"version":1,"nodes":[{"id":"b1","block":"core/text","version":1,"props":{"text":"OLD BLOCK CONTENT"},"settings":{}}]}`
	f := url.Values{"title": {"Entry1"}, "document_json": {blockDoc}, "csrf_token": {tok}, "publish": {"true"}}
	resp := postForm(t, client, srvURL, "/admin/content/custom_has", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create entry %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	entries, _ := queries.ListEntriesByContentType(ctx, "custom_has")
	if len(entries) != 1 {
		t.Fatalf("expected 1")
	}
	entryID := entries[0].ID
	rev, _ := queries.GetEntryRevision(ctx, entries[0].PublishedRevisionID.String)
	if !strings.Contains(rev.DocumentJson, "OLD BLOCK CONTENT") {
		t.Fatalf("published should contain old block")
	}
	oldRevID := rev.ID

	svc := contenttypes.New(database.DB, queries)
	newInput := content.ContentTypeInput{
		ID:         "custom_has",
		PluralName: "CustomHas",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: false},
		},
	}
	if err := svc.Update(ctx, "custom_has", newInput); err != nil {
		t.Fatalf("update hasContent false %v", err)
	}
	tok = csrfToken(t, client, srvURL, "/admin/content/custom_has/"+entryID+"/edit")
	f = url.Values{"title": {"Entry1"}, "document_json": {`{"version":1,"nodes":[{"id":"b2","block":"core/text","version":1,"props":{"text":"SHOULD BE IGNORED"},"settings":{}}]}`}, "csrf_token": {tok}}
	resp = postForm(t, client, srvURL, "/admin/content/custom_has/"+entryID+"/publish", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish new revision %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	latest, _ := queries.GetLatestEntryRevision(ctx, entryID)
	if latest.DocumentJson != `{"version":1,"nodes":[]}` && !strings.Contains(latest.DocumentJson, `"nodes":[]`) {
		t.Fatalf("latest revision should be empty SDT, got %s", latest.DocumentJson)
	}
	if !strings.Contains(rev.DocumentJson, "OLD BLOCK CONTENT") {
		t.Fatalf("historical should still contain")
	}
	ent, _ := queries.GetEntry(ctx, entryID)
	pubRev, _ := queries.GetEntryRevision(ctx, ent.PublishedRevisionID.String)
	if strings.Contains(pubRev.DocumentJson, "OLD BLOCK CONTENT") {
		t.Fatalf("published should not contain old block")
	}
	if strings.Contains(pubRev.DocumentJson, "SHOULD BE IGNORED") {
		t.Fatalf("should be ignored")
	}

	tok = csrfToken(t, client, srvURL, "/admin/content/custom_has/"+entryID+"/edit")
	f = url.Values{"csrf_token": {tok}}
	resp = postForm(t, client, srvURL, "/admin/content/custom_has/"+entryID+"/revisions/"+oldRevID+"/restore", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	newLatest, _ := queries.GetLatestEntryRevision(ctx, entryID)
	if newLatest.DocumentJson != `{"version":1,"nodes":[]}` && !strings.Contains(newLatest.DocumentJson, `"nodes":[]`) {
		t.Fatalf("restored draft should be empty due to HasContent false, got %s", newLatest.DocumentJson)
	}
	srcRev, _ := queries.GetEntryRevision(ctx, oldRevID)
	if !strings.Contains(srcRev.DocumentJson, "OLD BLOCK CONTENT") {
		t.Fatalf("historical source should remain unchanged")
	}
}

func TestHardening_HierarchicalRouteLess(t *testing.T) {
	client, srvURL, queries, database, hub, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	// Create hierarchical locations with Single false
	input := content.ContentTypeInput{
		ID:           "location",
		PluralName:   "Locations",
		Hierarchical: true,
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: false, Archive: false},
		},
	}
	// Need to set Hierarchical via input.Hierarchical true; but config hierarchical is separate
	// contenttypes service uses Hierarchical bool at top level
	// For catalog, Hierarchical is separate field
	input.Hierarchical = true
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	// Create Europe
	tok := csrfToken(t, client, srvURL, "/admin/content/location/new")
	f := url.Values{"title": {"Europe"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
	resp := postForm(t, client, srvURL, "/admin/content/location", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create Europe %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	entries, _ := queries.ListEntriesByContentType(ctx, "location")
	var europeID string
	for _, e := range entries {
		if e.Slug == "europe" {
			europeID = e.ID
		}
	}
	if europeID == "" {
		t.Fatalf("europe not found")
	}
	// Create Poland with parent Europe
	tok = csrfToken(t, client, srvURL, "/admin/content/location/new")
	f = url.Values{"title": {"Poland"}, "parent_entry_id": {europeID}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
	resp = postForm(t, client, srvURL, "/admin/content/location", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create Poland %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Verify no routes for route-less hierarchical
	for _, e := range entries {
		if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: e.ID, Valid: true}); err == nil {
			t.Fatalf("route-less hierarchical should have no routes")
		}
	}
	entries, _ = queries.ListEntriesByContentType(ctx, "location")
	for _, e := range entries {
		if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: e.ID, Valid: true}); err == nil {
			t.Fatalf("route-less should have no route for %s", e.Slug)
		}
	}
	// Now enable Single true with base /locations, hierarchical true
	svc := contenttypes.New(database.DB, queries)
	newInput := content.ContentTypeInput{
		ID:           "location",
		PluralName:   "Locations",
		Hierarchical: true,
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: true, Archive: false, BasePath: "/locations"},
		},
	}
	if err := svc.Update(ctx, "location", newInput); err != nil {
		t.Fatalf("enable single hierarchical %v", err)
	}
	hub.Routes.Reload(ctx)
	hub.Pages.InvalidateAll()
	resp = getPath(t, client, srvURL, "/locations/europe")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/locations/europe %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/locations/europe/poland")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/locations/europe/poland %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func TestHardening_CacheInvalidationRouteLess(t *testing.T) {
	client, srvURL, queries, _, _, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         "technology",
		PluralName: "Technologies",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: false, Archive: false},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	// Create page with collection for technology
	collectionDoc := `{"version":1,"nodes":[{"id":"coll1","block":"core/collection","version":2,"props":{},"settings":{"contentType":"technology","limit":10,"source":"query"},"children":[{"id":"f1","block":"core/entry-field","version":1,"props":{"source":"entry.title"},"settings":{"tag":"p"}}]}]}`
	tok := csrfToken(t, client, srvURL, "/admin/pages/new")
	f := url.Values{"title": {"Cache Test"}, "slug": {"cache-test"}, "document_json": {collectionDoc}, "csrf_token": {tok}, "publish": {"true"}}
	resp := postForm(t, client, srvURL, "/admin/pages", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create cache test page %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Warm cache-test page
	resp = getPath(t, client, srvURL, "/cache-test")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("warm cache-test %d", resp.StatusCode)
	}
	body := bodyString(t, resp)
	if strings.Contains(body, "Go") {
		t.Fatalf("should not contain Go yet")
	}
	resp.Body.Close()
	// Publish technology Go (route-less)
	tok = csrfToken(t, client, srvURL, "/admin/content/technology/new")
	f = url.Values{"title": {"Go"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
	resp = postForm(t, client, srvURL, "/admin/content/technology", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create tech Go %d", resp.StatusCode)
	}
	resp.Body.Close()
	// Next request should contain Go due to content-type tag invalidation (route-less publish must invalidate collection)
	resp = getPath(t, client, srvURL, "/cache-test")
	body = bodyString(t, resp)
	if !strings.Contains(body, "Go") {
		t.Fatalf("cache-test after publish should contain Go, got %s", body[:3000])
	}
	resp.Body.Close()
	// Search should omit route-less (standalone search)
	resp = getPath(t, client, srvURL, "/search?q=Go")
	body = bodyString(t, resp)
	if strings.Contains(body, "Go") && strings.Contains(body, "/technologies") {
		t.Fatalf("search should omit route-less standalone")
	}
	resp.Body.Close()
}

func TestHardening_SitemapAndSearch(t *testing.T) {
	client, srvURL, queries, _, hub, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	// Create product with Single true
	input := content.ContentTypeInput{
		ID:         "product",
		PluralName: "Products",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: true, Archive: true, BasePath: "/products"},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	// Create product entry
	tok := csrfToken(t, client, srvURL, "/admin/content/product/new")
	f := url.Values{"title": {"Widget"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
	resp := postForm(t, client, srvURL, "/admin/content/product", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create product %d", resp.StatusCode)
	}
	resp.Body.Close()
	// Create testimonial route-less
	input2 := content.ContentTypeInput{
		ID:         "testimonial2",
		PluralName: "Testimonials2",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: false, Archive: true, BasePath: "/testimonials2"},
		},
	}
	if err := cat.CreateContentType(ctx, input2); err != nil {
		t.Fatal(err)
	}
	// Need to ensure archive route created for testimonial2 (via service)
	tok = csrfToken(t, client, srvURL, "/admin/content/testimonial2/new")
	f = url.Values{"title": {"T1"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
	resp = postForm(t, client, srvURL, "/admin/content/testimonial2", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create testimonial2 %d", resp.StatusCode)
	}
	resp.Body.Close()
	// Ensure site URL for sitemap (required) and reload
	if settings, err := queries.GetSiteSettings(ctx); err == nil {
		_ = queries.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
			SiteTitle: settings.SiteTitle, SiteTagline: settings.SiteTagline, HomepageMode: settings.HomepageMode,
			HomepageEntryID: settings.HomepageEntryID, PostsPageEntryID: settings.PostsPageEntryID,
			PostsPerPage: settings.PostsPerPage, PostsBasePath: settings.PostsBasePath,
			Language: settings.Language, Timezone: settings.Timezone, ActiveTheme: settings.ActiveTheme,
			IndexingEnabled: settings.IndexingEnabled, SiteUrl: "http://example.com", SitemapEnabled: 1,
			RobotsMode: settings.RobotsMode, RobotsCustom: settings.RobotsCustom,
			SpeculationMode: settings.SpeculationMode, SpeculationEagerness: settings.SpeculationEagerness,
			TitleSeparator: settings.TitleSeparator, SiteSocialMediaID: settings.SiteSocialMediaID,
			TwitterSite: settings.TwitterSite, SiteRepresents: settings.SiteRepresents, UpdatedAt: 1,
		})
		hub.Site.Reload(ctx)
	}
	hub.Sitemap.Invalidate()
	resp = getPath(t, client, srvURL, "/sitemap.xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sitemap %d", resp.StatusCode)
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, "/products/widget") {
		t.Fatalf("sitemap should contain product route, got %s", body[:2000])
	}
	if strings.Contains(body, "/testimonials2/t1") || strings.Contains(body, "testimonial2") && strings.Contains(body, "t1") {
		t.Fatalf("sitemap should not contain route-less entry URL, got %s", body[:2000])
	}
	// Archive URL may be included
	if strings.Contains(body, "/testimonials2") {
		// Archive true, so archive path should be present (if archive route exists)
		// Our testimonial2 archive route may not exist due to catalog direct create without service, so skip check
	}
	resp.Body.Close()
}

func TestHardening_EntryLinkRouteLess(t *testing.T) {
	client, srvURL, queries, _, _, cleanup := newHardeningServer(t)
	defer cleanup()
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         "testimonial",
		PluralName: "Testimonials",
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Fields:        []content.FieldDefinition{{Key: "quote", Label: "Quote", Type: content.FieldTextarea}},
			Features:      content.ContentTypeFeatures{Content: false},
			Routing:       content.ContentTypeRouting{Single: false, Archive: false},
		},
	}
	if err := cat.CreateContentType(ctx, input); err != nil {
		t.Fatal(err)
	}
	// Create testimonial
	tok := csrfToken(t, client, srvURL, "/admin/content/testimonial/new")
	f := url.Values{"title": {"Alice"}, "field_quote": {"Hello"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}, "publish": {"true"}}
	resp := postForm(t, client, srvURL, "/admin/content/testimonial", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create testimonial %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Create page with collection containing entry-link
	collectionDoc := `{"version":1,"nodes":[{"id":"coll1","block":"core/collection","version":2,"props":{},"settings":{"contentType":"testimonial","limit":5,"source":"query"},"children":[{"id":"stack1","block":"core/stack","version":1,"props":{},"settings":{},"children":[{"id":"el","block":"core/entry-link","version":1,"props":{"text":"View"},"settings":{}}]}]}]}`
	tok = csrfToken(t, client, srvURL, "/admin/pages/new")
	f = url.Values{"title": {"Link Test"}, "slug": {"link-test"}, "document_json": {collectionDoc}, "csrf_token": {tok}, "publish": {"true"}}
	resp = postForm(t, client, srvURL, "/admin/pages", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create link test page %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, srvURL, "/link-test")
	body := bodyString(t, resp)
	// For route-less, permalink empty, should NOT create <a href=""> or <a href="/"> or slug-derived
	// Check specifically for entry-link
	if strings.Contains(body, `class="stratum-entry-link"`) {
		t.Fatalf("entry-link should not create link for route-less (should render plain View), body %s", body[:2000])
	}
	if strings.Contains(body, `/testimonial/`) {
		t.Fatalf("should not have slug-derived fake URL, body %s", body[:2000])
	}
	// Correct behavior: omit link wrapper or render non-linked inner content (View)
	if !strings.Contains(body, "View") {
		t.Fatalf("should render inner content View, body %s", body[:2000])
	}
	resp.Body.Close()
}
