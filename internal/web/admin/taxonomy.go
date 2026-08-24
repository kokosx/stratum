package admin

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

type taxonomyData struct {
	TaxonomyID   string
	TaxonomyName string
	PluralName   string
	Hierarchical bool
	Terms        []taxonomyTermData
	CSRFToken    string
	Section      string
}

type taxonomyTermData struct {
	ID          string
	Name        string
	Slug        string
	Description string
	ParentID    string
	ParentName  string
	Count       int64
	Path        string
	Depth       int
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	h.listTaxonomy(w, r, "category")
}

func (h *Handler) listTags(w http.ResponseWriter, r *http.Request) {
	h.listTaxonomy(w, r, "tag")
}

func (h *Handler) listTaxonomy(w http.ResponseWriter, r *http.Request, taxonomyID string) {
	tax, err := h.queries.GetTaxonomy(r.Context(), taxonomyID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	terms, err := h.queries.ListTermsByTaxonomyWithCounts(r.Context(), taxonomyID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	parentMap := map[string]string{}
	for _, t := range terms {
		parentMap[t.ID] = t.Name
	}
	termMap := map[string]db.Term{}
	for _, row := range terms {
		termMap[row.ID] = db.Term{ID: row.ID, TaxonomyID: row.TaxonomyID, ParentID: row.ParentID, Name: row.Name, Slug: row.Slug, Description: row.Description}
	}
	var dataTerms []taxonomyTermData
	for _, row := range terms {
		depth := 0
		parentID := ""
		parentName := ""
		if row.ParentID.Valid {
			parentID = row.ParentID.String
			parentName = parentMap[parentID]
			cur := row.ParentID.String
			for cur != "" {
				depth++
				if t, ok := termMap[cur]; ok && t.ParentID.Valid {
					cur = t.ParentID.String
				} else {
					break
				}
				if depth > 10 {
					break
				}
			}
		}
		path := ""
		if route, err := h.queries.GetRouteByTaxonomyTerm(r.Context(), db.GetRouteByTaxonomyTermParams{TaxonomyID: sql.NullString{String: taxonomyID, Valid: true}, TermID: sql.NullString{String: row.ID, Valid: true}}); err == nil {
			path = route.Path
		}
		count := row.PublishedCount
		dataTerms = append(dataTerms, taxonomyTermData{
			ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, ParentID: parentID, ParentName: parentName, Count: count, Path: path, Depth: depth,
		})
	}
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	state := ResolveNav(r.URL.Path)
	data := LayoutData{
		Title:         tax.PluralName,
		ActiveMenu:    "posts",
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content: taxonomyData{
			TaxonomyID:   taxonomyID,
			TaxonomyName: tax.SingularName,
			PluralName:   tax.PluralName,
			Hierarchical: tax.Hierarchical != 0,
			Terms:        dataTerms,
			CSRFToken:    token,
		},
	}
	if err := h.taxonomyTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render taxonomy %s: %v", taxonomyID, err)
	}
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	h.createTerm(w, r, "category", "/admin/posts/categories")
}

func (h *Handler) createTag(w http.ResponseWriter, r *http.Request) {
	h.createTerm(w, r, "tag", "/admin/posts/tags")
}

func (h *Handler) createTerm(w http.ResponseWriter, r *http.Request, taxonomyID, redirect string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	parentID := strings.TrimSpace(r.FormValue("parent_id"))
	description := strings.TrimSpace(r.FormValue("description"))
	if name == "" {
		h.setFlash(w, "Name is required.")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	if slug != "" {
		if err := routing.ValidateTermSlug(slug); err != nil {
			h.setFlash(w, err.Error())
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
	} else {
		slug = slugifyTerm(name)
	}
	svc := taxonomy.New(h.database, h.queries)
	_, err := svc.CreateTerm(r.Context(), taxonomyID, name, slug, description, parentID)
	if err != nil {
		log.Printf("create term %s %s: %v", taxonomyID, name, err)
		h.setFlash(w, "Could not create term: "+err.Error())
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Term created.")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	h.updateTerm(w, r, "category", "/admin/posts/categories")
}

func (h *Handler) updateTag(w http.ResponseWriter, r *http.Request) {
	h.updateTerm(w, r, "tag", "/admin/posts/tags")
}

func (h *Handler) updateTerm(w http.ResponseWriter, r *http.Request, taxonomyID, redirect string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	term, err := h.queries.GetTerm(r.Context(), id)
	if err != nil || term.TaxonomyID != taxonomyID {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	parentID := strings.TrimSpace(r.FormValue("parent_id"))
	description := strings.TrimSpace(r.FormValue("description"))
	if name == "" {
		h.setFlash(w, "Name is required.")
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	if slug == "" {
		slug = slugifyTerm(name)
	}
	if err := routing.ValidateTermSlug(slug); err != nil {
		h.setFlash(w, err.Error())
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	svc := taxonomy.New(h.database, h.queries)
	if _, err := svc.UpdateTerm(r.Context(), id, name, slug, description, parentID); err != nil {
		log.Printf("update term %s: %v", id, err)
		h.setFlash(w, "Could not update term: "+err.Error())
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Term updated.")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	h.deleteTerm(w, r, "category", "/admin/posts/categories")
}

func (h *Handler) deleteTag(w http.ResponseWriter, r *http.Request) {
	h.deleteTerm(w, r, "tag", "/admin/posts/tags")
}

func (h *Handler) deleteTerm(w http.ResponseWriter, r *http.Request, taxonomyID, redirect string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	term, err := h.queries.GetTerm(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if term.TaxonomyID != taxonomyID {
		http.NotFound(w, r)
		return
	}
	svc := taxonomy.New(h.database, h.queries)
	if err := svc.DeleteTerm(r.Context(), id); err != nil {
		log.Printf("delete term %s: %v", id, err)
		h.setFlash(w, "Could not delete term: "+err.Error())
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Term deleted.")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func slugifyTerm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if r == '-' {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		} else {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	res := b.String()
	res = strings.Trim(res, "-")
	if res == "" {
		res = "term"
	}
	return res
}

var _ = sql.ErrNoRows
