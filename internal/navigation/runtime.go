package navigation

import (
	"context"
	"sync"
	"sync/atomic"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Runtime holds an immutable snapshot of the navigation menus assigned to each
// location. Building the menu tree requires several database queries, so it is
// computed once at Reload and read with an atomic load on the hot path. The
// cheap per-request work is only marking the current path.
type Runtime struct {
	loader   *Loader
	reloadMu sync.Mutex
	snapshot atomic.Pointer[map[string]Menu]
}

func NewRuntime(queries *db.Queries) *Runtime {
	return &Runtime{loader: NewLoader(queries)}
}

// Reload rebuilds all location menus from the database.
func (r *Runtime) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	locations, err := r.loader.queries.ListNavigationLocations(ctx)
	if err != nil {
		return err
	}
	loaded := make(map[string]Menu)
	result := make(map[string]Menu, len(locations))
	for _, location := range locations {
		menu, ok := loaded[location.MenuID]
		if !ok {
			menu, err = r.loader.LoadMenu(ctx, location.MenuID)
			if err != nil {
				return err
			}
			loaded[location.MenuID] = menu
		}
		result[location.Location] = menu
	}
	r.snapshot.Store(&result)
	return nil
}

// LocationsForPath returns the location menus with the item at currentPath
// marked as Current (and its ancestors as Ancestor). The stored snapshot is
// never mutated: a shallow clone carries the marking.
func (r *Runtime) LocationsForPath(currentPath string) map[string]Menu {
	snap := r.snapshot.Load()
	if snap == nil {
		return nil
	}
	result := make(map[string]Menu, len(*snap))
	for location, menu := range *snap {
		result[location] = markMenuCurrent(menu, currentPath)
	}
	return result
}

// markMenuCurrent clones a menu and applies active-state marking. It recurses
// into children so the original menu tree stays immutable.
func markMenuCurrent(menu Menu, currentPath string) Menu {
	items := make([]MenuItem, len(menu.Items))
	for i, item := range menu.Items {
		items[i] = markItemCurrent(item, currentPath)
	}
	return Menu{ID: menu.ID, Name: menu.Name, Items: items}
}

func markItemCurrent(item MenuItem, currentPath string) MenuItem {
	children := make([]MenuItem, len(item.Children))
	for i, child := range item.Children {
		children[i] = markItemCurrent(child, currentPath)
	}
	current := currentPath != "" && item.URL == currentPath
	ancestor := !current
	for _, child := range children {
		if child.Current || child.Ancestor {
			ancestor = true
			break
		}
	}
	if current {
		ancestor = false
	}
	return MenuItem{
		ID:       item.ID,
		Label:    item.Label,
		URL:      item.URL,
		Current:  current,
		Ancestor: ancestor,
		Children: children,
	}
}
