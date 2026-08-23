package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

var entrySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// reservedSlugs are entry slugs that would collide with core system endpoints.
// Entry public paths are "/<slug>", so reserving these slugs keeps /admin,
// /stratum, /sitemap.xml and /robots.txt owned exclusively by the application.
var reservedSlugs = map[string]bool{
	"admin":       true,
	"stratum":     true,
	"sitemap.xml": true,
	"robots.txt":  true,
}

// entryFormData is the presentation model shared by every Entry editor
// (Pages, Posts, and future Content Types). The workspace HTML is identical;
// only the per-type flags and the action URLs differ.
type layoutTemplateOption struct {
	ID   string
	Name string
}

type entryFormData struct {
	Heading         string
	Action          string
	PublishAction   string
	BackURL         string
	Title           string
	Slug            string
	Excerpt         string
	SEOTitle        string
	SEODescription  string
	CanonicalURL    string
	FeaturedMediaID string
	SocialMediaID   string
	RobotsIndex     string // "inherit" | "1" | "0"
	RobotsFollow    string // "inherit" | "1" | "0"
	SchemaMode      string // "" | disabled | webpage | aboutpage | contactpage
	SiteURL         string
	PublicPath      string
	EntryID         string
	DocumentJSON    string
	EditorJSON      template.JS
	Error           string
	CSRFToken       string
	Dirty           string
	Status          string
	PublicURL       string
	ShowExcerpt     bool
	ShowSEO         bool
	ShowFeatured    bool
	IsPostsPage     bool
	PostsPagePath   string
	PostsPageWarning string
	HasUnpublishedChanges bool
	// Layout template selector
	ContentTypeID       string
	LayoutTemplateID    string
	LayoutTemplates     []layoutTemplateOption
	LayoutTemplateError string
}

// editorStatusView holds the values rendered into the editor status region via
// the "editor-status-region" template. It is the server-driven source for the
// Save Draft / Publish status indicator and the public URL link.
type editorStatusView struct {
	Dirty     string
	Status    string
	PublicURL string
}

type entryInput struct {
	title            string
	slug             string
	excerpt          string
	seoTitle         string
	seoDescription   string
	canonicalURL     string
	featuredMediaID  string
	socialMediaID    string
	robotsIndex      *bool
	robotsFollow     *bool
	schemaMode       string
	documentJSON     string
	layoutTemplateID string
}

// renderEntryForm bootstraps the shared block editor and renders the common
// Entry editor template. activeMenu selects the highlighted sidebar item.
func (h *Handler) renderEntryForm(w http.ResponseWriter, r *http.Request, data entryFormData, activeMenu string) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if h.blocks == nil {
		http.Error(w, "Block registry is not configured", http.StatusInternalServerError)
		return
	}
	if data.DocumentJSON == "" {
		data.DocumentJSON = `{"version":1,"nodes":[]}`
	}
	// Ensure layout selector is populated if content type is known but no options yet (e.g. direct render).
	if data.ContentTypeID != "" && data.LayoutTemplates == nil {
		data.LayoutTemplates = h.loadLayoutTemplateOptions(r.Context(), data.ContentTypeID)
	}
	// Default selection for new entries: ContentType default
	if data.EntryID == "" && data.LayoutTemplateID == "" && data.ContentTypeID != "" {
		if ct, err := h.queries.GetContentType(r.Context(), data.ContentTypeID); err == nil && ct.DefaultLayoutTemplateID.Valid {
			data.LayoutTemplateID = ct.DefaultLayoutTemplateID.String
		}
	}
	// Validate current selection: if it refers to unavailable template, surface warning
	if data.LayoutTemplateID != "" {
		found := false
		for _, opt := range data.LayoutTemplates {
			if opt.ID == data.LayoutTemplateID {
				found = true
				break
			}
		}
		if !found {
			// Check if template exists but unpublished/mismatched
			if tmpl, err := h.queries.GetLayoutTemplate(r.Context(), data.LayoutTemplateID); err == nil {
				if tmpl.ContentTypeID != data.ContentTypeID {
					data.LayoutTemplateError = "This template belongs to a different content type and cannot be used here."
				} else if !tmpl.PublishedRevisionID.Valid {
					data.LayoutTemplateError = "The selected layout template has not been published yet."
				} else {
					data.LayoutTemplateError = "The selected layout template is not available."
				}
			} else {
				data.LayoutTemplateError = "The selected layout template is not available."
			}
		}
	}
	doc, err := document.Decode([]byte(data.DocumentJSON))
	if err != nil {
		log.Printf("prepare editor document: %v", err)
		http.Error(w, "Invalid stored document", http.StatusInternalServerError)
		return
	}
	bootstrap, err := json.Marshal(editorBootstrap{
		Document: json.RawMessage(data.DocumentJSON), Catalog: h.blocks.EditorCatalog(), Definitions: h.blocks.EditorDefinitions(doc), PreviewURL: "/admin/editor/preview",
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// encoding/json escapes '<', '>' and '&', so this cannot terminate the script element.
	data.EditorJSON = template.JS(bootstrap)
	data.CSRFToken = token
	if err := h.entryTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: data.Heading, ActiveMenu: activeMenu, CSRFToken: token, Content: data}); err != nil {
		log.Printf("render entry form: %v", err)
	}
}

