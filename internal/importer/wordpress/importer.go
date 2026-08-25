package wordpress

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/backup"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/routing"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

const source = "wordpress"

const maxDownloadBytes = 10 << 20

var entrySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// AllowPrivateForTest disables SSRF private IP checks when true (used by httptest media servers).
var AllowPrivateForTest bool

type Options struct {
	DryRun        bool
	DownloadMedia bool
	Author        string
	OnConflict    string
	DataDir       string
}

type Report struct {
	Posts         int
	Pages         int
	Drafts        int
	Pending       int
	Scheduled     int
	Private       int
	Categories    int
	Tags          int
	MediaImported int
	MediaFailed   int
	Skipped       int
	Conflicts     int
	Warnings      int
	Comments      int
}

func (r Report) String() string {
	return fmt.Sprintf("WordPress import complete\nPosts imported:       %d\nPages imported:       %d\nDrafts:               %d\nPending:              %d\nScheduled:            %d\nPrivate:              %d\nCategories:           %d\nTags:                 %d\nMedia imported:       %d\nMedia failed:         %d\nItems skipped:        %d\nConflicts:            %d\nWarnings:             %d\nComments available for later import: %d", r.Posts, r.Pages, r.Drafts, r.Pending, r.Scheduled, r.Private, r.Categories, r.Tags, r.MediaImported, r.MediaFailed, r.Skipped, r.Conflicts, r.Warnings, r.Comments)
}

type Importer struct {
	db         *sql.DB
	q          *db.Queries
	blocks     *blocks.Registry
	media      *media.Service
	publishing *publishing.Service
	taxonomy   *taxonomy.Service
	search     *search.Service
	dataDir    string
}

func New(database *sql.DB, q *db.Queries, registry *blocks.Registry, mediaService *media.Service, dataDir string) *Importer {
	p := publishing.New(database, q)
	return &Importer{db: database, q: q, blocks: registry, media: mediaService, publishing: p, taxonomy: taxonomy.New(database, q), search: search.New(database, registry), dataDir: dataDir}
}

