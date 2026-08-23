package admin

import (
	"bytes"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/kokosx/stratum/internal/navigation"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

type menusData struct {
	Menus     []db.NavigationMenu
	Selected  *db.NavigationMenu
	Pages     []db.ListPublishedPagesForNavigationRow
	Entries   []db.ListPublishedEntriesForNavigationRow // generic, grouped by content_type_id in template
	Items     []menuItemData
	Locations []locationData
	Error     string
	Notice    string
	CSRFToken string
}

type menuItemData struct {
	navigation.AdminItem
	Depth int
}

type locationData struct {
	Name    string
	Label   string
	Checked bool
}

func (h *Handler) listMenus(w http.ResponseWriter, r *http.Request) {
	h.renderMenus(w, r, r.URL.Query().Get("menu"), "")
}

func (h *Handler) createMenu(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	menu, err := h.navigation.CreateMenu(r.Context(), r.FormValue("name"))
	if err != nil {
		h.renderMenus(w, r, "", err.Error())
		return
	}
	if h.runtime != nil {
		if rerr := h.runtime.ReloadNavigation(r.Context()); rerr != nil {
			log.Printf("reload navigation runtime: %v", rerr)
		}
	}
	h.setFlash(w, "Menu created.")
	http.Redirect(w, r, "/admin/menus?menu="+menu.ID, http.StatusSeeOther)
}

func (h *Handler) deleteMenu(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := h.navigation.DeleteMenu(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		if rerr := h.runtime.ReloadNavigation(r.Context()); rerr != nil {
			log.Printf("reload navigation runtime: %v", rerr)
		}
	}
	h.setFlash(w, "Menu deleted.")
	http.Redirect(w, r, "/admin/menus", http.StatusSeeOther)
}

func (h *Handler) updateMenu(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	menuID := r.PathValue("id")
	items, err := postedMenuItems(r)
	if err != nil {
		h.renderMenus(w, r, menuID, err.Error())
		return
	}
	action := r.URL.Query().Get("action")
	if action == "" {
		action = r.FormValue("action")
	}
	switch {
	case action == "add-entry":
		// Generic picker: any published, public, linkable entry (grouped by content type in UI).
		entries, listErr := h.queries.ListPublishedEntriesForNavigation(r.Context())
		if listErr != nil {
			err = listErr
			break
		}
		labels := make(map[string]string, len(entries))
		for _, e := range entries {
			labels[e.ID] = e.Title
		}
		for _, entryID := range r.Form["add_entry_id"] {
			if label, ok := labels[entryID]; ok {
				id, idErr := randomID()
				if idErr != nil {
					err = idErr
					break
				}
				items = append(items, navigation.ItemInput{ID: id, Position: len(items), Label: label, TargetType: "entry", EntryID: entryID})
			}
		}
	case action == "add-url":
		id, idErr := randomID()
		if idErr != nil {
			err = idErr
			break
		}
		items = append(items, navigation.ItemInput{ID: id, Position: len(items), Label: strings.TrimSpace(r.FormValue("custom_label")), TargetType: "url", URL: strings.TrimSpace(r.FormValue("custom_url"))})
	case action == "add-group":
		id, idErr := randomID()
		if idErr != nil {
			err = idErr
			break
		}
		items = append(items, navigation.ItemInput{ID: id, Position: len(items), Label: strings.TrimSpace(r.FormValue("group_label")), TargetType: "group"})
	case strings.HasPrefix(action, "delete:"):
		items = removeMenuItem(items, strings.TrimPrefix(action, "delete:"))
	case strings.HasPrefix(action, "move-up:"):
		moveSibling(items, strings.TrimPrefix(action, "move-up:"), -1)
	case strings.HasPrefix(action, "move-down:"):
		moveSibling(items, strings.TrimPrefix(action, "move-down:"), 1)
	case strings.HasPrefix(action, "indent:"):
		indentItem(items, strings.TrimPrefix(action, "indent:"))
	case strings.HasPrefix(action, "outdent:"):
		outdentItem(items, strings.TrimPrefix(action, "outdent:"))
	}
	locations := append([]string(nil), r.Form["locations"]...)
	for _, custom := range strings.Split(r.FormValue("custom_locations"), ",") {
		if strings.TrimSpace(custom) != "" {
			locations = append(locations, custom)
		}
	}
	if err == nil {
		err = h.navigation.SaveMenu(r.Context(), menuID, r.FormValue("name"), items, locations)
	}
	if err != nil {
		log.Printf("save navigation menu: %v", err)
		if isDatastarRequest(r) {
			h.renderMenuEditorFragment(w, r, menuID, err.Error(), "")
			return
		}
		h.renderMenus(w, r, menuID, err.Error())
		return
	}
	if h.runtime != nil {
		if rerr := h.runtime.ReloadNavigation(r.Context()); rerr != nil {
			log.Printf("reload navigation runtime: %v", rerr)
		}
	}
	if isDatastarRequest(r) {
		h.renderMenuEditorFragment(w, r, menuID, "", "Menu saved.")
		return
	}
	h.setFlash(w, "Menu saved.")
	http.Redirect(w, r, "/admin/menus?menu="+menuID, http.StatusSeeOther)
}

func (h *Handler) renderMenus(w http.ResponseWriter, r *http.Request, selectedID, formError string) {
	menus, err := h.queries.ListNavigationMenus(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if selectedID == "" && len(menus) > 0 {
		selectedID = menus[0].ID
	}
	data := menusData{Menus: menus, Error: formError}
	if selectedID != "" {
		if err := h.populateMenuEditorData(r, &data, selectedID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.CSRFToken = token
	layout := LayoutData{Title: "Menus", ActiveMenu: "menus", Flash: h.consumeFlash(w, r), CSRFToken: token, Content: data}
	if err := h.menusTemplate.ExecuteTemplate(w, "layout.html", layout); err != nil {
		log.Printf("render menus: %v", err)
	}
}

func (h *Handler) renderMenuEditorFragment(w http.ResponseWriter, r *http.Request, menuID, formError, notice string) {
	data := menusData{Error: formError, Notice: notice, CSRFToken: r.FormValue("csrf_token")}
	if err := h.populateMenuEditorData(r, &data, menuID); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.menusTemplate.ExecuteTemplate(&buf, "menu-editor", LayoutData{Content: data}); err != nil {
		log.Printf("render menu editor fragment: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Re-render the whole menu editor region and mirror the result in the
	// shared toast area. Datastar morphs #menu-editor-region in place, so no
	// full document reload happens.
	events := []sseEvent{patchElementsEvent("outer", "", buf.String())}
	if formError != "" {
		events = append(events, toastEvent("error", formError))
	} else if notice != "" {
		events = append(events, toastEvent("success", notice))
	}
	writeSSE(w, events...)
}

func (h *Handler) populateMenuEditorData(r *http.Request, data *menusData, selectedID string) error {
	menu, err := h.queries.GetNavigationMenu(r.Context(), selectedID)
	if err != nil {
		return err
	}
	data.Selected = &menu
	adminItems, err := h.navigationLoader.LoadAdminItems(r.Context(), selectedID)
	if err != nil {
		return err
	}
	depths := map[string]int{"": -1}
	for _, item := range adminItems {
		depth := depths[item.ParentID] + 1
		depths[item.ID] = depth
		data.Items = append(data.Items, menuItemData{AdminItem: item, Depth: depth})
	}
	data.Pages, err = h.queries.ListPublishedPagesForNavigation(r.Context())
	if err != nil {
		return err
	}
	data.Entries, _ = h.queries.ListPublishedEntriesForNavigation(r.Context())
	assigned, err := h.queries.ListNavigationLocationsForMenu(r.Context(), selectedID)
	if err != nil {
		return err
	}
	data.Locations = menuLocations(assigned, h.themes.Current().Schema.MenuLocations)
	return nil
}

func isDatastarRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Datastar-Request"), "true")
}

func postedMenuItems(r *http.Request) ([]navigation.ItemInput, error) {
	ids, labels := r.Form["item_id"], r.Form["item_label"]
	parents, targetTypes := r.Form["item_parent_id"], r.Form["item_target_type"]
	entryIDs, urls := r.Form["item_entry_id"], r.Form["item_url"]
	if len(ids) != len(labels) || len(ids) != len(parents) || len(ids) != len(targetTypes) || len(ids) != len(entryIDs) || len(ids) != len(urls) {
		return nil, errors.New("invalid menu item form")
	}
	items := make([]navigation.ItemInput, len(ids))
	for i := range ids {
		items[i] = navigation.ItemInput{ID: ids[i], ParentID: parents[i], Position: i, Label: labels[i], TargetType: targetTypes[i], EntryID: entryIDs[i], URL: urls[i]}
	}
	return items, nil
}

func removeMenuItem(items []navigation.ItemInput, id string) []navigation.ItemInput {
	remove := map[string]bool{id: true}
	changed := true
	for changed {
		changed = false
		for _, item := range items {
			if remove[item.ParentID] && !remove[item.ID] {
				remove[item.ID] = true
				changed = true
			}
		}
	}
	result := items[:0]
	for _, item := range items {
		if !remove[item.ID] {
			result = append(result, item)
		}
	}
	return result
}

func moveSibling(items []navigation.ItemInput, id string, direction int) {
	index := -1
	for i := range items {
		if items[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	parent := items[index].ParentID
	siblingIndexes := []int{}
	for i := range items {
		if items[i].ParentID == parent {
			siblingIndexes = append(siblingIndexes, i)
		}
	}
	sort.SliceStable(siblingIndexes, func(i, j int) bool {
		return items[siblingIndexes[i]].Position < items[siblingIndexes[j]].Position
	})
	for pos, itemIndex := range siblingIndexes {
		if itemIndex == index && pos+direction >= 0 && pos+direction < len(siblingIndexes) {
			other := siblingIndexes[pos+direction]
			items[index].Position, items[other].Position = items[other].Position, items[index].Position
			return
		}
	}
}

func indentItem(items []navigation.ItemInput, id string) {
	index := -1
	for i := range items {
		if items[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	parent, previous, previousPosition := items[index].ParentID, "", -1
	for i := range items {
		if items[i].ParentID == parent && items[i].Position < items[index].Position && items[i].Position > previousPosition {
			previous, previousPosition = items[i].ID, items[i].Position
		}
	}
	if previous != "" {
		items[index].ParentID = previous
		items[index].Position = nextChildPosition(items, previous)
	}
}

func nextChildPosition(items []navigation.ItemInput, parentID string) int {
	next := 0
	for _, item := range items {
		if item.ParentID == parentID && item.Position >= next {
			next = item.Position + 1
		}
	}
	return next
}

func outdentItem(items []navigation.ItemInput, id string) {
	byID := make(map[string]navigation.ItemInput, len(items))
	index := -1
	for i, item := range items {
		byID[item.ID] = item
		if item.ID == id {
			index = i
		}
	}
	if index >= 0 && items[index].ParentID != "" {
		items[index].ParentID = byID[items[index].ParentID].ParentID
	}
}

func menuLocations(assigned []string, declared []themes.MenuLocation) []locationData {
	labels := make(map[string]string, len(declared))
	all := make([]string, 0, len(declared))
	for _, location := range declared {
		labels[location.ID] = location.Label
		all = append(all, location.ID)
	}
	seen := map[string]bool{}
	for _, name := range all {
		seen[name] = true
	}
	for _, name := range assigned {
		if !seen[name] {
			all = append(all, name)
			seen[name] = true
		}
	}
	sort.Strings(all)
	checked := map[string]bool{}
	for _, name := range assigned {
		checked[name] = true
	}
	result := make([]locationData, 0, len(all))
	for _, name := range all {
		label := labels[name]
		if label == "" {
			label = name
		}
		result = append(result, locationData{Name: name, Label: label, Checked: checked[name]})
	}
	return result
}
