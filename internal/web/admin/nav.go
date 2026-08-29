package admin

import (
	"strings"

	"github.com/kokosx/stratum/internal/authz"
)

// AdminNavItem is the single source of truth for admin sidebar structure.
type AdminNavItem struct {
	ID       string
	Label    string
	Href     string
	Icon     string // simple key for inline SVG, not a full library
	Children []AdminNavItem
}

// NavState holds resolved active section/item for a request path.
type NavState struct {
	ActiveSection string
	ActiveItem    string
}

// AdminNav returns the hierarchical navigation. Source of truth lives here only.
func AdminNav() []AdminNavItem {
	return []AdminNavItem{
		{ID: "dashboard", Label: "Dashboard", Href: "/admin", Icon: "dashboard"},
		{ID: "posts", Label: "Posts", Href: "/admin/posts", Icon: "posts", Children: []AdminNavItem{
			{ID: "posts-all", Label: "All posts", Href: "/admin/posts"},
			{ID: "posts-new", Label: "Add post", Href: "/admin/posts/new"},
			{ID: "posts-categories", Label: "Categories", Href: "/admin/posts/categories"},
			{ID: "posts-tags", Label: "Tags", Href: "/admin/posts/tags"},
		}},
		{ID: "pages", Label: "Pages", Href: "/admin/pages", Icon: "pages", Children: []AdminNavItem{
			{ID: "pages-all", Label: "All pages", Href: "/admin/pages"},
			{ID: "pages-new", Label: "Add page", Href: "/admin/pages/new"},
		}},
		{ID: "comments", Label: "Comments", Href: "/admin/comments", Icon: "comments"},
		{ID: "forms", Label: "Forms", Href: "/admin/forms", Icon: "forms", Children: []AdminNavItem{
			{ID: "forms-all", Label: "All forms", Href: "/admin/forms"},
			{ID: "forms-new", Label: "Add form", Href: "/admin/forms/new"},
		}},
		{ID: "media", Label: "Media", Href: "/admin/media", Icon: "media", Children: []AdminNavItem{
			{ID: "media-library", Label: "Library", Href: "/admin/media"},
		}},
		{ID: "appearance", Label: "Appearance", Href: "/admin/appearance", Icon: "appearance", Children: []AdminNavItem{
			{ID: "appearance-styles", Label: "Styles", Href: "/admin/appearance"},
			{ID: "appearance-templates", Label: "Templates", Href: "/admin/appearance/templates"},
			{ID: "appearance-site-parts", Label: "Site Parts", Href: "/admin/appearance/site-parts"},
			{ID: "appearance-menus", Label: "Menus", Href: "/admin/menus"},
		}},
		{ID: "settings", Label: "Settings", Href: "/admin/settings/general", Icon: "settings", Children: []AdminNavItem{
			{ID: "settings-general", Label: "General", Href: "/admin/settings/general"},
			{ID: "settings-content-types", Label: "Content Types", Href: "/admin/settings/content-types"},
			{ID: "settings-reading", Label: "Reading", Href: "/admin/settings/reading"},
			{ID: "settings-seo", Label: "SEO & Crawling", Href: "/admin/settings/seo"},
			{ID: "settings-performance", Label: "Performance", Href: "/admin/settings/performance"},
		}},
		{ID: "tools", Label: "Tools", Href: "/admin/tools/site-health", Icon: "settings", Children: []AdminNavItem{
			{ID: "tools-site-health", Label: "Site Health", Href: "/admin/tools/site-health"},
			{ID: "tools-redirects", Label: "Redirects", Href: "/admin/tools/redirects"},
			{ID: "tools-not-found", Label: "Not Found", Href: "/admin/tools/not-found"},
		}},
		{ID: "users", Label: "Users", Href: "/admin/users", Icon: "users"},
	}
}

// FilterAdminNav keeps navigation as a view of the same central policy that
// protects routes. It is not a security boundary; handlers enforce the policy.
func FilterAdminNav(nav []AdminNavItem, role string) []AdminNavItem {
	filtered := make([]AdminNavItem, 0, len(nav))
	for _, item := range nav {
		if !authz.Allows(role, navPermission(item.Href)) {
			continue
		}
		children := make([]AdminNavItem, 0, len(item.Children))
		for _, child := range item.Children {
			if authz.Allows(role, navPermission(child.Href)) {
				children = append(children, child)
			}
		}
		item.Children = children
		filtered = append(filtered, item)
	}
	return filtered
}

func navPermission(path string) authz.Permission {
	switch {
	case strings.HasPrefix(path, "/admin/users"):
		return authz.ManageUsers
	case strings.HasPrefix(path, "/admin/menus"):
		return authz.ManageNavigation
	case strings.HasPrefix(path, "/admin/settings"), strings.HasPrefix(path, "/admin/appearance"), strings.HasPrefix(path, "/admin/tools"):
		return authz.ManageSite
	case strings.HasPrefix(path, "/admin/media"):
		return authz.ManageMedia
	case strings.HasPrefix(path, "/admin/comments"):
		return authz.ModerateComments
	case strings.HasPrefix(path, "/admin/posts/categories"), strings.HasPrefix(path, "/admin/posts/tags"):
		return authz.ManageTaxonomies
	case strings.HasPrefix(path, "/admin/pages"):
		return authz.EditAnyEntry
	case strings.HasPrefix(path, "/admin/posts"):
		return authz.ReadEntries
	default:
		return authz.ReadEntries
	}
}

