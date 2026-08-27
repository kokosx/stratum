package admin

import (
	"bytes"
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
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/richtext"
	"github.com/kokosx/stratum/internal/routing"
	"github.com/kokosx/stratum/internal/slug"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

var entrySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// reservedSlugs are entry slugs that would collide with core system endpoints.
// Entry public paths are "/<slug>", so reserving these slugs keeps /admin,
// /stratum, /sitemap.xml and /robots.txt owned exclusively by the application.
var reservedSlugs = map[string]bool{
	"admin":        true,
	"stratum":      true,
	"sitemap.xml":  true,
	"robots.txt":   true,
	"sitemap-xml":  true,
	"robots-txt":   true,
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
	ShowContent           bool
	ShowSlug              bool
	ShowVisibility        bool
	ShowTemplate          bool
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
	Revisions           []revisionHistoryItem
	CustomFields        []customFieldControl
	FieldValues         map[string]any
	Visibility          string
	Sticky              bool
	SupportsSticky      bool
	ScheduledAt         string
	ScheduledAtUnix     int64
	HasScheduled        bool
	ReviewState         string
	PublishError        string
	CommentsEnabled     bool
	SupportsComments    bool
}

type revisionHistoryItem struct {
	ID        string
	Number    int64
	Title     string
	Slug      string
	CreatedAt int64
	Published bool
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
	taxonomyValues   map[string][]string
	parentEntryID    string
	menuOrder        int64
	fields           map[string]any
	visibility       string
	password         string
	sticky           bool
	reviewState      string
	scheduledAt      string // raw datetime-local value
	commentsEnabled  bool
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
	definition, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), data.ContentTypeID)
	if err != nil {
		definition = content.DefinitionFor(data.ContentTypeID)
	}
	if data.Error != "" {
		data.FieldValues = rawFieldValues(r, definition)
	}
	data.CustomFields = customFieldControls(definition, data.FieldValues)
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
	if definition.Capabilities.Hierarchical {
		data.Hierarchical = true
		data.HierarchyParents, data.HierarchyWarning = h.hierarchyParentOptions(r.Context(), data.ContentTypeID, data.EntryID, data.ParentEntryID)
	}
	// Capability-driven visibility for entry editor
	data.ShowContent = definition.Capabilities.HasContent
	// Built-ins always have content; custom uses HasContent flag
	if data.ContentTypeID == pageContentType || data.ContentTypeID == postContentType {
		data.ShowContent = true
	} else if data.ShowContent == false && definition.Capabilities.HasContent {
		data.ShowContent = true
	}
	data.ShowSlug = definition.Routing.Single
	data.ShowVisibility = definition.Routing.Single
	data.ShowTemplate = definition.Routing.Single
	// SEO only makes sense when there is a public URL target
	if definition.Capabilities.HasSEO {
		if definition.Routing.Single || definition.Routing.Archive {
			// For archive-only, SEO for archive could exist but scope small – hide for now unless Single
			if definition.Routing.Single {
				data.ShowSEO = true
			} else {
				data.ShowSEO = false
			}
		} else {
			data.ShowSEO = false
		}
	} else {
		data.ShowSEO = false
	}
	// Preserve existing excerpt/featured flags but respect capability
	if definition.Capabilities.HasExcerpt {
		data.ShowExcerpt = true
	} else {
		data.ShowExcerpt = false
	}
	if definition.Capabilities.HasFeatured {
		data.ShowFeatured = true
	} else {
		data.ShowFeatured = false
	}
	// Ensure excerpt/featured hidden when HasContent false? No, they are independent per spec, but for Technology they are false.
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
			// Use submitted values after a validation error instead of rebuilding the
			// taxonomy controls from the last stored revision.
			assigned := map[string]bool{}
			postedValues := map[string][]string(nil)
			if data.Error != "" {
				postedValues = r.Form
			}
			if postedValues == nil && data.EntryID != "" {
				if rev, err := h.queries.GetLatestEntryRevision(r.Context(), data.EntryID); err == nil {
					if termRows, err := h.queries.ListTermsForRevision(r.Context(), rev.ID); err == nil {
						for _, tr := range termRows {
							assigned[tr.ID] = true
						}
					}
				}
			}
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
				if postedValues != nil && tax.Hierarchical != 0 {
					for _, id := range postedValues["taxonomy_"+tax.ID] {
						assigned[strings.TrimSpace(id)] = true
					}
				}
				// For flat taxonomy, preserve the exact submitted comma-separated text.
				assignedRaw := ""
				if tax.Hierarchical == 0 {
					if postedValues != nil {
						assignedRaw = strings.Join(postedValues["taxonomy_"+tax.ID], ",")
					} else {
						var names []string
						for _, t := range terms {
							if assigned[t.ID] {
								names = append(names, t.Name)
							}
						}
						assignedRaw = strings.Join(names, ", ")
					}
				}
				panels = append(panels, taxonomyPanelData{
					Taxonomy: taxonomyPanelTaxonomy{ID: tax.ID, PluralName: tax.PluralName, SingularName: tax.SingularName, Hierarchical: tax.Hierarchical != 0},
					Terms:    termOpts, AssignedIDs: assigned, AssignedTagRaw: assignedRaw,
				})
			}
			data.TaxonomyPanels = panels
		}
	}
	// Publishing panel defaults: populate from latest revision if editing and not already set (e.g. after validation error we preserve posted values via entryFormData)
	if data.EntryID != "" && data.Visibility == "" {
		if rev, err := h.queries.GetLatestEntryRevision(r.Context(), data.EntryID); err == nil {
			if data.Visibility == "" {
				data.Visibility = rev.Visibility
				if data.Visibility == "" {
					data.Visibility = "public"
				}
			}
			if !data.Sticky && rev.Sticky != 0 {
				data.Sticky = true
			}
			if data.ReviewState == "" {
				data.ReviewState = rev.ReviewState
			}
			data.SupportsSticky = content.DefinitionFor(data.ContentTypeID).Capabilities.SupportsSticky
			// Scheduled job
			if job, err := h.queries.GetActivePublicationJobByEntry(r.Context(), data.EntryID); err == nil {
				data.HasScheduled = true
				data.ScheduledAtUnix = job.ScheduledAt
				// Format for datetime-local in site timezone
				if settings, err := h.queries.GetSiteSettings(r.Context()); err == nil {
					loc := time.UTC
					if l, err := time.LoadLocation(settings.Timezone); err == nil {
						loc = l
					}
					data.ScheduledAt = time.Unix(job.ScheduledAt, 0).In(loc).Format("2006-01-02T15:04")
				} else {
					data.ScheduledAt = time.Unix(job.ScheduledAt, 0).Format("2006-01-02T15:04")
				}
			} else {
				data.SupportsSticky = content.DefinitionFor(data.ContentTypeID).Capabilities.SupportsSticky
			}
		}
	}
	if data.Visibility == "" {
		data.Visibility = "public"
	}
	if data.ReviewState == "" {
		data.ReviewState = "draft"
	}
	if data.SupportsSticky == false && data.ContentTypeID != "" {
		data.SupportsSticky = content.DefinitionFor(data.ContentTypeID).Capabilities.SupportsSticky
	}
	doc, err := document.Decode([]byte(data.DocumentJSON))
	if err != nil {
		log.Printf("prepare editor document: %v", err)
		http.Error(w, "Invalid stored document", http.StatusInternalServerError)
		return
	}
	// In-memory migration for editor: historical v1 text/heading get Rich Text control without mutating stored JSON.
	migratedDoc := migrateDocumentForEditor(doc)
	migratedJSON, err := json.Marshal(migratedDoc)
	if err != nil {
		migratedJSON = []byte(data.DocumentJSON)
		migratedDoc = doc
	}
	contentTypes, fieldCatalogs := h.editorOptions(r.Context())
	taxonomyCatalogs := h.taxonomyCatalogs(r.Context())
	bootstrap, err := json.Marshal(editorBootstrap{
		Document: json.RawMessage(migratedJSON), Catalog: h.blocks.EditorCatalog(), Definitions: h.blocks.EditorDefinitions(migratedDoc), PreviewURL: "/admin/editor/preview", ContentTypeID: data.ContentTypeID, ContentTypes: contentTypes, FieldCatalogs: fieldCatalogs, TaxonomyCatalogs: taxonomyCatalogs,
	})
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// encoding/json escapes '<', '>' and '&', so this cannot terminate the script element.
	data.EditorJSON = template.JS(bootstrap)
	data.CSRFToken = token
	state := ResolveNav(r.URL.Path)
	if err := h.entryTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: data.Heading, ActiveMenu: activeMenu, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: h.navForUser(r), CSRFToken: token, Content: data}); err != nil {
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
	rows, err := h.queries.ListPublishedLayoutTemplatesByContentTypeAndKind(ctx, db.ListPublishedLayoutTemplatesByContentTypeAndKindParams{ContentTypeID: contentTypeID, Kind: "single"})
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
	definition, definitionErr := content.NewCatalog(h.queries).GetDefinition(ctx, contentType)
	if definitionErr != nil {
		if contentType == pageContentType || contentType == postContentType {
			definition = content.DefinitionFor(contentType)
		} else {
			return fmt.Errorf("load content type: %w", definitionErr)
		}
	}
	// Capability-driven sanitization per STRATUMCMS correction spec
	if !definition.Routing.Single {
		// Route-less types: visibility must be public; slug auto-generated already, no private/password
		if input.visibility != "public" {
			input.visibility = "public"
			input.password = ""
		}
		// Layout templates only make sense with Single
		if input.layoutTemplateID != "" {
			// Silently clear; UI hides selector
			input.layoutTemplateID = ""
		}
	}
	if !definition.Capabilities.HasSEO {
		input.seoTitle = ""
		input.seoDescription = ""
		input.canonicalURL = ""
		input.robotsIndex = nil
		input.robotsFollow = nil
		input.schemaMode = ""
		input.socialMediaID = ""
	}
	if !definition.Capabilities.HasExcerpt {
		input.excerpt = ""
	}
	if !definition.Capabilities.HasFeatured {
		input.featuredMediaID = ""
	}
	if !definition.Capabilities.HasContent {
		// HasContent=false means live effective freeform entry document must be empty SDT.
		// Ignore any posted freeform document nodes and persist an empty document server-side.
		// Historical revisions remain immutable; only new revisions are emptied.
		doc = &document.Document{Version: 1, Nodes: []document.Node{}}
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
	termIDs, err := h.taxonomyTermIDsForInput(ctx, qtx, contentType, input.taxonomyValues)
	if err != nil {
		return err
	}
	fields, err := content.ValidateFields(definition, input.fields, content.FieldValidationOptions{
		MediaExists: func(id string) bool {
			_, err := qtx.GetMedia(ctx, id)
			return err == nil
		},
	})
	if err != nil {
		return fmt.Errorf("invalid custom fields: %w", err)
	}
	fieldsJSON, err := content.EncodeFieldSnapshot(fields)
	if err != nil {
		return fmt.Errorf("encode custom fields: %w", err)
	}

	// For route-less content types, slug is hidden and must be auto-allocated deterministically.
	// User must not get stuck on slug collision they cannot edit. Allocate unique slug with numeric suffix.
	if !definition.Routing.Single {
		base := slugify(input.title)
		allocated, allocErr := h.allocateUniqueSlug(ctx, qtx, contentType, base, entryID)
		if allocErr != nil {
			return allocErr
		}
		input.slug = allocated
	}

	revisionNumber := int64(1)
	var latest db.EntryRevision
	reuseLatest := false
	if create {
		// Bounded retry for concurrent create race (DB constraint is final authority)
		var lastErr error
		for attempt := 0; attempt < 3; attempt++ {
			err = qtx.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: contentType, Slug: input.slug, Status: "active", AuthorID: sql.NullString{String: authorID, Valid: true}, CreatedAt: now, UpdatedAt: now})
			if err == nil {
				break
			}
			if !definition.Routing.Single && isUniqueConstraintError(err) {
				// Allocate next suffix and retry
				nextBase := slugify(input.title)
				// Force next candidate by appending attempt offset via allocateUniqueSlug with exclusion of current attempt?
				// Simple: try next numeric suffix
				candidate := fmt.Sprintf("%s-%d", nextBase, attempt+3) // homepage-3, homepage-4...
				// Check if candidate is free; if not, re-allocate
				if allocated, allocErr := h.allocateUniqueSlug(ctx, qtx, contentType, nextBase, entryID); allocErr == nil {
					candidate = allocated
				}
				input.slug = candidate
				lastErr = err
				continue
			}
			break
		}
		if err != nil {
			if lastErr != nil && isUniqueConstraintError(err) {
				return fmt.Errorf("this slug is already in use")
			}
			return fmt.Errorf("save entry: %w", err)
		}
	} else {
		entry, getErr := qtx.GetEntry(ctx, entryID)
		if getErr != nil || entry.ContentTypeID != contentType {
			return sql.ErrNoRows
		}
		latest, getErr = qtx.GetLatestEntryRevision(ctx, entryID)
		if getErr != nil {
			return fmt.Errorf("get latest revision: %w", getErr)
		}
		revisionNumber = latest.RevisionNumber + 1
		// For route-less updates, slug may need re-allocation if title changed; already allocated above.
		// For Single types, keep posted slug; uniqueness will be enforced by DB and surfaced as error.
		err = qtx.UpdateEntryProjection(ctx, db.UpdateEntryProjectionParams{Slug: input.slug, Status: entry.Status, UpdatedAt: now, PublishedAt: entry.PublishedAt, ID: entryID})
		if err != nil && !definition.Routing.Single && isUniqueConstraintError(err) {
			// Retry once with next unique slug
			if allocated, allocErr := h.allocateUniqueSlug(ctx, qtx, contentType, slugify(input.title), entryID); allocErr == nil {
				input.slug = allocated
				err = qtx.UpdateEntryProjection(ctx, db.UpdateEntryProjectionParams{Slug: input.slug, Status: entry.Status, UpdatedAt: now, PublishedAt: entry.PublishedAt, ID: entryID})
			}
		}
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
		if tmpl.Kind != "single" {
			return errors.New("Archive templates cannot be assigned to entries")
		}
		if !tmpl.PublishedRevisionID.Valid {
			return errors.New("The selected layout template has not been published yet.")
		}
		layoutTemplateID = sql.NullString{String: tmplID, Valid: true}
	}
	// Derive revision-scoped publication metadata
	visibility := input.visibility
	if visibility == "" {
		visibility = "public"
	}
	reviewState := input.reviewState
	if reviewState == "" {
		reviewState = "draft"
	}
	var passwordHash sql.NullString
	stickyVal := int64(0)
	if input.sticky {
		if !content.DefinitionFor(contentType).Capabilities.SupportsSticky {
			return errors.New("this content type does not support sticky")
		}
		stickyVal = 1
	}
	commentsEnabled := int64(0)
	if content.DefinitionFor(contentType).Capabilities.SupportsComments && input.commentsEnabled {
		commentsEnabled = 1
	}
	if visibility == "password" {
		if strings.TrimSpace(input.password) != "" {
			hash, err := publishing.HashPassword(strings.TrimSpace(input.password))
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}
			passwordHash = sql.NullString{String: hash, Valid: true}
		} else if !create && latest.Visibility == "password" && latest.PasswordHash.Valid && latest.PasswordHash.String != "" {
			passwordHash = latest.PasswordHash
		} else {
			return errors.New("password is required for password protected visibility")
		}
	} else {
		// public/private must not have hash
		passwordHash = sql.NullString{}
	}
	if visibility == "public" || visibility == "private" {
		// ensure no hash
		passwordHash = sql.NullString{}
	}
	if !create && publish {
		matches, err := revisionMatchesInput(ctx, qtx, latest, input, string(documentJSON), fieldsJSON, excerpt, seoTitle, seoDescription, canonicalURL, featuredMediaID, socialMediaID, robotsIndex, robotsFollow, schemaMode, layoutTemplateID, termIDs, visibility, passwordHash, stickyVal, reviewState, commentsEnabled)
		if err != nil {
			return err
		}
		if matches {
			revisionID = latest.ID
			reuseLatest = true
		}
	}
	if !reuseLatest {
		if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
			ID: revisionID, EntryID: entryID, RevisionNumber: revisionNumber, Slug: input.slug, Title: input.title,
			Excerpt: excerpt, SeoTitle: seoTitle, SeoDescription: seoDescription, CanonicalUrl: canonicalURL,
			FeaturedMediaID: featuredMediaID, SocialMediaID: socialMediaID,
			SeoRobotsIndex: robotsIndex, SeoRobotsFollow: robotsFollow, SchemaMode: schemaMode,
			LayoutTemplateID: layoutTemplateID, ParentEntryID: nullableString(input.parentEntryID), MenuOrder: input.menuOrder,
			DocumentJson: string(documentJSON), FieldsJson: fieldsJSON, CreatedBy: sql.NullString{String: authorID, Valid: true}, CreatedAt: now,
			Visibility: visibility, PasswordHash: passwordHash, Sticky: stickyVal, ReviewState: reviewState, CommentsEnabled: commentsEnabled,
		}); err != nil {
			return fmt.Errorf("create entry revision: %w", err)
		}
	}
	// Revision-scoped taxonomy assignments (must be inside same tx, fails atomically)
	if !reuseLatest {
		dedup := make(map[string]struct{}, len(termIDs))
		for _, tid := range termIDs {
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
		// Use shared publication logic so admin immediate publish and scheduler share the same implementation.
		entry, err := qtx.GetEntry(ctx, entryID)
		if err != nil {
			return fmt.Errorf("load entry for publish: %w", err)
		}
		rev, err := qtx.GetEntryRevision(ctx, revisionID)
		if err != nil {
			return fmt.Errorf("load revision for publish: %w", err)
		}
		if err := publishing.PublishWithQueries(ctx, qtx, entry, rev, now); err != nil {
			return err
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

func revisionMatchesInput(ctx context.Context, q *db.Queries, revision db.EntryRevision, input entryInput, documentJSON, fieldsJSON string, excerpt, seoTitle, seoDescription, canonicalURL, featuredMediaID, socialMediaID sql.NullString, robotsIndex, robotsFollow sql.NullInt64, schemaMode string, layoutTemplateID sql.NullString, termIDs []string, visibility string, passwordHash sql.NullString, sticky int64, reviewState string, commentsEnabled int64) (bool, error) {
	if revision.Slug != input.slug || revision.Title != input.title || revision.DocumentJson != documentJSON || revision.FieldsJson != fieldsJSON || revision.Excerpt != excerpt || revision.SeoTitle != seoTitle || revision.SeoDescription != seoDescription || revision.CanonicalUrl != canonicalURL || revision.FeaturedMediaID != featuredMediaID || revision.SocialMediaID != socialMediaID || revision.SeoRobotsIndex != robotsIndex || revision.SeoRobotsFollow != robotsFollow || revision.SchemaMode != schemaMode || revision.LayoutTemplateID != layoutTemplateID || revision.ParentEntryID != nullableString(input.parentEntryID) || revision.MenuOrder != input.menuOrder || revision.Visibility != visibility || revision.ReviewState != reviewState || revision.Sticky != sticky || revision.CommentsEnabled != commentsEnabled {
		return false, nil
	}
	if visibility == "password" {
		// For password, compare hash presence and if input password was blank we reused latest hash, so compare directly
		if revision.PasswordHash != passwordHash {
			return false, nil
		}
	} else if revision.PasswordHash.Valid {
		return false, nil
	}
	terms, err := q.ListTermsForRevision(ctx, revision.ID)
	if err != nil {
		return false, err
	}
	if len(terms) != len(termIDs) {
		return false, nil
	}
	want := make(map[string]struct{}, len(termIDs))
	for _, id := range termIDs {
		want[id] = struct{}{}
	}
	for _, term := range terms {
		if _, ok := want[term.ID]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// restoreEntryRevision makes a historical revision the latest draft. Revisions
// are immutable, so restoring never changes the selected historical record.
// Restoration creates a NEW draft revision that respects the CURRENT ContentTypeDefinition.
// If current HasContent=false, the restored revision's document becomes empty SDT (historical remains untouched).
func (h *Handler) restoreEntryRevision(ctx context.Context, contentType, entryID, revisionID, authorID string) error {
	entry, revision, err := h.entryAndRevision(ctx, contentType, entryID, revisionID)
	if err != nil {
		return err
	}
	latest, err := h.queries.GetLatestEntryRevision(ctx, entryID)
	if err != nil {
		return err
	}
	newID, err := randomID()
	if err != nil {
		return err
	}
	tx, err := h.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	documentJSON := revision.DocumentJson
	if def, err := content.NewCatalog(qtx).GetDefinition(ctx, contentType); err == nil {
		if !def.Capabilities.HasContent {
			// Respect current capability: empty SDT even when restoring a historical revision with blocks
			documentJSON = `{"version":1,"nodes":[]}`
		}
	} else if !content.DefinitionFor(contentType).Capabilities.HasContent {
		documentJSON = `{"version":1,"nodes":[]}`
	}
	if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: newID, EntryID: entry.ID, RevisionNumber: latest.RevisionNumber + 1, Slug: revision.Slug,
		Title: revision.Title, Excerpt: revision.Excerpt, DocumentJson: documentJSON,
		SeoTitle: revision.SeoTitle, SeoDescription: revision.SeoDescription, CanonicalUrl: revision.CanonicalUrl,
		FeaturedMediaID: revision.FeaturedMediaID, SocialMediaID: revision.SocialMediaID,
		SeoRobotsIndex: revision.SeoRobotsIndex, SeoRobotsFollow: revision.SeoRobotsFollow,
		SchemaMode: revision.SchemaMode, LayoutTemplateID: revision.LayoutTemplateID,
		ParentEntryID: revision.ParentEntryID, MenuOrder: revision.MenuOrder,
		FieldsJson: revision.FieldsJson,
		CreatedBy:  nullableString(authorID), CreatedAt: time.Now().Unix(),
		Visibility: revision.Visibility, PasswordHash: revision.PasswordHash, Sticky: revision.Sticky, ReviewState: "draft", CommentsEnabled: revision.CommentsEnabled,
	}); err != nil {
		return err
	}
	terms, err := qtx.ListTermsForRevision(ctx, revision.ID)
	if err != nil {
		return err
	}
	for _, term := range terms {
		if err := qtx.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: newID, TermID: term.ID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *Handler) entryAndRevision(ctx context.Context, contentType, entryID, revisionID string) (db.Entry, db.EntryRevision, error) {
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil || entry.ContentTypeID != contentType {
		return db.Entry{}, db.EntryRevision{}, sql.ErrNoRows
	}
	revision, err := h.queries.GetEntryRevision(ctx, revisionID)
	if err != nil || revision.EntryID != entryID {
		return db.Entry{}, db.EntryRevision{}, sql.ErrNoRows
	}
	return entry, revision, nil
}

func (h *Handler) revisionHistory(ctx context.Context, entry db.Entry) []revisionHistoryItem {
	revisions, err := h.queries.ListEntryRevisions(ctx, entry.ID)
	if err != nil {
		return nil
	}
	items := make([]revisionHistoryItem, 0, len(revisions))
	for _, revision := range revisions {
		items = append(items, revisionHistoryItem{ID: revision.ID, Number: revision.RevisionNumber, Title: revision.Title, Slug: revision.Slug, CreatedAt: revision.CreatedAt, Published: entry.PublishedRevisionID.Valid && entry.PublishedRevisionID.String == revision.ID})
	}
	return items
}

func (h *Handler) restoreRevision(w http.ResponseWriter, r *http.Request, contentType, activeMenu string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.restoreEntryRevision(r.Context(), contentType, r.PathValue("id"), r.PathValue("revisionID"), user.ID); err != nil {
		http.NotFound(w, r)
		return
	}
	h.setFlash(w, "Revision restored as a draft.")
	http.Redirect(w, r, "/admin/"+activeMenu+"/"+r.PathValue("id")+"/edit", http.StatusSeeOther)
}

func (h *Handler) unpublishEntry(w http.ResponseWriter, r *http.Request, contentType, activeMenu string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	entry, err := h.queries.GetEntry(r.Context(), r.PathValue("id"))
	if err != nil || entry.ContentTypeID != contentType {
		http.NotFound(w, r)
		return
	}
	if h.publishing == nil {
		h.publishing = publishing.New(h.database, h.queries)
	}
	if err := h.publishing.Unpublish(r.Context(), entry.ID, time.Now().Unix()); err != nil {
		if errors.Is(err, content.ErrProtectedPage) {
			h.setFlash(w, "Change Site Settings before unpublishing the Homepage or Posts Page.")
		} else if errors.Is(err, content.ErrPublishedDescendants) {
			h.setFlash(w, "Move or unpublish child pages first.")
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/"+activeMenu+"/"+entry.ID+"/edit", http.StatusSeeOther)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateEntry(entry.ID, contentType)
		h.runtime.InvalidateContent()
		_ = h.runtime.ReloadRoutes(r.Context())
	}
	h.setFlash(w, "Entry unpublished.")
	http.Redirect(w, r, "/admin/"+activeMenu+"/"+entry.ID+"/edit", http.StatusSeeOther)
}

func (h *Handler) scheduleEntry(w http.ResponseWriter, r *http.Request, contentType, activeMenu string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	entryID := r.PathValue("id")
	input, err := readEntryInput(r, contentType)
	if err != nil {
		if isDatastarRequest(r) {
			h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, err)
			return
		}
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, err)
		return
	}
	scheduledAt, err := h.parseScheduledAt(r)
	if err != nil {
		if isDatastarRequest(r) {
			h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, err)
			return
		}
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, err)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// Create a draft revision from current input (preserve visibility etc.)
	if err := h.writeEntry(r.Context(), contentType, user.ID, entryID, input, false, false); err != nil {
		if isDatastarRequest(r) {
			h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, err)
			return
		}
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, err)
		return
	}
	latest, err := h.queries.GetLatestEntryRevision(r.Context(), entryID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	now := time.Now().Unix()
	if h.publishing == nil {
		h.publishing = publishing.New(h.database, h.queries)
	}
	if err := h.publishing.Schedule(r.Context(), entryID, latest.ID, scheduledAt, user.ID, now); err != nil {
		if isDatastarRequest(r) {
			h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, err)
			return
		}
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, err)
		return
	}
	if isDatastarRequest(r) {
		h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, nil)
		// Also need to patch scheduled status? The fragment will show scheduled via entryEditorStatus after reload.
		return
	}
	h.setFlash(w, contentTypeTitle(contentType)+" scheduled.")
	http.Redirect(w, r, "/admin/"+activeMenu+"/"+entryID+"/edit", http.StatusSeeOther)
}

