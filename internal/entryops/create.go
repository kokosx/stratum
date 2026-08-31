package entryops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/audit"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/slug"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// CreateDraft creates a new entry and its first revision as a draft.
// If entryID is empty a new ID is generated; otherwise the provided ID is used.
// Returns entryID, revisionID, revisionNumber.
func (s *Service) CreateDraft(ctx context.Context, actor authz.Actor, contentType, entryID string, patch EntryPatch) (string, string, int64, error) {
	// Authorization
	if !authz.Allowed(actor, authz.PermEntriesCreate, authz.Resource{ContentTypeID: contentType}, loadGrantsForActor(ctx, s, actor)) {
		return "", "", 0, &ForbiddenError{Permission: string(authz.PermEntriesCreate), Scope: "content_type:" + contentType}
	}

	// Load content type definition
	definition, err := s.getDefinition(ctx, s.queries, contentType)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: %v", ErrInvalidContentType, err)
	}

	// Validate required fields for create
	if patch.Title == nil || strings.TrimSpace(*patch.Title) == "" {
		return "", "", 0, fmt.Errorf("%w: title is required", ErrValidation)
	}
	title := strings.TrimSpace(*patch.Title)

	// Slug handling: if patch.Slug provided use it, else derive from title
	slugVal := ""
	if patch.Slug != nil && strings.TrimSpace(*patch.Slug) != "" {
		canonical := slug.Slugify(strings.TrimSpace(*patch.Slug))
		if canonical == "" {
			return "", "", 0, fmt.Errorf("%w: slug may contain lowercase letters, numbers, and hyphens only", ErrValidation)
		}
		if !entrySlugPattern.MatchString(canonical) {
			return "", "", 0, fmt.Errorf("%w: slug may contain lowercase letters, numbers, and hyphens only", ErrValidation)
		}
		slugVal = canonical
	} else {
		slugVal = slugify(title)
	}
	if !entrySlugPattern.MatchString(slugVal) {
		return "", "", 0, fmt.Errorf("%w: slug may contain lowercase letters, numbers, and hyphens only", ErrValidation)
	}
	// Reserved slug only at root (parent empty)
	parentEntryID := ""
	if patch.ParentSet && patch.ParentEntryID != nil {
		parentEntryID = strings.TrimSpace(*patch.ParentEntryID)
	}
	if parentEntryID == "" && reservedSlugs[slugVal] {
		return "", "", 0, fmt.Errorf("%w: this slug is reserved for a core Stratum endpoint", ErrValidation)
	}
	// Document handling
	var doc *document.Document
	if patch.DocumentSet && patch.Document != nil {
		doc = patch.Document
		// We'll marshal later after sanitization
	} else if patch.DocumentSet && patch.Document == nil {
		doc = &document.Document{Version: 1, Nodes: []document.Node{}}
	} else {
		// No document provided: default empty
		doc = &document.Document{Version: 1, Nodes: []document.Node{}}
		// If patch had no document but definition.HasContent true, we still need to allow empty doc? But spec says document required? For create we require document? We allow empty.
	}
	// At this stage, check if patch provided document JSON via raw? For MCP, DocumentSet true with document provided includes nodes.
	// We'll encode after capability sanitization.
	// Capability sanitization: build temporary entryInput for reuse
	tmpInput := entryInput{
		title: title, slug: slugVal,
		documentJSON: "", // will set after
		layoutTemplateID: valueOrEmpty(patch.LayoutTemplateID, patch.LayoutSet),
		parentEntryID:  parentEntryID,
		menuOrder:      valueOrInt64(patch.MenuOrder, 0),
		visibility:     valueOrEmpty(patch.Visibility, patch.Visibility != nil),
		password:       valueOrEmpty(patch.Password, patch.PasswordSet),
		sticky:         boolOrFalse(patch.Sticky),
		reviewState:    valueOrEmpty(patch.ReviewState, patch.ReviewState != nil),
		commentsEnabled: boolOrFalse(patch.CommentsEnabled),
	}
	// Optional fields
	if patch.Excerpt != nil {
		tmpInput.excerpt = strings.TrimSpace(*patch.Excerpt)
	}
	if patch.SEOTitle != nil {
		tmpInput.seoTitle = strings.TrimSpace(*patch.SEOTitle)
	}
	if patch.SEODescription != nil {
		tmpInput.seoDescription = strings.TrimSpace(*patch.SEODescription)
	}
	if patch.CanonicalURL != nil {
		tmpInput.canonicalURL = strings.TrimSpace(*patch.CanonicalURL)
		if !validCanonicalURL(tmpInput.canonicalURL) {
			return "", "", 0, fmt.Errorf("%w: canonical URL must be an absolute http(s) URL or start with /", ErrValidation)
		}
	}
	if patch.FeaturedMediaID != nil {
		tmpInput.featuredMediaID = strings.TrimSpace(*patch.FeaturedMediaID)
	}
	if patch.SocialMediaID != nil {
		tmpInput.socialMediaID = strings.TrimSpace(*patch.SocialMediaID)
	}
	if patch.RobotsIndex != nil {
		tmpInput.robotsIndex = patch.RobotsIndex
	}
	if patch.RobotsFollow != nil {
		tmpInput.robotsFollow = patch.RobotsFollow
	}
	if patch.SchemaMode != nil {
		tmpInput.schemaMode = strings.TrimSpace(*patch.SchemaMode)
	}
	if patch.FieldsSet {
		tmpInput.fields = patch.Fields
	}
	if patch.TaxonomySet {
		tmpInput.taxonomyValues = patch.TaxonomyValues
	}
	// Validate document decode before sanitization if doc provided
	if doc != nil {
		// Need to ensure document is valid; if HasContent false we'll replace
		if err := s.blocks.ValidateDocument(doc); err != nil {
			// Still allow empty for HasContent false? We'll handle after sanitization
			if definition.Capabilities.HasContent {
				return "", "", 0, fmt.Errorf("%w: invalid document: %v", ErrValidation, err)
			}
		}
		if err := layouts.ValidateEntryDocument(s.blocks, doc); err != nil {
			if definition.Capabilities.HasContent {
				return "", "", 0, fmt.Errorf("%w: invalid document: %v", ErrValidation, err)
			}
		}
	}
	// Apply capability sanitization (includes HasContent empty doc)
	doc, err = applyCapabilitySanitization(definition, doc, &tmpInput)
	if err != nil {
		return "", "", 0, err
	}
	// If HasContent false, doc is empty; else marshal provided doc
	docBytes, err := json.Marshal(doc)
	if err != nil {
		return "", "", 0, fmt.Errorf("encode document: %w", err)
	}
	tmpInput.documentJSON = string(docBytes)

	// Default visibility
	if tmpInput.visibility == "" {
		tmpInput.visibility = "public"
	}
	if tmpInput.visibility != "public" && tmpInput.visibility != "private" && tmpInput.visibility != "password" {
		return "", "", 0, fmt.Errorf("%w: invalid visibility", ErrValidation)
	}
	if tmpInput.reviewState == "" {
		tmpInput.reviewState = "draft"
	}
	if tmpInput.reviewState != "draft" && tmpInput.reviewState != "pending" {
		return "", "", 0, fmt.Errorf("%w: invalid review state", ErrValidation)
	}
	if tmpInput.visibility == "password" && strings.TrimSpace(tmpInput.password) == "" {
		return "", "", 0, fmt.Errorf("%w: password is required for password protected visibility", ErrValidation)
	}
	if !definition.Capabilities.SupportsSticky && tmpInput.sticky {
		return "", "", 0, errors.New("this content type does not support sticky")
	}

	// Transaction
	if s.db == nil {
		return "", "", 0, errors.New("database not configured")
	}
	now := time.Now().Unix()
	if entryID == "" {
		gen, err := randomID()
		if err != nil {
			return "", "", 0, err
		}
		entryID = gen
	}
	revisionID, err := randomID()
	if err != nil {
		return "", "", 0, err
	}
	// Resolve author (Entry author)
	authorID, err := s.resolveAuthorID(ctx, s.queries, actor)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	// Validate taxonomy, fields, hierarchy etc inside Tx
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", 0, fmt.Errorf("begin entry create: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return "", "", 0, fmt.Errorf("load site settings: %w", err)
	}
	isPostsPage := false // new entry cannot be posts page yet
	if err := validateHierarchyInput(ctx, qtx, contentType, entryID, tmpInput.parentEntryID, tmpInput.menuOrder, isPostsPage, settings.PostsPageEntryID); err != nil {
		return "", "", 0, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	termIDs, err := taxonomyTermIDsForInput(ctx, qtx, s.db, contentType, tmpInput.taxonomyValues)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	fields, err := content.ValidateFields(definition, tmpInput.fields, content.FieldValidationOptions{
		MediaExists: func(id string) bool {
			_, err := qtx.GetMedia(ctx, id)
			return err == nil
		},
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid custom fields: %w", err)
	}
	fieldsJSON, err := content.EncodeFieldSnapshot(fields)
	if err != nil {
		return "", "", 0, fmt.Errorf("encode custom fields: %w", err)
	}
	// For route-less, allocate unique slug
	if !definition.Routing.Single {
		base := slugify(tmpInput.title)
		allocated, allocErr := allocateUniqueSlug(ctx, qtx, contentType, base, entryID)
		if allocErr != nil {
			return "", "", 0, allocErr
		}
		tmpInput.slug = allocated
		slugVal = allocated
	}
	// Layout validation
	schemaMode := normalizeSchemaMode(tmpInput.schemaMode)
	var layoutTemplateID sql.NullString
	if strings.TrimSpace(tmpInput.layoutTemplateID) != "" {
		tmplID := strings.TrimSpace(tmpInput.layoutTemplateID)
		tmpl, err := qtx.GetLayoutTemplate(ctx, tmplID)
		if err != nil {
			return "", "", 0, errors.New("selected layout template not found")
		}
		if tmpl.ContentTypeID != contentType {
			return "", "", 0, fmt.Errorf("This template belongs to %s and cannot be used by a %s", tmpl.ContentTypeID, contentType)
		}
		if tmpl.Kind != "single" {
			return "", "", 0, errors.New("Archive templates cannot be assigned to entries")
		}
		if !tmpl.PublishedRevisionID.Valid {
			return "", "", 0, errors.New("The selected layout template has not been published yet.")
		}
		layoutTemplateID = sql.NullString{String: tmplID, Valid: true}
	}
	// Visibility etc
	visibility := tmpInput.visibility
	if visibility == "" {
		visibility = "public"
	}
	reviewState := tmpInput.reviewState
	if reviewState == "" {
		reviewState = "draft"
	}
	var passwordHash sql.NullString
	stickyVal := int64(0)
	if tmpInput.sticky {
		stickyVal = 1
	}
	commentsEnabled := int64(0)
	if content.DefinitionFor(contentType).Capabilities.SupportsComments && tmpInput.commentsEnabled {
		commentsEnabled = 1
	}
	if visibility == "password" {
		if strings.TrimSpace(tmpInput.password) != "" {
			hash, err := publishing.HashPassword(strings.TrimSpace(tmpInput.password))
			if err != nil {
				return "", "", 0, fmt.Errorf("hash password: %w", err)
			}
			passwordHash = sql.NullString{String: hash, Valid: true}
		} else {
			return "", "", 0, errors.New("password is required for password protected visibility")
		}
	}
	// Null handling for optional
	var excerpt, seoTitle, seoDesc, canonical, featured, social sql.NullString
	if tmpInput.excerpt != "" {
		excerpt = sql.NullString{String: tmpInput.excerpt, Valid: true}
	}
	if tmpInput.seoTitle != "" {
		seoTitle = sql.NullString{String: tmpInput.seoTitle, Valid: true}
	}
	if tmpInput.seoDescription != "" {
		seoDesc = sql.NullString{String: tmpInput.seoDescription, Valid: true}
	}
	if tmpInput.canonicalURL != "" {
		canonical = sql.NullString{String: tmpInput.canonicalURL, Valid: true}
	}
	if tmpInput.featuredMediaID != "" {
		featured = sql.NullString{String: tmpInput.featuredMediaID, Valid: true}
	}
	if tmpInput.socialMediaID != "" {
		social = sql.NullString{String: tmpInput.socialMediaID, Valid: true}
	}
	var robotsIndex, robotsFollow sql.NullInt64
	if tmpInput.robotsIndex != nil {
		v := int64(0)
		if *tmpInput.robotsIndex {
			v = 1
		}
		robotsIndex = sql.NullInt64{Int64: v, Valid: true}
	}
	if tmpInput.robotsFollow != nil {
		v := int64(0)
		if *tmpInput.robotsFollow {
			v = 1
		}
		robotsFollow = sql.NullInt64{Int64: v, Valid: true}
	}
	// Create Entry with retry for slug uniqueness (route-less)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		err = qtx.CreateEntry(ctx, db.CreateEntryParams{
			ID: entryID, ContentTypeID: contentType, Slug: tmpInput.slug,
			Status: "active", AuthorID: nullableString(authorID.String), CreatedAt: now, UpdatedAt: now,
		})
		if err == nil {
			break
		}
		if !definition.Routing.Single && isUniqueConstraintError(err) {
			nextBase := slugify(tmpInput.title)
			candidate := fmt.Sprintf("%s-%d", nextBase, attempt+3)
			if allocated, allocErr := allocateUniqueSlug(ctx, qtx, contentType, nextBase, entryID); allocErr == nil {
				candidate = allocated
			}
			tmpInput.slug = candidate
			lastErr = err
			continue
		}
		break
	}
	if err != nil {
		if lastErr != nil && isUniqueConstraintError(err) {
			return "", "", 0, fmt.Errorf("this slug is already in use")
		}
		return "", "", 0, fmt.Errorf("save entry: %w", err)
	}
	// Create revision
	createdByKind := string(actor.Kind)
	if createdByKind == "" {
		createdByKind = string(authz.ActorUser)
	}
	if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revisionID, EntryID: entryID, RevisionNumber: 1, Slug: tmpInput.slug, Title: tmpInput.title,
		Excerpt: excerpt, SeoTitle: seoTitle, SeoDescription: seoDesc, CanonicalUrl: canonical,
		FeaturedMediaID: featured, SocialMediaID: social,
		SeoRobotsIndex: robotsIndex, SeoRobotsFollow: robotsFollow, SchemaMode: schemaMode,
		LayoutTemplateID: layoutTemplateID, ParentEntryID: nullableString(tmpInput.parentEntryID), MenuOrder: tmpInput.menuOrder,
		DocumentJson: tmpInput.documentJSON, FieldsJson: fieldsJSON, CreatedBy: nullableString(actorIDForRevision(actor)), CreatedByKind: createdByKind, CreatedAt: now,
		Visibility: visibility, PasswordHash: passwordHash, Sticky: stickyVal, ReviewState: reviewState, CommentsEnabled: commentsEnabled,
	}); err != nil {
		return "", "", 0, fmt.Errorf("create entry revision: %w", err)
	}
	// Terms
	for _, tid := range termIDs {
		tid = strings.TrimSpace(tid)
		if tid == "" {
			continue
		}
		if _, err := qtx.GetTerm(ctx, tid); err != nil {
			return "", "", 0, fmt.Errorf("invalid term %s: %w", tid, err)
		}
		if err := qtx.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: revisionID, TermID: tid}); err != nil {
			// If we used wrong revisionID, fallback to entryID? But revisionID is correct
			return "", "", 0, fmt.Errorf("set term %s: %w", tid, err)
		}
	}
	// Audit in same tx
	if s.audit != nil {
		_ = s.audit.Record(ctx, qtx, actor, transportForActor(actor), audit.Event{
			Action: "entry.create", ResourceType: "entry", ResourceID: entryID, RevisionID: revisionID,
			Metadata: map[string]any{"content_type": contentType, "title": tmpInput.title},
		})
	}
	if err := tx.Commit(); err != nil {
		return "", "", 0, fmt.Errorf("commit entry create: %w", err)
	}
	// Post-commit: search? media variant?
	if s.media != nil {
		for _, mid := range []string{tmpInput.socialMediaID, tmpInput.featuredMediaID} {
			if mid == "" {
				continue
			}
			if _, err := s.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: mid, Kind: "social"}); err != nil {
				_ = s.media.GenerateSocialVariant(ctx, mid, media.FocalPoint{X: 0.5, Y: 0.5})
			}
		}
	}
	return entryID, revisionID, 1, nil
}

