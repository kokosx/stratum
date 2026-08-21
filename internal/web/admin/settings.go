package admin

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/site"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const maxSettingsFieldLen = 200

type settingsForm struct {
	SiteTitle            string
	Tagline              string
	SiteURL              string
	Language             string
	Timezone             string
	HomepageEntryID      string
	SiteIconMediaID      string
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
	Form             settingsForm
	Pages            []pageOption
	Errors           map[string]string
	Notice           string
	CSRFToken        string
	SiteURLWarning   bool
	SitemapPublicURL string
	SiteIconPreview  string
	SiteIconWarning  string
	Errored          bool
	// DisableSave controls the Save Changes button. It is false on the full
	// page (and on validation errors) so the form still submits without JS; it
	// is true only in the post-save Datastar fragment, where JS re-enables it on
	// edit. This preserves progressive enhancement.
	DisableSave bool
}

type pageOption struct {
	ID    string
	Title string
	Path  string
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
	data := settingsData{
		Form:       formFromRow(row, iconID.String),
		Pages:      h.listPageOptions(r),
		Errors:     map[string]string{},
		CSRFToken:  token,
		DisableSave: false,
	}
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
	data := settingsData{Form: form, Pages: h.listPageOptions(r), Errors: fieldErrors, CSRFToken: r.FormValue("csrf_token")}
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

	saved, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.Form = formFromRow(saved, form.SiteIconMediaID)
	data.Errors = map[string]string{}
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
// favicon variants from the chosen asset. An empty value clears the icon.
func (h *Handler) applySiteIcon(ctx context.Context, newIcon string) error {
	current, err := h.queries.GetSiteIconMediaID(ctx)
	if err != nil {
		return err
	}
	changed := (newIcon != "") != current.Valid || (current.Valid && current.String != newIcon)
	if !changed {
		return nil
	}
	null := sql.NullString{}
	if newIcon != "" {
		null = sql.NullString{String: newIcon, Valid: true}
	}
	if err := h.queries.UpdateSiteIconMediaID(ctx, null); err != nil {
		return err
	}
	if newIcon != "" {
		if err := h.media.GenerateFaviconVariants(ctx, newIcon); err != nil {
			return err
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
	if oldHome != newHome {
		if err := h.applyHomepageRoute(ctx, qtx, oldHome, newHome, time.Now().Unix()); err != nil {
			return err
		}
	}
	homepageMode := "latest_posts"
	if newHome != "" {
		homepageMode = "page"
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
		PostsPageEntryID:     current.PostsPageEntryID,
		PostsPerPage:         current.PostsPerPage,
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
		UpdatedAt:            time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// applyHomepageRoute keeps the chosen homepage page served at "/" and restores
// the previous homepage page's "/slug" route. Each page keeps exactly one
// entry-type route.
func (h *Handler) applyHomepageRoute(ctx context.Context, queries *db.Queries, oldHome, newHome string, now int64) error {
	if oldHome != "" {
		entry, err := queries.GetEntry(ctx, oldHome)
		if err == nil && entry.ContentTypeID == "page" {
			route, rerr := queries.GetEntryRoute(ctx, strToNullString(oldHome))
			if rerr == nil {
				if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
					ID: route.ID, Path: "/" + entry.Slug, EntryID: strToNullString(oldHome),
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
	return queries.UpdateRoute(ctx, db.UpdateRouteParams{
		ID: route.ID, Path: "/", EntryID: strToNullString(newHome), RouteType: "entry", UpdatedAt: now,
	})
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
		options = append(options, pageOption{ID: entry.ID, Title: title, Path: path})
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

func formFromRow(row db.GetSiteSettingsRow, siteIconMediaID string) settingsForm {
	return settingsForm{
		SiteTitle:            row.SiteTitle,
		Tagline:              row.SiteTagline,
		SiteURL:              row.SiteUrl,
		Language:             row.Language,
		Timezone:             row.Timezone,
		HomepageEntryID:      nullStringToStr(row.HomepageEntryID),
		SiteIconMediaID:      siteIconMediaID,
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
		HomepageEntryID:      strings.TrimSpace(r.FormValue("homepage_entry_id")),
		SiteIconMediaID:      strings.TrimSpace(r.FormValue("site_icon_media_id")),
		IndexingEnabled:      r.FormValue("indexing_enabled") == "on",
		SitemapEnabled:       r.FormValue("sitemap_enabled") == "on",
		RobotsMode:           strings.TrimSpace(r.FormValue("robots_mode")),
		RobotsCustom:         r.FormValue("robots_custom"),
		SpeculationEnabled:   r.FormValue("speculation_enabled") == "on",
		SpeculationMode:      strings.TrimSpace(r.FormValue("speculation_mode")),
		SpeculationEagerness: strings.TrimSpace(r.FormValue("speculation_eagerness")),
		TitleSeparator:       strings.TrimSpace(r.FormValue("title_separator")),
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
		}
	}
	if form.RobotsMode != "managed" && form.RobotsMode != "custom" {
		errors["robots_mode"] = "Robots mode must be managed or custom."
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
	return form, errors
}

func nullStringToStr(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
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
