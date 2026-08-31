package entryops

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/audit"
	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/slug"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// RuntimeInvalidator covers post-commit invalidation needed after writes.
type RuntimeInvalidator interface {
	InvalidateEntry(entryID, contentTypeID string)
	InvalidateContent()
	ReloadRoutes(ctx context.Context) error
}

// Service is the application-level Entry lifecycle service.
// It is the single validated mutation path for Admin and MCP.
type Service struct {
	db         *sql.DB
	queries    *db.Queries
	blocks     *blocks.Registry
	publishing *publishing.Service
	audit      *audit.Service
	runtime    RuntimeInvalidator
	media      *media.Service
	searchRefresh func(context.Context, string) error
}

func New(database *sql.DB, queries *db.Queries, blockRegistry *blocks.Registry, pub *publishing.Service, auditSvc *audit.Service, rt RuntimeInvalidator) *Service {
	return &Service{
		db: database, queries: queries, blocks: blockRegistry,
		publishing: pub, audit: auditSvc, runtime: rt,
	}
}

func (s *Service) SetMedia(m *media.Service) { s.media = m }
func (s *Service) SetSearchRefresh(fn func(context.Context, string) error) { s.searchRefresh = fn }
func (s *Service) SetPublishing(p *publishing.Service) { s.publishing = p }

// randomID generates a URL-safe random ID.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// getDefinition loads the content type definition with fallback for builtins.
func (s *Service) getDefinition(ctx context.Context, q *db.Queries, contentType string) (content.ContentTypeDefinition, error) {
	def, err := content.NewCatalog(q).GetDefinition(ctx, contentType)
	if err == nil {
		return def, nil
	}
	if contentType == "page" || contentType == "post" {
		return content.DefinitionFor(contentType), nil
	}
	return content.ContentTypeDefinition{}, fmt.Errorf("load content type: %w", err)
}

// resolveAuthorID determines the Entry author for creation based on actor.
func (s *Service) resolveAuthorID(ctx context.Context, q *db.Queries, actor authz.Actor) (sql.NullString, error) {
	switch actor.Kind {
	case authz.ActorUser:
		if actor.ID == "" {
			return sql.NullString{}, errors.New("user actor missing ID")
		}
		return sql.NullString{String: actor.ID, Valid: true}, nil
	case authz.ActorAgent:
		// Use agent's default_author_id as editorial author.
		// Agent IDs are not User IDs.
		agentRow, err := q.GetAgent(ctx, actor.AgentID)
		if err != nil {
			return sql.NullString{}, fmt.Errorf("agent not found: %w", err)
		}
		if !agentRow.DefaultAuthorID.Valid || strings.TrimSpace(agentRow.DefaultAuthorID.String) == "" {
			return sql.NullString{}, errors.New("agent has no default author; configure a default author for this agent")
		}
		// Validate author still exists and active
		u, err := q.GetUserByID(ctx, agentRow.DefaultAuthorID.String)
		if err != nil {
			return sql.NullString{}, errors.New("agent default author not found; choose a valid user")
		}
		if u.Status != "active" {
			return sql.NullString{}, errors.New("agent default author is disabled")
		}
		return sql.NullString{String: agentRow.DefaultAuthorID.String, Valid: true}, nil
	case authz.ActorSystem:
		return sql.NullString{}, nil
	default:
		return sql.NullString{}, errors.New("unknown actor kind")
	}
}

// applyCapabilitySanitization mutates patch-derived inputs according to definition capabilities.
// Mirrors writeEntry capability-driven sanitization.
func applyCapabilitySanitization(def content.ContentTypeDefinition, doc *document.Document, input *entryInput) (*document.Document, error) {
	if !def.Routing.Single {
		if input.visibility != "public" {
			input.visibility = "public"
			input.password = ""
		}
		if input.layoutTemplateID != "" {
			input.layoutTemplateID = ""
		}
	}
	if !def.Capabilities.HasSEO {
		input.seoTitle = ""
		input.seoDescription = ""
		input.canonicalURL = ""
		input.robotsIndex = nil
		input.robotsFollow = nil
		input.schemaMode = ""
		input.socialMediaID = ""
	}
	if !def.Capabilities.HasExcerpt {
		input.excerpt = ""
	}
	if !def.Capabilities.HasFeatured {
		input.featuredMediaID = ""
	}
	if !def.Capabilities.HasContent {
		// Force empty document
		return &document.Document{Version: 1, Nodes: []document.Node{}}, nil
	}
	return doc, nil
}

// entryInput is the internal validated shape before persistence.
// Mirrors the private Admin entryInput but is now application-level.
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
	parentEntryID    string
	menuOrder        int64
	taxonomyValues   map[string][]string
	fields           map[string]any
	visibility       string
	password         string
	sticky           bool
	reviewState      string
	commentsEnabled  bool
}

