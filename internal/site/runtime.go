package site

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/rendering"
)

// Runtime holds an immutable, render-ready snapshot of the site settings. Site
// settings change rarely, so the public hot path reads the snapshot with an
// atomic load instead of querying the database on every request.
type Runtime struct {
	queries  *db.Queries
	reloadMu sync.Mutex
	snapshot atomic.Pointer[Snapshot]
}

// Snapshot is the resolved set of global settings the public frontend needs.
// Expensive parsing (social links, timezone, speculation rules) happens once at
// Reload, not per request.
type Snapshot struct {
	SiteTitle       string
	SiteTagline     string
	SiteURL         string
	Language        string
	TimezoneName    string
	Location        *time.Location
	IndexingEnabled bool
	SitemapEnabled  bool
	RobotsMode      string
	RobotsCustom    string

	SpeculationMode      string
	SpeculationEagerness string
	SpeculationRulesJSON  string

	SocialLinks []rendering.SiteSocialLink

	LogoMediaID         string
	SiteIconMediaID     string
	GlobalSocialMediaID string
	TwitterSite         string

	HomepageMode     string
	HomepageEntryID  string
	PostsPageEntryID string
	PostsBasePath    string
	PostsPerPage     int64

	TitleSeparator string
	SiteRepresents string // "organization" or "person" (structured data publisher)
}

func NewRuntime(queries *db.Queries) *Runtime {
	return &Runtime{queries: queries}
}

// Reload rebuilds the snapshot from the database. It is safe to call from any
// write path; concurrent readers keep seeing the previous snapshot until the new
// one is atomically published.
func (r *Runtime) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	row, err := r.queries.GetSiteSettings(ctx)
	if err != nil {
		return err
	}

	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		loc = time.UTC
	}

	snap := &Snapshot{
		SiteTitle:            row.SiteTitle,
		SiteTagline:          row.SiteTagline,
		SiteURL:              row.SiteUrl,
		Language:             row.Language,
		TimezoneName:         row.Timezone,
		Location:             loc,
		IndexingEnabled:      row.IndexingEnabled != 0,
		SitemapEnabled:       row.SitemapEnabled != 0,
		RobotsMode:           row.RobotsMode,
		RobotsCustom:         row.RobotsCustom,
		SpeculationMode:      row.SpeculationMode,
		SpeculationEagerness: row.SpeculationEagerness,
		HomepageMode:         row.HomepageMode,
		HomepageEntryID:      nullStringToStr(row.HomepageEntryID),
		PostsPageEntryID:     nullStringToStr(row.PostsPageEntryID),
		PostsBasePath:        row.PostsBasePath,
		PostsPerPage:         row.PostsPerPage,
		TitleSeparator:       row.TitleSeparator,
		TwitterSite:          row.TwitterSite,
		SiteRepresents:       row.SiteRepresents,
	}
	if snap.SiteRepresents == "" {
		snap.SiteRepresents = "organization"
	}
	if row.SiteSocialMediaID.Valid {
		snap.GlobalSocialMediaID = row.SiteSocialMediaID.String
	}

	if row.SocialLinks.Valid && row.SocialLinks.String != "" {
		var links []rendering.SiteSocialLink
		if json.Unmarshal([]byte(row.SocialLinks.String), &links) == nil {
			snap.SocialLinks = links
		}
	}

	if row.SpeculationMode != "off" {
		if rules, rulesErr := BuildSpeculationRules(row.SpeculationMode, row.SpeculationEagerness); rulesErr == nil {
			snap.SpeculationRulesJSON = rules
		}
	}

	if row.SiteLogoMediaID.Valid {
		snap.LogoMediaID = row.SiteLogoMediaID.String
	}
	if icon, err := r.queries.GetSiteIconMediaID(ctx); err == nil && icon.Valid {
		snap.SiteIconMediaID = icon.String
	}

	r.snapshot.Store(snap)
	return nil
}

// Current returns the active snapshot. It is never nil after the first Reload.
func (r *Runtime) Current() *Snapshot {
	return r.snapshot.Load()
}

func nullStringToStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}
