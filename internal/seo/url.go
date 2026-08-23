package seo

import (
	"net/url"
	"strings"

	"github.com/kokosx/stratum/internal/routing"
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
func NormalizePath(path string) string { return routing.NormalizePath(path) }

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

// PaginatedPath returns the canonical path for page n of an archive at path.
func PaginatedPath(path string, page int) string { return routing.PaginatedPath(path, page) }

// DefaultPostsBase is the conventional default for the posts archive and all
// single post URLs. It is the value of posts_base_path when not customized.
const DefaultPostsBase = routing.DefaultPostsBase

// EntryPath is the single place that computes the public path for an Entry.
func EntryPath(contentTypeID, slug, postsBasePath string) string {
	return routing.EntryPath(contentTypeID, slug, postsBasePath)
}

// PostsArchivePath returns the base under which the post archive and all post
// singles are published.
func PostsArchivePath(postsBasePath string) string { return routing.PostsArchivePath(postsBasePath) }

// ValidatePostsBasePath enforces the structural rules for posts_base_path.
func ValidatePostsBasePath(p string) error { return routing.ValidatePostsBasePath(p) }
