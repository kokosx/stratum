package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/seo"
	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/structured"
)

const maxSettingsFieldLen = 200

type settingsForm struct {
	SiteTitle            string
	Tagline              string
	SiteURL              string
	Language             string
	Timezone             string
	SiteRepresents       string // "organization" or "person"
	HomepageEntryID      string
	PostsPageEntryID     string
	PostsBasePath        string
	PostsPerPage         int
	SiteIconMediaID      string
	SiteSocialMediaID    string
	TwitterSite          string
	IndexingEnabled      bool
	SitemapEnabled       bool
	RobotsMode           string
	RobotsCustom         string
	SpeculationEnabled   bool
	SpeculationMode      string
	SpeculationEagerness string
	TitleSeparator       string
}

type settingsData struct {
	Form               settingsForm
	Pages              []pageOption
	Errors             map[string]string
	Notice             string
	CSRFToken          string
	SiteURLWarning     bool
	SitemapPublicURL   string
	SiteIconPreview    string
	SiteIconWarning    string
	SiteSocialPreview  string
	SiteSocialWarning  string
	Errored            bool
	// DisableSave controls the Save Changes button. It is false on the full
	// page (and on validation errors) so the form still submits without JS; it
	// is true only in the post-save Datastar fragment, where JS re-enables it on
	// edit. This preserves progressive enhancement.
	DisableSave            bool
	LanguageOptions        []languageOption
	LanguageIsInOptions    bool
	TimezoneOptions        []timezoneOption
	TimezoneIsInOptions    bool
}

type languageOption struct {
	Value string
	Label string
}

type timezoneOption struct {
	Value string
	Label string
}

type pageOption struct {
	ID       string
	Title    string
	Path     string
	Disabled bool
}

// settings renders the Site Settings control panel.
func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	row, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	iconID, _ := h.queries.GetSiteIconMediaID(r.Context())
	socialID, _ := h.queries.GetSiteSocialMediaID(r.Context())
	langOpts := languageOptions()
	tzOpts := timezoneOptions()
	data := settingsData{
		Form:       formFromRow(row, iconID.String, socialID.String),
		Pages:      h.listPageOptions(r),
		Errors:     map[string]string{},
		CSRFToken:  token,
		DisableSave: false,
		LanguageOptions: langOpts,
		TimezoneOptions: tzOpts,
	}
	data.LanguageIsInOptions = isLanguageInOptions(data.Form.Language, langOpts)
	data.TimezoneIsInOptions = isTimezoneInOptions(data.Form.Timezone, tzOpts)
	if iconID.Valid {
		data.SiteIconPreview = "/media/" + iconID.String + "/180"
		if asset, aerr := h.media.Get(r.Context(), iconID.String); aerr == nil {
			if asset.Width > 0 && asset.Height > 0 {
				if asset.Width != asset.Height {
					data.SiteIconWarning = "The image is not square; it will be center-cropped for the favicon."
				} else if asset.Width < 512 {
					data.SiteIconWarning = "The image is small (" + strconv.Itoa(asset.Width) + "px); use at least 512×512 for crisp favicons."
				}
			}
		}
	}
	if socialID.Valid {
		// Preview prefers the dedicated 1200x630 social variant, falling back to thumb.
		data.SiteSocialPreview = "/media/" + socialID.String + "/social"
		if asset, aerr := h.media.Get(r.Context(), socialID.String); aerr == nil {
			if asset.Width > 0 && asset.Height > 0 {
				if asset.Width < 1200 || asset.Height < 630 {
					data.SiteSocialWarning = "The image is small (" + strconv.Itoa(asset.Width) + "×" + strconv.Itoa(asset.Height) + "); use at least 1200×630 for a crisp social preview."
				}
			}
		}
	}
	h.populateSettingsURLs(r, &data, row)
	layout := LayoutData{Title: "Settings", ActiveMenu: "settings", CSRFToken: token, Content: data}
	if err := h.settingsTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render settings: %v", err)
	}
}

// saveSettings persists the whole Site Settings form atomically and responds
// with a Datastar fragment (no full reload) when requested, or a redirect for
// the no-JS fallback.
func (h *Handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	current, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	form, fieldErrors := h.parseSettingsForm(r)
	langOpts := languageOptions()
	tzOpts := timezoneOptions()
	data := settingsData{Form: form, Pages: h.listPageOptions(r), Errors: fieldErrors, CSRFToken: r.FormValue("csrf_token"), LanguageOptions: langOpts, TimezoneOptions: tzOpts}
	data.LanguageIsInOptions = isLanguageInOptions(form.Language, langOpts)
	data.TimezoneIsInOptions = isTimezoneInOptions(form.Timezone, tzOpts)
	h.populateSettingsURLs(r, &data, current)

	if len(fieldErrors) > 0 {
		data.Errored = true
		if isDatastarRequest(r) {
			h.renderSettingsFragment(w, r, data, "")
			return
		}
		h.renderSettingsPage(w, r, data)
		return
	}

	if err := h.persistSettings(r.Context(), current, form); err != nil {
		log.Printf("save settings: %v", err)
		data.Errors["_"] = "Could not save settings."
		data.Errored = true
		if isDatastarRequest(r) {
			h.renderSettingsFragment(w, r, data, "")
			return
		}
		h.renderSettingsPage(w, r, data)
		return
	}

	// Site Icon is stored on its own column; update it (and regenerate favicon
	// variants) only when it actually changed.
	if err := h.applySiteIcon(r.Context(), form.SiteIconMediaID); err != nil {
		log.Printf("save site icon: %v", err)
		data.Errors["_"] = "Could not update the site icon."
		data.Errored = true
		if isDatastarRequest(r) {
			h.renderSettingsFragment(w, r, data, "")
			return
		}
		h.renderSettingsPage(w, r, data)
		return
	}
	if err := h.applySiteSocial(r.Context(), form.SiteSocialMediaID); err != nil {
		log.Printf("save site social image: %v", err)
		data.Errors["_"] = "Could not update the social image."
		data.Errored = true
		if isDatastarRequest(r) {
			h.renderSettingsFragment(w, r, data, "")
			return
		}
		h.renderSettingsPage(w, r, data)
		return
	}

	// The publish-facing runtime caches depend on site settings: reload the
	// snapshot and drop the full-page, sitemap and robots caches.
	if h.runtime != nil {
		if rerr := h.runtime.ReloadSite(r.Context()); rerr != nil {
			log.Printf("reload site runtime: %v", rerr)
		}
	}

	saved, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	socialID, _ := h.queries.GetSiteSocialMediaID(r.Context())
	iconID, _ := h.queries.GetSiteIconMediaID(r.Context())
	data.Form = formFromRow(saved, iconID.String, socialID.String)
	data.Errors = map[string]string{}
	data.LanguageIsInOptions = isLanguageInOptions(data.Form.Language, data.LanguageOptions)
	data.TimezoneIsInOptions = isTimezoneInOptions(data.Form.Timezone, data.TimezoneOptions)
	h.populateSettingsURLs(r, &data, saved)

	if isDatastarRequest(r) {
		h.renderSettingsFragment(w, r, data, "Settings saved.")
		return
	}
	h.setFlash(w, "Settings saved.")
	http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
}

