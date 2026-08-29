package creator

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

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/siteparts"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

var ErrCompleted = errors.New("site setup is already complete")

type Service struct {
	database *sql.DB
	queries  *db.Queries
	blocks   *blocks.Registry
	media    *media.Service
	themes   *themes.Runtime
	runtime  *runtimehub.Runtime
	search   *search.Service
}

func NewService(database *sql.DB, queries *db.Queries, registry *blocks.Registry, mediaService *media.Service, themeRuntime *themes.Runtime, runtime *runtimehub.Runtime, searchService *search.Service) *Service {
	return &Service{database: database, queries: queries, blocks: registry, media: mediaService, themes: themeRuntime, runtime: runtime, search: searchService}
}

func (s *Service) Preview(input Input) (Plan, error) {
	input.SiteTitle = strings.TrimSpace(input.SiteTitle)
	input.Tagline = strings.TrimSpace(input.Tagline)
	if input.SiteTitle == "" || len(input.SiteTitle) > 200 {
		return Plan{}, errors.New("site name is required and must be at most 200 characters")
	}
	preset, ok := presetByID(input.PresetID)
	if !ok {
		return Plan{}, errors.New("choose a starter site")
	}
	if input.PaletteID != "" && !IsValidPalette(input.PaletteID) {
		return Plan{}, errors.New("choose a valid color palette")
	}
	if input.HeaderStyleID != "" && !IsValidHeader(input.HeaderStyleID) {
		return Plan{}, errors.New("choose a valid header style")
	}
	if input.FooterStyleID != "" && !IsValidFooter(input.FooterStyleID) {
		return Plan{}, errors.New("choose a valid footer style")
	}
	if input.PaletteID == "" {
		input.PaletteID = DefaultPaletteForPreset(preset.ID)
	}
	if input.HeaderStyleID == "" {
		input.HeaderStyleID = DefaultHeaderForPreset(preset.ID)
	}
	if input.FooterStyleID == "" {
		input.FooterStyleID = DefaultFooterForPreset(preset.ID)
	}
	return Plan{Input: input, Preset: preset}, nil
}