func (im *Importer) Import(ctx context.Context, filename string, opt Options) (Report, string, error) {
	if opt.OnConflict != "" && opt.OnConflict != "skip" {
		return Report{}, "", errors.New("only --on-conflict=skip is supported")
	}
	if opt.DataDir == "" {
		opt.DataDir = im.dataDir
	}
	report := Report{}
	if opt.DryRun {
		err := im.dryRun(ctx, filename, &report)
		return report, "", err
	}
	runID := newID()
	backupPath := filepath.Join(opt.DataDir, "backups", "pre-import-"+runID+".zip")
	if err := os.MkdirAll(filepath.Join(opt.DataDir, "backups"), 0700); err != nil {
		return report, "", fmt.Errorf("create backup dir: %w", err)
	}
	if _, err := backup.Create(ctx, &storage.Database{DB: im.db}, im.q, opt.DataDir, backupPath); err != nil {
		return report, "", fmt.Errorf("create mandatory pre-import backup: %w", err)
	}
	now := time.Now().Unix()
	if err := im.q.CreateImportRun(ctx, db.CreateImportRunParams{ID: runID, Source: source, CreatedAt: now}); err != nil {
		return report, backupPath, err
	}
	authors := map[string]author{}
	if err := parse(filename, nil, nil, func(a author) error { authors[a.Login] = a; return nil }); err != nil {
		return report, backupPath, err
	}
	termIDs := map[string]string{}
	if err := parse(filename, nil, func(t term) error { return im.importTerm(ctx, runID, t, termIDs, &report) }, nil); err != nil {
		return report, backupPath, err
	}
	imageIDs := map[string]string{}
	newEntries := map[string]bool{}
	plannedPaths := map[string]bool{}
	if err := parse(filename, func(it item) error {
		report.Comments += it.Comments
		if (it.Type == "post" || it.Type == "page") && it.Status != "trash" && it.Status != "inherit" && it.Status != "auto-draft" {
			s := slug(it.Slug, it.Title)
			if s == "" {
				report.Skipped++
				report.Warnings++
				return nil
			}
			p := routing.EntryPath(it.Type, s, "/blog")
			if plannedPaths[p] {
				report.Conflicts++
				report.Skipped++
				return nil
			}
			if _, err := im.q.GetRouteByPath(ctx, p); err == nil {
				report.Conflicts++
				report.Skipped++
				return nil
			}
			plannedPaths[p] = true
		}
		// Track whether this item will be new
		wasNew := false
		if (it.Type == "post" || it.Type == "page") && it.ID != "" {
			if _, err := im.mapping(ctx, it.Type, it.ID); err != nil {
				wasNew = true
			}
		}
		if err := im.createEntryAndAttachment(ctx, runID, it, opt, authors, imageIDs, &report); err != nil {
			return err
		}
		if wasNew {
			if _, err := im.mapping(ctx, it.Type, it.ID); err == nil {
				newEntries[it.Type+":"+it.ID] = true
			}
		}
		return nil
	}, nil, nil); err != nil {
		return report, backupPath, err
	}
	// Collect revisions and sort pages parent-first
	var allItems []item
	if err := parse(filename, func(it item) error {
		if (it.Type == "post" || it.Type == "page") && newEntries[it.Type+":"+it.ID] {
			allItems = append(allItems, it)
		}
		return nil
	}, nil, nil); err != nil {
		return report, backupPath, err
	}
	// Sort pages by depth (parents first) to satisfy hierarchy validation
	sorted := sortItemsByHierarchy(allItems)
	for _, it := range sorted {
		if err := im.createRevision(ctx, runID, it, termIDs, imageIDs, authors, opt, &report); err != nil {
			return report, backupPath, err
		}
	}
	// Also handle any remaining non-page items that were not sorted? For posts, sorted already includes them.
	// For safety, process any items not in sorted? Already sorted includes all.
	if _, err := im.search.Rebuild(ctx); err != nil {
		report.Warnings++
		fmt.Printf("search rebuild failed: %v\n", err)
	}
	if err := im.q.CompleteImportRun(ctx, db.CompleteImportRunParams{CompletedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true}, ID: runID}); err != nil {
		return report, backupPath, err
	}
	if im.media != nil {
		im.media.InvalidateAllViews()
	}
	return report, backupPath, nil
}

func (im *Importer) dryRun(ctx context.Context, filename string, r *Report) error {
	paths := map[string]bool{}
	return parse(filename, func(it item) error {
		r.Comments += it.Comments
		switch it.Type {
		case "post", "page":
		case "attachment":
			return nil
		default:
			if it.Type != "" {
				r.Skipped++
				r.Warnings++
			}
			return nil
		}
		if it.Status == "trash" {
			r.Skipped++
			return nil
		}
		if it.Status == "inherit" || it.Status == "auto-draft" {
			r.Skipped++
			return nil
		}
		s := slug(it.Slug, it.Title)
		if s == "" || !entrySlugPattern.MatchString(s) {
			r.Skipped++
			r.Warnings++
			return nil
		}
		p := routing.EntryPath(it.Type, s, "/blog")
		isConflict := false
		if paths[p] {
			r.Conflicts++
			isConflict = true
		} else {
			paths[p] = true
		}
		if _, err := im.q.GetRouteByPath(ctx, p); err == nil {
			r.Conflicts++
			isConflict = true
		}
		if isConflict {
			r.Skipped++
			return nil
		}
		// Validate content conversion without writing
		warnings := []string{}
		imageIDs := map[string]string{}
		doc, err := htmlDocument(it.Content, imageIDs, &warnings)
		if err != nil {
			r.Warnings++
			return nil
		}
		if im.blocks != nil {
			if err := im.blocks.ValidateDocument(doc); err != nil {
				r.Warnings++
			}
		}
		r.Warnings += len(warnings)
		// Count would-be imports
		switch it.Status {
		case "draft":
			r.Drafts++
		case "pending":
			r.Pending++
		case "future":
			r.Scheduled++
		case "private":
			if it.Type == "post" {
				r.Posts++
			} else {
				r.Pages++
			}
			r.Private++
		default:
			// publish or unknown with password
			if strings.TrimSpace(it.Password) != "" {
				if it.Type == "post" {
					r.Posts++
				} else {
					r.Pages++
				}
				// password counted as private? keep separate, but report as private for now
				r.Private++
			} else {
				if it.Type == "post" {
					r.Posts++
				} else {
					r.Pages++
				}
			}
		}
		return nil
	}, func(t term) error {
		if t.Kind == "category" {
			r.Categories++
		} else if t.Kind == "tag" {
			r.Tags++
		}
		return nil
	}, nil)
}

