package taxonomy

import (
	"database/sql"
	"regexp"
	"strings"
)

// TaxonomyID is string alias for taxonomy id (e.g. "category", "tag")
type TaxonomyID string

const (
	TaxonomyCategory TaxonomyID = "category"
	TaxonomyTag      TaxonomyID = "tag"
)

// Taxonomy is domain view.
type Taxonomy struct {
	ID            string
	ContentTypeID string
	SingularName  string
	PluralName    string
	Hierarchical  bool
	Public        bool
	RouteBase     sql.NullString
	CreatedAt     int64
	UpdatedAt     int64
}

// Term is domain view.
type Term struct {
	ID          string
	TaxonomyID  string
	ParentID    sql.NullString
	Name        string
	Slug        string
	Description string
	CreatedAt   int64
	UpdatedAt   int64
}

var termSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// NormalizeSlug lowercases, trims, validates pattern.
func NormalizeSlug(s string) (string, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", false
	}
	if !termSlugPattern.MatchString(s) {
		return "", false
	}
	if reservedSlug(s) {
		return "", false
	}
	return s, true
}

func reservedSlug(s string) bool {
	reserved := map[string]bool{
		"admin": true, "stratum": true, "sitemap.xml": true, "robots.txt": true, "feed.xml": true,
		"media": true, "category": true, "tag": true, "blog": true,
	}
	return reserved[s]
}

// TaxonomyTermPath returns canonical path for term archive.
func TaxonomyTermPath(taxonomy Taxonomy, termSlug string) string {
	base := "/"
	if taxonomy.RouteBase.Valid && strings.TrimSpace(taxonomy.RouteBase.String) != "" {
		base = taxonomy.RouteBase.String
	}
	base = normalizePath(base)
	slug := strings.Trim(termSlug, "/")
	if slug == "" {
		return base
	}
	if base == "/" {
		return "/" + slug
	}
	return normalizePath(base + "/" + slug)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

// ValidateRouteBase ensures base path is safe.
func ValidateRouteBase(base string) error {
	base = strings.TrimSpace(base)
	if base == "" {
		return ErrInvalidRouteBase
	}
	if !strings.HasPrefix(base, "/") {
		return ErrInvalidRouteBase
	}
	if base == "/" {
		return ErrInvalidRouteBase
	}
	if strings.Contains(base, "//") || strings.Contains(base, " ") {
		return ErrInvalidRouteBase
	}
	reservedPrefixes := []string{"/admin", "/stratum", "/media", "/sitemap.xml", "/robots.txt", "/feed.xml", "/favicon.ico"}
	for _, p := range reservedPrefixes {
		if base == p || strings.HasPrefix(base, p+"/") {
			return ErrReservedRouteBase
		}
	}
	return nil
}

var (
	ErrInvalidRouteBase  = errorString("invalid route base")
	ErrReservedRouteBase = errorString("reserved route base")
	ErrInvalidSlug       = errorString("invalid slug")
	ErrDuplicateSlug     = errorString("duplicate slug")
	ErrParentNotAllowed  = errorString("parent not allowed for flat taxonomy")
	ErrParentNotFound    = errorString("parent not found")
	ErrParentSameTax     = errorString("parent must belong to same taxonomy")
	ErrSelfParent        = errorString("term cannot be parent of itself")
	ErrCycle             = errorString("cycle detected")
)

type errorString string

func (e errorString) Error() string { return string(e) }