// robotsPreview streams the exact robots.txt body that would be served, so the
// admin can preview managed or custom output before saving.
func (h *Handler) robotsPreview(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	current, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	mode := strings.TrimSpace(r.FormValue("robots_mode"))
	custom := r.FormValue("robots_custom")
	if mode == "" {
		mode = current.RobotsMode
	}
	body := site.BuildRobots(site.RobotsInput{
		Mode:            mode,
		IndexingEnabled: current.IndexingEnabled != 0,
		SitemapEnabled:  current.SitemapEnabled != 0,
		SiteURL:         current.SiteUrl,
		Custom:          custom,
	})
	if isDatastarRequest(r) {
		writeSSE(w, patchElementsEvent("inner", "#robots-preview", `<pre id="robots-preview" class="robots-preview">`+template.HTMLEscapeString(body)+`</pre>`))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(body))
}

// applySiteIcon updates the Site Icon column when it changed and regenerates the
// favicon variants from the chosen asset. It is atomic: the database is only
// updated after the required variants have been generated and verified, so a
// failure leaves the previous working icon intact.
func (h *Handler) applySiteIcon(ctx context.Context, newIcon string) error {
	current, err := h.queries.GetSiteIconMediaID(ctx)
	if err != nil {
		return err
	}
	changed := (newIcon != "") != current.Valid || (current.Valid && current.String != newIcon)
	if !changed {
		return nil
	}
	// Validate and generate before touching the site_settings row.
	if newIcon != "" {
		// Quick validation: asset must exist and not be SVG
		if asset, aerr := h.media.Get(ctx, newIcon); aerr == nil {
			if asset.MimeType == "image/svg+xml" {
				return fmt.Errorf("This image cannot be used as a Site Icon: SVG is not supported for favicons. Choose a square raster image (PNG, JPEG, WebP or GIF) of at least 512×512")
			}
			if asset.Width > 0 && asset.Height > 0 && asset.Width != asset.Height {
				// Allow but warn; generation will center-crop.
			}
		} else {
			return fmt.Errorf("Selected image is no longer available")
		}
		if err := h.media.GenerateFaviconVariants(ctx, newIcon); err != nil {
			// Preserve previous working icon; wrap with actionable message
			if errors.Is(err, media.ErrSVGUnsafe) {
				return fmt.Errorf("This image cannot be used as a Site Icon: SVG is not supported for favicons")
			}
			return fmt.Errorf("Site Icon generation failed: %w", err)
		}
	}
	null := sql.NullString{}
	if newIcon != "" {
		null = sql.NullString{String: newIcon, Valid: true}
	}
	if err := h.queries.UpdateSiteIconMediaID(ctx, null); err != nil {
		return err
	}
	return nil
}

// applySiteSocial updates the global social image. It regenerates the
// 1200x630 preview variant when a new image is chosen so the OG fallback is
// always available.
func (h *Handler) applySiteSocial(ctx context.Context, newSocial string) error {
	current, err := h.queries.GetSiteSocialMediaID(ctx)
	if err != nil {
		return err
	}
	changed := (newSocial != "") != current.Valid || (current.Valid && current.String != newSocial)
	if !changed {
		// Even when unchanged, ensure the social preview derivative exists for
		// older assets that were uploaded before the social pipeline existed.
		if newSocial != "" {
			if _, gerr := h.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: newSocial, Kind: "social"}); gerr != nil {
				_ = h.media.GenerateSocialVariant(ctx, newSocial, media.FocalPoint{X: 0.5, Y: 0.5})
			}
		}
		return nil
	}
	null := sql.NullString{}
	if newSocial != "" {
		null = sql.NullString{String: newSocial, Valid: true}
	}
	if err := h.queries.UpdateSiteSocialMediaID(ctx, null); err != nil {
		return err
	}
	if newSocial != "" {
		if err := h.media.GenerateSocialVariant(ctx, newSocial, media.FocalPoint{X: 0.5, Y: 0.5}); err != nil {
			// Non-fatal: the original can still be served if variant generation fails.
			_ = err
		}
	}
	return nil
}

