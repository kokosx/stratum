package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/navigation"
)

func TestMenuStructureControlsReorderAndNestItems(t *testing.T) {
	items := []navigation.ItemInput{
		{ID: "a", Position: 0},
		{ID: "b", Position: 1},
		{ID: "c", Position: 2},
	}
	moveSibling(items, "c", -1)
	if items[2].Position != 1 || items[1].Position != 2 {
		t.Fatalf("move did not exchange sibling positions: %#v", items)
	}
	indentItem(items, "c")
	if items[2].ParentID != "a" {
		t.Fatalf("indent parent = %q, want a", items[2].ParentID)
	}
	outdentItem(items, "c")
	if items[2].ParentID != "" {
		t.Fatalf("outdent parent = %q, want root", items[2].ParentID)
	}
	items[1].ParentID = "a"
	items[2].ParentID = "b"
	items = removeMenuItem(items, "a")
	if len(items) != 0 {
		t.Fatalf("removing parent retained descendants: %#v", items)
	}
}

func TestDatastarIndentUpdatesOnlyEditorFragmentAndPersistsParent(t *testing.T) {
	handler, _ := newTestHandler(t)
	ctx := context.Background()
	items := []navigation.ItemInput{
		{ID: "first", Position: 0, Label: "First", TargetType: "url", URL: "/first"},
		{ID: "second", Position: 1, Label: "Second", TargetType: "url", URL: "/second"},
	}
	if err := handler.navigation.SaveMenu(ctx, "default-main-navigation", "Main Navigation", items, []string{"primary"}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":       {"test-token"},
		"name":             {"Main Navigation"},
		"item_id":          {"first", "second"},
		"item_parent_id":   {"", ""},
		"item_target_type": {"url", "url"},
		"item_entry_id":    {"", ""},
		"item_url":         {"/first", "/second"},
		"item_label":       {"First", "Second"},
		"locations":        {"primary"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/menus/default-main-navigation?action=indent:second", strings.NewReader(form.Encode()))
	request.SetPathValue("id", "default-main-navigation")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Datastar-Request", "true")
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	response := httptest.NewRecorder()

	handler.updateMenu(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "text/html") || !strings.Contains(response.Body.String(), `id="menu-editor-region"`) || strings.Contains(response.Body.String(), "<!doctype") {
		t.Fatalf("response is not a Datastar editor fragment: %s", response.Body.String())
	}
	stored, err := handler.queries.ListNavigationItemsByMenu(ctx, "default-main-navigation")
	if err != nil {
		t.Fatal(err)
	}
	parents := map[string]string{}
	for _, item := range stored {
		if item.ParentID.Valid {
			parents[item.ID] = item.ParentID.String
		}
	}
	if parents["second"] != "first" {
		t.Fatalf("second parent = %q, want first", parents["second"])
	}

	form["item_parent_id"] = []string{"", "first"}
	request = httptest.NewRequest(http.MethodPost, "/admin/menus/default-main-navigation?action=outdent:second", strings.NewReader(form.Encode()))
	request.SetPathValue("id", "default-main-navigation")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Datastar-Request", "true")
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	response = httptest.NewRecorder()
	handler.updateMenu(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("outdent status = %d, body = %s", response.Code, response.Body.String())
	}
	stored, err = handler.queries.ListNavigationItemsByMenu(ctx, "default-main-navigation")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range stored {
		if item.ID == "second" && item.ParentID.Valid {
			t.Fatalf("second remained nested under %q after outdent", item.ParentID.String)
		}
	}
}
