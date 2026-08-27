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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/backup"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/comments"
	"github.com/kokosx/stratum/internal/datalock"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/routing"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

const source = "wordpress"

var entrySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Options struct {
	DryRun        bool
	DownloadMedia bool
	Author        string
	OnConflict    string
	DataDir       string
}

type Report struct {
	Posts            int
	Pages            int
	Drafts           int
	Pending          int
	Scheduled        int
	Private          int
	Password         int
	Published        int
	Categories       int
	Tags             int
	MediaImported    int
	MediaFailed      int
	Skipped          int
	Conflicts        int
	Warnings         int
	Comments         int
	CommentsImported int
	CommentsSkipped  int
	PingbacksSkipped int
}

func (r Report) String() string {
	base := fmt.Sprintf("WordPress import complete\nPosts imported:       %d\nPages imported:       %d\nDrafts:               %d\nPending:              %d\nScheduled:            %d\nPrivate:              %d\nPassword:             %d\nPublished:            %d\nCategories:           %d\nTags:                 %d\nMedia imported:       %d\nMedia failed:         %d\nItems skipped:        %d\nConflicts:            %d\nWarnings:             %d", r.Posts, r.Pages, r.Drafts, r.Pending, r.Scheduled, r.Private, r.Password, r.Published, r.Categories, r.Tags, r.MediaImported, r.MediaFailed, r.Skipped, r.Conflicts, r.Warnings)
	base += fmt.Sprintf("\nComments available for later import: %d\nComments imported:    %d\nComments skipped:     %d\nPingbacks skipped:    %d", r.Comments, r.CommentsImported, r.CommentsSkipped, r.PingbacksSkipped)
	return base
}

type Importer struct {
	db         *sql.DB
	q          *db.Queries
	blocks     *blocks.Registry
	media      *media.Service
	publishing *publishing.Service
	taxonomy   *taxonomy.Service
	search     *search.Service
	comments   *comments.Service
	dataDir    string
	// Downloader overrides the SSRF-hardened attachment fetcher when non-nil
	// (test seam). Production always uses newDownloader(); there is no global bypass.
	Downloader *Downloader
}