// persistSettings applies the validated form inside a single transaction,
// updating the homepage route when it changed and writing the settings row.
// Either everything commits or nothing does.
func (h *Handler) persistSettings(ctx context.Context, current db.GetSiteSettingsRow, form settingsForm) error {
	if h.database == nil {
		return errors.New("admin database is not configured")
	}
	tx, err := h.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)
	oldHome := nullStringToStr(current.HomepageEntryID)
	newHome := strings.TrimSpace(form.HomepageEntryID)
	oldPosts := nullStringToStr(current.PostsPageEntryID)
	newPosts := strings.TrimSpace(form.PostsPageEntryID)
	oldBase := current.PostsBasePath
	newBase := strings.TrimSpace(form.PostsBasePath)
	if newBase == "" {
		newBase = seo.DefaultPostsBase
	}
	// Single source of truth: derive PostsBase from Posts Page slug when a
	// Posts Page is selected. This eliminates the two-value magic where
	// Posts Page path and PostsBase could diverge (/aktualnosci vs /blog).
	if newPosts != "" {
		if entry, err := qtx.GetEntry(ctx, newPosts); err == nil {
			derived := "/" + strings.Trim(entry.Slug, "/")
			derived = seo.NormalizePath(derived)
			if derived != "" && derived != "/" {
				newBase = derived
			}
		}
	}
	if err := seo.ValidatePostsBasePath(newBase); err != nil && newPosts == "" {
		return err
	}
	// When Posts Page is set, validate the derived path as well (it must not
	// collide with reserved prefixes; slug validation already prevents most).
	if newPosts != "" {
		if err := seo.ValidatePostsBasePath(newBase); err != nil {
			return err
		}
	}
	if form.PostsPerPage < 1 {
		form.PostsPerPage = int(current.PostsPerPage)
		if form.PostsPerPage < 1 {
			form.PostsPerPage = 10
		}
	}
	now := time.Now().Unix()
	if err := h.applyReadingRoutes(ctx, qtx, oldHome, newHome, oldPosts, newPosts, oldBase, newBase, now); err != nil {
		return err
	}
	// Posts page validation: ensure paginated block invariant (publish-time
	// invariant must also hold for existing published page being assigned).
	if newPosts != "" {
		if err := h.validatePostsPageAssignment(ctx, qtx, newPosts); err != nil {
			return err
		}
	}
	homepageMode := "latest_posts"
	if newHome != "" {
		homepageMode = "page"
	}
	// The column is CHECK-constrained; forms posted before the setting existed
	// carry an empty value which defaults to organization here.
	siteRepresents := form.SiteRepresents
	if siteRepresents != structured.RepresentsOrganization && siteRepresents != structured.RepresentsPerson {
		siteRepresents = structured.RepresentsOrganization
	}
	normalizedURL, urlErr := site.ValidateSiteURL(form.SiteURL)
	if urlErr != nil {
		return urlErr
	}
	err = qtx.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
		SiteTitle:            strings.TrimSpace(form.SiteTitle),
		SiteTagline:          strings.TrimSpace(form.Tagline),
		HomepageMode:         homepageMode,
		HomepageEntryID:      strToNullString(newHome),
		PostsPageEntryID:     strToNullString(newPosts),
		PostsPerPage:         int64(func() int { if form.PostsPerPage > 0 { return form.PostsPerPage }; return 10 }()),
		PostsBasePath:        newBase,
		Language:             strings.TrimSpace(form.Language),
		Timezone:             strings.TrimSpace(form.Timezone),
		ActiveTheme:          current.ActiveTheme,
		IndexingEnabled:      boolToInt(form.IndexingEnabled),
		SiteUrl:              normalizedURL,
		SitemapEnabled:       boolToInt(form.SitemapEnabled),
		RobotsMode:           form.RobotsMode,
		RobotsCustom:         form.RobotsCustom,
		SpeculationMode:      form.SpeculationMode,
		SpeculationEagerness: form.SpeculationEagerness,
		TitleSeparator:       form.TitleSeparator,
		SiteSocialMediaID:    strToNullString(form.SiteSocialMediaID),
		TwitterSite:          strings.TrimSpace(form.TwitterSite),
		SiteRepresents:       siteRepresents,
		UpdatedAt:            time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// applyHomepageRoute keeps the chosen homepage page served at "/" and restores