func (h *Handler) loadLayoutTemplateOptions(ctx context.Context, contentTypeID string) []layoutTemplateOption {
	rows, err := h.queries.ListPublishedLayoutTemplatesByContentType(ctx, contentTypeID)
	if err != nil {
		return nil
	}
	opts := make([]layoutTemplateOption, 0, len(rows))
	for _, r := range rows {
		opts = append(opts, layoutTemplateOption{ID: r.ID, Name: r.Name})
	}
	return opts
}

// writeEntry persists a new revision for an Entry and optionally publishes it.
// It is shared by Pages and Posts; contentType selects the entry kind and
// publish controls whether the public document is updated.
func (h *Handler) writeEntry(ctx context.Context, contentType, authorID, entryID string, input entryInput, create, publish bool) error {
	if h.database == nil {
		return errors.New("admin database is not configured")
	}
	doc, err := document.Decode([]byte(input.documentJSON))
	if err != nil {
		return fmt.Errorf("invalid document: %w", err)
	}
	if h.blocks == nil {
		return errors.New("block registry is not configured")
	}
	if err := layouts.ValidateEntryDocument(h.blocks, doc); err != nil {
		return fmt.Errorf("invalid document: %w", err)
	}
	documentJSON, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode document: %w", err)
	}
	now := time.Now().Unix()
	revisionID, err := randomID()
	if err != nil {
		return err
	}
	var excerpt, seoTitle, seoDescription, canonicalURL, featuredMediaID, socialMediaID sql.NullString
	if input.excerpt != "" {
		excerpt = sql.NullString{String: input.excerpt, Valid: true}
	}
	if input.seoTitle != "" {
		seoTitle = sql.NullString{String: input.seoTitle, Valid: true}
	}
	if input.seoDescription != "" {
		seoDescription = sql.NullString{String: input.seoDescription, Valid: true}
	}
	if input.canonicalURL != "" {
		canonicalURL = sql.NullString{String: input.canonicalURL, Valid: true}
	}
	if input.featuredMediaID != "" {
		featuredMediaID = sql.NullString{String: input.featuredMediaID, Valid: true}
	}
	if input.socialMediaID != "" {
		socialMediaID = sql.NullString{String: input.socialMediaID, Valid: true}
	}
	var robotsIndex, robotsFollow sql.NullInt64
	if input.robotsIndex != nil {
		v := int64(0)
		if *input.robotsIndex {
			v = 1
		}
		robotsIndex = sql.NullInt64{Int64: v, Valid: true}
	}
	if input.robotsFollow != nil {
		v := int64(0)
		if *input.robotsFollow {
			v = 1
		}
		robotsFollow = sql.NullInt64{Int64: v, Valid: true}
	}
	tx, err := h.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin entry write: %w", err)
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)

	revisionNumber := int64(1)
	if create {
		err = qtx.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: contentType, Slug: input.slug, Status: "active", AuthorID: sql.NullString{String: authorID, Valid: true}, CreatedAt: now, UpdatedAt: now})
	} else {
		entry, getErr := qtx.GetEntry(ctx, entryID)
		if getErr != nil || entry.ContentTypeID != contentType {
			return sql.ErrNoRows
		}
		latest, getErr := qtx.GetLatestEntryRevision(ctx, entryID)
		if getErr != nil {
			return fmt.Errorf("get latest revision: %w", getErr)
		}
		revisionNumber = latest.RevisionNumber + 1
		err = qtx.UpdateEntry(ctx, db.UpdateEntryParams{Slug: input.slug, Status: entry.Status, AuthorID: sql.NullString{String: authorID, Valid: true}, UpdatedAt: now, PublishedAt: entry.PublishedAt, ID: entryID})
	}
	if err != nil {
		return fmt.Errorf("save entry: %w", err)
	}
	schemaMode := normalizeSchemaMode(input.schemaMode)
	// Validate layout template selection
	var layoutTemplateID sql.NullString
	if strings.TrimSpace(input.layoutTemplateID) != "" {
		tmplID := strings.TrimSpace(input.layoutTemplateID)
		tmpl, err := qtx.GetLayoutTemplate(ctx, tmplID)
		if err != nil {
			return errors.New("selected layout template not found")
		}
		if tmpl.ContentTypeID != contentType {
			return fmt.Errorf("This template belongs to %s and cannot be used by a %s", tmpl.ContentTypeID, contentType)
		}
		if !tmpl.PublishedRevisionID.Valid {
			return errors.New("The selected layout template has not been published yet.")
		}
		layoutTemplateID = sql.NullString{String: tmplID, Valid: true}
	}
	if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revisionID, EntryID: entryID, RevisionNumber: revisionNumber, Title: input.title,
		Excerpt: excerpt, SeoTitle: seoTitle, SeoDescription: seoDescription, CanonicalUrl: canonicalURL,
		FeaturedMediaID: featuredMediaID, SocialMediaID: socialMediaID,
		SeoRobotsIndex: robotsIndex, SeoRobotsFollow: robotsFollow, SchemaMode: schemaMode,
		LayoutTemplateID: layoutTemplateID, DocumentJson: string(documentJSON), CreatedBy: sql.NullString{String: authorID, Valid: true}, CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("create entry revision: %w", err)
	}
	if publish {
		// Validate posts-page shell: only one paginated archive Posts block.
		if err := validatePostsBlocksForPublish(ctx, qtx, entryID, doc); err != nil {
			return err
		}
		// Central routing policy: compute the public path via routing.EntryPath.
		settings, _ := qtx.GetSiteSettings(ctx)
		postsBase := ""
		if settings.PostsBasePath != "" {
			postsBase = settings.PostsBasePath
		}
		computedPath := routing.EntryPath(contentType, input.slug, postsBase)
		// If this entry is the current Posts Page shell, its route is the archive
		// route (type=archive) at PostsBase, not a normal entry route derived from slug.
		// Publishing the shell must atomically move the archive route, redirect the old
		// archive, update posts_base_path, and remap post singles so the new slug is
		// live without a separate Settings save (P0 correctness).
		isPostsPage := settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entryID
		if isPostsPage && contentType == "page" {
			oldBase := settings.PostsBasePath
			if oldBase == "" {
				oldBase = routing.DefaultPostsBase
			}
			newBase := routing.NormalizePath("/" + strings.Trim(input.slug, "/"))
			if oldBase != newBase {
				if err := routing.ValidatePostsBasePath(newBase); err != nil {
					return err
				}
				if err := routing.SyncPostsPageSlugChanged(ctx, qtx, entryID, input.slug, oldBase, newBase, settings.HomepageMode, now); err != nil {
					return err
				}
			}
			// Archive shell has no entry-type route; its public presence is the archive route.
		} else {
			if err := h.upsertEntryRoute(ctx, qtx, entryID, computedPath, now); err != nil {
				return err
			}
		}
		// Record the first publication before it can be overwritten: published_at
		// moves on every re-publish, but structured data needs a stable
		// datePublished that survives later updates.
		if err := qtx.SetFirstPublishedAtIfNull(ctx, db.SetFirstPublishedAtIfNullParams{FirstPublishedAt: sql.NullInt64{Int64: now, Valid: true}, ID: entryID}); err != nil {
			return fmt.Errorf("record first publication: %w", err)
		}
		if err := qtx.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
			return fmt.Errorf("publish entry revision: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit entry write: %w", err)
	}
	// Ensure dedicated 1200x630 derivatives exist for chosen SEO images so the
	// public OG tag can always serve the /social variant. Failures are non-fatal
	// (the original can still be served, and the public fallback handles it).
	if h.media != nil {
		for _, mid := range []string{input.socialMediaID, input.featuredMediaID} {
			if mid == "" {
				continue
			}
			if _, err := h.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: mid, Kind: "social"}); err != nil {
				_ = h.media.GenerateSocialVariant(ctx, mid, media.FocalPoint{X: 0.5, Y: 0.5})
			}
		}
	}
	return nil
}