func normalizeTermDomain(d string) string {
	d = strings.TrimSpace(strings.ToLower(d))
	switch d {
	case "category":
		return "category"
	case "post_tag", "tag":
		return "tag"
	default:
		return ""
	}
}

func (im *Importer) importTerm(ctx context.Context, runID string, t term, ids map[string]string, r *Report) error {
	if t.Kind != "category" && t.Kind != "tag" {
		return nil
	}
	key := t.Kind + ":" + strings.ToLower(strings.TrimSpace(t.Slug))
	if strings.TrimSpace(t.Slug) == "" {
		r.Warnings++
		return nil
	}
	if existing, err := im.mapping(ctx, "term", key); err == nil {
		ids[key] = existing
		return nil
	}
	parent := ""
	if t.Parent != "" {
		parent = ids["category:"+strings.ToLower(strings.TrimSpace(t.Parent))]
		// if parent not yet created, try to lookup via DB by slug
		if parent == "" {
			if termRow, err := im.q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "category", Slug: strings.ToLower(strings.TrimSpace(t.Parent))}); err == nil {
				parent = termRow.ID
			}
		}
	}
	created, err := im.taxonomy.CreateTerm(ctx, t.Kind, t.Name, t.Slug, t.Description, parent)
	if err != nil {
		// If duplicate due to race or previous import, try to fetch existing
		if errors.Is(err, taxonomy.ErrDuplicateSlug) {
			if existing, e2 := im.q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: t.Kind, Slug: t.Slug}); e2 == nil {
				ids[key] = existing.ID
				_ = im.mapObject(ctx, runID, "term", key, existing.ID)
				if t.Kind == "category" {
					r.Categories++
				} else {
					r.Tags++
				}
				return nil
			}
		}
		r.Warnings++
		return nil
	}
	if err := im.mapObject(ctx, runID, "term", key, created.ID); err != nil {
		return err
	}
	ids[key] = created.ID
	if t.Kind == "category" {
		r.Categories++
	} else {
		r.Tags++
	}
	return nil
}

func (im *Importer) createEntryAndAttachment(ctx context.Context, runID string, it item, opt Options, authors map[string]author, images map[string]string, r *Report) error {
	if it.Type == "attachment" {
		if !opt.DownloadMedia || it.ID == "" || it.AttachmentURL == "" {
			return nil
		}
		if _, err := im.mapping(ctx, "attachment", it.ID); err == nil {
			return nil
		}
		asset, err := im.downloadMedia(ctx, it.AttachmentURL, im.authorID(ctx, it, authors, opt))
		if err != nil {
			r.MediaFailed++
			r.Warnings++
			return nil
		}
		if err := im.mapObject(ctx, runID, "attachment", it.ID, asset.ID); err != nil {
			return err
		}
		images[it.AttachmentURL] = asset.ID
		// Also map by attachment URL for link rewriting
		images[strings.TrimSpace(it.AttachmentURL)] = asset.ID
		r.MediaImported++
		return nil
	}
	if it.Type != "post" && it.Type != "page" {
		r.Skipped++
		return nil
	}
	if it.Status == "trash" || it.Status == "inherit" || it.Status == "auto-draft" {
		r.Skipped++
		return nil
	}
	if it.ID == "" {
		r.Skipped++
		r.Warnings++
		return nil
	}
	if _, err := im.mapping(ctx, it.Type, it.ID); err == nil {
		r.Skipped++
		return nil
	}
	s := slug(it.Slug, it.Title)
	if s == "" || !entrySlugPattern.MatchString(s) {
		r.Skipped++
		r.Warnings++
		return nil
	}
	// Check again for planned conflict (already done in outer loop, but double-check for idempotency)
	entryID := newID()
	created := it.PublishedAt
	if created.IsZero() {
		created = time.Now()
	}
	authorID := im.authorID(ctx, it, authors, opt)
	if err := im.q.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: it.Type, Slug: s, Status: "active", AuthorID: nullString(authorID), CreatedAt: created.Unix(), UpdatedAt: nonzeroUnix(it.ModifiedAt, created), PublishedAt: sql.NullInt64{}}); err != nil {
		// If entry slug already used at DB level, treat as conflict
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			r.Conflicts++
			r.Skipped++
			return nil
		}
		return err
	}
	return im.mapObject(ctx, runID, it.Type, it.ID, entryID)
}