func valueOrEmpty(ptr *string, set bool) string {
	if !set || ptr == nil {
		return ""
	}
	return strings.TrimSpace(*ptr)
}
func valueOrInt64(ptr *int64, def int64) int64 {
	if ptr == nil {
		return def
	}
	return *ptr
}
func boolOrFalse(ptr *bool) bool {
	if ptr == nil {
		return false
	}
	return *ptr
}

func actorIDForRevision(actor authz.Actor) string {
	if actor.Kind == authz.ActorAgent {
		return actor.AgentID
	}
	return actor.ID
}

func transportForActor(actor authz.Actor) string {
	switch actor.Kind {
	case authz.ActorAgent:
		return "mcp"
	case authz.ActorUser:
		return "admin"
	default:
		return "system"
	}
}

func loadGrantsForActor(ctx context.Context, s *Service, actor authz.Actor) []authz.AgentGrant {
	if actor.Kind != authz.ActorAgent {
		return nil
	}
	// Load grants from DB via queries
	rows, err := s.queries.ListAgentGrants(ctx, actor.AgentID)
	if err != nil {
		return nil
	}
	out := make([]authz.AgentGrant, 0, len(rows))
	for _, r := range rows {
		out = append(out, authz.AgentGrant{Permission: r.Permission, Scope: r.Scope})
	}
	return out
}