func (h *Handler) cancelScheduleEntry(w http.ResponseWriter, r *http.Request, contentType, activeMenu string) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	entryID := r.PathValue("id")
	if h.publishing == nil {
		h.publishing = publishing.New(h.database, h.queries)
	}
	if err := h.publishing.CancelSchedule(r.Context(), entryID, time.Now().Unix()); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if isDatastarRequest(r) {
		// Re-render status fragment
		view := h.editorStatusView(r, entryID, false, nil)
		var buf bytes.Buffer
		_ = h.entryTemplate.ExecuteTemplate(&buf, "editor-status-region", view)
		writeSSE(w, patchElementsEvent("inner", "#editor-status-region", buf.String()), toastEvent("success", "Schedule cancelled."))
		return
	}
	h.setFlash(w, "Schedule cancelled.")
	http.Redirect(w, r, "/admin/"+activeMenu+"/"+entryID+"/edit", http.StatusSeeOther)
}

func (h *Handler) submitReviewEntry(w http.ResponseWriter, r *http.Request, contentType, activeMenu string) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, patchElementsEvent("outer", "", editorErrorFragment(errors.New("invalid security token"))), toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	entryID := r.PathValue("id")
	input, err := readEntryInput(r, contentType)
	if err != nil {
		if isDatastarRequest(r) {
			h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, err)
			return
		}
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, err)
		return
	}
	// Force pending review state via dedicated endpoint – avoids relying on clicked button value in Datastar form serialization.
	input.reviewState = "pending"
	if _, _, err := h.entryAndLatestRevision(r.Context(), entryID, contentType); err != nil {
		http.NotFound(w, r)
		return
	}
	user, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	saveErr := h.writeEntry(r.Context(), contentType, user.ID, entryID, input, false, false)
	if isDatastarRequest(r) {
		h.editorSaveFragment(w, r, contentType, activeMenu, entryID, false, input, saveErr)
		return
	}
	if saveErr != nil {
		h.renderEntryError(w, r, contentType, activeMenu, entryID, input, saveErr)
		return
	}
	h.setFlash(w, "Submitted for review.")
	http.Redirect(w, r, "/admin/"+activeMenu+"/"+entryID+"/edit", http.StatusSeeOther)
}

