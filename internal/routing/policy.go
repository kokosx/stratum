package routing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kokosx/stratum/internal/content"
)

// DefaultPostsBase is the conventional default for the posts archive and all
// single post URLs. It is the value of posts_base_path when not customized.
const DefaultPostsBase = "/blog"

// NormalizePath applies the site-wide trailing-slash policy to a URL path:
// paths start with "/", the root stays "/" and other trailing slashes are
// removed ("/blog/" becomes "/blog").
func NormalizePath(path string) string {
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

// PostsArchivePath returns the base under which the post archive and all post
// singles are published. Never returns empty; falls back to DefaultPostsBase.
func PostsArchivePath(postsBasePath string) string {
	p := NormalizePath(strings.TrimSpace(postsBasePath))
	if p == "" || p == "/" {
		return DefaultPostsBase
	}
	return p
}

// EntryPath is the single place that computes the public path for an Entry.
// It uses ContentTypeDefinition.RoutingPolicy so generic handlers never branch
// on concrete type names (INVARIANT 2). Page → "/{slug}", Post (and any
// future archived type) → "{postsBase}/{slug}" (default /blog).
func EntryPath(contentTypeID, slug, postsBasePath string) string {
	s := strings.Trim(slug, "/")
	if s == "" {
		return "/"
	}
	def := content.DefinitionFor(contentTypeID)
	if def.IsArchived() {
		base := PostsArchivePath(postsBasePath)
		if base == "/" {
			return "/" + s
		}
		return NormalizePath(base + "/" + s)
	}
	return "/" + s
}

// ChildEntryPath derives a child path from the parent's effective public route.
// In particular, a Homepage parent at "/" does not leak its stored slug into a
// child's URL.
func ChildEntryPath(parentPath, slug string) string {
	parentPath = NormalizePath(parentPath)
	slug = strings.Trim(slug, "/")
	if slug == "" {
		return parentPath
	}
	if parentPath == "/" {
		return "/" + slug
	}
	return NormalizePath(parentPath + "/" + slug)
}

// PaginatedPath returns the canonical path for page n of an archive at path.
// Page 1 is the archive itself and later pages live under /page/N.
func PaginatedPath(path string, page int) string {
	path = NormalizePath(path)
	if page <= 1 {
		return path
	}
	if path == "/" {
		return fmt.Sprintf("/page/%d", page)
	}
	return fmt.Sprintf("%s/page/%d", path, page)
}

// ValidatePostsBasePath enforces the structural rules for posts_base_path
// before it is written to settings and used to create archive routes.
func ValidatePostsBasePath(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return errors.New("Posts URL base must not be empty")
	}
	if !strings.HasPrefix(p, "/") {
		return errors.New("Posts URL base must start with /")
	}
	if p == "/" {
		return errors.New("Posts URL base must not be /")
	}
	if strings.Contains(p, "?") || strings.Contains(p, "#") {
		return errors.New("Posts URL base must not contain query string or fragment")
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return errors.New("Posts URL base must not end with / (except for root)")
	}
	if strings.Contains(p, "//") {
		return errors.New("Posts URL base must not contain //")
	}
	if strings.Contains(p, " ") {
		return errors.New("Posts URL base must not contain whitespace")
	}
	if strings.Contains(p, "/./") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/.") || strings.HasSuffix(p, "/..") {
		return errors.New("Posts URL base must not contain . or .. segments")
	}
	if strings.Contains(strings.ToLower(p), "%2f") {
		return errors.New("Posts URL base must not contain encoded slash")
	}
	segments := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for _, seg := range segments {
		if seg == "" {
			return errors.New("Posts URL base must not contain empty segments")
		}
		for _, ch := range seg {
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return errors.New("Posts URL base may contain only lowercase letters, numbers and hyphens")
		}
		if strings.HasPrefix(seg, "-") || strings.HasSuffix(seg, "-") {
			return errors.New("Posts URL base segments must not start or end with hyphen")
		}
	}
	reservedPrefixes := []string{"/admin", "/stratum", "/media"}
	for _, prefix := range reservedPrefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return fmt.Errorf("Posts URL base %s conflicts with reserved path %s", p, prefix)
		}
	}
	reservedExact := []string{"/sitemap.xml", "/robots.txt", "/feed.xml", "/favicon.ico"}
	for _, r := range reservedExact {
		if p == r {
			return fmt.Errorf("Posts URL base %s conflicts with reserved path %s", p, r)
		}
	}
	return nil
}

// ArchivePathFor returns the public path for an archive of the given content type.
// It is driven by ContentTypeDefinition.RoutingPolicy, so adding a new archived
// type does not require a new `if contentType=="post"` branch.
func ArchivePathFor(contentTypeID, postsBasePath string, homepageMode string) string {
	def := content.DefinitionFor(contentTypeID)
	if !def.IsArchived() {
		return ""
	}
	if homepageMode == "latest_posts" {
		return "/"
	}
	return PostsArchivePath(postsBasePath)
}

// ContentTypeForArchive returns the content type that owns an archive at path,
// or "" if none. It iterates over all known archived types so new types do
// not need a hardcoded branch.
func ContentTypeForArchive(path string, postsBasePath string, homepageMode string) string {
	for ct := range content.KnownDefinitions() {
		if ArchivePathFor(string(ct), postsBasePath, homepageMode) == path {
			return string(ct)
		}
	}
	// Fallback for custom types not in KnownDefinitions (e.g. DB-only): check
	// generic archived fallback via DefinitionFor path match.
	if def := content.DefinitionFor("post"); def.IsArchived() && ArchivePathFor("post", postsBasePath, homepageMode) == path {
		return "post"
	}
	return ""
}

// TaxonomyTermPath is central taxonomy path builder (DO NOT construct "/category/"+slug in handlers).
func TaxonomyTermPath(routeBase, termSlug string) string {
	base := NormalizePath(strings.TrimSpace(routeBase))
	if base == "" || base == "/" {
		base = "/category"
	}
	slug := strings.Trim(strings.ToLower(strings.TrimSpace(termSlug)), "/")
	if slug == "" {
		return base
	}
	if base == "/" {
		return "/" + slug
	}
	return NormalizePath(base + "/" + slug)
}

// ValidateTermSlug validates a term slug.
func ValidateTermSlug(slug string) error {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return errors.New("slug is required")
	}
	// same pattern as entry slugs
	for _, ch := range slug {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return errors.New("slug may contain lowercase letters, numbers, and hyphens only")
	}
	if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") || strings.Contains(slug, "--") {
		// allow simple check; stricter pattern is enforced by NormalizeSlug
	}
	if slug == "admin" || slug == "stratum" || slug == "sitemap.xml" || slug == "robots.txt" || slug == "feed.xml" {
		return errors.New("slug is reserved")
	}
	return nil
}

// ValidateTaxonomyRouteBase validates a taxonomy route_base.
func ValidateTaxonomyRouteBase(base string) error {
	base = strings.TrimSpace(base)
	if base == "" {
		return errors.New("route base must not be empty")
	}
	return ValidatePostsBasePath(base) // reuse same strict rules; category/tag both pass
}
