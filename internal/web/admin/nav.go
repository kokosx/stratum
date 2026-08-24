package admin

import "strings"

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
			{ID: "posts-all", Label: "All Posts", Href: "/admin/posts"},
			{ID: "posts-new", Label: "Add New", Href: "/admin/posts/new"},
			{ID: "posts-categories", Label: "Categories", Href: "/admin/posts/categories"},
			{ID: "posts-tags", Label: "Tags", Href: "/admin/posts/tags"},
		}},
		{ID: "pages", Label: "Pages", Href: "/admin/pages", Icon: "pages", Children: []AdminNavItem{
			{ID: "pages-all", Label: "All Pages", Href: "/admin/pages"},
			{ID: "pages-new", Label: "Add New", Href: "/admin/pages/new"},
		}},
		{ID: "media", Label: "Media", Href: "/admin/media", Icon: "media", Children: []AdminNavItem{
			{ID: "media-library", Label: "Library", Href: "/admin/media"},
			{ID: "media-new", Label: "Add New", Href: "/admin/media"},
		}},
		{ID: "appearance", Label: "Appearance", Href: "/admin/appearance", Icon: "appearance", Children: []AdminNavItem{
			{ID: "appearance-styles", Label: "Styles", Href: "/admin/appearance"},
			{ID: "appearance-menus", Label: "Menus", Href: "/admin/menus"},
			{ID: "appearance-templates", Label: "Layout Templates", Href: "/admin/appearance/templates"},
		}},
		{ID: "settings", Label: "Settings", Href: "/admin/settings/general", Icon: "settings", Children: []AdminNavItem{
			{ID: "settings-general", Label: "General", Href: "/admin/settings/general"},
			{ID: "settings-reading", Label: "Reading", Href: "/admin/settings/reading"},
			{ID: "settings-seo", Label: "SEO & Crawling", Href: "/admin/settings/seo"},
			{ID: "settings-performance", Label: "Performance", Href: "/admin/settings/performance"},
		}},
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
	case strings.HasPrefix(path, "/admin/media"):
		// No separate /media/new route yet; both Library and Add New point to /admin/media.
		// Keep Library active for now.
		return NavState{ActiveSection: "media", ActiveItem: "media-library"}
	case strings.HasPrefix(path, "/admin/appearance"):
		if strings.HasPrefix(path, "/admin/appearance/templates") {
			return NavState{ActiveSection: "appearance", ActiveItem: "appearance-templates"}
		}
		return NavState{ActiveSection: "appearance", ActiveItem: "appearance-styles"}
	case strings.HasPrefix(path, "/admin/menus"):
		return NavState{ActiveSection: "appearance", ActiveItem: "appearance-menus"}
	case strings.HasPrefix(path, "/admin/settings"):
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
	}
	return NavState{}
}
