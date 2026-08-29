package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type Severity string

const (
	SeverityGood     Severity = "good"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type CheckResult struct {
	ID          string
	Severity    Severity
	Title       string
	Description string
	ActionURL   string
	ActionLabel string
}

type IntegrityIssue struct {
	SourceType    string
	SourceID      string
	SourceLabel   string
	ReferenceType string
	Target        string
	Message       string
	EditURL       string
}

type Service struct {
	db      *sql.DB
	queries *db.Queries
}

func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

// entryEditURL resolves the canonical admin edit path for an entry using its content type.
func entryEditURL(contentTypeID, entryID string) string {
	switch contentTypeID {
	case "page":
		return "/admin/pages/" + entryID + "/edit"
	case "post":
		return "/admin/posts/" + entryID + "/edit"
	case "":
		return "/admin/pages/" + entryID + "/edit"
	default:
		return "/admin/content/" + contentTypeID + "/" + entryID + "/edit"
	}
}

// Run executes all health checks synchronously. It loads lookup maps once.
func (s *Service) Run(ctx context.Context) ([]CheckResult, []IntegrityIssue, error) {
	var results []CheckResult
	var issues []IntegrityIssue

	settings, err := s.queries.GetSiteSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	// Build lookup maps — any DB failure here must not masquerade as missing content.
	routes, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load routes: %w", err)
	}
	routesByPath := make(map[string]db.Route, len(routes))
	redirectTargets := make(map[string]string)
	for _, r := range routes {
		routesByPath[r.Path] = r
		if r.RouteType == "redirect" && r.RedirectTo.Valid {
			redirectTargets[r.Path] = r.RedirectTo.String
		}
	}
	_ = redirectTargets
	// Redirect diagnostics via internal/redirects logic (inline to avoid import cycle)
	redirectLoopIssues := detectLoops(routes)
	redirectChainIssues := detectChains(routes)

	// Load media, forms, templates, site parts lookup — fail cleanly on DB error
	mediaMap, err := s.loadMediaMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load media: %w", err)
	}
	formMap, err := s.loadFormMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load forms: %w", err)
	}
	sitePartMap, err := s.loadSitePartMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load site parts: %w", err)
	}
	templateMap, err := s.loadTemplateMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load templates: %w", err)
	}
	entryPublishedMap, err := s.loadPublishedEntryMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load entries: %w", err)
	}

	// 1: Site URL
	if strings.TrimSpace(settings.SiteUrl) == "" {
		results = append(results, CheckResult{ID: "site_url", Severity: SeverityWarning, Title: "Site URL is not configured", Description: "Canonical links and sitemap URLs may be incomplete.", ActionURL: "/admin/settings/general", ActionLabel: "Configure Site URL"})
	} else {
		results = append(results, CheckResult{ID: "site_url", Severity: SeverityGood, Title: "Site URL is configured", Description: "Canonical links and sitemap use " + settings.SiteUrl, ActionURL: "/admin/settings/general", ActionLabel: "Settings"})
	}
	// 2: Indexing
	if settings.IndexingEnabled == 0 {
		results = append(results, CheckResult{ID: "indexing", Severity: SeverityWarning, Title: "Search engine indexing is disabled", Description: "This site asks search engines not to index it. Remove the noindex if this is unintentional.", ActionURL: "/admin/settings/seo", ActionLabel: "SEO settings"})
	} else {
		results = append(results, CheckResult{ID: "indexing", Severity: SeverityGood, Title: "Indexing enabled", Description: "Search engines are allowed to index this site."})
	}
	// 3: Sitemap
	if settings.SitemapEnabled == 0 {
		results = append(results, CheckResult{ID: "sitemap", Severity: SeverityWarning, Title: "Sitemap is disabled", Description: "XML sitemap is not generated.", ActionURL: "/admin/settings/seo", ActionLabel: "SEO settings"})
	} else if strings.TrimSpace(settings.SiteUrl) == "" {
		results = append(results, CheckResult{ID: "sitemap", Severity: SeverityWarning, Title: "Sitemap enabled without Site URL", Description: "Sitemap URLs will be incomplete without Site URL.", ActionURL: "/admin/settings/general", ActionLabel: "Configure Site URL"})
	} else {
		// Try internal generation via queries (ListSitemapEntries) – check does not fail
		if _, err := s.queries.ListSitemapEntries(ctx); err != nil {
			results = append(results, CheckResult{ID: "sitemap", Severity: SeverityCritical, Title: "Sitemap generation failed", Description: err.Error(), ActionURL: "/sitemap.xml", ActionLabel: "View sitemap"})
		} else {
			results = append(results, CheckResult{ID: "sitemap", Severity: SeverityGood, Title: "Sitemap is enabled", Description: "Sitemap will be generated at /sitemap.xml."})
		}
	}
	// 4: Homepage
	if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String != "" {
		if entry, err := s.queries.GetEntry(ctx, settings.HomepageEntryID.String); err != nil {
			results = append(results, CheckResult{ID: "homepage", Severity: SeverityCritical, Title: "Homepage references missing entry", Description: "Homepage entry " + settings.HomepageEntryID.String + " does not exist.", ActionURL: "/admin/settings/reading", ActionLabel: "Reading settings"})
		} else if entry.Status == "trash" || !entry.PublishedRevisionID.Valid {
			results = append(results, CheckResult{ID: "homepage", Severity: SeverityWarning, Title: "Homepage is not published", Description: "Homepage entry exists but is not published or is in trash.", ActionURL: "/admin/pages/" + entry.ID + "/edit", ActionLabel: "Edit page"})
		} else if _, ok := routesByPath["/"]; !ok {
			results = append(results, CheckResult{ID: "homepage", Severity: SeverityWarning, Title: "Homepage route missing", Description: "Homepage is configured but no route resolves at /.", ActionURL: "/admin/settings/reading", ActionLabel: "Reading settings"})
		} else {
			results = append(results, CheckResult{ID: "homepage", Severity: SeverityGood, Title: "Homepage route resolves", Description: "Homepage is published and route at / is active."})
		}
	} else {
		// No static homepage, that's fine – homepageMode latest_posts
		results = append(results, CheckResult{ID: "homepage", Severity: SeverityGood, Title: "Homepage displays latest posts", Description: "No static homepage configured."})
	}
	// 5: Posts page
	if settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String != "" {
		if entry, err := s.queries.GetEntry(ctx, settings.PostsPageEntryID.String); err != nil {
			results = append(results, CheckResult{ID: "posts_page", Severity: SeverityCritical, Title: "Posts page references missing entry", Description: "Posts page entry does not exist.", ActionURL: "/admin/settings/reading", ActionLabel: "Reading settings"})
		} else if entry.Status == "trash" || !entry.PublishedRevisionID.Valid {
			results = append(results, CheckResult{ID: "posts_page", Severity: SeverityWarning, Title: "Posts page is not published", Description: "Posts page entry is not published or is in trash.", ActionURL: "/admin/pages/" + entry.ID + "/edit", ActionLabel: "Edit page"})
		} else {
			results = append(results, CheckResult{ID: "posts_page", Severity: SeverityGood, Title: "Posts page is configured", Description: "Posts page is published."})
		}
	}
	// 6: Default templates dangling
	cts, err := s.queries.ListContentTypes(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load content types: %w", err)
	}
	for _, ct := range cts {
		if ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String != "" {
			if _, ok := templateMap[ct.DefaultLayoutTemplateID.String]; !ok {
				results = append(results, CheckResult{ID: "default_template_" + ct.ID, Severity: SeverityWarning, Title: "Default template missing for " + ct.PluralName, Description: "Default template " + ct.DefaultLayoutTemplateID.String + " does not exist.", ActionURL: "/admin/appearance/templates", ActionLabel: "Templates"})
			}
		}
		if ct.DefaultArchiveTemplateID.Valid && ct.DefaultArchiveTemplateID.String != "" {
			if _, ok := templateMap[ct.DefaultArchiveTemplateID.String]; !ok {
				results = append(results, CheckResult{ID: "default_archive_" + ct.ID, Severity: SeverityWarning, Title: "Default archive template missing for " + ct.PluralName, Description: "Archive template " + ct.DefaultArchiveTemplateID.String + " does not exist.", ActionURL: "/admin/appearance/templates", ActionLabel: "Templates"})
			}
		}
	}
	// 7: Redirect loops/chains
	for _, loop := range redirectLoopIssues {
		results = append(results, CheckResult{ID: "redirect_loop_" + strings.Join(loop, "_"), Severity: SeverityCritical, Title: "Redirect loop detected", Description: strings.Join(loop, " → "), ActionURL: "/admin/tools/redirects", ActionLabel: "Review Redirects"})
	}
	for _, chain := range redirectChainIssues {
		if len(chain) >= 3 {
			results = append(results, CheckResult{ID: "redirect_chain_" + strings.Join(chain, "_"), Severity: SeverityWarning, Title: "Redirect chain detected", Description: strings.Join(chain, " → "), ActionURL: "/admin/tools/redirects", ActionLabel: "Fix chain"})
		}
	}
	// 8: Redirect targets broken (internal)
	for _, r := range routes {
		if r.RouteType != "redirect" || !r.RedirectTo.Valid {
			continue
		}
		target := r.RedirectTo.String
		if !strings.HasPrefix(target, "/") {
			continue // external is syntactically valid
		}
		// For internal target with query, check the PATH only
		targetPath := target
		if idx := strings.Index(target, "?"); idx != -1 {
			targetPath = target[:idx]
		}
		if idx := strings.Index(targetPath, "#"); idx != -1 {
			targetPath = targetPath[:idx]
		}
		targetPath = strings.TrimSpace(targetPath)
		if targetPath == "" {
			targetPath = target
		}
		// Normalize path for lookup like routes are stored normalized
		// Use simple NormalizePath via routing if available, otherwise direct compare
		// We use targetPath as stored; routes are already normalized.
		if _, ok := routesByPath[targetPath]; ok {
			continue
		}
		// Also check if target is a redirect itself? Already counted as route, so internal redirect target exists via redirect route, which is valid (second hop)
		// But if not found, it's broken unless redirect target is valid page that is not published? We treat missing as warning
		results = append(results, CheckResult{ID: "redirect_target_" + r.Path, Severity: SeverityWarning, Title: "Redirect points to missing path", Description: r.Path + " → " + target + " target does not resolve.", ActionURL: "/admin/tools/redirects", ActionLabel: "Edit redirect"})
	}
	// 9: Menu integrity
	menuIssues := s.checkMenus(ctx, entryPublishedMap, routesByPath)
	for _, iss := range menuIssues {
		results = append(results, CheckResult{ID: "menu_" + iss.SourceID + "_" + iss.Target, Severity: SeverityWarning, Title: iss.Message, Description: "Menu references missing or unpublished entry.", ActionURL: iss.EditURL, ActionLabel: "Edit menu"})
		issues = append(issues, iss)
	}
	// 10: Media, Forms, SiteParts, Templates scanning via SDT
	integrityIssues := s.scanSDT(ctx, mediaMap, formMap, sitePartMap, templateMap, entryPublishedMap, routesByPath)
	issues = append(issues, integrityIssues...)
	for _, iss := range integrityIssues {
		sev := SeverityWarning
		if strings.Contains(iss.SourceType, "published") || strings.Contains(iss.ReferenceType, "media") {
			sev = SeverityWarning
		}
		results = append(results, CheckResult{ID: "integrity_" + iss.SourceID + "_" + iss.Target, Severity: sev, Title: iss.Message, Description: fmt.Sprintf("%s %s references missing %s %s", iss.SourceType, iss.SourceLabel, iss.ReferenceType, iss.Target), ActionURL: iss.EditURL, ActionLabel: "Edit " + iss.SourceType})
	}

	// Order: critical, warning, good, then deterministic
	ordered := make([]CheckResult, 0, len(results))
	for _, sev := range []Severity{SeverityCritical, SeverityWarning, SeverityGood} {
		for _, r := range results {
			if r.Severity == sev {
				ordered = append(ordered, r)
			}
		}
	}
	return ordered, issues, nil
}