func (h *Handler) upsertEntryRoute(ctx context.Context, queries *db.Queries, entryID, path string, now int64) error {
	// The entry configured as the homepage is always served at "/", regardless
	// of its slug; applyHomepageRoute owns that mapping. Pinning here also
	// self-heals a homepage that an earlier publish moved to /<slug>: the stale
	// redirect occupying "/" below is deleted and the route returns to "/".
	if settings, err := queries.GetSiteSettings(ctx); err == nil && settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entryID {
		path = "/"
	}
	if path != "/" && reservedSlugs[strings.TrimPrefix(path, "/")] {
		return errors.New("this slug is reserved for a core Stratum endpoint")
	}
	// Reject the new slug only when it is occupied by a different entry. A
	// pre-existing redirect route at this path (left behind by an earlier slug
	// change) is safe to reclaim.
	byPath, err := queries.GetRouteByPath(ctx, path)
	if err == nil && byPath.EntryID.Valid && byPath.EntryID.String != entryID {
		return errors.New("a route already uses this slug")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check entry route: %w", err)
	}
	// If the new path holds a stale redirect (from an earlier slug change),
	// drop it so the entry owns the path cleanly.
	if err == nil && !byPath.EntryID.Valid {
		if delErr := queries.DeleteRoute(ctx, byPath.ID); delErr != nil {
			return fmt.Errorf("clear stale redirect: %w", delErr)
		}
	}

	route, err := queries.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		routeID, idErr := randomID()
		if idErr != nil {
			return idErr
		}
		return queries.CreateRoute(ctx, db.CreateRouteParams{ID: routeID, Path: path, EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	}
	if err != nil {
		return fmt.Errorf("get entry route: %w", err)
	}
	if route.Path == path {
		return nil
	}

	oldPath := route.Path
	// Move the live entry route to the new path first. This frees the old path
	// so the redirect below becomes its own row instead of clobbering the entry
	// route.
	if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{ID: route.ID, Path: path, EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", UpdatedAt: now}); err != nil {
		return fmt.Errorf("move entry route: %w", err)
	}
	// Keep the old path as a 301 redirect to the new one so existing links and
	// search indexes keep working.
	return h.upsertRedirectRoute(ctx, queries, oldPath, path, now)
}

// upsertRedirectRoute records (or refreshes) a 301 redirect from source to
// target. It always uses its own route row so it never clobbers the entry's
// live route.
//
// Redirect history is kept flat, never chained: before writing source→target,
// every existing redirect that pointed at source is retargeted straight to
// target. If an entry moved A→B earlier (A→B on record) and now moves B→C,
// the result is A→C and B→C — not A→B→C.
func (h *Handler) upsertRedirectRoute(ctx context.Context, queries *db.Queries, source, target string, now int64) error {
	// Retarget inbound redirects of the old target directly to the new one.
	inbound, err := queries.ListRedirectsToTarget(ctx, sql.NullString{String: source, Valid: true})
	if err != nil {
		return fmt.Errorf("list redirects to %s: %w", source, err)
	}
	for _, inboundRoute := range inbound {
		if inboundRoute.Path == target {
			continue
		}
		if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
			ID:             inboundRoute.ID,
			Path:           inboundRoute.Path,
			EntryID:        sql.NullString{},
			RouteType:      "redirect",
			RedirectTo:     sql.NullString{String: target, Valid: true},
			RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true},
			UpdatedAt:      now,
		}); err != nil {
			return fmt.Errorf("flatten redirect chain %s: %w", inboundRoute.Path, err)
		}
	}
	existing, err := queries.GetRouteByPath(ctx, source)
	if err == nil {
		return queries.UpdateRoute(ctx, db.UpdateRouteParams{
			ID:             existing.ID,
			Path:           source,
			EntryID:        sql.NullString{},
			RouteType:      "redirect",
			RedirectTo:     sql.NullString{String: target, Valid: true},
			RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true},
			UpdatedAt:      now,
		})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check redirect route: %w", err)
	}
	routeID, idErr := randomID()
	if idErr != nil {
		return idErr
	}
	return queries.CreateRoute(ctx, db.CreateRouteParams{
		ID:             routeID,
		Path:           source,
		EntryID:        sql.NullString{},
		RouteType:      "redirect",
		RedirectTo:     sql.NullString{String: target, Valid: true},
		RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true},
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

func (h *Handler) entryAndLatestRevision(ctx context.Context, entryID, contentType string) (db.Entry, db.EntryRevision, error) {
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil || entry.ContentTypeID != contentType {
		return db.Entry{}, db.EntryRevision{}, sql.ErrNoRows
	}
	revision, err := h.queries.GetLatestEntryRevision(ctx, entryID)
	return entry, revision, err
}

func (h *Handler) currentUser(r *http.Request) (auth.User, error) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return auth.User{}, err
	}
	return h.auth.UserForToken(r.Context(), cookie.Value)
}

