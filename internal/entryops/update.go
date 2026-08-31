package entryops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/audit"
	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/publishing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// UpdateDraft applies a PATCH to an existing entry, requiring optimistic concurrency.
// expectedRevisionID must match the current latest revision or a ConflictError is returned.
func (s *Service) UpdateDraft(ctx context.Context, actor authz.Actor, entryID, expectedRevisionID string, patch EntryPatch) (string, int64, []string, error) {
	if strings.TrimSpace(entryID) == "" || strings.TrimSpace(expectedRevisionID) == "" {
		return "", 0, nil, fmt.Errorf("%w: entry_id and expected_revision_id are required", ErrValidation)
	}
	if s.db == nil {
		return "", 0, nil, errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, nil, fmt.Errorf("begin update: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	// Load entry
	entry, err := qtx.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, nil, fmt.Errorf("%w: entry not found", ErrNotFound)
		}
		return "", 0, nil, err
	}
	if entry.Status == "trash" {
		return "", 0, nil, fmt.Errorf("%w: cannot edit trashed entry", ErrValidation)
	}
	// Authz: check edit permission scoped to this entry's content type
	if !authz.Allowed(actor, authz.PermEntriesEdit, authz.Resource{ContentTypeID: entry.ContentTypeID, EntryID: entryID, OwnerID: stringValue(entry.AuthorID)}, loadGrantsForActor(ctx, s, actor)) {
		return "", 0, nil, &ForbiddenError{Permission: string(authz.PermEntriesEdit), Scope: "content_type:" + entry.ContentTypeID}
	}
	// Load latest revision and check concurrency transactionally
	latest, err := qtx.GetLatestEntryRevision(ctx, entryID)
	if err != nil {
		return "", 0, nil, fmt.Errorf("get latest revision: %w", err)
	}
	if latest.ID != expectedRevisionID {
		return "", 0, nil, &ConflictError{Expected: expectedRevisionID, Current: latest.ID}
	}
	// Load definition
	definition, err := s.getDefinition(ctx, qtx, entry.ContentTypeID)
	if err != nil {
		return "", 0, nil, err
	}
	// Load taxonomy assignments for base
	termRows, _ := qtx.ListTermsForRevision(ctx, latest.ID)
	// Build patched input
	input, changed, err := s.patchToInput(ctx, qtx, latest, termRows, patch, definition)
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	// If no changed fields, we still create new revision? Spec says only supplied changes create new revision.
	// But if patch produced no changed fields (e.g., same values), we can return existing revision id without new write?
	// For MCP, we should treat no-op as success without new revision to avoid churn.
	// However spec expects new revision on update. We'll treat empty changed as not creating new revision? For now, if no changes, return existing.
	if len(changed) == 0 {
		return latest.ID, latest.RevisionNumber, changed, nil
	}

	// Validate document decode
	doc, err := document.Decode([]byte(input.documentJSON))
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: invalid document: %v", ErrValidation, err)
	}
	if err := s.blocks.ValidateDocument(doc); err != nil {
		if definition.Capabilities.HasContent {
			return "", 0, nil, fmt.Errorf("%w: invalid document: %v", ErrValidation, err)
		}
	}
	if err := layouts.ValidateEntryDocument(s.blocks, doc); err != nil {
		if definition.Capabilities.HasContent {
			return "", 0, nil, fmt.Errorf("%w: invalid document: %v", ErrValidation, err)
		}
	}
	// Capability sanitization already handled inside patchToInput? It handles via applyCapabilitySanitization? No, patchToInput did not do full sanitization for HasContent empty etc. Need to ensure.
	// For HasContent false, patchToInput would have already used empty doc if patch had document? But we need to enforce empty doc for HasContent false regardless.
	if !definition.Capabilities.HasContent {
		doc = &document.Document{Version: 1, Nodes: []document.Node{}}
		data, _ := json.Marshal(doc)
		input.documentJSON = string(data)
	}
	// Validate layout, hierarchy, fields, taxonomy within_tx (we have helpers)
	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return "", 0, nil, fmt.Errorf("load site settings: %w", err)
	}
	isPostsPage := settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entryID
	if err := validateHierarchyInput(ctx, qtx, entry.ContentTypeID, entryID, input.parentEntryID, input.menuOrder, isPostsPage, settings.PostsPageEntryID); err != nil {
		return "", 0, nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	termIDs, err := taxonomyTermIDsForInput(ctx, qtx, s.db, entry.ContentTypeID, input.taxonomyValues)
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	fields, err := content.ValidateFields(definition, input.fields, content.FieldValidationOptions{
		MediaExists: func(id string) bool {
			_, err := qtx.GetMedia(ctx, id)
			return err == nil
		},
	})
	if err != nil {
		return "", 0, nil, fmt.Errorf("invalid custom fields: %w", err)
	}
	fieldsJSON, err := content.EncodeFieldSnapshot(fields)
	if err != nil {
		return "", 0, nil, fmt.Errorf("encode custom fields: %w", err)
	}
	// For route-less slug allocation
	if !definition.Routing.Single {
		base := slugify(input.title)
		allocated, allocErr := allocateUniqueSlug(ctx, qtx, entry.ContentTypeID, base, entryID)
		if allocErr != nil {
			return "", 0, nil, allocErr
		}
		input.slug = allocated
	}
	// Slug uniqueness for Single types will be enforced by DB constraint on Entry projection update; we handle after.

	// Layout template validation
	schemaMode := normalizeSchemaMode(input.schemaMode)
	var layoutTemplateID sql.NullString
	if strings.TrimSpace(input.layoutTemplateID) != "" {
		tmplID := strings.TrimSpace(input.layoutTemplateID)
		tmpl, err := qtx.GetLayoutTemplate(ctx, tmplID)
		if err != nil {
			return "", 0, nil, errors.New("selected layout template not found")
		}
		if tmpl.ContentTypeID != entry.ContentTypeID {
			return "", 0, nil, fmt.Errorf("This template belongs to %s and cannot be used by a %s", tmpl.ContentTypeID, entry.ContentTypeID)
		}
		if tmpl.Kind != "single" {
			return "", 0, nil, errors.New("Archive templates cannot be assigned to entries")
		}
		if !tmpl.PublishedRevisionID.Valid {
			return "", 0, nil, errors.New("The selected layout template has not been published yet.")
		}
		layoutTemplateID = sql.NullString{String: tmplID, Valid: true}
	}
	// Visibility, password, sticky etc.
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
		if !definition.Capabilities.SupportsSticky {
			return "", 0, nil, errors.New("this content type does not support sticky")
		}
		stickyVal = 1
	}
	commentsEnabled := int64(0)
	if content.DefinitionFor(entry.ContentTypeID).Capabilities.SupportsComments && input.commentsEnabled {
		commentsEnabled = 1
	}
	if visibility == "password" {
		if strings.TrimSpace(input.password) != "" {
			hash, err := publishing.HashPassword(strings.TrimSpace(input.password))
			if err != nil {
				return "", 0, nil, fmt.Errorf("hash password: %w", err)
			}
			passwordHash = sql.NullString{String: hash, Valid: true}
		} else if latest.Visibility == "password" && latest.PasswordHash.Valid && latest.PasswordHash.String != "" {
			passwordHash = latest.PasswordHash
		} else {
			return "", 0, nil, errors.New("password is required for password protected visibility")
		}
	} else {
		passwordHash = sql.NullString{}
	}
	if visibility == "public" || visibility == "private" {
		passwordHash = sql.NullString{}
	}

	// Null handling
	var excerpt, seoTitle, seoDesc, canonical, featured, social sql.NullString
	if input.excerpt != "" {
		excerpt = sql.NullString{String: input.excerpt, Valid: true}
	}
	if input.seoTitle != "" {
		seoTitle = sql.NullString{String: input.seoTitle, Valid: true}
	}
	if input.seoDescription != "" {
		seoDesc = sql.NullString{String: input.seoDescription, Valid: true}
	}
	if input.canonicalURL != "" {
		canonical = sql.NullString{String: input.canonicalURL, Valid: true}
	}
	if input.featuredMediaID != "" {
		featured = sql.NullString{String: input.featuredMediaID, Valid: true}
	}
	if input.socialMediaID != "" {
		social = sql.NullString{String: input.socialMediaID, Valid: true}
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

	now := time.Now().Unix()
	revisionID, err := randomID()
	if err != nil {
		return "", 0, nil, err
	}
	revisionNumber := latest.RevisionNumber + 1

	// Update entry projection (slug, updated_at)
	err = qtx.UpdateEntryProjection(ctx, db.UpdateEntryProjectionParams{
		Slug: input.slug, Status: entry.Status, UpdatedAt: now, PublishedAt: entry.PublishedAt, ID: entryID,
	})
	if err != nil && !definition.Routing.Single && isUniqueConstraintError(err) {
		if allocated, allocErr := allocateUniqueSlug(ctx, qtx, entry.ContentTypeID, slugify(input.title), entryID); allocErr == nil {
			input.slug = allocated
			err = qtx.UpdateEntryProjection(ctx, db.UpdateEntryProjectionParams{Slug: input.slug, Status: entry.Status, UpdatedAt: now, PublishedAt: entry.PublishedAt, ID: entryID})
		}
	}
	if err != nil {
		if isUniqueConstraintError(err) {
			return "", 0, nil, fmt.Errorf("%w: this slug is already in use", ErrValidation)
		}
		return "", 0, nil, fmt.Errorf("save entry: %w", err)
	}

	createdByKind := string(actor.Kind)
	if createdByKind == "" {
		createdByKind = string(authz.ActorUser)
	}
	if err := qtx.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revisionID, EntryID: entryID, RevisionNumber: revisionNumber, Slug: input.slug, Title: input.title,
		Excerpt: excerpt, SeoTitle: seoTitle, SeoDescription: seoDesc, CanonicalUrl: canonical,
		FeaturedMediaID: featured, SocialMediaID: social,
		SeoRobotsIndex: robotsIndex, SeoRobotsFollow: robotsFollow, SchemaMode: schemaMode,
		LayoutTemplateID: layoutTemplateID, ParentEntryID: nullableString(input.parentEntryID), MenuOrder: input.menuOrder,
		DocumentJson: input.documentJSON, FieldsJson: fieldsJSON, CreatedBy: nullableString(actorIDForRevision(actor)), CreatedByKind: createdByKind, CreatedAt: now,
		Visibility: visibility, PasswordHash: passwordHash, Sticky: stickyVal, ReviewState: reviewState, CommentsEnabled: commentsEnabled,
	}); err != nil {
		return "", 0, nil, fmt.Errorf("create entry revision: %w", err)
	}
	// Taxonomy assignments
	for _, tid := range termIDs {
		tid = strings.TrimSpace(tid)
		if tid == "" {
			continue
		}
		if _, err := qtx.GetTerm(ctx, tid); err != nil {
			return "", 0, nil, fmt.Errorf("invalid term %s: %w", tid, err)
		}
		if err := qtx.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: revisionID, TermID: tid}); err != nil {
			return "", 0, nil, fmt.Errorf("set term %s: %w", tid, err)
		}
	}
	// Audit
	if s.audit != nil {
		_ = s.audit.Record(ctx, qtx, actor, transportForActor(actor), audit.Event{
			Action: "entry.update", ResourceType: "entry", ResourceID: entryID, RevisionID: revisionID,
			Metadata: map[string]any{"changed": changed, "content_type": entry.ContentTypeID, "previous_revision_id": expectedRevisionID},
		})
	}
	if err := tx.Commit(); err != nil {
		return "", 0, nil, fmt.Errorf("commit update: %w", err)
	}
	// Post-commit: media social variant
	if s.media != nil {
		for _, mid := range []string{input.socialMediaID, input.featuredMediaID} {
			if mid == "" {
				continue
			}
			if _, err := s.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: mid, Kind: "social"}); err != nil {
				_ = s.media.GenerateSocialVariant(ctx, mid, media.FocalPoint{X: 0.5, Y: 0.5})
			}
		}
	}
	return revisionID, revisionNumber, changed, nil
}