func (s *Service) loadMediaMap(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM media`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			m[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) loadFormMap(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM forms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			m[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) loadSitePartMap(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM site_parts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			m[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) loadTemplateMap(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM layout_templates`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			m[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) loadPublishedEntryMap(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM entries WHERE status='active' AND published_revision_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			m[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) checkMenus(ctx context.Context, publishedEntryMap map[string]bool, routesByPath map[string]db.Route) []IntegrityIssue {
	var issues []IntegrityIssue
	rows2, err := s.db.QueryContext(ctx, `SELECT ni.id, ni.menu_id, ni.label, ni.entry_id, m.name as menu_name FROM navigation_items ni LEFT JOIN navigation_menus m ON m.id=ni.menu_id WHERE ni.target_type='entry'`)
	if err != nil {
		return issues
	}
	defer rows2.Close()
	for rows2.Next() {
		var nid, mid, label, eid, menuName sql.NullString
		if err := rows2.Scan(&nid, &mid, &label, &eid, &menuName); err != nil {
			continue
		}
		if !eid.Valid || eid.String == "" {
			continue
		}
		if _, ok := publishedEntryMap[eid.String]; !ok {
			// Check if entry exists but unpublished
			var exists bool
			_ = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entries WHERE id=?)`, eid.String).Scan(&exists)
			msg := fmt.Sprintf("Menu “%s” links to unpublished Page “%s”.", menuName.String, label.String)
			if !exists {
				msg = fmt.Sprintf("Menu “%s” links to missing entry “%s”.", menuName.String, label.String)
			}
			issues = append(issues, IntegrityIssue{
				SourceType: "menu", SourceID: mid.String, SourceLabel: menuName.String,
				ReferenceType: "entry", Target: eid.String, Message: msg, EditURL: "/admin/menus?menu=" + mid.String,
			})
		}
	}
	return issues
}

func (s *Service) scanSDT(ctx context.Context, mediaMap, formMap, sitePartMap, templateMap map[string]bool, publishedMap map[string]bool, routesByPath map[string]db.Route) []IntegrityIssue {
	var issues []IntegrityIssue
	// Helper to walk document
	walk := func(doc *document.Document, fn func(node document.Node)) {
		var rec func([]document.Node)
		rec = func(nodes []document.Node) {
			for _, n := range nodes {
				fn(n)
				if len(n.Children) > 0 {
					rec(n.Children)
				}
			}
		}
		rec(doc.Nodes)
	}
	// Collect all SDT sources: entries published + drafts? Default to published for health, but also check draft issues?
	// For Epic5, prioritize public output.
	// Entries published revisions
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, r.title, r.document_json, r.featured_media_id, r.social_media_id, r.layout_template_id, e.content_type_id
		FROM entries e JOIN entry_revisions r ON r.id = e.published_revision_id
		WHERE e.status='active' AND e.published_revision_id IS NOT NULL`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var eid, title, docJSON, feat, social, tmpl, ct sql.NullString
			var featS, socialS, tmplS string
			_ = feat
			_ = social
			_ = tmpl
			if err := rows.Scan(&eid, &title, &docJSON, &featS, &socialS, &tmplS, &ct); err != nil {
				continue
			}
			// Check featured/social/template refs
			if featS != "" && !mediaMap[featS] {
				issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "media", Target: featS, Message: "Featured image references missing media.", EditURL: entryEditURL(ct.String, eid.String)})
			}
			if socialS != "" && !mediaMap[socialS] {
				issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "media", Target: socialS, Message: "Social image references missing media.", EditURL: entryEditURL(ct.String, eid.String)})
			}
			if tmplS != "" && !templateMap[tmplS] {
				issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "template", Target: tmplS, Message: "Entry references missing template.", EditURL: entryEditURL(ct.String, eid.String)})
			}
			// SDT refs
			if docJSON.Valid && docJSON.String != "" {
				doc, err := document.Decode([]byte(docJSON.String))
				if err == nil && doc != nil {
					walk(doc, func(n document.Node) {
						// Decode props/settings maps
						var props map[string]any
						if len(n.Props) > 0 {
							_ = json.Unmarshal(n.Props, &props)
						}
						var settings map[string]any
						if len(n.Settings) > 0 {
							_ = json.Unmarshal(n.Settings, &settings)
						}
						switch n.Block {
						case "core/image":
							if props != nil {
								if mid, ok := props["mediaId"].(string); ok && mid != "" && !mediaMap[mid] {
									issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "media", Target: mid, Message: "Image block references missing media.", EditURL: entryEditURL(ct.String, eid.String)})
								}
							}
						case "core/gallery":
							if props != nil {
								if ids, ok := props["mediaIds"].([]any); ok {
									for _, v := range ids {
										if mid, ok := v.(string); ok && mid != "" && !mediaMap[mid] {
											issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "media", Target: mid, Message: "Gallery references missing media.", EditURL: entryEditURL(ct.String, eid.String)})
										}
									}
								}
							}
						case "core/form":
							if settings != nil {
								if fid, ok := settings["formId"].(string); ok && fid != "" && !formMap[fid] {
									issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "form", Target: fid, Message: "Form block references missing form.", EditURL: entryEditURL(ct.String, eid.String)})
								}
							}
						case "core/site-part":
							if settings != nil {
								if sid, ok := settings["sitePartId"].(string); ok && sid != "" && !sitePartMap[sid] {
									issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "sitePart", Target: sid, Message: "Site part block references missing site part.", EditURL: entryEditURL(ct.String, eid.String)})
								}
							}
						case "core/button":
							if props != nil {
								if href, ok := props["href"].(string); ok && strings.HasPrefix(href, "/") {
									if _, ok := routesByPath[href]; !ok {
										// Also allow redirect
										issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "internal_link", Target: href, Message: "Button links to missing path " + href, EditURL: entryEditURL(ct.String, eid.String)})
									}
								}
							}
						}
						// Richtext link marks - props.text may be richtext json with link hrefs starting with /
						if props != nil {
							if txt, ok := props["text"]; ok {
								// txt could be string or object with version/content
								b, _ := json.Marshal(txt)
								var rt struct {
									Version int `json:"version"`
									Content []struct {
										Text  string `json:"text"`
										Marks []struct {
											Type string `json:"type"`
											Href string `json:"href"`
										} `json:"marks"`
									} `json:"content"`
								}
								if err := json.Unmarshal(b, &rt); err == nil && rt.Version == 1 {
									for _, run := range rt.Content {
										for _, mk := range run.Marks {
											if mk.Type == "link" && strings.HasPrefix(mk.Href, "/") {
												if _, ok := routesByPath[mk.Href]; !ok {
													issues = append(issues, IntegrityIssue{SourceType: "entry", SourceID: eid.String, SourceLabel: title.String, ReferenceType: "internal_link", Target: mk.Href, Message: "Text link to missing path " + mk.Href, EditURL: entryEditURL(ct.String, eid.String)})
												}
											}
										}
									}
								}
							}
						}
					})
				}
			}
		}
	}
	// Site icon / logo / social
	var siteIcon, siteLogo string
	_ = s.db.QueryRowContext(ctx, `SELECT site_icon_media_id, site_logo_media_id FROM site_settings WHERE id=1`).Scan(&siteIcon, &siteLogo)
	// Actually site_icon_media_id may be in site_settings via other column; we skip detailed.

	// Templates scanning for missing media/form/sitePart/internal links
	tRows, _ := s.db.QueryContext(ctx, `SELECT ltr.document_json, lt.id, lt.name FROM layout_template_revisions ltr JOIN layout_templates lt ON lt.published_revision_id = ltr.id`)
	if tRows != nil {
		defer tRows.Close()
		for tRows.Next() {
			var docJSON, tid, tname string
			if err := tRows.Scan(&docJSON, &tid, &tname); err != nil {
				continue
			}
			doc, err := document.Decode([]byte(docJSON))
			if err != nil || doc == nil {
				continue
			}
			walk(doc, func(n document.Node) {
				var props map[string]any
				var settings map[string]any
				if len(n.Props) > 0 {
					_ = json.Unmarshal(n.Props, &props)
				}
				if len(n.Settings) > 0 {
					_ = json.Unmarshal(n.Settings, &settings)
				}
				switch n.Block {
				case "core/image":
					if props != nil {
						if mid, ok := props["mediaId"].(string); ok && mid != "" && !mediaMap[mid] {
							issues = append(issues, IntegrityIssue{SourceType: "template", SourceID: tid, SourceLabel: tname, ReferenceType: "media", Target: mid, Message: "Image block references missing media.", EditURL: "/admin/appearance/templates/" + tid + "/edit"})
						}
					}
				case "core/form":
					if settings != nil {
						if fid, ok := settings["formId"].(string); ok && fid != "" && !formMap[fid] {
							issues = append(issues, IntegrityIssue{SourceType: "template", SourceID: tid, SourceLabel: tname, ReferenceType: "form", Target: fid, Message: "Form block references missing form.", EditURL: "/admin/appearance/templates/" + tid + "/edit"})
						}
					}
				case "core/site-part":
					if settings != nil {
						if sid, ok := settings["sitePartId"].(string); ok && sid != "" && !sitePartMap[sid] {
							issues = append(issues, IntegrityIssue{SourceType: "template", SourceID: tid, SourceLabel: tname, ReferenceType: "sitePart", Target: sid, Message: "Site part block references missing site part.", EditURL: "/admin/appearance/templates/" + tid + "/edit"})
						}
					}
				}
			})
		}
	}
	// Site parts
	spRows, _ := s.db.QueryContext(ctx, `SELECT spr.document_json, sp.id, sp.name FROM site_part_revisions spr JOIN site_parts sp ON sp.published_revision_id = spr.id`)
	if spRows != nil {
		defer spRows.Close()
		for spRows.Next() {
			var docJSON, sid, sname string
			if err := spRows.Scan(&docJSON, &sid, &sname); err != nil {
				continue
			}
			doc, err := document.Decode([]byte(docJSON))
			if err != nil || doc == nil {
				continue
			}
			walk(doc, func(n document.Node) {
				var props map[string]any
				var settings map[string]any
				if len(n.Props) > 0 {
					_ = json.Unmarshal(n.Props, &props)
				}
				if len(n.Settings) > 0 {
					_ = json.Unmarshal(n.Settings, &settings)
				}
				if n.Block == "core/image" && props != nil {
					if mid, ok := props["mediaId"].(string); ok && mid != "" && !mediaMap[mid] {
						issues = append(issues, IntegrityIssue{SourceType: "sitePart", SourceID: sid, SourceLabel: sname, ReferenceType: "media", Target: mid, Message: "Image references missing media.", EditURL: "/admin/appearance/site-parts/" + sid + "/edit"})
					}
				}
			})
		}
	}

	return issues
}

