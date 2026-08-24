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
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
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

type taxonomyTermOption struct {
	ID       string
	Name     string
	Slug     string
	ParentID string
	Depth    int
}

type taxonomyPanelData struct {
	Taxonomy       taxonomyPanelTaxonomy
	Terms          []taxonomyTermOption
	AssignedIDs    map[string]bool
	AssignedTagRaw string // comma-joined names for flat taxonomy
}

type taxonomyPanelTaxonomy struct {
	ID           string
	PluralName   string
	SingularName string
	Hierarchical bool
}

type entryFormData struct {
	Heading               string
	Action                string
	PublishAction         string
	BackURL               string
	Title                 string
	Slug                  string
	Excerpt               string
	SEOTitle              string
	SEODescription        string
	CanonicalURL          string
	FeaturedMediaID       string
	SocialMediaID         string
	RobotsIndex           string // "inherit" | "1" | "0"
	RobotsFollow          string // "inherit" | "1" | "0"
	SchemaMode            string // "" | disabled | webpage | aboutpage | contactpage
	SiteURL               string
	PublicPath            string
	EntryID               string
	DocumentJSON          string
	EditorJSON            template.JS
	Error                 string
	CSRFToken             string
	Dirty                 string
	Status                string
	PublicURL             string
	ShowExcerpt           bool
	ShowSEO               bool
	ShowFeatured          bool
	IsPostsPage           bool
	PostsPagePath         string
	PostsPageWarning      string
	HasUnpublishedChanges bool
	// Layout template selector
	ContentTypeID       string
	LayoutTemplateID    string
	LayoutTemplates     []layoutTemplateOption
	LayoutTemplateError string
	TaxonomyPanels      []taxonomyPanelData
	ParentEntryID       string
	MenuOrder           int64
	HierarchyParents    []hierarchyParentOption
	HierarchyWarning    string
	Hierarchical        bool
}