func New(database *sql.DB, q *db.Queries, registry *blocks.Registry, mediaService *media.Service, dataDir string) *Importer {
	p := publishing.New(database, q)
	return &Importer{db: database, q: q, blocks: registry, media: mediaService, publishing: p, taxonomy: taxonomy.New(database, q), search: search.New(database, registry), comments: comments.NewService(database, q), dataDir: dataDir}
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
	// Non-dry-run import must hold exclusive dataDir lock (see internal/datalock).
	// Ownership: the importer owns the lock for the whole mutation phase; the CLI
	// never acquires it separately, so there is no double-acquire risk.
	lock, err := datalock.Acquire(opt.DataDir)
	if err != nil {
		return Report{}, "", fmt.Errorf("cannot import while Stratum is serving this data directory: %w", err)
	}
	defer lock.Close()

	runID, err := newID()
	if err != nil {
		return report, "", err
	}
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
	// --author fail-fast validation before any mutation.
	if opt.Author != "" && !opt.DryRun {
		email := strings.ToLower(strings.TrimSpace(opt.Author))
		if _, err := im.q.GetUserByEmail(ctx, email); err != nil {
			return report, backupPath, fmt.Errorf("fallback author %q does not exist in Stratum; create the user first", email)
		}
	}

	plan, err := im.planRoutes(ctx, filename)
	if err != nil {
		return report, backupPath, err
	}
	for wpID, perr := range plan.errors {
		report.Warnings++
		fmt.Printf("route plan rejected %s: %v\n", wpID, perr)
	}

	termIDs, termSlugIndex := map[string]string{}, map[string]string{}
	if err := parse(filename, nil, func(t term) error { return im.importTerm(ctx, runID, t, termIDs, termSlugIndex, &report) }, nil); err != nil {
		return report, backupPath, err
	}
	// Taxonomy PASS 2: resolve category parents now that every mapping exists.
	if err := im.resolveTermParents(ctx, filename, &report); err != nil {
		return report, backupPath, err
	}

	imageIDs := map[string]string{}
	newEntries := map[string]bool{}
	if err := parse(filename, func(it item) error {
		if (it.Type == "post" || it.Type == "page") && it.Status != "trash" && it.Status != "inherit" && it.Status != "auto-draft" {
			if perr, rejected := plan.errors[it.ID]; rejected {
				report.Conflicts++
				report.Skipped++
				_ = perr
				return nil
			}
			if s := slug(it.Slug, it.Title); s == "" {
				report.Skipped++
				report.Warnings++
				return nil
			}
		}
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
	var allItems []item
	if err := parse(filename, func(it item) error {
		if it.Type != "post" && it.Type != "page" {
			return nil
		}
		if it.Status == "trash" || it.Status == "inherit" || it.Status == "auto-draft" {
			return nil
		}
		if newEntries[it.Type+":"+it.ID] {
			allItems = append(allItems, it)
			return nil
		}
		// Resume semantics: a mapping from a previous PARTIAL run (entry shell
		// exists but no revision was ever written) must not permanently skip
		// the item. Completed items keep skipping untouched.
		if entryID, err := im.mapping(ctx, it.Type, it.ID); err == nil && !im.entryHasRevision(ctx, entryID) {
			allItems = append(allItems, it)
		}
		return nil
	}, nil, nil); err != nil {
		return report, backupPath, err
	}
	sorted := sortItemsByHierarchy(allItems)
	for _, it := range sorted {
		if err := im.createRevision(ctx, runID, it, termIDs, termSlugIndex, imageIDs, authors, opt, &report); err != nil {
			return report, backupPath, err
		}
	}
	if im.comments != nil {
		if err := im.importComments(ctx, filename, runID, &report); err != nil {
			report.Warnings++
			fmt.Printf("comment import failed: %v\n", err)
		}
	}
	if _, err := im.search.Rebuild(ctx); err != nil {
		report.Warnings++
		fmt.Println("search rebuild failed; run: stratum search rebuild")
	}
	if err := im.q.CompleteImportRun(ctx, db.CompleteImportRunParams{CompletedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true}, ID: runID}); err != nil {
		return report, backupPath, err
	}
	if im.media != nil {
		im.media.InvalidateAllViews()
	}
	return report, backupPath, nil
}

// routePlan is the shared pre-mutation plan used by both dry-run and real import.
type routePlan struct {
	effective map[string]string // WP ID -> prospective public path
	errors    map[string]error  // WP ID -> rejection reason (conflict/cycle/reserved)
}

// planRoutes builds the deterministic hierarchy-aware route plan from current
// site settings (posts_base_path is read live, never hardcoded).
func (im *Importer) planRoutes(ctx context.Context, filename string) (*routePlan, error) {
	postsBase := currentPostsBase(ctx, im.q)
	var items []item
	if err := parse(filename, func(it item) error {
		switch it.Type {
		case "post", "page":
			if it.Status != "trash" && it.Status != "inherit" && it.Status != "auto-draft" {
				items = append(items, it)
			}
		}
		return nil
	}, nil, nil); err != nil {
		return nil, err
	}
	mapped := map[string]string{}
	for _, it := range items {
		if internalID, err := im.mapping(ctx, it.Type, it.ID); err == nil {
			mapped[it.ID] = internalID
		}
	}
	effective, errorsMap, _ := buildRoutePlan(ctx, im.q, items, postsBase, mapped)
	return &routePlan{effective: effective, errors: errorsMap}, nil
}

func (im *Importer) dryRun(ctx context.Context, filename string, r *Report) error {
	plan, err := im.planRoutes(ctx, filename)
	if err != nil {
		return err
	}
	for range plan.errors {
		r.Conflicts++
		r.Skipped++
	}
	return parse(filename, func(it item) error {
		r.Comments += len(it.Comments)
		switch it.Type {
		case "post", "page":
		case "attachment":
			// Validate attachment URL syntax/scheme only; no network in dry-run.
			if u := strings.TrimSpace(it.AttachmentURL); u != "" {
				parsed, perr := url.Parse(u)
				if perr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
					r.Warnings++
				}
			}
			return nil
		default:
			if it.Type != "" {
				r.Skipped++
				r.Warnings++
			}
			return nil
		}
		if it.Status == "trash" || it.Status == "inherit" || it.Status == "auto-draft" {
			r.Skipped++
			return nil
		}
		if _, rejected := plan.errors[it.ID]; rejected {
			return nil // already counted above
		}
		s := slug(it.Slug, it.Title)
		if s == "" || !entrySlugPattern.MatchString(s) {
			r.Skipped++
			r.Warnings++
			return nil
		}
		// Validate content conversion against the REAL block registry (no writes).
		warnings := []string{}
		doc, cerr := htmlDocument(it.Content, map[string]string{}, &warnings)
		if cerr != nil {
			r.Warnings++
			return nil
		}
		if im.blocks != nil {
			if verr := im.blocks.ValidateDocument(doc); verr != nil {
				r.Warnings++
			}
		}
		r.Warnings += len(warnings)
		countAccepted(r, it)
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

// countAccepted applies consistent report semantics: Posts/Pages count accepted
// objects regardless of publish state; status counters are additive and disjoint
// (Password counts as Password+Published-style acceptance, never as Private).
func countAccepted(r *Report, it item) {
	if it.Type == "post" {
		r.Posts++
	} else {
		r.Pages++
	}
	hasPassword := strings.TrimSpace(it.Password) != ""
	switch it.Status {
	case "draft":
		r.Drafts++
	case "pending":
		r.Pending++
	case "future":
		r.Scheduled++
	case "private":
		r.Private++
	default:
		if hasPassword {
			r.Password++
		} else {
			r.Published++
		}
	}
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

// termExternalID is the durable mapping key: taxonomy kind + numeric WP term ID.
// Slugs may change between exports; WP IDs are stable.
func termExternalID(kind, wpTermID string) string {
	return kind + ":" + strings.TrimSpace(wpTermID)
}

// importTerm is TAXONOMY PASS 1: create/resolve every supported term WITHOUT its
// parent relation, so WXR ordering can never affect hierarchy. Parents are
// resolved in resolveTermParents (PASS 2) once the complete mapping exists.
func (im *Importer) importTerm(ctx context.Context, runID string, t term, ids map[string]string, slugIndex map[string]string, r *Report) error {
	if t.Kind != "category" && t.Kind != "tag" {
		return nil
	}
	if strings.TrimSpace(t.Slug) == "" || strings.TrimSpace(t.ID) == "" {
		r.Warnings++
		return nil
	}
	external := termExternalID(t.Kind, t.ID)
	slugKey := t.Kind + ":" + strings.ToLower(strings.TrimSpace(t.Slug))
	// Already mapped in a previous run → reuse.
	if existing, err := im.mapping(ctx, "term", external); err == nil && existing != "" {
		ids[external] = existing
		slugIndex[slugKey] = existing
		return nil
	}
	// PASS 1 creates with NO parent.
	created, err := im.taxonomy.CreateTerm(ctx, t.Kind, t.Name, t.Slug, t.Description, "")
	if err != nil {
		// Existing Stratum term with same slug: reuse it and record WP mapping
		// (never duplicate, never mutate the existing term).
		if errors.Is(err, taxonomy.ErrDuplicateSlug) {
			if existing, e2 := im.q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: t.Kind, Slug: strings.ToLower(strings.TrimSpace(t.Slug))}); e2 == nil {
				ids[external] = existing.ID
				slugIndex[slugKey] = existing.ID
				if mapErr := im.mapObject(ctx, runID, "term", external, existing.ID); mapErr != nil {
					return mapErr
				}
				return nil
			}
		}
		r.Warnings++
		fmt.Printf("term %q (%s): %v\n", t.Name, t.Slug, err)
		return nil
	}
	if err := im.mapObject(ctx, runID, "term", external, created.ID); err != nil {
		return err
	}
	ids[external] = created.ID
	slugIndex[slugKey] = created.ID
	if t.Kind == "category" {
		r.Categories++
	} else {
		r.Tags++
	}
	return nil
}

// resolveTermParents is TAXONOMY PASS 2: attach category parents using the now-
// complete WP-term-ID mapping. Child-before-parent WXR order is irrelevant here.
func (im *Importer) resolveTermParents(ctx context.Context, filename string, r *Report) error {
	type rel struct{ childWP, parentSlug string }
	var rels []rel
	if err := parse(filename, nil, func(t term) error {
		if t.Kind == "category" && strings.TrimSpace(t.Parent) != "" && strings.TrimSpace(t.ID) != "" {
			rels = append(rels, rel{childWP: t.ID, parentSlug: strings.ToLower(strings.TrimSpace(t.Parent))})
		}
		return nil
	}, nil); err != nil {
		return err
	}
	for _, r2 := range rels {
		child, err := im.mapping(ctx, "term", termExternalID("category", r2.childWP))
		if err != nil || child == "" {
			continue
		}
		parent := ""
		// Resolve parent by WP ID first (any parent term recorded this run),
		// then fall back to slug lookup for pre-existing terms.
		if err := parse(filename, nil, func(t term) error {
			if parent == "" && t.Kind == "category" && strings.EqualFold(strings.TrimSpace(t.Slug), r2.parentSlug) {
				if id, e := im.mapping(ctx, "term", termExternalID("category", t.ID)); e == nil {
					parent = id
				}
			}
			return nil
		}, nil); err != nil {
			return err
		}
		if parent == "" {
			if row, e := im.q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "category", Slug: r2.parentSlug}); e == nil {
				parent = row.ID
			}
		}
		if parent == "" || parent == child {
			r.Warnings++
			continue
		}
		if err := im.taxonomy.SetParent(ctx, child, parent); err != nil {
			// Cycle/self/missing-parent rejections keep hierarchy valid; warn only.
			r.Warnings++
			fmt.Printf("category parent %s -> %s rejected: %v\n", child, parent, err)
		}
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
	entryID, idErr := newID()
	if idErr != nil {
		return idErr
	}
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
	// Failure cleanup: a shell Entry without its durable mapping is useless and
	// would be invisible to future reruns — remove it instead of leaving an
	// orphan. Both errors surface via errors.Join.
	if mapErr := im.mapObject(ctx, runID, it.Type, it.ID, entryID); mapErr != nil {
		if delErr := im.q.DeleteEntry(ctx, entryID); delErr != nil {
			return errors.Join(mapErr, delErr)
		}
		return mapErr
	}
	return nil
}

// entryHasRevision reports whether the mapped entry already carries any revision.
func (im *Importer) entryHasRevision(ctx context.Context, entryID string) bool {
	_, err := im.q.GetLatestEntryRevision(ctx, entryID)
	return err == nil
}

func (im *Importer) createRevision(ctx context.Context, runID string, it item, terms, termSlugIndex, images map[string]string, authors map[string]author, opt Options, r *Report) error {
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
	// Re-check the EFFECTIVE path (real posts_base_path + hierarchy) before
	// publishing; mirrors the shared plan so dry-run and import agree.
	postsBase := currentPostsBase(ctx, im.q)
	expectedPath := routing.EntryPath(it.Type, s, postsBase)
	if it.Type == "page" && it.ParentID != "" && it.ParentID != "0" {
		if parentEntryID, e := im.mapping(ctx, "page", it.ParentID); e == nil {
			if parentRoute, e2 := im.q.GetEntryRoute(ctx, sql.NullString{String: parentEntryID, Valid: true}); e2 == nil {
				expectedPath = routing.ChildEntryPath(parentRoute.Path, s)
			} else if parentRev, e3 := im.q.GetLatestEntryRevision(ctx, parentEntryID); e3 == nil && parentRev.Slug != "" {
				expectedPath = routing.ChildEntryPath("/"+parentRev.Slug, s)
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
	revID, revIDErr := newID()
	if revIDErr != nil {
		return revIDErr
	}
	now := time.Now()
	createdAt := nonzeroUnix(it.ModifiedAt, now)
	if !it.PublishedAt.IsZero() && it.Status == "publish" {
		// Use published date for revision creation time to preserve history
		createdAt = it.PublishedAt.Unix()
	}
	commentsEnabled := int64(0)
	if it.Type == "post" {
		commentsEnabled = 1
	}
	if err := im.q.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: entryID, RevisionNumber: 1, Slug: s, Title: it.Title, Excerpt: sql.NullString{String: it.Excerpt, Valid: true}, DocumentJson: string(raw), FeaturedMediaID: sql.NullString{}, ParentEntryID: nullString(parent), MenuOrder: it.MenuOrder, FieldsJson: "{}", CreatedBy: nullString(im.authorID(ctx, it, authors, opt)), CreatedAt: createdAt, Visibility: visibility, PasswordHash: hash, Sticky: 0, ReviewState: review, CommentsEnabled: commentsEnabled}); err != nil {
		return err
	}
	for _, t := range it.Terms {
		domain := normalizeTermDomain(t.Domain)
		if domain == "" {
			continue
		}
		slugKey := domain + ":" + strings.ToLower(strings.TrimSpace(t.Slug))
		if tid := termSlugIndex[slugKey]; tid != "" {
			if err := im.q.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: revID, TermID: tid}); err != nil {
				return err
			}
		}
	}
	if thumbnail := it.Meta["_thumbnail_id"]; thumbnail != "" && thumbnail != "0" {
		if mediaID, e := im.mapping(ctx, "attachment", thumbnail); e == nil {
			if e := im.q.SetCommentFeaturedMedia(ctx, db.SetCommentFeaturedMediaParams{FeaturedMediaID: nullString(mediaID), ID: revID}); e != nil {
				return e
			}
		} else {
			r.Warnings++
		}
	}
	r.Warnings += len(warnings)
	switch it.Status {
	case "draft":
		countAccepted(r, it)
		return nil
	case "pending":
		countAccepted(r, it)
		return nil
	case "future":
		target := it.PublishedAt
		if target.IsZero() || !target.After(now) {
			r.Warnings++
			r.Drafts++
			return nil
		}
		countAccepted(r, it)
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
	// Historical date preservation applies ONLY to published items; draft,
	// pending and scheduled entries keep import-time bookkeeping timestamps so
	// the published_at/first_published_at invariants stay intact.
	published := nonzeroUnix(it.PublishedAt, now)
	modified := nonzeroUnix(it.ModifiedAt, now)
	if err := im.q.SetImportedPublishedDates(ctx, db.SetImportedPublishedDatesParams{CreatedAt: published, UpdatedAt: modified, PublishedAt: sql.NullInt64{Int64: published, Valid: true}, FirstPublishedAt: sql.NullInt64{Int64: published, Valid: true}, ID: entryID}); err != nil {
		return err
	}
	countAccepted(r, it)
	return nil
}

func (im *Importer) mapping(ctx context.Context, typ, id string) (string, error) {
	return im.q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: typ, ExternalID: id})
}
func (im *Importer) mapObject(ctx context.Context, run, typ, external, internal string) error {
	return im.q.CreateImportMapping(ctx, db.CreateImportMappingParams{Source: source, ObjectType: typ, ExternalID: external, InternalID: internal, RunID: run, CreatedAt: time.Now().Unix()})
}