// the previous homepage page's "/slug" route. Each page keeps exactly one
// entry-type route; the page's own slug URL is preserved as a 301 redirect to
// "/" so setting a page as the index never breaks its existing public link.
func (h *Handler) applyHomepageRoute(ctx context.Context, queries *db.Queries, oldHome, newHome string, now int64) error {
	if oldHome != "" && oldHome != newHome {
		entry, err := queries.GetEntry(ctx, oldHome)
		if err == nil && entry.ContentTypeID == "page" {
			slugPath := "/" + entry.Slug
			// Free the slug path before the entry reclaims it: drop any redirect
			// left there by the homepage assignment (or an earlier slug change).
			if stale, rerr := queries.GetRouteByPath(ctx, slugPath); rerr == nil {
				if !stale.EntryID.Valid {
					if delErr := queries.DeleteRoute(ctx, stale.ID); delErr != nil {
						return delErr
					}
				}
			} else if !errors.Is(rerr, sql.ErrNoRows) {
				return rerr
			}
			route, rerr := queries.GetEntryRoute(ctx, strToNullString(oldHome))
			if rerr == nil && route.Path != slugPath {
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: route.ID, Path: slugPath, EntryID: strToNullString(oldHome),
					RouteType: "entry", UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
		}
	}
	if newHome == "" {
		route, err := queries.GetRouteByPath(ctx, "/")
		if err == nil && route.EntryID.Valid && route.EntryID.String == oldHome {
			if err := queries.DeleteRoute(ctx, route.ID); err != nil {
				return err
			}
		}
		return nil
	}
	entry, err := queries.GetEntry(ctx, newHome)
	if err != nil {
		return err
	}
	if entry.ContentTypeID != "page" {
		return errors.New("homepage must be a Page")
	}
	route, err := queries.GetEntryRoute(ctx, strToNullString(newHome))
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := randomID()
		if idErr != nil {
			return idErr
		}
		return queries.CreateRoute(ctx, db.CreateRouteParams{
			ID: id, Path: "/", EntryID: strToNullString(newHome), RouteType: "entry",
			CreatedAt: now, UpdatedAt: now,
		})
	}
	if err != nil {
		return err
	}
	oldPath := route.Path
	if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
		ID: route.ID, Path: "/", EntryID: strToNullString(newHome), RouteType: "entry", UpdatedAt: now,
	}); err != nil {
		return err
	}
	// The page's own URL must keep working once it becomes the index: redirect
	// the old path to "/" so existing links and search indexes survive.
	if oldPath != "" && oldPath != "/" {
		return h.upsertRedirectRoute(ctx, queries, oldPath, "/", now)
	}
	return nil
}