// ResolveNav maps a request path to active section/item. It is the single
// central route/navigation-state resolver; handlers must not guess active strings.
func ResolveNav(path string) NavState {
	// Strip query string and normalization: remove trailing slash except root.
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if i := strings.Index(path, "#"); i >= 0 {
		path = path[:i]
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	// Direct matches by prefix.
	switch {
	case path == "/admin":
		return NavState{ActiveSection: "dashboard", ActiveItem: "dashboard"}
	case strings.HasPrefix(path, "/admin/comments"):
		return NavState{ActiveSection: "comments", ActiveItem: "comments"}
	case strings.HasPrefix(path, "/admin/forms"):
		if path == "/admin/forms/new" {
			return NavState{ActiveSection: "forms", ActiveItem: "forms-new"}
		}
		return NavState{ActiveSection: "forms", ActiveItem: "forms-all"}
	case strings.HasPrefix(path, "/admin/posts"):
		if strings.HasPrefix(path, "/admin/posts/categories") {
			return NavState{ActiveSection: "posts", ActiveItem: "posts-categories"}
		}
		if strings.HasPrefix(path, "/admin/posts/tags") {
			return NavState{ActiveSection: "posts", ActiveItem: "posts-tags"}
		}
		if path == "/admin/posts/new" {
			return NavState{ActiveSection: "posts", ActiveItem: "posts-new"}
		}
		if path == "/admin/posts" {
			return NavState{ActiveSection: "posts", ActiveItem: "posts-all"}
		}
		// /admin/posts/{id}/edit or similar -> section only (parent expanded)
		return NavState{ActiveSection: "posts", ActiveItem: "posts-all"}
	case strings.HasPrefix(path, "/admin/pages"):
		if path == "/admin/pages/new" {
			return NavState{ActiveSection: "pages", ActiveItem: "pages-new"}
		}
		if path == "/admin/pages" {
			return NavState{ActiveSection: "pages", ActiveItem: "pages-all"}
		}
		return NavState{ActiveSection: "pages", ActiveItem: "pages-all"}
	case strings.HasPrefix(path, "/admin/content/"):
		rest := strings.TrimPrefix(path, "/admin/content/")
		parts := strings.Split(rest, "/")
		if len(parts) > 0 && parts[0] != "" {
			id := "content-" + parts[0]
			item := id + "-all"
			if len(parts) > 1 && parts[1] == "new" {
				item = id + "-new"
			}
			return NavState{ActiveSection: id, ActiveItem: item}
		}
	case strings.HasPrefix(path, "/admin/media"):
		return NavState{ActiveSection: "media", ActiveItem: "media-library"}
	case strings.HasPrefix(path, "/admin/appearance"):
		if strings.HasPrefix(path, "/admin/appearance/site-parts") {
			return NavState{ActiveSection: "appearance", ActiveItem: "appearance-site-parts"}
		}
		if strings.HasPrefix(path, "/admin/appearance/templates") {
			return NavState{ActiveSection: "appearance", ActiveItem: "appearance-templates"}
		}
		return NavState{ActiveSection: "appearance", ActiveItem: "appearance-styles"}
	case strings.HasPrefix(path, "/admin/menus"):
		return NavState{ActiveSection: "appearance", ActiveItem: "appearance-menus"}
	case strings.HasPrefix(path, "/admin/settings"):
		if strings.HasPrefix(path, "/admin/settings/content-types") {
			return NavState{ActiveSection: "settings", ActiveItem: "settings-content-types"}
		}
		if strings.HasPrefix(path, "/admin/settings/general") {
			return NavState{ActiveSection: "settings", ActiveItem: "settings-general"}
		}
		if strings.HasPrefix(path, "/admin/settings/reading") {
			return NavState{ActiveSection: "settings", ActiveItem: "settings-reading"}
		}
		if strings.HasPrefix(path, "/admin/settings/seo") {
			return NavState{ActiveSection: "settings", ActiveItem: "settings-seo"}
		}
		if strings.HasPrefix(path, "/admin/settings/performance") {
			return NavState{ActiveSection: "settings", ActiveItem: "settings-performance"}
		}
		return NavState{ActiveSection: "settings", ActiveItem: "settings-general"}
	case strings.HasPrefix(path, "/admin/tools"):
		if strings.HasPrefix(path, "/admin/tools/redirects") {
			return NavState{ActiveSection: "tools", ActiveItem: "tools-redirects"}
		}
		if strings.HasPrefix(path, "/admin/tools/not-found") {
			return NavState{ActiveSection: "tools", ActiveItem: "tools-not-found"}
		}
		return NavState{ActiveSection: "tools", ActiveItem: "tools-site-health"}
	case strings.HasPrefix(path, "/admin/users"):
		return NavState{ActiveSection: "users", ActiveItem: "users"}
	}
	return NavState{}
}