func (h *Handler) parseScheduledAt(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.FormValue("scheduled_at"))
	if raw == "" {
		return 0, errors.New("scheduled time is required")
	}
	settings, err := h.queries.GetSiteSettings(r.Context())
	timezone := "UTC"
	if err == nil && strings.TrimSpace(settings.Timezone) != "" {
		timezone = settings.Timezone
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	// datetime-local may be "2006-01-02T15:04" or with seconds
	var t time.Time
	t, err = time.ParseInLocation("2006-01-02T15:04", raw, loc)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02T15:04:05", raw, loc)
		if err != nil {
			return 0, errors.New("invalid scheduled time")
		}
	}
	unix := t.Unix()
	if unix <= time.Now().Unix() {
		return 0, errors.New("scheduled time must be in the future")
	}
	return unix, nil
}

func (h *Handler) previewRevision(w http.ResponseWriter, r *http.Request, contentType string) {
	_, revision, err := h.entryAndRevision(r.Context(), contentType, r.PathValue("id"), r.PathValue("revisionID"))
	if err != nil || h.documentPreview == nil {
		http.NotFound(w, r)
		return
	}
	doc, err := document.Decode([]byte(revision.DocumentJson))
	if err != nil {
		http.Error(w, "Invalid revision document", http.StatusUnprocessableEntity)
		return
	}
	postsBase := ""
	if settings, err := h.queries.GetSiteSettings(r.Context()); err == nil {
		postsBase = settings.PostsBasePath
	}
	path := routing.EntryPath(contentType, revision.Slug, postsBase)
	if content.DefinitionFor(contentType).Capabilities.Hierarchical && revision.ParentEntryID.Valid {
		parentRoute, parentErr := h.queries.GetEntryRoute(r.Context(), revision.ParentEntryID)
		if parentErr != nil {
			http.Error(w, "Preview parent is unavailable", http.StatusUnprocessableEntity)
			return
		}
		path = routing.ChildEntryPath(parentRoute.Path, revision.Slug)
	}
	page, err := h.documentPreview(r.Context(), rendering.RenderInput{Document: doc, Title: revision.Title, Slug: revision.Slug, Excerpt: stringValue(revision.Excerpt), SEOTitle: stringValue(revision.SeoTitle), SEODescription: stringValue(revision.SeoDescription), Path: path, EntryID: r.PathValue("id"), LayoutTemplateID: stringValue(revision.LayoutTemplateID), ContentTypeID: contentType, Fields: fieldValues(revision.FieldsJson), FeaturedMediaID: stringValue(revision.FeaturedMediaID)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	_, _ = w.Write(page)
}

func (h *Handler) upsertEntryRoute(ctx context.Context, queries *db.Queries, entryID, path string, now int64) error {
	return routing.UpsertEntryRoute(ctx, queries, entryID, path, now)
}

func validateHierarchyInput(ctx context.Context, q *db.Queries, contentType, entryID, parentEntryID string, menuOrder int64, isPostsPage bool, postsPageID sql.NullString) error {
	def := content.DefinitionFor(contentType)
	if catalogDef, err := content.NewCatalog(q).GetDefinition(ctx, contentType); err == nil {
		def = catalogDef
	}
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

func readEntryInput(r *http.Request, contentTypes ...string) (entryInput, error) {
	contentType := ""
	if len(contentTypes) > 0 {
		contentType = contentTypes[0]
	}
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
		taxonomyValues:   postedTaxonomyValues(r),
		fields:           rawFieldValues(r, content.DefinitionFor(contentType)),
		visibility:       strings.TrimSpace(r.FormValue("visibility")),
		password:         r.FormValue("password"),
		sticky:           r.FormValue("sticky") == "1" || r.FormValue("sticky") == "on" || r.FormValue("sticky") == "true",
		reviewState:      strings.TrimSpace(r.FormValue("review_state")),
		scheduledAt:      strings.TrimSpace(r.FormValue("scheduled_at")),
		commentsEnabled:  r.FormValue("comments_enabled") == "1" || r.FormValue("comments_enabled") == "on" || r.FormValue("comments_enabled") == "true",
	}
	if input.visibility == "" {
		input.visibility = "public"
	}
	if input.visibility != "public" && input.visibility != "private" && input.visibility != "password" {
		return input, errors.New("invalid visibility")
	}
	if input.visibility == "password" && strings.TrimSpace(input.password) == "" {
		// Allow blank password when editing existing password-protected revision and keeping previous hash.
		// Validation of required password will happen in writeEntry when creating new revision with no existing hash.
	}
	if !content.DefinitionFor(contentType).Capabilities.SupportsSticky && input.sticky {
		return input, errors.New("this content type does not support sticky")
	}
	if input.reviewState == "" {
		input.reviewState = "draft"
	}
	if input.reviewState != "draft" && input.reviewState != "pending" {
		return input, errors.New("invalid review state")
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
	// Server is canonical: always normalize submitted slug through Slugify.
	// JS preview is advisory only; empty input still derives from title.
	if input.slug != "" {
		canonical := slug.Slugify(input.slug)
		if canonical == "" {
			return input, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
		}
		input.slug = canonical
	} else {
		input.slug = slugify(input.title)
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

func postedTaxonomyValues(r *http.Request) map[string][]string {
	values := make(map[string][]string)
	for key, items := range r.Form {
		if strings.HasPrefix(key, "taxonomy_") {
			values[key] = append([]string(nil), items...)
		}
	}
	return values
}

func (h *Handler) taxonomyTermIDsForInput(ctx context.Context, q *db.Queries, contentType string, values map[string][]string) ([]string, error) {
	// Generic: list taxonomies for content type, then collect assignments
	taxRows, err := q.ListTaxonomiesByContentType(ctx, contentType)
	if err != nil {
		return nil, nil
	}
	svc := taxonomy.New(h.database, h.queries)
	var out []string
	seen := map[string]bool{}
	for _, tax := range taxRows {
		key := "taxonomy_" + tax.ID
		if tax.Hierarchical != 0 {
			ids := values[key]
			// also handle single value via FormValue? r.Form already contains all
			if len(ids) == 0 {
				if v := strings.Join(values[key], ","); v != "" {
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
				t, err := q.GetTerm(ctx, id)
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
			raw := strings.TrimSpace(strings.Join(values[key], ","))
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
				if t, err := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: tax.ID, Slug: slug}); err == nil {
					if !seen[t.ID] {
						seen[t.ID] = true
						seen[lower+"_tag"] = true
						out = append(out, t.ID)
					}
					continue
				}
				// create missing tag
				created, err := svc.CreateTermWithQueries(ctx, q, tax.ID, name, slug, "", "")
				if err != nil {
					// if duplicate race, fetch again
					if t, err2 := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: tax.ID, Slug: slug}); err2 == nil {
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
	canonical := slug.Slugify(s)
	if canonical == "" {
		return "tag"
	}
	return canonical
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

func migrateDocumentForEditor(doc *document.Document) *document.Document {
	if doc == nil {
		return nil
	}
	// Use document migration registry if available, but keep simple in-memory copy for editor.
	// This intentionally does not check registry existence; v2 is always present after migration 044.
	clone := *doc
	clone.Nodes = migrateNodesForEditor(doc.Nodes)
	return &clone
}

func migrateNodesForEditor(nodes []document.Node) []document.Node {
	out := make([]document.Node, len(nodes))
	for i, n := range nodes {
		if (n.Block == "core/text" || n.Block == "core/heading") && n.Version == 1 {
			var props map[string]any
			if json.Unmarshal(n.Props, &props) == nil {
				if v, ok := props["text"].(string); ok {
					props["text"] = richtext.RichText{Version: richtext.Version, Content: []richtext.Run{{Text: v}}}
					if data, err := json.Marshal(props); err == nil {
						n.Props = data
						n.Version = 2
					}
				}
			}
		}
		n.Children = migrateNodesForEditor(n.Children)
		out[i] = n
	}
	return out
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

func slugify(title string) string {
	canonical := slug.Slugify(title)
	if canonical == "" {
		return "item"
	}
	return canonical
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "UNIQUE")
}

// allocateUniqueSlug implements deterministic unique internal slug allocation for route-less content.
// It slugifies the title, checks conflicts per content type (UNIQUE(content_type_id, slug)), and finds
// an available slug with human-readable numeric suffixes: homepage, homepage-2, homepage-3...
// Has a hard bounded loop (100 attempts) and behaves safely under concurrent create attempts
// (DB constraint remains final authority; caller should retry on constraint error).
func (h *Handler) allocateUniqueSlug(ctx context.Context, qtx *db.Queries, contentType, baseSlug, entryID string) (string, error) {
	baseSlug = strings.TrimSpace(baseSlug)
	if baseSlug == "" {
		baseSlug = "item"
	}
	if len(baseSlug) > 100 {
		baseSlug = baseSlug[:100]
	}
	// Bounded loop
	for i := 0; i < 100; i++ {
		candidate := baseSlug
		if i > 0 {
			suffix := fmt.Sprintf("-%d", i+1)
			// Ensure total length <=100
			if len(baseSlug)+len(suffix) > 100 {
				candidate = baseSlug[:100-len(suffix)] + suffix
			} else {
				candidate = baseSlug + suffix
			}
		}
		existing, err := qtx.GetFlatEntryBySlug(ctx, db.GetFlatEntryBySlugParams{ContentTypeID: contentType, Slug: candidate})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return candidate, nil
			}
			return "", err
		}
		if existing.ID == entryID {
			return candidate, nil
		}
		// Conflict, try next suffix
	}
	return "", fmt.Errorf("could not allocate unique slug for %q", baseSlug)
}
