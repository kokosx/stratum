package admin

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/analytics"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// analyticsData is the shared data for analytics templates.
type analyticsData struct {
	Range         string // today, 7d, 30d, etc
	SinceDay      string
	SinceHour     int64
	ActiveTab     string // overview, content, etc
	Overview      analytics.Overview
	ContentRows   []analytics.PageRow
	ContentDetail *contentDetailData
	Acquisition   acquisitionData
	Technology    map[string][]analytics.DimRow
	Crawlers      []analytics.DimRow
	CrawlerTotal  int64
	Performance   perfData
	Settings      analyticsSettingsData
	Health        analytics.Health
	Notice        string
	Error         string
	CSRFToken     string
	IsEnabled     bool
	// For empty state
	HasData bool
}

type contentDetailData struct {
	EntryID string
	Title   string
	Path    string
	Rows    []db.AnalyticsPageDaily
}

type acquisitionData struct {
	Sources      map[string]int64
	TopReferrers []analytics.DimRow
	TopSources   []analytics.DimRow
	TopMediums   []analytics.DimRow
	TopCampaigns []analytics.DimRow
}

type perfData struct {
	SiteHourly    db.SumAnalyticsSiteHourlyRow
	Slowest       []analytics.PageRow
	Speculative   int64
	Dropped       uint64
	CacheHitRatio float64
	AvgMs         float64
}

type analyticsSettingsData struct {
	Enabled             bool
	RetentionDays       int64
	HourlyRetentionDays int64
	AllowedRetentions   []int64
	AllowedHourly       []int64
}

func (h *Handler) analyticsRange(r *http.Request) (string, string, int64) {
	rng := strings.TrimSpace(r.URL.Query().Get("range"))
	if rng == "" {
		rng = "30d"
	}
	allowed := map[string]bool{"today": true, "7d": true, "30d": true, "90d": true, "12m": true}
	if !allowed[rng] {
		rng = "30d"
	}
	_, sinceDay, sinceHour := analytics.ParseRange(rng)
	// Map 12m -> 365d for ParseRange helper
	if rng == "12m" {
		_, sinceDay, sinceHour = analytics.ParseRange("365d")
	}
	return rng, sinceDay, sinceHour
}