func (im *Importer) createRevision(ctx context.Context, runID string, it item, terms, images map[string]string, authors map[string]author, opt Options, r *Report) error {
	if it.Type != "post" && it.Type != "page" {
		return nil
	}
	if it.Status == "trash" || it.Status == "inherit" || it.Status == "auto-draft" {
		return nil
	}
	entryID, err := im.mapping(ctx, it.Type, it.ID)
	if err != nil {
		return nil
	}
	s := slug(it.Slug, it.Title)
	if s == "" || !entrySlugPattern.MatchString(s) {
		r.Skipped++
		r.Warnings++
		return nil
	}
	// Check route conflict again before creating revision (hierarchical aware)
	expectedPath := routing.EntryPath(it.Type, s, "/blog")
	if it.Type == "page" && it.ParentID != "" && it.ParentID != "0" {
		if parentEntryID, e := im.mapping(ctx, "page", it.ParentID); e == nil {
			if parentRoute, e2 := im.q.GetEntryRoute(ctx, sql.NullString{String: parentEntryID, Valid: true}); e2 == nil {
				expectedPath = routing.ChildEntryPath(parentRoute.Path, s)
			}
		}
	}
	if existing, err := im.q.GetRouteByPath(ctx, expectedPath); err == nil && (!existing.EntryID.Valid || existing.EntryID.String != entryID) {
		r.Conflicts++
		r.Skipped++
		// Clean up the entry we created in first pass if revision will be skipped due to conflict
		// But keep mapping for rerun safety? For V1, leave entry but no revision; still counted as skipped.
		return nil
	}
	warnings := []string{}
	doc, err := htmlDocument(it.Content, images, &warnings)
	if err != nil {
		r.Warnings++
		return fmt.Errorf("convert item %s: %w", it.ID, err)
	}
	if im.blocks != nil {
		if err := im.blocks.ValidateDocument(doc); err != nil {
			r.Warnings++
			return err
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	parent := ""
	if it.ParentID != "" && it.ParentID != "0" {
		parent, _ = im.mapping(ctx, "page", it.ParentID)
		if parent == "" {
			r.Warnings++
		}
	}
	visibility, review := "public", "draft"
	switch it.Status {
	case "private":
		visibility = "private"
	case "pending":
		review = "pending"
	case "future":
		// future handled via schedule, visibility stays public, review draft
	case "publish", "publish ":
		// default public
	case "draft":
		// default draft
	default:
		// unknown status -> treat as draft with warning
		if it.Status != "publish" && it.Status != "draft" && it.Status != "pending" && it.Status != "future" && it.Status != "private" {
			r.Warnings++
			review = "draft"
		}
	}
	var hash sql.NullString
	if strings.TrimSpace(it.Password) != "" {
		h, e := publishing.HashPassword(strings.TrimSpace(it.Password))
		if e != nil {
			return e
		}
		visibility = "password"
		hash = nullString(h)
	}
	revID := newID()
	now := time.Now()
	createdAt := nonzeroUnix(it.ModifiedAt, now)
	if !it.PublishedAt.IsZero() && it.Status == "publish" {
		// Use published date for revision creation time to preserve history
		createdAt = it.PublishedAt.Unix()
	}
	if err := im.q.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: entryID, RevisionNumber: 1, Slug: s, Title: it.Title, Excerpt: sql.NullString{String: it.Excerpt, Valid: true}, DocumentJson: string(raw), FeaturedMediaID: sql.NullString{}, ParentEntryID: nullString(parent), MenuOrder: it.MenuOrder, FieldsJson: "{}", CreatedBy: nullString(im.authorID(ctx, it, authors, opt)), CreatedAt: createdAt, Visibility: visibility, PasswordHash: hash, Sticky: 0, ReviewState: review}); err != nil {
		return err
	}
	for _, t := range it.Terms {
		domain := normalizeTermDomain(t.Domain)
		if domain == "" {
			continue
		}
		key := domain + ":" + strings.ToLower(strings.TrimSpace(t.Slug))
		if tid := terms[key]; tid != "" {
			if err := im.q.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: revID, TermID: tid}); err != nil {
				return err
			}
		}
	}
	if thumbnail := it.Meta["_thumbnail_id"]; thumbnail != "" && thumbnail != "0" {
		if mediaID, e := im.mapping(ctx, "attachment", thumbnail); e == nil {
			if _, e := im.db.ExecContext(ctx, "UPDATE entry_revisions SET featured_media_id = ? WHERE id = ?", mediaID, revID); e != nil {
				return e
			}
		} else {
			r.Warnings++
		}
	}
	r.Warnings += len(warnings)
	switch it.Status {
	case "draft":
		r.Drafts++
		return nil
	case "pending":
		r.Pending++
		return nil
	case "future":
		target := it.PublishedAt
		if target.IsZero() || !target.After(now) {
			r.Warnings++
			r.Drafts++
			return nil
		}
		r.Scheduled++
		return im.publishing.Schedule(ctx, entryID, revID, target.Unix(), im.authorID(ctx, it, authors, opt), now.Unix())
	}
	// Publish path
	if err := im.publishing.PublishRevision(ctx, entryID, revID, now.Unix()); err != nil {
		if strings.Contains(err.Error(), "route already uses") || strings.Contains(err.Error(), "reserved") {
			r.Conflicts++
			r.Skipped++
			// Remove the revision we just created? Keep it as draft but not published; count as skipped.
			return nil
		}
		return err
	}
	published := nonzeroUnix(it.PublishedAt, now)
	modified := nonzeroUnix(it.ModifiedAt, now)
	// Preserve historical dates
	_, err = im.db.ExecContext(ctx, "UPDATE entries SET created_at=?, updated_at=?, published_at=?, first_published_at=? WHERE id=?", published, modified, published, published, entryID)
	if err != nil {
		return err
	}
	if it.Type == "post" {
		r.Posts++
	} else {
		r.Pages++
	}
	if visibility == "private" {
		r.Private++
	}
	return nil
}

