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
type entryFormData struct {
	Heading        string
	Action         string
	PublishAction  string
	BackURL        string
	Title          string
	Slug           string
	Excerpt        string
	SEOTitle       string
	SEODescription string
	CanonicalURL   string
	SiteURL        string
	PublicPath     string
	DocumentJSON   string
	EditorJSON     template.JS
	Error          string
	CSRFToken      string
	Dirty          string
	Status         string
	PublicURL      string
	ShowExcerpt    bool
	ShowSEO        bool
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
	title          string
	slug           string
	excerpt        string
	seoTitle       string
	seoDescription string
	canonicalURL   string
	documentJSON   string
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
	if err := h.blocks.ValidateDocument(doc); err != nil {
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
	var excerpt, seoTitle, seoDescription, canonicalURL sql.NullString
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
	if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revisionID, EntryID: entryID, RevisionNumber: revisionNumber, Title: input.title, Excerpt: excerpt, SeoTitle: seoTitle, SeoDescription: seoDescription, CanonicalUrl: canonicalURL, DocumentJson: string(documentJSON), CreatedBy: sql.NullString{String: authorID, Valid: true}, CreatedAt: now}); err != nil {
		return fmt.Errorf("create entry revision: %w", err)
	}
	if publish {
		if err := h.upsertEntryRoute(ctx, qtx, entryID, "/"+input.slug, now); err != nil {
			return err
		}
		if err := qtx.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
			return fmt.Errorf("publish entry revision: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit entry write: %w", err)
	}
	return nil
}

func (h *Handler) upsertEntryRoute(ctx context.Context, queries *db.Queries, entryID, path string, now int64) error {
	if path != "/" && reservedSlugs[strings.TrimPrefix(path, "/")] {
		return errors.New("this slug is reserved for a core Stratum endpoint")
	}
	byPath, err := queries.GetRouteByPath(ctx, path)
	if err == nil && (!byPath.EntryID.Valid || byPath.EntryID.String != entryID) {
		return errors.New("a route already uses this slug")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check entry route: %w", err)
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
	return queries.UpdateRoute(ctx, db.UpdateRouteParams{ID: route.ID, Path: path, EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", UpdatedAt: now})
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
		title:          strings.TrimSpace(r.FormValue("title")),
		slug:           strings.TrimSpace(r.FormValue("slug")),
		excerpt:        strings.TrimSpace(r.FormValue("excerpt")),
		seoTitle:       strings.TrimSpace(r.FormValue("seo_title")),
		seoDescription: strings.TrimSpace(r.FormValue("seo_description")),
		canonicalURL:   strings.TrimSpace(r.FormValue("canonical_url")),
		documentJSON:   postedDocument(r),
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

func postedDocument(r *http.Request) string { return r.FormValue("document_json") }

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
	return "Could not save the entry."
}