func readEntryInput(r *http.Request) (entryInput, error) {
	input := entryInput{
			title:            strings.TrimSpace(r.FormValue("title")),
			slug:             strings.TrimSpace(r.FormValue("slug")),
			excerpt:          strings.TrimSpace(r.FormValue("excerpt")),
			seoTitle:         strings.TrimSpace(r.FormValue("seo_title")),
			seoDescription:   strings.TrimSpace(r.FormValue("seo_description")),
			canonicalURL:     strings.TrimSpace(r.FormValue("canonical_url")),
			featuredMediaID:  strings.TrimSpace(r.FormValue("featured_media_id")),
			socialMediaID:    strings.TrimSpace(r.FormValue("social_media_id")),
			robotsIndex:      parseRobotsOverride(r.FormValue("seo_robots_index")),
			robotsFollow:     parseRobotsOverride(r.FormValue("seo_robots_follow")),
			schemaMode:       strings.TrimSpace(r.FormValue("schema_mode")),
			documentJSON:     postedDocument(r),
			layoutTemplateID: strings.TrimSpace(r.FormValue("layout_template_id")),
		}
	if input.title == "" {
		return input, errors.New("title is required")
	}
	if !entrySlugPattern.MatchString(input.slug) {
		return input, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
	}
	if reservedSlugs[input.slug] {
		return input, errors.New("this slug is reserved for a core Stratum endpoint")
	}
	if !validCanonicalURL(input.canonicalURL) {
		return input, errors.New("canonical URL must be an absolute http(s) URL or start with /")
	}
	if input.documentJSON == "" {
		return input, errors.New("document is required")
	}
	return input, nil
}