func (h *Handler) analyticsOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	overview, _ := h.analyticsReader.GetOverview(r.Context(), sinceDay, sinceHour)
	health := analytics.Health{}
	if h.analyticsService != nil {
		health = h.analyticsService.Health()
	}
	snap := h.runtime.Site.Current()
	isEnabled := true
	if snap != nil {
		isEnabled = snap.AnalyticsEnabled
	}
	data := analyticsData{
		Range:     rng,
		SinceDay:  sinceDay,
		SinceHour: sinceHour,
		ActiveTab: "overview",
		Overview:  overview,
		Health:    health,
		IsEnabled: isEnabled,
		HasData:   overview.Views > 0 || overview.TotalRequests > 0,
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsContent(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	rows, _ := h.analyticsReader.GetContentList(r.Context(), sinceDay, 50, 0)
	// resolve titles: if entry exists, use latest title else path
	for i, row := range rows {
		if row.EntryID != "" {
			if title := h.resolveEntryTitle(r.Context(), row.EntryID); title != "" {
				rows[i].Title = title
			} else {
				rows[i].Title = row.Path
			}
		} else {
			rows[i].Title = row.Path
		}
	}
	_ = sinceHour
	health := analytics.Health{}
	if h.analyticsService != nil {
		health = h.analyticsService.Health()
	}
	snap := h.runtime.Site.Current()
	isEnabled := true
	if snap != nil {
		isEnabled = snap.AnalyticsEnabled
	}
	data := analyticsData{
		Range:       rng,
		SinceDay:    sinceDay,
		SinceHour:   sinceHour,
		ActiveTab:   "content",
		ContentRows: rows,
		Health:      health,
		IsEnabled:   isEnabled,
		HasData:     len(rows) > 0,
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsContentDetail(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	entryID := r.PathValue("entryID")
	if entryID == "" {
		http.NotFound(w, r)
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	rows, _ := h.analyticsReader.GetPageDailyByEntry(r.Context(), entryID, sinceDay)
	title := h.resolveEntryTitle(r.Context(), entryID)
	if title == "" {
		if len(rows) > 0 {
			title = rows[0].Path
		} else {
			title = entryID
		}
	}
	path := ""
	if len(rows) > 0 {
		path = rows[0].Path
	}
	detail := &contentDetailData{
		EntryID: entryID,
		Title:   title,
		Path:    path,
		Rows:    rows,
	}
	_ = sinceHour
	health := analytics.Health{}
	if h.analyticsService != nil {
		health = h.analyticsService.Health()
	}
	snap := h.runtime.Site.Current()
	isEnabled := true
	if snap != nil {
		isEnabled = snap.AnalyticsEnabled
	}
	data := analyticsData{
		Range:         rng,
		SinceDay:      sinceDay,
		SinceHour:     sinceHour,
		ActiveTab:     "content",
		ContentDetail: detail,
		Health:        health,
		IsEnabled:     isEnabled,
		HasData:       len(rows) > 0,
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsAcquisition(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	sources, refs, srcs, mdms, camps := h.analyticsReader.GetAcquisition(r.Context(), sinceDay)
	_ = sinceHour
	data := analyticsData{
		Range:     rng,
		SinceDay:  sinceDay,
		SinceHour: sinceHour,
		ActiveTab: "acquisition",
		Acquisition: acquisitionData{
			Sources:      sources,
			TopReferrers: refs,
			TopSources:   srcs,
			TopMediums:   mdms,
			TopCampaigns: camps,
		},
		IsEnabled: true,
		HasData:   len(refs) > 0 || len(srcs) > 0,
	}
	if snap := h.runtime.Site.Current(); snap != nil {
		data.IsEnabled = snap.AnalyticsEnabled
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsTechnology(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	tech := h.analyticsReader.GetTechnology(r.Context(), sinceDay)
	_ = sinceHour
	data := analyticsData{
		Range:      rng,
		SinceDay:   sinceDay,
		SinceHour:  sinceHour,
		ActiveTab:  "technology",
		Technology: tech,
		IsEnabled:  true,
		HasData:    len(tech["browser"]) > 0,
	}
	if snap := h.runtime.Site.Current(); snap != nil {
		data.IsEnabled = snap.AnalyticsEnabled
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsCrawlers(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	rows, total := h.analyticsReader.GetCrawlers(r.Context(), sinceDay)
	_ = sinceHour
	_ = total
	data := analyticsData{
		Range:     rng,
		SinceDay:  sinceDay,
		SinceHour: sinceHour,
		ActiveTab: "crawlers",
		Crawlers:  rows,
		IsEnabled: true,
		HasData:   len(rows) > 0,
	}
	if snap := h.runtime.Site.Current(); snap != nil {
		data.IsEnabled = snap.AnalyticsEnabled
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsPerformance(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	rng, sinceDay, sinceHour := h.analyticsRange(r)
	siteHourly, _ := h.analyticsReader.GetPerformance(r.Context(), sinceHour)
	slowest, _ := h.analyticsReader.GetSlowestContent(r.Context(), sinceDay)
	health := analytics.Health{}
	if h.analyticsService != nil {
		health = h.analyticsService.Health()
	}
	// Calculate cache hit ratio and avg duration
	var hitRatio float64
	totalCache := siteHourly.TotalHits + siteHourly.TotalMisses
	if totalCache > 0 {
		hitRatio = float64(siteHourly.TotalHits) / float64(totalCache)
	}
	var avgMs float64
	if siteHourly.TotalCount > 0 {
		avgMs = float64(siteHourly.TotalDuration) / float64(siteHourly.TotalCount) / 1000.0
	}
	data := analyticsData{
		Range:     rng,
		SinceDay:  sinceDay,
		SinceHour: sinceHour,
		ActiveTab: "performance",
		Performance: perfData{
			SiteHourly:    siteHourly,
			Slowest:       slowest,
			Speculative:   siteHourly.TotalSpeculative,
			Dropped:       health.Dropped,
			CacheHitRatio: hitRatio,
			AvgMs:         avgMs,
		},
		Health:    health,
		IsEnabled: true,
		HasData:   siteHourly.TotalRequests > 0,
	}
	if snap := h.runtime.Site.Current(); snap != nil {
		data.IsEnabled = snap.AnalyticsEnabled
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	_, sinceDay, sinceHour := h.analyticsRange(r)
	row, err := h.queries.GetSiteSettings(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	health := analytics.Health{}
	if h.analyticsService != nil {
		health = h.analyticsService.Health()
	}
	data := analyticsData{
		ActiveTab: "settings",
		SinceDay:  sinceDay,
		SinceHour: sinceHour,
		Range:     "30d",
		Settings: analyticsSettingsData{
			Enabled:             row.AnalyticsEnabled != 0,
			RetentionDays:       row.AnalyticsRetentionDays,
			HourlyRetentionDays: row.AnalyticsHourlyRetentionDays,
			AllowedRetentions:   []int64{90, 180, 365, 730, 1095},
			AllowedHourly:       []int64{30, 90, 180},
		},
		Health:    health,
		CSRFToken: token,
		IsEnabled: row.AnalyticsEnabled != 0,
		HasData:   true,
	}
	h.renderAnalytics(w, r, data)
}

func (h *Handler) analyticsSaveSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	enabled := r.FormValue("analytics_enabled") == "1" || r.FormValue("analytics_enabled") == "on"
	retStr := r.FormValue("analytics_retention_days")
	hourlyStr := r.FormValue("analytics_hourly_retention_days")
	ret, err := strconv.ParseInt(retStr, 10, 64)
	if err != nil {
		ret = 730
	}
	hourly, err := strconv.ParseInt(hourlyStr, 10, 64)
	if err != nil {
		hourly = 90
	}
	allowedRet := map[int64]bool{90: true, 180: true, 365: true, 730: true, 1095: true}
	allowedHourly := map[int64]bool{30: true, 90: true, 180: true}
	if !allowedRet[ret] {
		ret = 730
	}
	if !allowedHourly[hourly] {
		hourly = 90
	}
	now := time.Now().Unix()
	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}
	if err := h.queries.UpdateAnalyticsSettings(r.Context(), db.UpdateAnalyticsSettingsParams{
		AnalyticsEnabled:             enabledInt,
		AnalyticsRetentionDays:       ret,
		AnalyticsHourlyRetentionDays: hourly,
		UpdatedAt:                    now,
	}); err != nil {
		log.Printf("analytics save settings: %v", err)
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Could not save settings"))
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		_ = h.runtime.ReloadSite(r.Context())
	}
	if isDatastarRequest(r) {
		// Patch settings fragment and toast
		// For simplicity, send toast and reload page fragment via toastEvent + patchElementsEvent?
		// We'll just send toast; user can refresh to see updated state.
		writeSSE(w, toastEvent("success", "Analytics settings saved"))
		return
	}
	h.setFlash(w, "Analytics settings saved.")
	http.Redirect(w, r, "/admin/analytics/settings", http.StatusSeeOther)
}

func (h *Handler) analyticsClear(w http.ResponseWriter, r *http.Request) {
	if !h.requireManageSite(w, r) {
		return
	}
	if !h.validCSRF(r) {
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Invalid security token"))
			return
		}
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	// Use service Clear to coordinate with worker
	var err error
	if h.analyticsService != nil {
		// Give 5s timeout
		cctx, cancel := contextWithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = h.analyticsService.Clear(cctx)
	} else {
		// Fallback direct DB clear (should not happen) - use raw DB
		if h.database != nil {
			_, err = h.database.ExecContext(ctx, `DELETE FROM analytics_site_hourly`)
			if err == nil {
				_, _ = h.database.ExecContext(ctx, `DELETE FROM analytics_page_daily`)
				_, _ = h.database.ExecContext(ctx, `DELETE FROM analytics_dimension_daily`)
				_, _ = h.database.ExecContext(ctx, `DELETE FROM analytics_transition_daily`)
			}
		}
	}
	if err != nil {
		log.Printf("analytics clear: %v", err)
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", "Could not clear analytics"))
			return
		}
		http.Error(w, "Could not clear analytics", http.StatusInternalServerError)
		return
	}
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", "Analytics data cleared"))
		return
	}
	h.setFlash(w, "Analytics data cleared.")
	http.Redirect(w, r, "/admin/analytics", http.StatusSeeOther)
}

func contextWithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

func (h *Handler) resolveEntryTitle(ctx context.Context, entryID string) string {
	// Try to get title from latest revision
	var title string
	// Use raw query to avoid sqlc missing helper; try GetPublishedEntryByID if published exists, else fallback
	if row, err := h.queries.GetPublishedEntryByID(ctx, entryID); err == nil {
		title = row.Title
	}
	if title == "" {
		// try list revisions?
		// Fallback to entry slug
		if e, err := h.queries.GetEntry(ctx, entryID); err == nil {
			title = e.Slug
		}
	}
	return title
}

func (h *Handler) renderAnalytics(w http.ResponseWriter, r *http.Request, data analyticsData) {
	token, _ := h.csrfToken(w, r)
	data.CSRFToken = token
	// Ensure CSRF token present for forms
	layout := h.analyticsTemplate
	// Use layout.html
	// Build LayoutData
	state := ResolveNav(r.URL.Path)
	ld := LayoutData{
		Title:         "Analytics",
		ActiveMenu:    state.ActiveSection,
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		CSRFToken:     token,
		Content:       data,
	}
	if data.Notice != "" {
		ld.Flash = data.Notice
	}
	if err := layout.ExecuteTemplate(w, "layout.html", ld); err != nil {
		log.Printf("render analytics: %v", err)
	}
}

// isDatastarRequest is already defined in datastar.go; reuse via same file package.