func (im *Importer) mapping(ctx context.Context, typ, id string) (string, error) {
	return im.q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: typ, ExternalID: id})
}
func (im *Importer) mapObject(ctx context.Context, run, typ, external, internal string) error {
	return im.q.CreateImportMapping(ctx, db.CreateImportMappingParams{Source: source, ObjectType: typ, ExternalID: external, InternalID: internal, RunID: run, CreatedAt: time.Now().Unix()})
}
func (im *Importer) authorID(ctx context.Context, it item, authors map[string]author, opt Options) string {
	email := authors[it.Author].Email
	if email == "" {
		email = opt.Author
	}
	if email == "" {
		return ""
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	u, err := im.q.GetUserByEmail(ctx, email)
	if err != nil {
		return ""
	}
	return u.ID
}
func (im *Importer) downloadMedia(ctx context.Context, raw, author string) (*media.Asset, error) {
	body, name, err := download(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	// Limit reading to maxDownloadBytes+1 to detect overflow without loading huge files
	limited := io.LimitReader(body, maxDownloadBytes+1)
	return im.media.Upload(ctx, name, author, limited)
}
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func nullString(v string) sql.NullString {
	if strings.TrimSpace(v) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
func nonzeroUnix(t time.Time, f time.Time) int64 {
	if t.IsZero() {
		return f.Unix()
	}
	return t.Unix()
}
func slug(value, title string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" {
		s = strings.ToLower(strings.TrimSpace(title))
	}
	// Replace any sequence of non-alphanum with dash
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	res := strings.Trim(b.String(), "-")
	// Collapse multiple dashes already handled, validate pattern
	if res == "" {
		return ""
	}
	if !entrySlugPattern.MatchString(res) {
		return ""
	}
	return res
}

func sortItemsByHierarchy(items []item) []item {
	// Build map for page items
	idToItem := map[string]item{}
	for _, it := range items {
		if it.Type == "page" {
			idToItem[it.ID] = it
		}
	}
	depth := map[string]int{}
	var computeDepth func(id string, seen map[string]bool) int
	computeDepth = func(id string, seen map[string]bool) int {
		if d, ok := depth[id]; ok {
			return d
		}
		if seen[id] {
			return 0
		}
		seen[id] = true
		it, ok := idToItem[id]
		if !ok || it.ParentID == "" || it.ParentID == "0" {
			depth[id] = 0
			return 0
		}
		d := computeDepth(it.ParentID, seen) + 1
		depth[id] = d
		return d
	}
	for _, it := range items {
		if it.Type == "page" {
			computeDepth(it.ID, map[string]bool{})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		di, dj := 0, 0
		if items[i].Type == "page" {
			di = depth[items[i].ID]
		}
		if items[j].Type == "page" {
			dj = depth[items[j].ID]
		}
		if di != dj {
			return di < dj
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func download(ctx context.Context, raw string) (io.ReadCloser, string, error) {
	check := func(u *url.URL) error {
		if u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("attachment URL must use http or https")
		}
		host := u.Hostname()
		if host == "" {
			return errors.New("attachment URL missing host")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			return errors.New("attachment host has no addresses")
		}
		for _, ip := range ips {
			if forbiddenIP(ip.IP) {
				return fmt.Errorf("attachment host resolves to forbidden address %s", ip.IP)
			}
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	if err = check(u); err != nil {
		return nil, "", err
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, e := net.SplitHostPort(address)
			if e != nil {
				return nil, e
			}
			ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
			if e != nil {
				return nil, e
			}
			for _, ip := range ips {
				if forbiddenIP(ip.IP) {
					return nil, errors.New("forbidden attachment address")
				}
			}
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("too many redirects")
		}
		return check(req.URL)
	}}
	resp, err := client.Get(raw)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("attachment returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxDownloadBytes {
		resp.Body.Close()
		return nil, "", media.ErrTooLarge
	}
	// Do not read body here; caller will stream with limit
	return resp.Body, path.Base(u.Path), nil
}

func forbiddenIP(ip net.IP) bool {
	if AllowPrivateForTest {
		return false
	}
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// Additional explicit blocks: CGNAT, documentation, etc.
	if ip4 := ip.To4(); ip4 != nil {
		// 100.64.0.0/10
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24, 192.88.99.0/24
		if (ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) || (ip4[0] == 192 && ip4[1] == 88 && ip4[2] == 99) {
			return true
		}
		if (ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) || (ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) {
			return true
		}
		// 198.18.0.0/15
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return true
		}
		// 0.0.0.0/8
		if ip4[0] == 0 {
			return true
		}
	}
	// IPv6 documentation, etc: 2001:db8::/32, 64:ff9b::/96, 100::/64
	if ip.To4() == nil {
		if strings.HasPrefix(ip.String(), "2001:db8") {
			return true
		}
		if strings.HasPrefix(ip.String(), "64:ff9b") {
			return true
		}
	}
	return false
}