type hierarchyParentOption struct {
	ID    string
	Title string
	Label string
	Depth int
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
	TermIDs          []string
	parentEntryID    string
	menuOrder        int64
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
	if content.DefinitionFor(data.ContentTypeID).Capabilities.Hierarchical {
		data.Hierarchical = true
		data.HierarchyParents, data.HierarchyWarning = h.hierarchyParentOptions(r.Context(), data.ContentTypeID, data.EntryID, data.ParentEntryID)
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
	// Taxonomy panels: generic by content type (no if contentType=="post")
	if len(data.TaxonomyPanels) == 0 && data.ContentTypeID != "" {
		if taxRows, err := h.queries.ListTaxonomiesByContentType(r.Context(), data.ContentTypeID); err == nil {
			// assigned term IDs from latest revision
			assigned := map[string]bool{}
			assignedTagNames := map[string][]string{}
			if data.EntryID != "" {
				if rev, err := h.queries.GetLatestEntryRevision(r.Context(), data.EntryID); err == nil {
					if termRows, err := h.queries.ListTermsForRevision(r.Context(), rev.ID); err == nil {
						for _, tr := range termRows {
							assigned[tr.ID] = true
						}
					}
				}
			}
			// Also consider posted TermIDs if present (error re-render): data.TermIDs already?
			// For re-render after validation error, entryInput.TermIDs may be in data? We pass via data? Not yet, but we can use map.
			panels := make([]taxonomyPanelData, 0, len(taxRows))
			for _, tax := range taxRows {
				terms, _ := h.queries.ListTermsByTaxonomy(r.Context(), tax.ID)
				termOpts := make([]taxonomyTermOption, 0, len(terms))
				termMap := map[string]taxonomyTermOption{}
				for _, t := range terms {
					termMap[t.ID] = taxonomyTermOption{ID: t.ID, Name: t.Name, Slug: t.Slug, ParentID: t.ParentID.String}
				}
				// compute depth
				for _, t := range terms {
					depth := 0
					cur := t.ParentID
					visited := map[string]bool{}
					for cur.Valid {
						if visited[cur.String] {
							break
						}
						visited[cur.String] = true
						depth++
						if depth > 10 {
							break
						}
						if p, ok := termMap[cur.String]; ok && p.ParentID != "" {
							cur = sql.NullString{String: p.ParentID, Valid: true}
						} else {
							break
						}
					}
					termOpts = append(termOpts, taxonomyTermOption{ID: t.ID, Name: t.Name, Slug: t.Slug, ParentID: t.ParentID.String, Depth: depth})
				}
				// For flat taxonomy, build comma-joined assigned names
				assignedRaw := ""
				if tax.Hierarchical == 0 {
					var names []string
					for _, t := range terms {
						if assigned[t.ID] {
							names = append(names, t.Name)
						}
					}
					assignedRaw = strings.Join(names, ", ")
					assignedTagNames[tax.ID] = names
				}
				panels = append(panels, taxonomyPanelData{
					Taxonomy: taxonomyPanelTaxonomy{ID: tax.ID, PluralName: tax.PluralName, SingularName: tax.SingularName, Hierarchical: tax.Hierarchical != 0},
					Terms:    termOpts, AssignedIDs: assigned, AssignedTagRaw: assignedRaw,
				})
			}
			data.TaxonomyPanels = panels
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
	state := ResolveNav(r.URL.Path)
	if err := h.entryTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: data.Heading, ActiveMenu: activeMenu, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: AdminNav(), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render entry form: %v", err)
	}
}

func (h *Handler) hierarchyParentOptions(ctx context.Context, contentTypeID, entryID, selectedParentID string) ([]hierarchyParentOption, string) {
	rows, err := h.queries.ListLatestHierarchyForContentType(ctx, contentTypeID)
	if err != nil {
		return nil, "Could not load the page hierarchy."
	}
	nodes := make([]content.HierarchyNode, 0, len(rows))
	status := make(map[string]string, len(rows))
	for _, row := range rows {
		parent := ""
		if row.ParentEntryID.Valid {
			parent = row.ParentEntryID.String
		}
		nodes = append(nodes, content.HierarchyNode{EntryID: row.EntryID, Slug: row.Slug, ParentEntryID: parent, MenuOrder: row.MenuOrder, Title: row.Title})
		status[row.EntryID] = row.Status
	}
	tree, err := content.NewHierarchy(nodes)
	if err != nil {
		return nil, "The current page hierarchy is invalid; choose a different parent."
	}
	excluded := map[string]bool{entryID: true}
	for _, descendant := range tree.Descendants(entryID) {
		excluded[descendant.EntryID] = true
	}
	settings, _ := h.queries.GetSiteSettings(ctx)
	var options []hierarchyParentOption
	for _, node := range nodes {
		if excluded[node.EntryID] || status[node.EntryID] == "trash" || (settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == node.EntryID) {
			continue
		}
		depth := tree.Depth(node.EntryID)
		options = append(options, hierarchyParentOption{ID: node.EntryID, Title: node.Title, Label: strings.Repeat("— ", depth) + node.Title, Depth: depth})
	}
	for _, option := range options {
		if option.ID == selectedParentID {
			return options, ""
		}
	}
	if selectedParentID != "" {
		return options, "The currently selected parent is no longer valid. Choose a different parent before saving."
	}
	return options, ""
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
	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return fmt.Errorf("load site settings: %w", err)
	}
	isPostsPage := settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entryID
	if err := validateHierarchyInput(ctx, qtx, contentType, entryID, input.parentEntryID, input.menuOrder, isPostsPage, settings.PostsPageEntryID); err != nil {
		return err
	}

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
		LayoutTemplateID: layoutTemplateID, ParentEntryID: nullableString(input.parentEntryID), MenuOrder: input.menuOrder,
		DocumentJson: string(documentJSON), CreatedBy: sql.NullString{String: authorID, Valid: true}, CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("create entry revision: %w", err)
	}
	// Revision-scoped taxonomy assignments (must be inside same tx, fails atomically)
	{
		dedup := make(map[string]struct{}, len(input.TermIDs))
		for _, tid := range input.TermIDs {
			tid = strings.TrimSpace(tid)
			if tid == "" {
				continue
			}
			if _, ok := dedup[tid]; ok {
				continue
			}
			dedup[tid] = struct{}{}
			if _, err := qtx.GetTerm(ctx, tid); err != nil {
				return fmt.Errorf("invalid term %s: %w", tid, err)
			}
			if err := qtx.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: revisionID, TermID: tid}); err != nil {
				return fmt.Errorf("set term %s: %w", tid, err)
			}
		}
	}
	if publish {
		// Validate posts-page shell: only one paginated archive Posts block.
		if err := validatePostsBlocksForPublish(ctx, qtx, entryID, doc); err != nil {
			return err
		}
		// Central routing policy: compute the public path via routing.EntryPath.
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
		} else if content.DefinitionFor(contentType).Capabilities.Hierarchical {
			if _, err := routing.SyncHierarchyPublish(ctx, qtx, routing.HierarchyEntry{
				EntryID: entryID, ContentTypeID: contentType, Slug: input.slug, Status: "active", Title: input.title,
				ParentEntryID: input.parentEntryID, MenuOrder: input.menuOrder,
			}, now); err != nil {
				return err
			}
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
	return routing.UpsertEntryRoute(ctx, queries, entryID, path, now)
}