// applyReadingRoutes is the single transactional writer for homepage mode,
// homepage page, posts page and posts base path. It maintains:
// - / entry route (or redirect) for homepage
// - one archive route (type=archive) at the effective archive root, optionally
//   pointing at a shell Page (entry_id = Posts Page ID)
// - no duplicate single route for a Page that is the archive shell
// - redirects for base changes and shell swaps (flattening chains)
func (h *Handler) applyReadingRoutes(ctx context.Context, queries *db.Queries, oldHome, newHome, oldPosts, newPosts, oldBase, newBase string, now int64) error {
	if oldHome != newHome {
		if err := h.applyHomepageRoute(ctx, queries, oldHome, newHome, now); err != nil {
			return err
		}
	}
	if newBase == "" {
		newBase = seo.DefaultPostsBase
	}
	if err := seo.ValidatePostsBasePath(newBase); err != nil {
		return err
	}
	isLatest := newHome == ""
	archPath := "/"
	if !isLatest {
		archPath = seo.PostsArchivePath(newBase)
	}
	oldArch := seo.PostsArchivePath(oldBase)
	if oldBase == "" {
		oldArch = archPath
	}

	// ---------- Posts page shell removal (oldPosts -> not shell) ----------
	if oldPosts != "" && oldPosts != newPosts {
		// Demote the previous shell: archive route at old location keeps archive
		// type but loses its entry_id; the page itself regains a normal entry route.
		oldEntry, err := queries.GetEntry(ctx, oldPosts)
		if err == nil && oldEntry.ContentTypeID == "page" {
			// Find the archive route that currently points at this entry (could be
			// at oldArch or at archPath if base changed)
			var archRoute *db.Route
			for _, path := range []string{oldArch, archPath} {
				if rt, rerr := queries.GetRouteByPath(ctx, path); rerr == nil && rt.RouteType == "archive" && rt.EntryID.Valid && rt.EntryID.String == oldPosts {
					tmp := rt
					archRoute = &tmp
					break
				}
			}
			if archRoute != nil {
				// Keep the archive at its current path but as shell-less
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: archRoute.ID, Path: archRoute.Path, EntryID: sql.NullString{Valid: false}, RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
			// Restore normal page route at /{slug}
			slugPath := "/" + oldEntry.Slug
			if rt, rerr := queries.GetRouteByPath(ctx, slugPath); rerr == nil && !rt.EntryID.Valid {
				// stale redirect there – clear it so the entry can reclaim the path
				if delErr := queries.DeleteRoute(ctx, rt.ID); delErr != nil {
					return delErr
				}
			} else if rerr == nil && rt.EntryID.Valid && rt.EntryID.String != oldPosts {
				// different entry owns the slug – should not happen; skip restore
			} else {
				// Create or ensure entry route at slug path
				if _, rerr := queries.GetEntryRoute(ctx, strToNullString(oldPosts)); errors.Is(rerr, sql.ErrNoRows) {
					id, idErr := randomID()
					if idErr != nil {
						return idErr
					}
					if err := queries.CreateRoute(ctx, db.CreateRouteParams{
						ID: id, Path: slugPath, EntryID: strToNullString(oldPosts), RouteType: "entry", CreatedAt: now, UpdatedAt: now,
					}); err != nil {
						return err
					}
				} else if rerr == nil {
					// Entry route still points at archive path – move it back to slug path
					if ar, arErr := queries.GetEntryRoute(ctx, strToNullString(oldPosts)); arErr == nil && ar.Path != slugPath {
						if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{ID: ar.ID, Path: slugPath, EntryID: strToNullString(oldPosts), RouteType: "entry", UpdatedAt: now}); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	// ---------- Posts page shell addition (newPosts becomes shell) ----------
	if newPosts != "" && newPosts != oldPosts {
		newEntry, err := queries.GetEntry(ctx, newPosts)
		if err != nil || newEntry.ContentTypeID != "page" {
			return errors.New("posts page must be a Page")
		}
		if newEntry.Status == "trash" {
			return errors.New("posts page must not be trashed")
		}
		// Shell must not equal homepage
		if newPosts == newHome && newHome != "" {
			return errors.New("Homepage and Posts page must be different")
		}
		slugPath := "/" + newEntry.Slug

		// Conflict check: archive path must not be occupied by a different entry's route
		if archPath != "/" {
			if rt, rerr := queries.GetRouteByPath(ctx, archPath); rerr == nil && rt.RouteType == "entry" && rt.EntryID.Valid && rt.EntryID.String != newPosts {
				return fmt.Errorf("The Posts URL base %s conflicts with Page %s.", archPath, newEntry.Slug)
			}
		}

		// Promote the page's route to archive at archPath
		// Cases:
		//  - page already owns archPath as entry (slug == blog && base /blog) -> convert that route
		//  - page owns slugPath entry -> move it to archPath as archive + redirect
		//  - archive already exists shell-less -> adopt it
		//  - else create new archive

		// Handle case where page's entry route is at slugPath and archPath is different
		// We need to free slugPath later with a redirect to archPath.
		needsSlugRedirect := false
		if slugPath != archPath {
			if er, rerr := queries.GetEntryRoute(ctx, strToNullString(newPosts)); rerr == nil {
				if er.Path == slugPath {
					needsSlugRedirect = true
				} else if er.Path != archPath {
					// entry route at some other path (previous slug)
					needsSlugRedirect = true
				}
			}
		}

		if rt, rerr := queries.GetRouteByPath(ctx, archPath); errors.Is(rerr, sql.ErrNoRows) {
			// No archive there – try to move the page's entry route
			if er, rerr2 := queries.GetEntryRoute(ctx, strToNullString(newPosts)); rerr2 == nil {
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: er.ID, Path: archPath, EntryID: strToNullString(newPosts), RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
				}); err != nil {
					return err
				}
			} else {
				id, idErr := randomID()
				if idErr != nil {
					return idErr
				}
				if err := queries.CreateRoute(ctx, db.CreateRouteParams{
					ID: id, Path: archPath, EntryID: strToNullString(newPosts), RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
		} else if rerr == nil && rt.RouteType == "archive" {
			if !rt.EntryID.Valid {
				// shell-less archive – adopt it
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: rt.ID, Path: archPath, EntryID: strToNullString(newPosts), RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
				}); err != nil {
					return err
				}
			} else if rt.EntryID.String != newPosts {
				// different shell already occupies (should have been demoted)
				return fmt.Errorf("The Posts URL base %s is already used as archive shell.", archPath)
			}
			// If entry still has a separate entry route at slugPath, remove it (it will redirect)
			if slugPath != archPath {
				if er, rerr2 := queries.GetEntryRoute(ctx, strToNullString(newPosts)); rerr2 == nil && er.Path != archPath {
					if err := queries.DeleteRoute(ctx, er.ID); err != nil {
						return err
					}
				}
			}
		} else if rerr == nil && rt.RouteType == "entry" {
			// entry-occupied path – only allowed if it's the same entry becoming archive
			if !rt.EntryID.Valid || rt.EntryID.String != newPosts {
				return fmt.Errorf("The Posts URL base %s conflicts with an existing route.", archPath)
			}
			if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
				ID: rt.ID, Path: archPath, EntryID: strToNullString(newPosts), RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		if needsSlugRedirect && slugPath != archPath && slugPath != "/" {
			if err := h.upsertRedirectRoute(ctx, queries, slugPath, archPath, now); err != nil {
				return err
			}
		}
	}

	// ---------- Archive mount move due to homepage mode change (even when base unchanged) ----------
	if oldArch != archPath {
		if oldBase != "" && oldBase != newBase {
			// Base change already handles archive move + post remap below; skip duplicate move here
		} else {
			// Homepage mode switch (e.g., /blog <-> /) with same shell
			if rt, rerr := queries.GetRouteByPath(ctx, oldArch); rerr == nil && rt.RouteType == "archive" {
				if _, er := queries.GetRouteByPath(ctx, archPath); errors.Is(er, sql.ErrNoRows) {
					if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
						ID: rt.ID, Path: archPath, EntryID: rt.EntryID, RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
					}); err != nil {
						return err
					}
					if err := h.upsertRedirectRoute(ctx, queries, oldArch, archPath, now); err != nil {
						return err
					}
				} else {
					// Archive already at new mount (e.g., shell addition already created it) – ensure redirect
					if err := h.upsertRedirectRoute(ctx, queries, oldArch, archPath, now); err != nil {
						return err
					}
				}
			} else {
				// No archive at old path – still ensure redirect if old was archive-like
				if oldArch != "/" {
					if err := h.upsertRedirectRoute(ctx, queries, oldArch, archPath, now); err != nil {
						return err
					}
				}
			}
		}
	}

	// ---------- Base change handling (post routes + shell archive move) ----------
	if oldBase != "" && oldBase != newBase && oldArch != archPath {
		// Move shell archive if it exists at oldArch
		if rt, rerr := queries.GetRouteByPath(ctx, oldArch); rerr == nil && rt.RouteType == "archive" {
			// Archive at old base – move to new base, preserving shell entry_id
			if existing, er := queries.GetRouteByPath(ctx, archPath); er == nil && existing.RouteType == "archive" {
				// Already an archive at new path (shell already handled) – just redirect old
			} else {
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: rt.ID, Path: archPath, EntryID: rt.EntryID, RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
		}
		// Redirect old archive path to new (for shell-less or already-moved case)
		if err := h.upsertRedirectRoute(ctx, queries, oldArch, archPath, now); err != nil {
			return err
		}
		// Remap published posts that lived under old base
		posts, _ := queries.ListEntriesByContentType(ctx, "post")
		for _, p := range posts {
			if p.Status != "active" {
				continue
			}
			rt, rerr := queries.GetEntryRoute(ctx, strToNullString(p.ID))
			if rerr != nil {
				continue
			}
			// Only remap if old path is under oldArch prefix
			if oldArch == "/" {
				// homepage archive at root – posts base was root? posts were at /{slug} – now should be at archPath/{slug}
				// This case occurs when moving from latest_posts home (posts at "/") to static home with /blog base: very rare.
				// Only remap if new path differs
				newP := seo.EntryPath("post", p.Slug, newBase)
				if newP == rt.Path {
					continue
				}
				oldPath := rt.Path
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: rt.ID, Path: newP, EntryID: strToNullString(p.ID), RouteType: "entry", UpdatedAt: now,
				}); err != nil {
					return err
				}
				if err := h.upsertRedirectRoute(ctx, queries, oldPath, newP, now); err != nil {
					return err
				}
				continue
			}
			if !strings.HasPrefix(rt.Path, oldArch+"/") && rt.Path != oldArch {
				continue
			}
			newP := seo.EntryPath("post", p.Slug, newBase)
			if newP == rt.Path {
				continue
			}
			// Capture old path before update for redirect
			oldPath := rt.Path
			if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
				ID: rt.ID, Path: newP, EntryID: strToNullString(p.ID), RouteType: "entry", UpdatedAt: now,
			}); err != nil {
				return err
			}
			if err := h.upsertRedirectRoute(ctx, queries, oldPath, newP, now); err != nil {
				return err
			}
		}
	}
	// Also handle post remap when archive mount changed due to homepage switch but base unchanged?
	// Posts base remains same, so no post remap needed (per spec posts stay at /blog). Intentionally no remap here.

	// Ensure archive route exists at the mount point (if not already handled by shell logic)
	if rt, rerr := queries.GetRouteByPath(ctx, archPath); errors.Is(rerr, sql.ErrNoRows) {
		id, idErr := randomID()
		if idErr != nil {
			return idErr
		}
		if err := queries.CreateRoute(ctx, db.CreateRouteParams{
			ID: id, Path: archPath, EntryID: sql.NullString{Valid: false}, RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
	} else if rerr == nil && rt.RouteType != "archive" && archPath != "/" {
		if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
			ID: rt.ID, Path: archPath, EntryID: sql.NullString{Valid: false}, RouteType: "archive", ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
		}); err != nil {
			return err
		}
	}

	// If explicit posts base differs from archive mount (latest-posts home case), redirect base -> archive root
	explicit := seo.PostsArchivePath(newBase)
	if explicit != archPath {
		if err := h.upsertRedirectRoute(ctx, queries, explicit, archPath, now); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) listPageOptions(r *http.Request) []pageOption {
	entries, err := h.queries.ListEntriesByContentType(r.Context(), "page")
	if err != nil {
		return nil
	}
	options := make([]pageOption, 0, len(entries))
	for _, entry := range entries {
		if entry.Status == "trash" {
			continue
		}
		title := entry.Title.String
		if title == "" {
			title = "(untitled)"
		}
		path := ""
		if entry.PublishedRevisionID.Valid && entry.PublicPath.Valid {
			path = entry.PublicPath.String
		}
		disabled := !entry.PublishedRevisionID.Valid
		options = append(options, pageOption{ID: entry.ID, Title: title, Path: path, Disabled: disabled})
	}
	return options
}

func (h *Handler) populateSettingsURLs(r *http.Request, data *settingsData, row db.GetSiteSettingsRow) {
	data.SiteURLWarning = strings.TrimSpace(row.SiteUrl) == ""
	if row.SitemapEnabled != 0 && !data.SiteURLWarning {
		data.SitemapPublicURL = strings.TrimRight(row.SiteUrl, "/") + "/sitemap.xml"
	}
}

func (h *Handler) renderSettingsPage(w http.ResponseWriter, r *http.Request, data settingsData) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.CSRFToken = token
	layout := LayoutData{Title: "Settings", ActiveMenu: "settings", CSRFToken: token, Content: data}
	if err := h.settingsTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render settings: %v", err)
	}
}

