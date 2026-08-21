package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

var pageSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type pageFormData struct {
	Heading       string
	Action        string
	PublishAction string
	Title         string
	Slug          string
	Content       string
	Error         string
	CSRFToken     string
}

type pageInput struct {
	title   string
	slug    string
	content string
}

func (h *Handler) newPage(w http.ResponseWriter, r *http.Request) {
	h.renderPageForm(w, pageFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages"})
}

func (h *Handler) createPage(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	input, err := readPageInput(r)
	if err != nil {
		h.renderPageForm(w, pageFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages", Title: r.FormValue("title"), Slug: r.FormValue("slug"), Content: r.FormValue("content"), Error: err.Error()})
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	entryID, err := randomID()
	if err == nil {
		err = h.writePage(r.Context(), user.ID, entryID, input, true, r.FormValue("publish") != "")
	}
	if err != nil {
		log.Printf("create page: %v", err)
		h.renderPageForm(w, pageFormData{Heading: "Add New Page", Action: "/admin/pages", PublishAction: "/admin/pages", Title: input.title, Slug: input.slug, Content: input.content, Error: pageWriteError(err)})
		return
	}
	if r.FormValue("publish") != "" {
		h.setFlash(w, "Page published.")
	} else {
		h.setFlash(w, "Page saved as draft.")
	}
	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (h *Handler) editPage(w http.ResponseWriter, r *http.Request) {
	entry, revision, err := h.pageAndLatestRevision(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	content, err := textContent(revision.DocumentJson)
	if err != nil {
		log.Printf("read page document: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.renderPageForm(w, pageFormData{Heading: "Edit Page", Action: "/admin/pages/" + entry.ID, PublishAction: "/admin/pages/" + entry.ID + "/publish", Title: revision.Title, Slug: entry.Slug, Content: content})
}

func (h *Handler) savePage(w http.ResponseWriter, r *http.Request) {
	h.updatePage(w, r, false)
}

func (h *Handler) publishPage(w http.ResponseWriter, r *http.Request) {
	h.updatePage(w, r, true)
}

func (h *Handler) updatePage(w http.ResponseWriter, r *http.Request, publish bool) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	entryID := r.PathValue("id")
	input, err := readPageInput(r)
	if err != nil {
		h.renderPageForm(w, pageFormData{Heading: "Edit Page", Action: "/admin/pages/" + entryID, PublishAction: "/admin/pages/" + entryID + "/publish", Title: r.FormValue("title"), Slug: r.FormValue("slug"), Content: r.FormValue("content"), Error: err.Error()})
		return
	}
	if _, _, err := h.pageAndLatestRevision(r.Context(), entryID); err != nil {
		http.NotFound(w, r)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.writePage(r.Context(), user.ID, entryID, input, false, publish); err != nil {
		log.Printf("save page: %v", err)
		h.renderPageForm(w, pageFormData{Heading: "Edit Page", Action: "/admin/pages/" + entryID, PublishAction: "/admin/pages/" + entryID + "/publish", Title: input.title, Slug: input.slug, Content: input.content, Error: pageWriteError(err)})
		return
	}
	if publish {
		h.setFlash(w, "Page published.")
	} else {
		h.setFlash(w, "Page saved as draft.")
	}
	http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
}

func (h *Handler) writePage(ctx context.Context, authorID, entryID string, input pageInput, create, publish bool) error {
	if h.database == nil {
		return errors.New("admin database is not configured")
	}
	now := time.Now().Unix()
	revisionID, err := randomID()
	if err != nil {
		return err
	}
	tx, err := h.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin page write: %w", err)
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)

	revisionNumber := int64(1)
	nodeID := ""
	if create {
		err = qtx.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: input.slug, Status: "active", AuthorID: sql.NullString{String: authorID, Valid: true}, CreatedAt: now, UpdatedAt: now})
	} else {
		entry, getErr := qtx.GetEntry(ctx, entryID)
		if getErr != nil || entry.ContentTypeID != "page" {
			return sql.ErrNoRows
		}
		latest, getErr := qtx.GetLatestEntryRevision(ctx, entryID)
		if getErr != nil {
			return fmt.Errorf("get latest revision: %w", getErr)
		}
		nodeID, getErr = textNodeID(latest.DocumentJson)
		if getErr != nil {
			return fmt.Errorf("read latest page document: %w", getErr)
		}
		revisionNumber = latest.RevisionNumber + 1
		err = qtx.UpdateEntry(ctx, db.UpdateEntryParams{Slug: input.slug, Status: entry.Status, AuthorID: sql.NullString{String: authorID, Valid: true}, UpdatedAt: now, PublishedAt: entry.PublishedAt, ID: entryID})
	}
	if err != nil {
		return fmt.Errorf("save page entry: %w", err)
	}
	documentJSON, err := textDocument(input.content, nodeID)
	if err != nil {
		return err
	}
	if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revisionID, EntryID: entryID, RevisionNumber: revisionNumber, Title: input.title, DocumentJson: documentJSON, CreatedBy: sql.NullString{String: authorID, Valid: true}, CreatedAt: now}); err != nil {
		return fmt.Errorf("create page revision: %w", err)
	}
	if publish {
		if err := h.upsertPageRoute(ctx, qtx, entryID, input.slug, now); err != nil {
			return err
		}
		if err := qtx.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
			return fmt.Errorf("publish page revision: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit page write: %w", err)
	}
	return nil
}

func (h *Handler) upsertPageRoute(ctx context.Context, queries *db.Queries, entryID, slug string, now int64) error {
	path := "/" + slug
	byPath, err := queries.GetRouteByPath(ctx, path)
	if err == nil && (!byPath.EntryID.Valid || byPath.EntryID.String != entryID) {
		return errors.New("a route already uses this slug")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check page route: %w", err)
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
		return fmt.Errorf("get page route: %w", err)
	}
	return queries.UpdateRoute(ctx, db.UpdateRouteParams{ID: route.ID, Path: path, EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", UpdatedAt: now})
}

func (h *Handler) pageAndLatestRevision(ctx context.Context, entryID string) (db.Entry, db.EntryRevision, error) {
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil || entry.ContentTypeID != "page" {
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

func (h *Handler) renderPageForm(w http.ResponseWriter, data pageFormData) {
	token, err := h.csrfToken(w)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.CSRFToken = token
	if err := h.pageTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: data.Heading, ActiveMenu: "pages", CSRFToken: token, Content: data}); err != nil {
		log.Printf("render page form: %v", err)
	}
}

func readPageInput(r *http.Request) (pageInput, error) {
	input := pageInput{title: strings.TrimSpace(r.FormValue("title")), slug: strings.TrimSpace(r.FormValue("slug")), content: r.FormValue("content")}
	if input.title == "" {
		return input, errors.New("title is required")
	}
	if !pageSlugPattern.MatchString(input.slug) {
		return input, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
	}
	return input, nil
}

func textDocument(content, nodeID string) (string, error) {
	props, err := json.Marshal(map[string]string{"text": content})
	if err != nil {
		return "", err
	}
	if nodeID == "" {
		nodeID, err = randomID()
		if err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(document.Document{Version: 1, Nodes: []document.Node{{ID: nodeID, Block: "core/text", Version: 1, Props: props}}})
	return string(encoded), err
}

func textNodeID(documentJSON string) (string, error) {
	doc, err := document.Decode([]byte(documentJSON))
	if err != nil {
		return "", err
	}
	for _, node := range doc.Nodes {
		if node.Block == "core/text" && node.Version == 1 {
			return node.ID, nil
		}
	}
	return "", nil
}

func textContent(documentJSON string) (string, error) {
	doc, err := document.Decode([]byte(documentJSON))
	if err != nil {
		return "", err
	}
	for _, node := range doc.Nodes {
		if node.Block != "core/text" || node.Version != 1 {
			continue
		}
		var props struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(node.Props, &props); err != nil {
			return "", err
		}
		return props.Text, nil
	}
	return "", nil
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func pageWriteError(err error) string {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "unique constraint") {
		return "a page already uses this slug"
	}
	if strings.Contains(err.Error(), "route already uses") {
		return err.Error()
	}
	return "Could not save the page."
}
