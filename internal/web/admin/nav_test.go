package admin

import "testing"

func TestResolveNav_PostsExpandedAllPostsActive(t *testing.T) {
	s := ResolveNav("/admin/posts")
	if s.ActiveSection != "posts" || s.ActiveItem != "posts-all" {
		t.Fatalf("got %+v want posts/posts-all", s)
	}
}

func TestResolveNav_PostsNewAddNewActive(t *testing.T) {
	s := ResolveNav("/admin/posts/new")
	if s.ActiveSection != "posts" || s.ActiveItem != "posts-new" {
		t.Fatalf("got %+v", s)
	}
}

func TestResolveNav_PostsEditExpanded(t *testing.T) {
	s := ResolveNav("/admin/posts/abc123/edit")
	if s.ActiveSection != "posts" {
		t.Fatalf("got %+v want posts expanded", s)
	}
	if s.ActiveItem != "posts-all" {
		t.Fatalf("edit should also be posts-all, got %s", s.ActiveItem)
	}
}

func TestResolveNav_PostsCategories(t *testing.T) {
	s := ResolveNav("/admin/posts/categories")
	if s.ActiveSection != "posts" || s.ActiveItem != "posts-categories" {
		t.Fatalf("got %+v want posts/posts-categories", s)
	}
}

func TestResolveNav_PostsTags(t *testing.T) {
	s := ResolveNav("/admin/posts/tags")
	if s.ActiveSection != "posts" || s.ActiveItem != "posts-tags" {
		t.Fatalf("got %+v want posts/posts-tags", s)
	}
}

func TestResolveNav_AppearanceTemplates(t *testing.T) {
	s := ResolveNav("/admin/appearance/templates")
	if s.ActiveSection != "appearance" || s.ActiveItem != "appearance-templates" {
		t.Fatalf("got %+v", s)
	}
	s2 := ResolveNav("/admin/appearance/templates/xyz/edit")
	if s2.ActiveSection != "appearance" || s2.ActiveItem != "appearance-templates" {
		t.Fatalf("edit template got %+v", s2)
	}
}

func TestResolveNav_SettingsReading(t *testing.T) {
	s := ResolveNav("/admin/settings/reading")
	if s.ActiveSection != "settings" || s.ActiveItem != "settings-reading" {
		t.Fatalf("got %+v", s)
	}
	s2 := ResolveNav("/admin/settings")
	if s2.ActiveSection != "settings" || s2.ActiveItem != "settings-general" {
		t.Fatalf("settings root got %+v", s2)
	}
}

func TestResolveNav_MenusUnderAppearance(t *testing.T) {
	s := ResolveNav("/admin/menus")
	if s.ActiveSection != "appearance" || s.ActiveItem != "appearance-menus" {
		t.Fatalf("got %+v", s)
	}
}

func TestAdminNavStructureNotHardcodedInHandlers(t *testing.T) {
	nav := AdminNav()
	if len(nav) < 6 {
		t.Fatalf("nav too short %d", len(nav))
	}
	found := map[string]bool{}
	for _, item := range nav {
		found[item.ID] = true
	}
	for _, want := range []string{"posts", "pages", "media", "appearance", "settings", "dashboard"} {
		if !found[want] {
			t.Fatalf("missing nav %s", want)
		}
	}
	// Check Posts has Categories/Tags
	for _, item := range nav {
		if item.ID == "posts" {
			childIDs := map[string]bool{}
			for _, c := range item.Children {
				childIDs[c.ID] = true
			}
			if !childIDs["posts-categories"] || !childIDs["posts-tags"] {
				t.Fatalf("posts missing categories/tags children: %+v", childIDs)
			}
		}
	}
}

func TestAdminNavSettingsHasSectionLinks(t *testing.T) {
	for _, item := range AdminNav() {
		if item.ID != "settings" {
			continue
		}

		children := map[string]string{}
		for _, child := range item.Children {
			children[child.ID] = child.Href
		}
		for id, href := range map[string]string{
			"settings-general":     "/admin/settings/general",
			"settings-reading":     "/admin/settings/reading",
			"settings-seo":         "/admin/settings/seo",
			"settings-performance": "/admin/settings/performance",
		} {
			if children[id] != href {
				t.Fatalf("settings child %q = %q, want %q", id, children[id], href)
			}
		}
		return
	}

	t.Fatal("settings navigation item not found")
}
