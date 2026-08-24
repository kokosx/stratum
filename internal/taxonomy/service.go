package taxonomy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Service owns taxonomy operations.
type Service struct {
	db      *sql.DB
	queries *db.Queries
}

// New creates a service.
func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

// ListTaxonomiesForContentType returns taxonomies for a content type.
func (s *Service) ListTaxonomiesForContentType(ctx context.Context, contentTypeID string) ([]Taxonomy, error) {
	rows, err := s.queries.ListTaxonomiesByContentType(ctx, contentTypeID)
	if err != nil {
		return nil, err
	}
	out := make([]Taxonomy, 0, len(rows))
	for _, r := range rows {
		out = append(out, taxonomyFromRow(r))
	}
	return out, nil
}

// ListTaxonomies returns all.
func (s *Service) ListTaxonomies(ctx context.Context) ([]Taxonomy, error) {
	rows, err := s.queries.ListTaxonomies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Taxonomy, 0, len(rows))
	for _, r := range rows {
		out = append(out, taxonomyFromRow(r))
	}
	return out, nil
}

func taxonomyFromRow(r db.Taxonomy) Taxonomy {
	return Taxonomy{
		ID: r.ID, ContentTypeID: r.ContentTypeID, SingularName: r.SingularName, PluralName: r.PluralName,
		Hierarchical: r.Hierarchical != 0, Public: r.Public != 0, RouteBase: r.RouteBase, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func termFromRow(r db.Term) Term {
	return Term{ID: r.ID, TaxonomyID: r.TaxonomyID, ParentID: r.ParentID, Name: r.Name, Slug: r.Slug, Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// ListTerms returns terms for taxonomy ordered by name.
func (s *Service) ListTerms(ctx context.Context, taxonomyID string) ([]Term, error) {
	rows, err := s.queries.ListTermsByTaxonomy(ctx, taxonomyID)
	if err != nil {
		return nil, err
	}
	out := make([]Term, 0, len(rows))
	for _, r := range rows {
		out = append(out, termFromRow(r))
	}
	return out, nil
}

// ListTermsWithCounts returns terms with published counts.
func (s *Service) ListTermsWithCounts(ctx context.Context, taxonomyID string) ([]TermWithCount, error) {
	rows, err := s.queries.ListTermsByTaxonomyWithCounts(ctx, taxonomyID)
	if err != nil {
		return nil, err
	}
	out := make([]TermWithCount, 0, len(rows))
	for _, r := range rows {
		t := Term{ID: r.ID, TaxonomyID: r.TaxonomyID, ParentID: r.ParentID, Name: r.Name, Slug: r.Slug, Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
		out = append(out, TermWithCount{Term: t, Count: r.PublishedCount})
	}
	return out, nil
}

type TermWithCount struct {
	Term  Term
	Count int64
}

// GetTerm returns term by id.
func (s *Service) GetTerm(ctx context.Context, id string) (Term, error) {
	r, err := s.queries.GetTerm(ctx, id)
	if err != nil {
		return Term{}, err
	}
	return termFromRow(r), nil
}

// CreateTerm validates and creates a term and its archive route atomically.
func (s *Service) CreateTerm(ctx context.Context, taxonomyID, name, slug, description, parentID string) (Term, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Term{}, err
	}
	defer tx.Rollback()
	term, err := s.CreateTermWithQueries(ctx, s.queries.WithTx(tx), taxonomyID, name, slug, description, parentID)
	if err != nil {
		return Term{}, err
	}
	if err := tx.Commit(); err != nil {
		return Term{}, err
	}
	return term, nil
}

// CreateTermWithQueries creates a term using the caller's transaction.
// It is used by entry writes so tag creation and revision assignment commit together.
func (s *Service) CreateTermWithQueries(ctx context.Context, q *db.Queries, taxonomyID, name, slug, description, parentID string) (Term, error) {
	taxRow, err := q.GetTaxonomy(ctx, taxonomyID)
	if err != nil {
		return Term{}, err
	}
	tax := taxonomyFromRow(taxRow)
	name = strings.TrimSpace(name)
	if name == "" {
		return Term{}, errors.New("name is required")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = slugify(name)
	}
	norm, ok := NormalizeSlug(slug)
	if !ok {
		return Term{}, ErrInvalidSlug
	}
	slug = norm
	var parent sql.NullString
	if parentID != "" {
		if !tax.Hierarchical {
			return Term{}, ErrParentNotAllowed
		}
		parent = sql.NullString{String: parentID, Valid: true}
		pr, err := q.GetTerm(ctx, parentID)
		if err != nil {
			return Term{}, ErrParentNotFound
		}
		if pr.TaxonomyID != taxonomyID {
			return Term{}, ErrParentSameTax
		}
	} else {
		parent = sql.NullString{Valid: false}
	}
	if _, err := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: taxonomyID, Slug: slug}); err == nil {
		return Term{}, ErrDuplicateSlug
	}
	id, err := randomID()
	if err != nil {
		return Term{}, err
	}
	now := time.Now().Unix()
	if err := q.CreateTerm(ctx, db.CreateTermParams{ID: id, TaxonomyID: taxonomyID, ParentID: parent, Name: name, Slug: slug, Description: strings.TrimSpace(description), CreatedAt: now, UpdatedAt: now}); err != nil {
		return Term{}, err
	}
	if tax.Public {
		path := TaxonomyTermPath(tax, slug)
		if err := validateNotReserved(path); err != nil {
			return Term{}, err
		}
		if byPath, err := q.GetRouteByPath(ctx, path); err == nil && byPath.EntryID.Valid {
			return Term{}, errors.New("route already occupied")
		}
		if byPath, err := q.GetRouteByPath(ctx, path); err == nil && !byPath.EntryID.Valid && byPath.RouteType == "redirect" {
			if err := q.DeleteRoute(ctx, byPath.ID); err != nil {
				return Term{}, err
			}
		}
		rid, err := randomID()
		if err != nil {
			return Term{}, err
		}
		if err := q.CreateRoute(ctx, db.CreateRouteParams{
			ID: rid, Path: path, RouteType: routing.RouteTypeArchive, ContentTypeID: sql.NullString{String: tax.ContentTypeID, Valid: true},
			TaxonomyID: sql.NullString{String: taxonomyID, Valid: true}, TermID: sql.NullString{String: id, Valid: true},
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return Term{}, err
		}
	}
	return Term{ID: id, TaxonomyID: taxonomyID, ParentID: parent, Name: name, Slug: slug, Description: strings.TrimSpace(description), CreatedAt: now, UpdatedAt: now}, nil
}

// UpdateTerm validates and updates term and moves its route.
func (s *Service) UpdateTerm(ctx context.Context, id, name, slug, description, parentID string) (Term, error) {
	existing, err := s.queries.GetTerm(ctx, id)
	if err != nil {
		return Term{}, err
	}
	taxRow, err := s.queries.GetTaxonomy(ctx, existing.TaxonomyID)
	if err != nil {
		return Term{}, err
	}
	tax := taxonomyFromRow(taxRow)
	name = strings.TrimSpace(name)
	if name == "" {
		return Term{}, errors.New("name is required")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = slugify(name)
	}
	norm, ok := NormalizeSlug(slug)
	if !ok {
		return Term{}, ErrInvalidSlug
	}
	slug = norm
	if slug != existing.Slug {
		if _, err := s.queries.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: existing.TaxonomyID, Slug: slug}); err == nil {
			return Term{}, ErrDuplicateSlug
		}
	}
	var parent sql.NullString
	if parentID != "" {
		if !tax.Hierarchical {
			return Term{}, ErrParentNotAllowed
		}
		if parentID == id {
			return Term{}, ErrSelfParent
		}
		parent = sql.NullString{String: parentID, Valid: true}
		pr, err := s.queries.GetTerm(ctx, parentID)
		if err != nil {
			return Term{}, ErrParentNotFound
		}
		if pr.TaxonomyID != existing.TaxonomyID {
			return Term{}, ErrParentSameTax
		}
		visited := map[string]bool{id: true}
		cur := parentID
		for cur != "" {
			if visited[cur] {
				return Term{}, ErrCycle
			}
			visited[cur] = true
			p, err := s.queries.GetTerm(ctx, cur)
			if err != nil || !p.ParentID.Valid {
				break
			}
			cur = p.ParentID.String
		}
	} else {
		parent = sql.NullString{Valid: false}
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Term{}, err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	oldSlug := existing.Slug
	oldPath := TaxonomyTermPath(tax, oldSlug)
	newPath := TaxonomyTermPath(tax, slug)
	if err := qtx.UpdateTerm(ctx, db.UpdateTermParams{ParentID: parent, Name: name, Slug: slug, Description: strings.TrimSpace(description), UpdatedAt: now, ID: id}); err != nil {
		return Term{}, err
	}
	if tax.Public && oldPath != newPath {
		if byPath, err := qtx.GetRouteByPath(ctx, newPath); err == nil && byPath.EntryID.Valid {
			return Term{}, errors.New("route already occupied")
		}
		var route *db.Route
		if r, err := qtx.GetTermArchiveRoute(ctx, db.GetTermArchiveRouteParams{TaxonomyID: sql.NullString{String: existing.TaxonomyID, Valid: true}, TermID: sql.NullString{String: id, Valid: true}}); err == nil {
			tmp := r
			route = &tmp
		} else if r, err := qtx.GetRouteByPath(ctx, oldPath); err == nil && r.TermID.Valid && r.TermID.String == id {
			tmp := r
			route = &tmp
		}
		if route != nil {
			if byPath, err := qtx.GetRouteByPath(ctx, newPath); err == nil && byPath.ID != route.ID {
				if byPath.RouteType != "redirect" {
					return Term{}, errors.New("route already occupied")
				}
				if err := qtx.DeleteRoute(ctx, byPath.ID); err != nil {
					return Term{}, err
				}
			}
			if err := qtx.UpdateRoute(ctx, db.UpdateRouteParams{
				ID: route.ID, Path: newPath, EntryID: sql.NullString{},
				RouteType: routing.RouteTypeArchive, ContentTypeID: sql.NullString{String: tax.ContentTypeID, Valid: true},
				TaxonomyID: sql.NullString{String: tax.ID, Valid: true}, TermID: sql.NullString{String: id, Valid: true},
				UpdatedAt: now,
			}); err != nil {
				return Term{}, err
			}
		} else {
			rid, err := randomID()
			if err != nil {
				return Term{}, err
			}
			if err := qtx.CreateRoute(ctx, db.CreateRouteParams{
				ID: rid, Path: newPath, RouteType: routing.RouteTypeArchive, ContentTypeID: sql.NullString{String: tax.ContentTypeID, Valid: true},
				TaxonomyID: sql.NullString{String: tax.ID, Valid: true}, TermID: sql.NullString{String: id, Valid: true},
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return Term{}, err
			}
		}
		if err := routing.UpsertRedirectRoute(ctx, qtx, oldPath, newPath, now); err != nil {
			return Term{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Term{}, err
	}
	return Term{ID: id, TaxonomyID: existing.TaxonomyID, ParentID: parent, Name: name, Slug: slug, Description: strings.TrimSpace(description), CreatedAt: existing.CreatedAt, UpdatedAt: now}, nil
}

// DeleteTerm reparents children and deletes term and its route.
func (s *Service) DeleteTerm(ctx context.Context, id string) error {
	existing, err := s.queries.GetTerm(ctx, id)
	if err != nil {
		return err
	}
	taxRow, err := s.queries.GetTaxonomy(ctx, existing.TaxonomyID)
	if err != nil {
		return err
	}
	var oldPath string
	if taxRow.Public != 0 {
		tax := taxonomyFromRow(taxRow)
		oldPath = TaxonomyTermPath(tax, existing.Slug)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	now := time.Now().Unix()
	children, err := qtx.ListChildTerms(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return err
	}
	for _, child := range children {
		newParent := existing.ParentID
		if err := qtx.UpdateTerm(ctx, db.UpdateTermParams{ParentID: newParent, Name: child.Name, Slug: child.Slug, Description: child.Description, UpdatedAt: now, ID: child.ID}); err != nil {
			return err
		}
	}
	if oldPath != "" {
		if rt, err := qtx.GetTermArchiveRoute(ctx, db.GetTermArchiveRouteParams{TaxonomyID: sql.NullString{String: existing.TaxonomyID, Valid: true}, TermID: sql.NullString{String: id, Valid: true}}); err == nil {
			if err := qtx.DeleteRoute(ctx, rt.ID); err != nil {
				return err
			}
		} else if rt, err := qtx.GetRouteByPath(ctx, oldPath); err == nil && rt.TermID.Valid && rt.TermID.String == id {
			if err := qtx.DeleteRoute(ctx, rt.ID); err != nil {
				return err
			}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if err := qtx.DeleteTerm(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TermsForRevision returns terms assigned to a revision.
func (s *Service) TermsForRevision(ctx context.Context, revisionID string) ([]Term, error) {
	rows, err := s.queries.ListTermsForRevision(ctx, revisionID)
	if err != nil {
		return nil, err
	}
	out := make([]Term, 0, len(rows))
	for _, r := range rows {
		out = append(out, termFromRow(r))
	}
	return out, nil
}

// SetTermsForRevision replaces assignments for a revision in a transaction.
func (s *Service) SetTermsForRevision(ctx context.Context, qtx *db.Queries, revisionID string, termIDs []string) error {
	if err := qtx.DeleteTermsForRevision(ctx, revisionID); err != nil {
		return err
	}
	for _, tid := range termIDs {
		if strings.TrimSpace(tid) == "" {
			continue
		}
		if _, err := qtx.GetTerm(ctx, tid); err != nil {
			return err
		}
		if err := qtx.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: revisionID, TermID: tid}); err != nil {
			return err
		}
	}
	return nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if r == '-' {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		} else {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		res = "term"
	}
	return res
}

func validateNotReserved(path string) error {
	reserved := []string{"/admin", "/stratum", "/media", "/sitemap.xml", "/robots.txt", "/feed.xml", "/favicon.ico"}
	for _, p := range reserved {
		if path == p || strings.HasPrefix(path, p+"/") {
			return ErrReservedRouteBase
		}
	}
	return nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
