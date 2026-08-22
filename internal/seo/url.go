package seo

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// BaseURL returns the absolute origin used to build every public absolute URL
// (canonicals, Open Graph tags, JSON-LD). A configured site URL always wins
// over the request origin, so production pages never accidentally depend on
// the incoming Host header. The request-origin fallback exists for
// development and local setups where no site URL has been configured yet.
func BaseURL(siteURL, origin string) string {
	base := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(origin), "/")
	}
	return base
}

// Canonical builds the absolute canonical URL for a page. This is the single
// place in Stratum where canonicals are assembled; themes, structured data and
// social tags all consume its output.
//
// Precedence:
//  1. override: an absolute http(s) URL passes through unchanged apart from
//     normalisation (external canonicals are allowed); a root-relative path is
//     joined onto Base.
//  2. otherwise: Base + path.
//
// Trailing-slash policy: exactly one slash is kept on the root path ("/") and
// trailing slashes are stripped everywhere else, so each URL has exactly one
// canonical spelling.
func Canonical(siteURL, origin, path, override string) string {
	override = strings.TrimSpace(override)
	switch {
	case override == "":
	case strings.HasPrefix(override, "http://"), strings.HasPrefix(override, "https://"):
		if parsed, err := url.Parse(override); err == nil && parsed.Host != "" {
			return normalizeAbsolute(parsed)
		}
		return override
	case strings.HasPrefix(override, "/"):
		return BaseURL(siteURL, origin) + NormalizePath(override)
	default:
		return override
	}
	return BaseURL(siteURL, origin) + NormalizePath(path)
}

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

// normalizeAbsolute rewrites an absolute URL into the canonical spelling:
// lowercase scheme/host, trailing-slash policy on the path, no fragment.
func normalizeAbsolute(u *url.URL) string {
	host := strings.ToLower(u.Host)
	out := strings.ToLower(u.Scheme) + "://" + host + NormalizePath(u.Path)
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

// PaginatedPath returns the canonical path for page n of an archive at path,
// preparing for blog pagination: page 1 is the archive itself and later pages
// live under /page/N ("/blog/page/2"). Every paginated page is its own
// canonical entity — callers must never collapse it onto the first page.
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

// DefaultPostsBase is the conventional default for the posts archive and all
// single post URLs. It is the value of posts_base_path when not customized.
const DefaultPostsBase = "/blog"

// EntryPath is the single place that computes the public path for an Entry.
// Page → "/{slug}"
// Post → "{postsBase}/{slug}" (default /blog)
// Future content types can extend here without touching dozens of call sites.
func EntryPath(contentTypeID, slug, postsBasePath string) string {
	s := strings.Trim(slug, "/")
	if s == "" {
		return "/"
	}
	if contentTypeID == "post" {
		base := PostsArchivePath(postsBasePath)
		if base == "/" {
			return "/" + s
		}
		return NormalizePath(base + "/" + s)
	}
	return "/" + s
}

// PostsArchivePath returns the base under which the post archive and all post
// singles are published. Never returns empty; falls back to DefaultPostsBase.
func PostsArchivePath(postsBasePath string) string {
	p := NormalizePath(strings.TrimSpace(postsBasePath))
	if p == "" || p == "/" {
		// "/" as posts base is allowed only for "latest posts as homepage" mode,
		// but we still surface a non-root archive path for explicit /blog links.
		// Callers that want root archive use the homepage mode.
		return DefaultPostsBase
	}
	return p
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
	if strings.Contains(p, "?") || strings.Contains(p, "#") {
		return errors.New("Posts URL base must not contain query string or fragment")
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return errors.New("Posts URL base must not end with / (except for root)")
	}
	reserved := []string{"/admin", "/stratum", "/media", "/sitemap.xml", "/robots.txt", "/feed.xml"}
	for _, r := range reserved {
		if p == r {
			return fmt.Errorf("Posts URL base %s conflicts with reserved path %s", p, r)
		}
	}
	return nil
}