// validCanonicalURL accepts an empty value, an absolute http(s) URL, or a
// root-relative path. It deliberately does not block on length.
func validCanonicalURL(value string) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return true
	}
	return strings.HasPrefix(value, "/")
}

// parseRobotsOverride interprets the tri-state robots form value:
// "" or "inherit" => nil (inherit), "1"/"index"/"true" => true,
// "0"/"noindex"/"false" => false.
func parseRobotsOverride(value string) *bool {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" || v == "inherit" {
		return nil
	}
	switch v {
	case "1", "true", "index", "follow":
		b := true
		return &b
	case "0", "false", "noindex", "nofollow":
		b := false
		return &b
	}
	return nil
}

func robotsFormValue(v sql.NullInt64) string {
	if !v.Valid {
		return "inherit"
	}
	if v.Int64 != 0 {
		return "1"
	}
	return "0"
}

func robotsInputFormValue(v *bool) string {
	if v == nil {
		return "inherit"
	}
	if *v {
		return "1"
	}
	return "0"
}

func normalizeSchemaMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "disabled", "webpage", "aboutpage", "contactpage":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func postedDocument(r *http.Request) string { return r.FormValue("document_json") }

// validatePostsBlocksForPublish enforces that a Posts Page shell contains at
// most one paginated archive Posts block. Two paginated listings sharing the
// same archive URL would be ambiguous, so the publish is rejected.
func validatePostsBlocksForPublish(ctx context.Context, qtx *db.Queries, entryID string, doc *document.Document) error {
	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return nil // no settings → not a posts page, no validation
	}
	if !settings.PostsPageEntryID.Valid || settings.PostsPageEntryID.String != entryID {
		return nil
	}
	if doc == nil {
		return nil
	}
	count := 0
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/posts" {
				// Default source is archive, default pagination is true.
				source := "archive"
				pagination := true
				if len(n.Settings) > 0 {
					var s map[string]any
					if json.Unmarshal(n.Settings, &s) == nil {
						if v, ok := s["source"].(string); ok && v != "" {
							source = v
						}
						if v, ok := s["pagination"].(bool); ok {
							pagination = v
						}
					}
				}
				if source == "archive" && pagination {
					count++
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(doc.Nodes)
	if count > 1 {
		return errors.New("Only one paginated archive Posts block can be used on a Posts Page.")
	}
	return nil
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func entryWriteError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint") {
		return "this slug is already in use"
	}
	if strings.Contains(msg, "route already uses") {
		return err.Error()
	}
	if strings.Contains(msg, "invalid document") {
		return err.Error()
	}
	if strings.Contains(msg, "Layout template") || strings.Contains(msg, "layout template") || strings.Contains(msg, "Content Slot") || strings.Contains(msg, "template belongs") {
		return err.Error()
	}
	return "Could not save the entry."
}