func validateHierarchyInput(ctx context.Context, q *db.Queries, contentType, entryID, parentEntryID string, menuOrder int64, isPostsPage bool, postsPageID sql.NullString) error {
	def := content.DefinitionFor(contentType)
	if !def.Capabilities.Hierarchical {
		if parentEntryID != "" {
			return errors.New("this content type does not support a parent")
		}
		return nil
	}
	if menuOrder < 0 {
		return errors.New("order must be a non-negative integer")
	}
	if isPostsPage && parentEntryID != "" {
		return errors.New("the Posts Page cannot have a parent")
	}
	if parentEntryID != "" && postsPageID.Valid && parentEntryID == postsPageID.String {
		return errors.New("the Posts Page cannot be selected as a parent")
	}
	rows, err := q.ListLatestHierarchyForContentType(ctx, contentType)
	if err != nil {
		return err
	}
	nodes := make([]content.HierarchyNode, 0, len(rows))
	parentFound := parentEntryID == ""
	for _, row := range rows {
		parent := ""
		if row.ParentEntryID.Valid {
			parent = row.ParentEntryID.String
		}
		if row.EntryID == entryID {
			parent = parentEntryID
		}
		if row.EntryID == parentEntryID {
			if row.Status == "trash" {
				return errors.New("the selected parent is in Trash")
			}
			parentFound = true
		}
		nodes = append(nodes, content.HierarchyNode{EntryID: row.EntryID, Slug: row.Slug, ParentEntryID: parent, MenuOrder: row.MenuOrder, Title: row.Title})
	}
	if !parentFound {
		return errors.New("the selected parent does not exist in this content type")
	}
	_, err = content.NewHierarchy(nodes)
	return err
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// upsertRedirectRoute delegates to routing subsystem so taxonomy slug changes reuse the same flattening logic.
func (h *Handler) upsertRedirectRoute(ctx context.Context, queries *db.Queries, source, target string, now int64) error {
	return routing.UpsertRedirectRoute(ctx, queries, source, target, now)
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
		parentEntryID:    strings.TrimSpace(r.FormValue("parent_entry_id")),
	}
	if raw := strings.TrimSpace(r.FormValue("menu_order")); raw != "" {
		order, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || order < 0 {
			return input, errors.New("order must be a non-negative integer")
		}
		input.menuOrder = order
	}
	if input.title == "" {
		return input, errors.New("title is required")
	}
	if !entrySlugPattern.MatchString(input.slug) {
		return input, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
	}
	// A reserved segment is only unsafe at the root. Hierarchical children such
	// as /company/admin are valid; publish validates the final public path.
	if input.parentEntryID == "" && reservedSlugs[input.slug] {
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

func (h *Handler) taxonomyTermIDsForRequest(ctx context.Context, r *http.Request, contentType string) ([]string, error) {
	// Generic: list taxonomies for content type, then collect assignments
	taxRows, err := h.queries.ListTaxonomiesByContentType(ctx, contentType)
	if err != nil {
		return nil, nil
	}
	svc := taxonomy.New(h.database, h.queries)
	var out []string
	seen := map[string]bool{}
	for _, tax := range taxRows {
		key := "taxonomy_" + tax.ID
		if tax.Hierarchical != 0 {
			ids := r.Form[key]
			// also handle single value via FormValue? r.Form already contains all
			if len(ids) == 0 {
				// try FormValue comma? but hierarchical expects checkboxes, so just check PostForm
				if v := r.FormValue(key); v != "" {
					ids = strings.Split(v, ",")
				}
			}
			for _, id := range ids {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if seen[id] {
					continue
				}
				// validate term exists and belongs to taxonomy
				t, err := h.queries.GetTerm(ctx, id)
				if err != nil {
					return nil, fmt.Errorf("invalid term %s", id)
				}
				if t.TaxonomyID != tax.ID {
					return nil, fmt.Errorf("term %s does not belong to %s", id, tax.ID)
				}
				seen[id] = true
				out = append(out, id)
			}
		} else {
			raw := strings.TrimSpace(r.FormValue(key))
			if raw == "" {
				continue
			}
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				name := strings.TrimSpace(p)
				if name == "" {
					continue
				}
				// deduplicate case-insensitively
				lower := strings.ToLower(name)
				if seen[lower+"_tag"] {
					continue
				}
				// try slug lookup
				slug := taxonomySlugify(name)
				if t, err := h.queries.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: tax.ID, Slug: slug}); err == nil {
					if !seen[t.ID] {
						seen[t.ID] = true
						seen[lower+"_tag"] = true
						out = append(out, t.ID)
					}
					continue
				}
				// create missing tag
				created, err := svc.CreateTerm(ctx, tax.ID, name, slug, "", "")
				if err != nil {
					// if duplicate race, fetch again
					if t, err2 := h.queries.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: tax.ID, Slug: slug}); err2 == nil {
						if !seen[t.ID] {
							seen[t.ID] = true
							out = append(out, t.ID)
						}
						continue
					}
					return nil, err
				}
				if !seen[created.ID] {
					seen[created.ID] = true
					seen[lower+"_tag"] = true
					out = append(out, created.ID)
				}
			}
		}
	}
	return out, nil
}

func taxonomySlugify(s string) string {
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
	res := strings.Trim(b.String(), "-")
	if res == "" {
		res = "tag"
	}
	return res
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