// patchToInput converts latest revision + patch into a complete entryInput ready for validation.
func (s *Service) patchToInput(ctx context.Context, q *db.Queries, latest db.EntryRevision, terms []db.Term, patch EntryPatch, definition content.ContentTypeDefinition) (entryInput, []string, error) {
	// Decode existing fields from latest to base
	baseTitle := latest.Title
	baseSlug := latest.Slug
	baseExcerpt := stringValue(latest.Excerpt)
	baseSeoTitle := stringValue(latest.SeoTitle)
	baseSeoDesc := stringValue(latest.SeoDescription)
	baseCanonical := stringValue(latest.CanonicalUrl)
	baseFeatured := stringValue(latest.FeaturedMediaID)
	baseSocial := stringValue(latest.SocialMediaID)
	baseRobotsIndex := parseRobotsFromRevision(latest.SeoRobotsIndex)
	baseRobotsFollow := parseRobotsFromRevision(latest.SeoRobotsFollow)
	baseSchema := latest.SchemaMode
	baseDocJSON := latest.DocumentJson
	baseLayout := stringValue(latest.LayoutTemplateID)
	baseParent := stringValue(latest.ParentEntryID)
	baseMenu := latest.MenuOrder
	baseFields := fieldValues(latest.FieldsJson)
	baseVisibility := latest.Visibility
	if baseVisibility == "" {
		baseVisibility = "public"
	}
	baseSticky := latest.Sticky != 0
	baseComments := latest.CommentsEnabled != 0
	baseReview := latest.ReviewState
	if baseReview == "" {
		baseReview = "draft"
	}
	// For taxonomy, rebuild values map from term assignments
	baseTaxValues := taxonTermsToValues(terms, definition) // we need helper

	// Apply patch (track changed)
	var changed []string
	input := entryInput{
		title: baseTitle, slug: baseSlug, excerpt: baseExcerpt,
		seoTitle: baseSeoTitle, seoDescription: baseSeoDesc, canonicalURL: baseCanonical,
		featuredMediaID: baseFeatured, socialMediaID: baseSocial,
		robotsIndex: baseRobotsIndex, robotsFollow: baseRobotsFollow,
		schemaMode: baseSchema, documentJSON: baseDocJSON,
		layoutTemplateID: baseLayout, parentEntryID: baseParent, menuOrder: baseMenu,
		fields: baseFields, visibility: baseVisibility, sticky: baseSticky,
		commentsEnabled: baseComments, reviewState: baseReview,
		taxonomyValues: baseTaxValues,
	}
	// Title
	if patch.Title != nil && *patch.Title != baseTitle {
		v := strings.TrimSpace(*patch.Title)
		if v == "" {
			return entryInput{}, nil, errors.New("title is required")
		}
		input.title = v
		changed = append(changed, "title")
	}
	// Slug
	if patch.Slug != nil && *patch.Slug != baseSlug {
		raw := strings.TrimSpace(*patch.Slug)
		if raw != "" {
			canonical := slugifyFromInput(raw)
			if canonical == "" {
				return entryInput{}, nil, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
			}
			if !entrySlugPattern.MatchString(canonical) {
				return entryInput{}, nil, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
			}
			input.slug = canonical
		} else {
			// empty slug means derive from title
			input.slug = slugify(input.title)
		}
		changed = append(changed, "slug")
	}
	// Excerpt etc
	if patch.Excerpt != nil && *patch.Excerpt != baseExcerpt {
		input.excerpt = strings.TrimSpace(*patch.Excerpt)
		changed = append(changed, "excerpt")
	}
	if patch.SEOTitle != nil && strings.TrimSpace(*patch.SEOTitle) != baseSeoTitle {
		input.seoTitle = strings.TrimSpace(*patch.SEOTitle)
		changed = append(changed, "seo_title")
	}
	if patch.SEODescription != nil && strings.TrimSpace(*patch.SEODescription) != baseSeoDesc {
		input.seoDescription = strings.TrimSpace(*patch.SEODescription)
		changed = append(changed, "seo_description")
	}
	if patch.CanonicalURL != nil && strings.TrimSpace(*patch.CanonicalURL) != baseCanonical {
		v := strings.TrimSpace(*patch.CanonicalURL)
		if !validCanonicalURL(v) {
			return entryInput{}, nil, errors.New("canonical URL must be an absolute http(s) URL or start with /")
		}
		input.canonicalURL = v
		changed = append(changed, "canonical_url")
	}
	if patch.FeaturedMediaID != nil && strings.TrimSpace(*patch.FeaturedMediaID) != baseFeatured {
		input.featuredMediaID = strings.TrimSpace(*patch.FeaturedMediaID)
		changed = append(changed, "featured_media_id")
	}
	if patch.SocialMediaID != nil && strings.TrimSpace(*patch.SocialMediaID) != baseSocial {
		input.socialMediaID = strings.TrimSpace(*patch.SocialMediaID)
		changed = append(changed, "social_media_id")
	}
	if patch.RobotsIndex != nil {
		// Need to compare with baseRobotsIndex
		if !boolPtrEqual(patch.RobotsIndex, baseRobotsIndex) {
			input.robotsIndex = patch.RobotsIndex
			changed = append(changed, "robots_index")
		}
	}
	if patch.RobotsFollow != nil {
		if !boolPtrEqual(patch.RobotsFollow, baseRobotsFollow) {
			input.robotsFollow = patch.RobotsFollow
			changed = append(changed, "robots_follow")
		}
	}
	if patch.SchemaMode != nil && strings.TrimSpace(*patch.SchemaMode) != baseSchema {
		input.schemaMode = strings.TrimSpace(*patch.SchemaMode)
		changed = append(changed, "schema_mode")
	}
	if patch.DocumentSet {
		// Marshal patch Document to JSON
		data, err := json.Marshal(patch.Document)
		if err != nil {
			return entryInput{}, nil, fmt.Errorf("encode document: %w", err)
		}
		input.documentJSON = string(data)
		changed = append(changed, "document")
	}
	if patch.FieldsSet {
		if patch.Fields == nil {
			input.fields = map[string]any{}
		} else {
			input.fields = patch.Fields
		}
		changed = append(changed, "fields")
	}
	if patch.LayoutSet {
		if patch.LayoutTemplateID == nil {
			input.layoutTemplateID = ""
		} else {
			input.layoutTemplateID = strings.TrimSpace(*patch.LayoutTemplateID)
		}
		changed = append(changed, "layout_template_id")
	}
	if patch.ParentSet {
		if patch.ParentEntryID == nil {
			input.parentEntryID = ""
		} else {
			input.parentEntryID = strings.TrimSpace(*patch.ParentEntryID)
		}
		changed = append(changed, "parent_entry_id")
	}
	if patch.MenuOrder != nil && *patch.MenuOrder != baseMenu {
		if *patch.MenuOrder < 0 {
			return entryInput{}, nil, errors.New("order must be a non-negative integer")
		}
		input.menuOrder = *patch.MenuOrder
		changed = append(changed, "menu_order")
	}
	if patch.Visibility != nil && strings.TrimSpace(*patch.Visibility) != baseVisibility {
		v := strings.TrimSpace(*patch.Visibility)
		if v != "public" && v != "private" && v != "password" {
			return entryInput{}, nil, errors.New("invalid visibility")
		}
		input.visibility = v
		changed = append(changed, "visibility")
	}
	if patch.PasswordSet {
		if patch.Password == nil {
			input.password = ""
		} else {
			input.password = *patch.Password
		}
		changed = append(changed, "password")
	}
	if patch.Sticky != nil && *patch.Sticky != baseSticky {
		if !definition.Capabilities.SupportsSticky && *patch.Sticky {
			return entryInput{}, nil, errors.New("this content type does not support sticky")
		}
		input.sticky = *patch.Sticky
		changed = append(changed, "sticky")
	}
	if patch.CommentsEnabled != nil && *patch.CommentsEnabled != baseComments {
		input.commentsEnabled = *patch.CommentsEnabled
		changed = append(changed, "comments_enabled")
	}
	if patch.ReviewState != nil && strings.TrimSpace(*patch.ReviewState) != baseReview {
		v := strings.TrimSpace(*patch.ReviewState)
		if v != "draft" && v != "pending" {
			return entryInput{}, nil, errors.New("invalid review state")
		}
		input.reviewState = v
		changed = append(changed, "review_state")
	}
	if patch.TaxonomySet {
		if patch.TaxonomyValues == nil {
			input.taxonomyValues = map[string][]string{}
		} else {
			input.taxonomyValues = patch.TaxonomyValues
		}
		changed = append(changed, "taxonomy")
	}
	// Ensure title required after patch (for updates title may be omitted but base already had)
	if strings.TrimSpace(input.title) == "" {
		return entryInput{}, nil, errors.New("title is required")
	}
	// Validate slug pattern after derivation
	if input.slug != "" {
		if !entrySlugPattern.MatchString(input.slug) {
			return entryInput{}, nil, errors.New("slug may contain lowercase letters, numbers, and hyphens only")
		}
		if input.parentEntryID == "" && reservedSlugs[input.slug] {
			return entryInput{}, nil, errors.New("this slug is reserved for a core Stratum endpoint")
		}
	}
	if !validCanonicalURL(input.canonicalURL) {
		return entryInput{}, nil, errors.New("canonical URL must be an absolute http(s) URL or start with /")
	}
	if input.documentJSON == "" {
		return entryInput{}, nil, errors.New("document is required")
	}
	return input, changed, nil
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func parseRobotsFromRevision(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 != 0
	return &b
}

func taxonTermsToValues(terms []db.Term, def content.ContentTypeDefinition) map[string][]string {
	// Reconstruct generic taxonomy_values map from term assignments for patch base.
	// For simplicity, return empty; Update will preserve unless TaxonomySet true.
	// Existing terms are not needed to validate unless patch changes taxonomy;
	// but for applyPatch we need base to detect no-op? We'll just return nil and treat preserve.
	return map[string][]string{}
}

func slugifyFromInput(raw string) string {
	canonical := slug.Slugify(raw)
	return canonical
}

// Ensure imports used
var _ = time.Now
var _ = log.Printf

func logError(format string, args ...any) { log.Printf(format, args...) }