func detectLoops(routes []db.Route) [][]string {
	graph := make(map[string]string)
	for _, r := range routes {
		if r.RouteType == "redirect" && r.RedirectTo.Valid && len(r.RedirectTo.String) > 0 && r.RedirectTo.String[0] == '/' {
			graph[r.Path] = r.RedirectTo.String
		}
	}
	var loops [][]string
	visited := map[string]bool{}
	for node := range graph {
		if visited[node] {
			continue
		}
		path := []string{}
		seen := map[string]int{}
		cur := node
		for i := 0; i < 32; i++ {
			if idx, ok := seen[cur]; ok {
				loop := append([]string{}, path[idx:]...)
				loop = append(loop, cur)
				loops = append(loops, loop)
				break
			}
			seen[cur] = len(path)
			path = append(path, cur)
			visited[cur] = true
			next, ok := graph[cur]
			if !ok {
				break
			}
			cur = next
		}
	}
	return loops
}

func detectChains(routes []db.Route) [][]string {
	graph := make(map[string]string)
	for _, r := range routes {
		if r.RouteType == "redirect" && r.RedirectTo.Valid && r.RedirectTo.String[0] == '/' {
			graph[r.Path] = r.RedirectTo.String
		}
	}
	// Find heads
	isTarget := make(map[string]bool)
	for _, t := range graph {
		isTarget[t] = true
	}
	var chains [][]string
	for start := range graph {
		if isTarget[start] {
			continue
		}
		chain := []string{start}
		cur := start
		seen := map[string]bool{start: true}
		for i := 0; i < 32; i++ {
			next, ok := graph[cur]
			if !ok {
				break
			}
			if seen[next] {
				break
			}
			chain = append(chain, next)
			seen[next] = true
			cur = next
		}
		if len(chain) >= 3 {
			chains = append(chains, chain)
		}
	}
	return chains
}