func (h *Handler) renderSettingsFragment(w http.ResponseWriter, r *http.Request, data settingsData, notice string) {
	var buf bytes.Buffer
	if err := h.settingsTemplate.ExecuteTemplate(&buf, "content", LayoutData{Content: data}); err != nil {
		log.Printf("render settings fragment: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	events := []sseEvent{patchElementsEvent("outer", "#settings-content", buf.String())}
	if notice != "" {
		events = append(events, toastEvent("success", notice))
	} else if msg, ok := data.Errors["_"]; ok {
		events = append(events, toastEvent("error", msg))
	}
	writeSSE(w, events...)
}

func formFromRow(row db.GetSiteSettingsRow, siteIconMediaID, siteSocialMediaID string) settingsForm {
	return settingsForm{
		SiteTitle:            row.SiteTitle,
		Tagline:              row.SiteTagline,
		SiteURL:              row.SiteUrl,
		Language:             row.Language,
		Timezone:             row.Timezone,
		SiteRepresents:       siteRepresentsFormValue(row.SiteRepresents),
		HomepageEntryID:      nullStringToStr(row.HomepageEntryID),
		PostsPageEntryID:     nullStringToStr(row.PostsPageEntryID),
		PostsBasePath:        row.PostsBasePath,
		PostsPerPage:         int(row.PostsPerPage),
		SiteIconMediaID:      siteIconMediaID,
		SiteSocialMediaID:    siteSocialMediaID,
		TwitterSite:          row.TwitterSite,
		IndexingEnabled:      row.IndexingEnabled != 0,
		SitemapEnabled:       row.SitemapEnabled != 0,
		RobotsMode:           row.RobotsMode,
		RobotsCustom:         row.RobotsCustom,
		SpeculationEnabled:   row.SpeculationMode != "off",
		SpeculationMode:      row.SpeculationMode,
		SpeculationEagerness: row.SpeculationEagerness,
		TitleSeparator:       row.TitleSeparator,
	}
}

func (h *Handler) parseSettingsForm(r *http.Request) (settingsForm, map[string]string) {
	_ = r.ParseForm()
	form := settingsForm{
		SiteTitle:            strings.TrimSpace(r.FormValue("site_title")),
		Tagline:              strings.TrimSpace(r.FormValue("tagline")),
		SiteURL:              r.FormValue("site_url"),
		Language:             strings.TrimSpace(r.FormValue("language")),
		Timezone:             strings.TrimSpace(r.FormValue("timezone")),
		SiteRepresents:       siteRepresentsFormValue(r.FormValue("site_represents")),
		HomepageEntryID:      strings.TrimSpace(r.FormValue("homepage_entry_id")),
		PostsPageEntryID:     strings.TrimSpace(r.FormValue("posts_page_entry_id")),
		PostsBasePath:        strings.TrimSpace(r.FormValue("posts_base_path")),
		PostsPerPage:         parsePostsPerPage(r.FormValue("posts_per_page")),
		SiteIconMediaID:      strings.TrimSpace(r.FormValue("site_icon_media_id")),
		SiteSocialMediaID:    strings.TrimSpace(r.FormValue("site_social_media_id")),
		TwitterSite:          strings.TrimSpace(r.FormValue("twitter_site")),
		IndexingEnabled:      r.FormValue("indexing_enabled") == "on",
		SitemapEnabled:       r.FormValue("sitemap_enabled") == "on",
		RobotsMode:           strings.TrimSpace(r.FormValue("robots_mode")),
		RobotsCustom:         r.FormValue("robots_custom"),
		SpeculationEnabled:   r.FormValue("speculation_enabled") == "on",
		SpeculationMode:      strings.TrimSpace(r.FormValue("speculation_mode")),
		SpeculationEagerness: strings.TrimSpace(r.FormValue("speculation_eagerness")),
		TitleSeparator:       strings.TrimSpace(r.FormValue("title_separator")),
	}
	choice := strings.TrimSpace(r.FormValue("homepage_mode_choice"))
	if choice == "latest" {
		form.HomepageEntryID = ""
	}
	if form.SpeculationEnabled && form.SpeculationMode == "" {
		form.SpeculationMode = "prefetch"
	}
	if !form.SpeculationEnabled {
		form.SpeculationMode = "off"
	}
	errors := map[string]string{}

	if form.SiteTitle == "" {
		errors["site_title"] = "Site title is required."
	} else if len(form.SiteTitle) > maxSettingsFieldLen {
		errors["site_title"] = "Site title is too long."
	}
	if len(form.Tagline) > maxSettingsFieldLen {
		errors["tagline"] = "Tagline is too long."
	}
	if normalized, err := site.ValidateSiteURL(form.SiteURL); err != nil {
		errors["site_url"] = err.Error()
	} else {
		form.SiteURL = normalized
	}
	if err := site.ValidateLanguage(form.Language); err != nil {
		errors["language"] = err.Error()
	}
	if err := site.ValidateTimezone(form.Timezone); err != nil {
		errors["timezone"] = err.Error()
	}
	if form.HomepageEntryID != "" {
		entry, err := h.queries.GetEntry(r.Context(), form.HomepageEntryID)
		if err != nil || entry.ContentTypeID != "page" {
			errors["homepage_entry_id"] = "Homepage must be an existing Page."
		} else if entry.Status == "trash" {
			errors["homepage_entry_id"] = "Homepage must not be trashed."
		} else if !entry.PublishedRevisionID.Valid {
			errors["homepage_entry_id"] = "Homepage must be published before it can be used."
		}
	}
	if form.HomepageEntryID != "" && form.PostsPageEntryID != "" && form.HomepageEntryID == form.PostsPageEntryID {
		errors["posts_page_entry_id"] = "Homepage and Posts page must be different."
	}
	if form.PostsPageEntryID != "" {
		entry, err := h.queries.GetEntry(r.Context(), form.PostsPageEntryID)
		if err != nil || entry.ContentTypeID != "page" || entry.Status == "trash" {
			errors["posts_page_entry_id"] = "Posts page must be an existing (non-trashed) Page."
		} else if !entry.PublishedRevisionID.Valid {
			errors["posts_page_entry_id"] = "Posts page must be published before it can be used as archive shell."
		}
	}
	if form.PostsBasePath != "" {
		if err := seo.ValidatePostsBasePath(form.PostsBasePath); err != nil {
			errors["posts_base_path"] = err.Error()
		}
	}
	if form.PostsPerPage < 1 || form.PostsPerPage > 100 {
		errors["posts_per_page"] = "Posts per page must be between 1 and 100."
	}
	if form.RobotsMode != "managed" && form.RobotsMode != "custom" {
		errors["robots_mode"] = "Robots mode must be managed or custom."
	}
	if form.SiteRepresents != structured.RepresentsOrganization && form.SiteRepresents != structured.RepresentsPerson {
		errors["site_represents"] = "Site represents must be Organization or Person."
	}
	if err := site.ValidateRobotsSize(form.RobotsCustom); err != nil {
		errors["robots_custom"] = err.Error()
	}
	if !site.ValidSpeculationMode(form.SpeculationMode) {
		errors["speculation_mode"] = "Unsupported speculation mode."
	}
	if !site.ValidSpeculationEagerness(form.SpeculationEagerness) {
		errors["speculation_eagerness"] = "Unsupported speculation eagerness."
	}
	if len(form.TitleSeparator) > 8 {
		errors["title_separator"] = "Title separator is too long."
	}
	if form.SiteIconMediaID != "" {
		if _, err := h.media.Get(r.Context(), form.SiteIconMediaID); err != nil {
			errors["site_icon_media_id"] = "Selected image is no longer available."
		}
	}
	if form.SiteSocialMediaID != "" {
		if _, err := h.media.Get(r.Context(), form.SiteSocialMediaID); err != nil {
			errors["site_social_media_id"] = "Selected image is no longer available."
		}
	}
	if form.TwitterSite != "" && len(form.TwitterSite) > 200 {
		errors["twitter_site"] = "Twitter handle is too long."
	}
	if form.TwitterSite != "" && !(strings.HasPrefix(form.TwitterSite, "@") || strings.HasPrefix(form.TwitterSite, "http://") || strings.HasPrefix(form.TwitterSite, "https://")) {
		// Allow handles with or without @, or full URLs. If it contains a space, reject.
		if strings.Contains(form.TwitterSite, " ") {
			errors["twitter_site"] = "Twitter handle must not contain spaces."
		}
	}
	return form, errors
}

func parsePostsPerPage(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return 10
	}
	if n > 100 {
		return 100
	}
	return n
}

func nullStringToStr(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

// siteRepresentsFormValue normalizes the "Site represents" choice. An empty
// value (forms posted before the setting existed) defaults to organization;
// anything unrecognized passes through so validation reports it.
func siteRepresentsFormValue(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return structured.RepresentsOrganization
	}
	return v
}

func strToNullString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func languageOptions() []languageOption {
	return []languageOption{
		{Value: "en", Label: "English"},
		{Value: "en-US", Label: "English (United States)"},
		{Value: "en-GB", Label: "English (United Kingdom)"},
		{Value: "pl", Label: "Polski"},
		{Value: "de", Label: "Deutsch"},
		{Value: "fr", Label: "Français"},
		{Value: "es", Label: "Español"},
		{Value: "it", Label: "Italiano"},
		{Value: "pt", Label: "Português"},
		{Value: "nl", Label: "Nederlands"},
		{Value: "ja", Label: "日本語"},
		{Value: "zh", Label: "中文"},
		{Value: "ru", Label: "Русский"},
		{Value: "uk", Label: "Українська"},
		{Value: "cs", Label: "Čeština"},
	}
}

func timezoneOptions() []timezoneOption {
	// Human-readable list of common IANA zones. Full list would be huge; this covers major regions
	// and the server can still preserve any valid custom zone via the fallback option.
	return []timezoneOption{
		{Value: "UTC", Label: "UTC — Coordinated Universal Time"},
		{Value: "Europe/Warsaw", Label: "Europe/Warsaw — Warsaw"},
		{Value: "Europe/Berlin", Label: "Europe/Berlin — Berlin"},
		{Value: "Europe/Paris", Label: "Europe/Paris — Paris"},
		{Value: "Europe/London", Label: "Europe/London — London"},
		{Value: "Europe/Rome", Label: "Europe/Rome — Rome"},
		{Value: "Europe/Madrid", Label: "Europe/Madrid — Madrid"},
		{Value: "Europe/Prague", Label: "Europe/Prague — Prague"},
		{Value: "America/New_York", Label: "America/New_York — New York"},
		{Value: "America/Chicago", Label: "America/Chicago — Chicago"},
		{Value: "America/Los_Angeles", Label: "America/Los_Angeles — Los Angeles"},
		{Value: "America/Sao_Paulo", Label: "America/Sao_Paulo — São Paulo"},
		{Value: "Asia/Tokyo", Label: "Asia/Tokyo — Tokyo"},
		{Value: "Asia/Shanghai", Label: "Asia/Shanghai — Shanghai"},
		{Value: "Asia/Dubai", Label: "Asia/Dubai — Dubai"},
		{Value: "Australia/Sydney", Label: "Australia/Sydney — Sydney"},
	}
}

func isLanguageInOptions(val string, opts []languageOption) bool {
	for _, o := range opts {
		if o.Value == val {
			return true
		}
	}
	return false
}

func isTimezoneInOptions(val string, opts []timezoneOption) bool {
	for _, o := range opts {
		if o.Value == val {
			return true
		}
	}
	return false
}

func (h *Handler) validatePostsPageAssignment(ctx context.Context, qtx *db.Queries, entryID string) error {
	entry, err := qtx.GetEntry(ctx, entryID)
	if err != nil || entry.ContentTypeID != "page" || entry.Status == "trash" {
		return errors.New("Posts page must be an existing (non-trashed) Page.")
	}
	if !entry.PublishedRevisionID.Valid {
		return errors.New("Posts page must be published before it can be used as archive shell.")
	}
	rev, err := qtx.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil {
		return err
	}
	doc, err := document.Decode([]byte(rev.DocumentJson))
	if err != nil {
		return err
	}
	count := 0
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/posts" {
				source := "archive"
				pagination := true
				if len(n.Settings) > 0 {
					var s map[string]any
					if json.Unmarshal(n.Settings, &s) == nil {
						if v, ok := s["source"].(string); ok && v != "" {
							source = v
						}
						// Treat "automatic" as archive for pagination counting (alias)
						if source == "automatic" {
							source = "archive"
						}
						if v, ok := s["pagination"].(bool); ok {
							pagination = v
						}
					}
				}
				if (source == "archive" || source == "automatic") && pagination {
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
		return errors.New("Only one paginated Posts block can be used on a Posts Page.")
	}
	return nil
}