// authorID resolves attribution: 1) WP author email → existing Stratum user,
// 2) --author fallback → existing Stratum user, 3) NULL author. Never creates
// accounts and never imports WordPress credentials.
func (im *Importer) authorID(ctx context.Context, it item, authors map[string]author, opt Options) string {
	if wpEmail := strings.ToLower(strings.TrimSpace(authors[it.Author].Email)); wpEmail != "" {
		if u, err := im.q.GetUserByEmail(ctx, wpEmail); err == nil {
			return u.ID
		}
	}
	if fb := strings.ToLower(strings.TrimSpace(opt.Author)); fb != "" {
		if u, err := im.q.GetUserByEmail(ctx, fb); err == nil {
			return u.ID
		}
	}
	return ""
}
func (im *Importer) downloader() *Downloader {
	if im.Downloader != nil {
		return im.Downloader
	}
	return newDownloader()
}

func (im *Importer) downloadMedia(ctx context.Context, raw, author string) (*media.Asset, error) {
	body, name, err := im.downloader().Get(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	// Limit reading to maxDownloadBytes+1 to detect overflow without loading huge files
	limited := io.LimitReader(body, maxDownloadBytes+1)
	return im.media.Upload(ctx, name, author, limited)
}

// newID generates a random ID and FAILS CLOSED on entropy errors — unique IDs
// are correctness-critical for mappings, entries, and revisions.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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
	s = strings.NewReplacer(
		"ą", "a", "ć", "c", "ę", "e", "ł", "l", "ń", "n", "ó", "o", "ś", "s", "ź", "z", "ż", "z",
		"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "æ", "ae",
		"è", "e", "é", "e", "ê", "e", "ë", "e",
		"ì", "i", "í", "i", "î", "i", "ï", "i",
		"ò", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o",
		"ù", "u", "ú", "u", "û", "u", "ü", "u",
		"ý", "y", "ÿ", "y",
		"ñ", "n", "ç", "c",
		"ß", "ss",
	).Replace(s)
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

func currentPostsBase(ctx context.Context, q *db.Queries) string {
	row, err := q.GetSiteSettings(ctx)
	if err != nil {
		return "/blog"
	}
	base := strings.TrimSpace(row.PostsBasePath)
	if base == "" {
		return "/blog"
	}
	return base
}

func effectivePathForItem(it item, postsBase string, parentPath string) string {
	s := slug(it.Slug, it.Title)
	if s == "" {
		return ""
	}
	if it.Type == "page" {
		if parentPath != "" {
			return routing.ChildEntryPath(parentPath, s)
		}
		return routing.EntryPath(it.Type, s, postsBase)
	}
	// post and others
	return routing.EntryPath(it.Type, s, postsBase)
}

func buildRoutePlan(ctx context.Context, q *db.Queries, items []item, postsBase string, mapped map[string]string) (map[string]string, map[string]error, []string) {
	// Returns map wpID -> effective path, map of errors, and list of duplicate paths
	idToItem := map[string]item{}
	for _, it := range items {
		if it.Type == "post" || it.Type == "page" {
			idToItem[it.ID] = it
		}
	}
	// First, compute parent graph and detect cycles
	parentOf := map[string]string{}
	for _, it := range items {
		if it.Type == "page" && it.ParentID != "" && it.ParentID != "0" {
			parentOf[it.ID] = it.ParentID
		}
	}
	// Detect cycles via DFS
	visited := map[string]int{} // 0 unvisited, 1 visiting, 2 done
	var hasCycle bool
	var cycleErr = make(map[string]error)
	var dfs func(id string)
	dfs = func(id string) {
		if visited[id] == 1 {
			hasCycle = true
			cycleErr[id] = errors.New("hierarchy cycle detected")
			return
		}
		if visited[id] == 2 {
			return
		}
		visited[id] = 1
		if parent, ok := parentOf[id]; ok {
			dfs(parent)
		}
		visited[id] = 2
	}
	for id := range idToItem {
		if visited[id] == 0 {
			dfs(id)
		}
	}
	effective := map[string]string{}
	errorsMap := map[string]error{}
	// Also need to handle self-parent
	for id, it := range idToItem {
		if it.ParentID == id {
			errorsMap[id] = errors.New("self-parent")
			continue
		}
		if hasCycle {
			if _, ok := cycleErr[id]; ok {
				errorsMap[id] = cycleErr[id]
				continue
			}
		}
	}
	// Compute effective paths via recursion with memo
	memo := map[string]string{}
	var computePath func(id string, seen map[string]bool) (string, error)
	computePath = func(id string, seen map[string]bool) (string, error) {
		if p, ok := memo[id]; ok {
			return p, nil
		}
		if seen[id] {
			return "", errors.New("cycle")
		}
		seen[id] = true
		it, ok := idToItem[id]
		if !ok {
			return "", errors.New("missing item")
		}
		s := slug(it.Slug, it.Title)
		if s == "" {
			return "", errors.New("empty slug")
		}
		if it.Type != "page" {
			p := routing.EntryPath(it.Type, s, postsBase)
			memo[id] = p
			return p, nil
		}
		// page
		parentID := it.ParentID
		if parentID == "" || parentID == "0" {
			p := routing.EntryPath(it.Type, s, postsBase)
			memo[id] = p
			return p, nil
		}
		// Check if parent is in import plan or existing DB
		if _, ok := idToItem[parentID]; ok {
			parentPath, err := computePath(parentID, seen)
			if err != nil {
				return "", err
			}
			p := routing.ChildEntryPath(parentPath, s)
			memo[id] = p
			return p, nil
		}
		// Parent not in import, try existing DB mapping
		// For plan, we need to resolve via DB if parent already imported
		// For now, treat as root if parent not found
		// The caller will handle missing parent as warning/skip
		p := routing.EntryPath(it.Type, s, postsBase)
		memo[id] = p
		return p, nil
	}
	for id := range idToItem {
		if _, ok := errorsMap[id]; ok {
			continue
		}
		p, err := computePath(id, map[string]bool{})
		if err != nil {
			errorsMap[id] = err
			continue
		}
		effective[id] = p
	}
	// Detect duplicate effective paths WITHIN the plan. Two candidates sharing
	// one path are only benign when both WP IDs already map to the SAME entry
	// (i.e. a rerun of the same object).
	seenPaths := map[string]string{}
	dupPaths := []string{}
	for id, p := range effective {
		if other, ok := seenPaths[p]; ok {
			if mapped[id] != "" && mapped[id] == mapped[other] {
				continue // same underlying entry: rerun, not a conflict
			}
			errorsMap[id] = fmt.Errorf("duplicate effective path %s conflicts with %s", p, other)
			if _, exists := errorsMap[other]; !exists {
				errorsMap[other] = fmt.Errorf("duplicate effective path %s conflicts with %s", p, id)
			}
			dupPaths = append(dupPaths, p)
		} else {
			seenPaths[p] = id
		}
	}
	// Detect collision with existing routes. A route owned by this very WP
	// item's already-imported entry is NOT a conflict (rerun safety); anything
	// else occupying the prospective path is.
	for id, p := range effective {
		if _, ok := errorsMap[id]; ok {
			continue
		}
		if existing, err := q.GetRouteByPath(ctx, p); err == nil {
			ownerID := mapped[id]
			if !(existing.EntryID.Valid && ownerID != "" && existing.EntryID.String == ownerID) {
				errorsMap[id] = fmt.Errorf("route already exists %s", p)
				continue
			}
		}
		if p == "/admin" || strings.HasPrefix(p, "/admin/") || p == "/stratum" || strings.HasPrefix(p, "/stratum/") {
			errorsMap[id] = fmt.Errorf("reserved path %s", p)
		}
	}
	return effective, errorsMap, dupPaths
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

func (im *Importer) importComments(ctx context.Context, filename, runID string, report *Report) error {
	// Collect all comments with their entry mapping
	type pending struct {
		wpID      string
		entryID   string
		comment   importComment
		stratumID string
	}
	var pendings []pending
	if err := parse(filename, func(it item) error {
		if len(it.Comments) == 0 {
			return nil
		}
		entryID, err := im.mapping(ctx, it.Type, it.ID)
		if err != nil {
			// Entry not imported, skip comments for this item
			report.Warnings++
			return nil
		}
		for _, c := range it.Comments {
			// Skip pingbacks/trackbacks
			if strings.EqualFold(c.Type, "pingback") || strings.EqualFold(c.Type, "trackback") {
				report.PingbacksSkipped++
				continue
			}
			// Map status
			status := comments.StatusPending
			switch strings.ToLower(strings.TrimSpace(c.Approved)) {
			case "1", "approve", "approved":
				status = comments.StatusApproved
			case "0", "":
				status = comments.StatusPending
			case "spam":
				status = comments.StatusSpam
			case "trash":
				status = comments.StatusTrash
			default:
				status = comments.StatusPending
				report.Warnings++
			}
			// Check idempotency via imported source
			if _, err := im.q.GetCommentByImportID(ctx, db.GetCommentByImportIDParams{ImportedSource: sql.NullString{String: source, Valid: true}, ImportedExternalID: sql.NullString{String: c.ID, Valid: true}}); err == nil {
				report.CommentsSkipped++
				continue
			}
			// Also check import_mappings
			if _, err := im.mapping(ctx, "comment", c.ID); err == nil {
				report.CommentsSkipped++
				continue
			}
			body := comments.HTMLToText(c.Content)
			if strings.TrimSpace(body) == "" {
				body = "(empty)"
			}
			createdAt := c.CreatedAt
			if createdAt.IsZero() {
				createdAt = time.Now()
			}
			// Determine author
			authorName := strings.TrimSpace(c.Author)
			if authorName == "" {
				authorName = "Anonymous"
			}
			email := strings.TrimSpace(c.Email)
			if email == "" {
				email = "unknown@example.invalid"
			}
			// Create comment with no parent first
			stratumID, err := func() (string, error) {
				// Use comments service to create, but we need to handle parent later
				// Directly use queries for import to avoid moderation/rate limiting
				id, idErr := newID()
				if idErr != nil {
					return "", idErr
				}
				err := im.q.CreateComment(ctx, db.CreateCommentParams{
					ID: id, EntryID: entryID, ParentID: sql.NullString{}, Status: status,
					AuthorName: authorName, AuthorEmail: email, AuthorUrl: strings.TrimSpace(c.URL),
					UserID: sql.NullString{}, Body: body, CreatedAt: createdAt.Unix(), UpdatedAt: createdAt.Unix(),
					ImportedSource: sql.NullString{String: source, Valid: true}, ImportedExternalID: sql.NullString{String: c.ID, Valid: true},
				})
				if err != nil {
					return "", err
				}
				// Also create mapping for rerun
				_ = im.mapObject(ctx, runID, "comment", c.ID, id)
				return id, nil
			}()
			if err != nil {
				report.Warnings++
				continue
			}
			pendings = append(pendings, pending{wpID: c.ID, entryID: entryID, comment: c, stratumID: stratumID})
			report.CommentsImported++
			// Invalidate if approved
			if status == comments.StatusApproved && im.comments != nil {
				// Use service invalidator if available, else direct
				if im.q != nil {
					// Invalidate via search? For now, use comments service invalidator if set
				}
			}
		}
		return nil
	}, nil, nil); err != nil {
		return err
	}
	// Second pass: resolve parent_id with depth handling
	wpToStratum := map[string]string{}
	for _, p := range pendings {
		wpToStratum[p.wpID] = p.stratumID
	}
	for _, p := range pendings {
		if strings.TrimSpace(p.comment.ParentID) == "" || strings.TrimSpace(p.comment.ParentID) == "0" {
			continue
		}
		parentStratumID, ok := wpToStratum[p.comment.ParentID]
		if !ok {
			report.Warnings++
			continue
		}
		// Check depth: walk parent chain
		depth := 0
		cur := parentStratumID
		seen := map[string]bool{}
		for cur != "" {
			if seen[cur] {
				break
			}
			seen[cur] = true
			depth++
			if depth >= comments.MaxDepth {
				break
			}
			// Find parent of cur
			var parentOfCur string
			for _, pp := range pendings {
				if pp.stratumID == cur {
					if pid := strings.TrimSpace(pp.comment.ParentID); pid != "" && pid != "0" {
						if ps, ok := wpToStratum[pid]; ok {
							parentOfCur = ps
						}
					}
					break
				}
			}
			if parentOfCur == "" {
				// Try DB lookup for already existing parent's parent
				if c, err := im.q.GetComment(ctx, cur); err == nil && c.ParentID.Valid {
					parentOfCur = c.ParentID.String
				} else {
					break
				}
			}
			cur = parentOfCur
			if len(seen) > 10 {
				break
			}
		}
		if depth >= comments.MaxDepth {
			// Flatten: find ancestor at depth MaxDepth-1
			ancestor := parentStratumID
			for i := 0; i < depth-(comments.MaxDepth-1); i++ {
				if c, err := im.q.GetComment(ctx, ancestor); err == nil && c.ParentID.Valid {
					ancestor = c.ParentID.String
				} else {
					// Fallback to walk pendings
					for _, pp := range pendings {
						if pp.stratumID == ancestor {
							if pid := strings.TrimSpace(pp.comment.ParentID); pid != "" {
								if ps, ok := wpToStratum[pid]; ok {
									ancestor = ps
									break
								}
							}
							ancestor = ""
							break
						}
					}
					break
				}
			}
			// If still too deep, make top-level
			if ancestor == "" {
				continue
			}
			parentStratumID = ancestor
			report.Warnings++
		}
		// Update parent
		_ = im.q.UpdateCommentParent(ctx, db.UpdateCommentParentParams{ParentID: sql.NullString{String: parentStratumID, Valid: true}, UpdatedAt: time.Now().Unix(), ID: p.stratumID})
	}
	return nil
}