func (s *Service) Skip(ctx context.Context) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	completed, err := qtx.GetOnboardingCompleted(ctx)
	if err != nil {
		return err
	}
	if completed != 0 {
		return ErrCompleted
	}
	if err := qtx.SetOnboardingCompleted(ctx, db.SetOnboardingCompletedParams{OnboardingCompleted: 1, UpdatedAt: time.Now().Unix()}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Create(ctx context.Context, plan Plan, authorID string) (result Result, err error) {
	validated, err := s.Preview(plan.Input)
	if err != nil {
		return Result{}, err
	}
	plan = validated
	spec := specForPlan(plan)
	artifacts, err := s.buildArtifacts(plan, spec)
	if err != nil {
		return Result{}, err
	}
	createdMedia, err := createStarterMedia(ctx, s.media, authorID, plan.Input.PaletteID, spec.images)
	if err != nil {
		cleanupStarterMedia(context.Background(), s.media, createdMedia)
		return Result{}, fmt.Errorf("create starter media: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			cleanupStarterMedia(context.Background(), s.media, createdMedia)
		}
	}()

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	completed, err := qtx.GetOnboardingCompleted(ctx)
	if err != nil {
		return Result{}, err
	}
	if completed != 0 {
		return Result{}, ErrCompleted
	}
	if count, err := qtx.CountEntries(ctx); err != nil {
		return Result{}, err
	} else if count != 0 {
		return Result{}, errors.New("starter sites can only be created on an empty site")
	}

	if spec.contentType != nil {
		if err := content.NewCatalog(qtx).CreateContentType(ctx, *spec.contentType); err != nil {
			return Result{}, fmt.Errorf("create content type: %w", err)
		}
	}
	if artifacts.archiveContentType != "" {
		if err := qtx.CreateRoute(ctx, db.CreateRouteParams{ID: newID(), Path: spec.archivePath, RouteType: "archive", ContentTypeID: nullable(artifacts.archiveContentType), CreatedAt: artifacts.now, UpdatedAt: artifacts.now}); err != nil {
			return Result{}, fmt.Errorf("create archive route: %w", err)
		}
	}
	for _, tmpl := range artifacts.templates {
		if err := createTemplate(ctx, qtx, tmpl, authorID, artifacts.now); err != nil {
			return Result{}, err
		}
	}
	if err := qtx.SetContentTypeDefaultLayoutTemplate(ctx, db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: nullable(artifacts.pageTemplateID), UpdatedAt: artifacts.now, ID: "page"}); err != nil {
		return Result{}, err
	}
	if artifacts.dynamicTemplateID != "" {
		if err := qtx.SetContentTypeDefaultLayoutTemplate(ctx, db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: nullable(artifacts.dynamicTemplateID), UpdatedAt: artifacts.now, ID: artifacts.dynamicContentType}); err != nil {
			return Result{}, err
		}
	}
	if artifacts.archiveTemplateID != "" {
		if err := qtx.SetContentTypeDefaultArchiveTemplate(ctx, db.SetContentTypeDefaultArchiveTemplateParams{DefaultArchiveTemplateID: nullable(artifacts.archiveTemplateID), UpdatedAt: artifacts.now, ID: artifacts.dynamicContentType}); err != nil {
			return Result{}, err
		}
	}
	if artifacts.formID != "" {
		if err := qtx.CreateForm(ctx, db.CreateFormParams{ID: artifacts.formID, Name: spec.form.Name, SchemaVersion: forms.SchemaVersion, DefinitionJson: artifacts.formJSON, Active: 1, CreatedAt: artifacts.now, UpdatedAt: artifacts.now}); err != nil {
			return Result{}, fmt.Errorf("create form: %w", err)
		}
	}
	for _, entry := range artifacts.entries {
		if entry.id == artifacts.homepageID {
			if err := qtx.CreateEntry(ctx, db.CreateEntryParams{ID: entry.id, ContentTypeID: entry.contentType, Slug: entry.slug, Status: "active", AuthorID: nullable(authorID), CreatedAt: artifacts.now, UpdatedAt: artifacts.now}); err != nil {
				return Result{}, fmt.Errorf("create homepage: %w", err)
			}
			break
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE site_settings SET site_title=?, site_tagline=?, homepage_mode='page', homepage_entry_id=?, posts_page_entry_id=NULL, posts_base_path='/blog', site_icon_media_id=?, updated_at=? WHERE id=1`, plan.Input.SiteTitle, plan.Input.Tagline, artifacts.homepageID, createdMedia.iconID, artifacts.now); err != nil {
		return Result{}, fmt.Errorf("update site settings: %w", err)
	}
	for index, entry := range artifacts.entries {
		featured := sql.NullString{}
		if len(createdMedia.imageIDs) > 0 && entry.contentType != "page" {
			featured = nullable(createdMedia.imageIDs[index%len(createdMedia.imageIDs)])
		}
		if err := createPublishedEntry(ctx, qtx, entry, authorID, featured, artifacts.now, entry.id == artifacts.homepageID); err != nil {
			return Result{}, err
		}
	}
	if err := createMenu(ctx, qtx, artifacts, artifacts.now); err != nil {
		return Result{}, err
	}
	for _, part := range artifacts.siteParts {
		if err := createSitePart(ctx, qtx, part, authorID, artifacts.now); err != nil {
			return Result{}, err
		}
	}
	styleJSON, err := json.Marshal(artifacts.styles)
	if err != nil {
		return Result{}, err
	}
	activeTheme := s.themes.Current()
	if err := qtx.UpsertThemeCustomization(ctx, db.UpsertThemeCustomizationParams{ThemeID: activeTheme.ThemeID, ThemeVersion: int64(activeTheme.Version), SettingsJson: string(styleJSON), CustomCss: ""}); err != nil {
		return Result{}, fmt.Errorf("save site styles: %w", err)
	}
	if err := qtx.SetOnboardingCompleted(ctx, db.SetOnboardingCompletedParams{OnboardingCompleted: 1, UpdatedAt: artifacts.now}); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit site creation: %w", err)
	}
	committed = true

	result = Result{HomepageID: artifacts.homepageID, Pages: artifacts.pageCount, Entries: len(artifacts.entries), Forms: boolCount(artifacts.formID != "")}
	if s.runtime != nil {
		if err := s.runtime.ReloadSite(ctx); err != nil {
			result.Warnings = append(result.Warnings, "Site settings could not be refreshed immediately.")
		}
		if err := s.runtime.ReloadNavigation(ctx); err != nil {
			result.Warnings = append(result.Warnings, "Navigation could not be refreshed immediately.")
		}
		if err := s.runtime.ReloadRoutes(ctx); err != nil {
			result.Warnings = append(result.Warnings, "Routes could not be refreshed immediately.")
		}
		if err := s.runtime.ReloadTheme(ctx); err != nil {
			result.Warnings = append(result.Warnings, "Site styles could not be refreshed immediately.")
		}
		s.runtime.InvalidateContent()
	}
	if s.search != nil {
		if _, err := s.search.Rebuild(context.Background()); err != nil {
			log.Printf("creator search rebuild: %v", err)
			result.Warnings = append(result.Warnings, "Site created, but Search index could not be rebuilt.")
		}
	}
	return result, nil
}

type templateArtifact struct {
	id, revisionID, name, contentType, kind string
	doc                                     *document.Document
}
type partArtifact struct {
	id, revisionID, location, name string
	doc                            *document.Document
}
type entryArtifact struct {
	id, revisionID, contentType, slug, title, excerpt, fields string
	doc                                                       *document.Document
	layoutID                                                  string
}
type creationArtifacts struct {
	now                                                                          int64
	homepageID, pageTemplateID, homepageTemplateID                               string
	dynamicContentType, dynamicTemplateID, archiveContentType, archiveTemplateID string
	formID, formJSON, menuID                                                     string
	templates                                                                    []templateArtifact
	siteParts                                                                    []partArtifact
	entries                                                                      []entryArtifact
	styles                                                                       map[string]any
	pageCount                                                                    int
}

func (s *Service) buildArtifacts(plan Plan, spec presetSpec) (creationArtifacts, error) {
	a := creationArtifacts{now: time.Now().Unix(), homepageID: newID(), pageTemplateID: newID(), homepageTemplateID: newID(), menuID: newID()}
	if spec.form != nil {
		a.formID = newID()
		def := forms.Definition{Fields: []forms.Field{{ID: newID(), Key: "name", Type: forms.FieldText, Label: "Name", Required: true}, {ID: newID(), Key: "email", Type: forms.FieldEmail, Label: "Email", Required: true}}, SubmitLabel: "Send message", SuccessMessage: "Thanks. Your message has been received."}
		if spec.form.Phone {
			def.Fields = append(def.Fields, forms.Field{ID: newID(), Key: "phone", Type: forms.FieldText, Label: "Phone"})
		}
		def.Fields = append(def.Fields, forms.Field{ID: newID(), Key: "message", Type: forms.FieldTextarea, Label: "Message", Required: true})
		if err := forms.ValidateDefinition(spec.form.Name, def); err != nil {
			return a, err
		}
		encoded, _ := json.Marshal(def)
		a.formJSON = string(encoded)
	}
	dynamicType := "post"
	if spec.contentType != nil {
		dynamicType = string(spec.contentType.ID)
	}
	a.dynamicContentType = dynamicType
	pageDoc := pageTemplate(newID())
	homeDoc := homepageTemplate(newID(), plan.Preset.ID, plan.Input.Tagline, landingFormID(plan.Preset.ID, a.formID))
	a.templates = append(a.templates, templateArtifact{id: a.pageTemplateID, revisionID: newID(), name: "Page", contentType: "page", kind: "single", doc: pageDoc}, templateArtifact{id: a.homepageTemplateID, revisionID: newID(), name: "Homepage", contentType: "page", kind: "single", doc: homeDoc})
	if spec.contentType == nil || spec.contentType.Config.Routing.Single {
		a.dynamicTemplateID = newID()
		single := singleTemplate(newID(), plan.Preset.ID)
		a.templates = append(a.templates, templateArtifact{id: a.dynamicTemplateID, revisionID: newID(), name: spec.preset.Name + " Single", contentType: dynamicType, kind: "single", doc: single})
	}
	if spec.archivePath != "" {
		a.archiveContentType = dynamicType
		a.archiveTemplateID = newID()
		archive := archiveTemplate(newID(), plan.Preset.ID)
		a.templates = append(a.templates, templateArtifact{id: a.archiveTemplateID, revisionID: newID(), name: spec.preset.Name + " Archive", contentType: dynamicType, kind: "archive", doc: archive})
	}
	for _, tmpl := range a.templates {
		def, err := content.NewCatalog(s.queries).GetDefinition(context.Background(), tmpl.contentType)
		if err != nil && spec.contentType != nil && tmpl.contentType == string(spec.contentType.ID) {
			def = content.ContentTypeDefinition{Capabilities: content.Capabilities{HasContent: spec.contentType.Config.Features.Content}}
		} else if err != nil {
			return a, err
		}
		hasContent := def.Capabilities.HasContent
		if err := layouts.ValidateTemplateDocument(s.blocks, tmpl.doc, tmpl.kind, &hasContent); err != nil {
			return a, fmt.Errorf("validate %s template: %w", tmpl.name, err)
		}
	}
	home := entryArtifact{id: a.homepageID, revisionID: newID(), contentType: "page", slug: "home", title: plan.Input.SiteTitle, excerpt: plan.Input.Tagline, fields: "{}", doc: emptyDocument(newID()), layoutID: a.homepageTemplateID}
	a.entries = append(a.entries, home)
	a.pageCount++
	for _, page := range spec.pages {
		formID := ""
		if page.Form {
			formID = a.formID
		}
		a.entries = append(a.entries, entryArtifact{id: newID(), revisionID: newID(), contentType: "page", slug: page.Slug, title: page.Title, excerpt: page.Body, fields: "{}", doc: bodyDocument(newID(), page.Body, formID)})
		a.pageCount++
	}
	for _, seed := range spec.seedEntries {
		fieldsJSON, err := s.validatedFields(dynamicType, spec.contentType, seed.Fields)
		if err != nil {
			return a, err
		}
		a.entries = append(a.entries, entryArtifact{id: newID(), revisionID: newID(), contentType: dynamicType, slug: seed.Slug, title: seed.Title, excerpt: seed.Excerpt, fields: fieldsJSON, doc: bodyDocument(newID(), seed.Body, "")})
	}
	for _, entry := range a.entries {
		if err := layouts.ValidateEntryDocument(s.blocks, entry.doc); err != nil {
			return a, fmt.Errorf("validate %s: %w", entry.title, err)
		}
	}
	for _, location := range []string{"header", "footer"} {
		var doc *document.Document
		if location == "header" {
			doc = sitePartDocumentForHeader(newID(), plan.Input.HeaderStyleID)
		} else {
			doc = sitePartDocumentForFooter(newID(), plan.Input.FooterStyleID)
		}
		part := partArtifact{id: newID(), revisionID: newID(), location: location, name: strings.ToUpper(location[:1]) + location[1:], doc: doc}
		if err := siteparts.ValidateSitePartDocument(s.blocks, part.doc); err != nil {
			return a, fmt.Errorf("validate %s: %w", location, err)
		}
		a.siteParts = append(a.siteParts, part)
	}
	settings := s.themes.Current().Settings
	a.styles = make(map[string]any, len(settings)+len(spec.styles))
	for key, value := range settings {
		a.styles[key] = value
	}
	for key, value := range spec.styles {
		a.styles[key] = value
	}
	validatedStyles, err := s.themes.ValidateSettings(a.styles)
	if err != nil {
		return a, fmt.Errorf("validate site styles: %w", err)
	}
	a.styles = validatedStyles
	return a, nil
}

func (s *Service) validatedFields(contentType string, input *content.ContentTypeInput, raw map[string]any) (string, error) {
	if input == nil {
		return "{}", nil
	}
	def := content.ContentTypeDefinition{ID: content.ContentTypeID(contentType), Fields: input.Config.Fields}
	values, err := content.ValidateFields(def, raw, content.FieldValidationOptions{})
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(values)
	return string(encoded), err
}

func createTemplate(ctx context.Context, q *db.Queries, tmpl templateArtifact, authorID string, now int64) error {
	encoded, _ := json.Marshal(tmpl.doc)
	if err := q.CreateLayoutTemplate(ctx, db.CreateLayoutTemplateParams{ID: tmpl.id, Name: tmpl.name, ContentTypeID: tmpl.contentType, Kind: tmpl.kind, PublishedRevisionID: sql.NullString{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("create template %s: %w", tmpl.name, err)
	}
	if err := q.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: tmpl.revisionID, TemplateID: tmpl.id, RevisionNumber: 1, DocumentJson: string(encoded), CreatedBy: nullable(authorID), CreatedAt: now}); err != nil {
		return err
	}
	return q.SetLayoutTemplatePublishedRevision(ctx, db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: nullable(tmpl.revisionID), UpdatedAt: now, ID: tmpl.id})
}

func createPublishedEntry(ctx context.Context, q *db.Queries, entry entryArtifact, authorID string, featured sql.NullString, now int64, precreated bool) error {
	encoded, _ := json.Marshal(entry.doc)
	if !precreated {
		if err := q.CreateEntry(ctx, db.CreateEntryParams{ID: entry.id, ContentTypeID: entry.contentType, Slug: entry.slug, Status: "active", AuthorID: nullable(authorID), CreatedAt: now, UpdatedAt: now}); err != nil {
			return fmt.Errorf("create %s: %w", entry.title, err)
		}
	}
	rev := db.CreateEntryRevisionParams{ID: entry.revisionID, EntryID: entry.id, RevisionNumber: 1, Slug: entry.slug, Title: entry.title, Excerpt: nullable(entry.excerpt), DocumentJson: string(encoded), SeoTitle: nullable(entry.title), SeoDescription: nullable(entry.excerpt), FeaturedMediaID: featured, SchemaMode: "", LayoutTemplateID: nullable(entry.layoutID), FieldsJson: entry.fields, CreatedBy: nullable(authorID), CreatedAt: now, Visibility: "public", ReviewState: "draft"}
	if err := q.CreateEntryRevision(ctx, rev); err != nil {
		return fmt.Errorf("create %s revision: %w", entry.title, err)
	}
	stored, err := q.GetEntryRevision(ctx, entry.revisionID)
	if err != nil {
		return err
	}
	model, err := q.GetEntry(ctx, entry.id)
	if err != nil {
		return err
	}
	return publishing.PublishWithQueries(ctx, q, model, stored, now)
}

func createMenu(ctx context.Context, q *db.Queries, a creationArtifacts, now int64) error {
	if err := q.CreateNavigationMenu(ctx, db.CreateNavigationMenuParams{ID: a.menuID, Name: "Primary Menu", Slug: "primary-menu", CreatedAt: now, UpdatedAt: now}); err != nil {
		return err
	}
	type menuItem struct {
		label, targetType, entryID, url string
	}
	items := make([]menuItem, 0, 4)
	for _, entry := range a.entries {
		if entry.id == a.homepageID {
			items = append(items, menuItem{label: "Home", targetType: "entry", entryID: entry.id})
			break
		}
	}
	if a.archiveContentType != "" {
		label := map[string]string{"post": "Blog", "project": "Work", "product": "Products", "service": "Services"}[a.archiveContentType]
		path := map[string]string{"post": "/blog", "project": "/work", "product": "/products", "service": "/services"}[a.archiveContentType]
		items = append(items, menuItem{label: label, targetType: "url", url: path})
	}
	for _, entry := range a.entries {
		if entry.contentType == "page" && entry.id != a.homepageID {
			items = append(items, menuItem{label: entry.title, targetType: "entry", entryID: entry.id})
		}
	}
	for position, item := range items {
		if err := q.CreateNavigationItem(ctx, db.CreateNavigationItemParams{ID: newID(), MenuID: a.menuID, Position: int64(position), Label: item.label, TargetType: item.targetType, EntryID: nullable(item.entryID), Url: nullable(item.url), CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
	}
	if err := q.UpsertNavigationLocation(ctx, db.UpsertNavigationLocationParams{Location: "primary", MenuID: a.menuID}); err != nil {
		return err
	}
	return q.UpsertNavigationLocation(ctx, db.UpsertNavigationLocationParams{Location: "footer", MenuID: a.menuID})
}

func createSitePart(ctx context.Context, q *db.Queries, part partArtifact, authorID string, now int64) error {
	encoded, _ := json.Marshal(part.doc)
	if err := q.CreateSitePart(ctx, db.CreateSitePartParams{ID: part.id, Name: part.name, PublishedRevisionID: sql.NullString{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		return err
	}
	if err := q.CreateSitePartRevision(ctx, db.CreateSitePartRevisionParams{ID: part.revisionID, SitePartID: part.id, RevisionNumber: 1, DocumentJson: string(encoded), CreatedBy: nullable(authorID), CreatedAt: now}); err != nil {
		return err
	}
	if err := q.SetSitePartPublishedRevision(ctx, db.SetSitePartPublishedRevisionParams{PublishedRevisionID: nullable(part.revisionID), UpdatedAt: now, ID: part.id}); err != nil {
		return err
	}
	return q.SetSitePartLocation(ctx, db.SetSitePartLocationParams{Location: part.location, SitePartID: nullable(part.id), UpdatedAt: now})
}

func landingFormID(id PresetID, formID string) string {
	if id == PresetLanding {
		return formID
	}
	return ""
}
func nullable(value string) sql.NullString { return sql.NullString{String: value, Valid: value != ""} }
func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
