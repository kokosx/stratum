package navigation

import (
	"context"
	"database/sql"
	"fmt"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type Menu struct {
	ID    string
	Name  string
	Items []MenuItem
}

type MenuItem struct {
	ID       string
	Label    string
	URL      string
	Current  bool
	Ancestor bool
	Children []MenuItem
}

type AdminItem struct {
	ID         string
	ParentID   string
	Position   int
	Label      string
	TargetType string
	EntryID    string
	URL        string
	Public     bool
	TargetName string
	TargetPath string
}

type Loader struct{ queries *db.Queries }

func NewLoader(queries *db.Queries) *Loader { return &Loader{queries: queries} }

func (l *Loader) LoadLocations(ctx context.Context) (map[string]Menu, error) {
	return l.LoadLocationsForPath(ctx, "")
}

// LoadLocationsForPath prepares presentation-ready active state once, before
// templates see navigation. Themes never compare URLs ad hoc.
func (l *Loader) LoadLocationsForPath(ctx context.Context, currentPath string) (map[string]Menu, error) {
	locations, err := l.queries.ListNavigationLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list navigation locations: %w", err)
	}
	result := make(map[string]Menu, len(locations))
	loaded := make(map[string]Menu)
	for _, location := range locations {
		menu, ok := loaded[location.MenuID]
		if !ok {
			menu, err = l.LoadMenu(ctx, location.MenuID)
			if err != nil {
				return nil, err
			}
			markCurrent(menu.Items, currentPath)
			loaded[location.MenuID] = menu
		}
		result[location.Location] = menu
	}
	return result, nil
}

func markCurrent(items []MenuItem, currentPath string) bool {
	containsCurrent := false
	for i := range items {
		items[i].Current = currentPath != "" && items[i].URL == currentPath
		childCurrent := markCurrent(items[i].Children, currentPath)
		items[i].Ancestor = !items[i].Current && childCurrent
		containsCurrent = containsCurrent || items[i].Current || childCurrent
	}
	return containsCurrent
}

func (l *Loader) LoadMenu(ctx context.Context, menuID string) (Menu, error) {
	menu, err := l.queries.GetNavigationMenu(ctx, menuID)
	if err != nil {
		return Menu{}, fmt.Errorf("get navigation menu: %w", err)
	}
	rows, err := l.queries.ListNavigationItemsByMenu(ctx, menuID)
	if err != nil {
		return Menu{}, fmt.Errorf("list navigation items: %w", err)
	}
	children := make(map[string][]db.ListNavigationItemsByMenuRow)
	for _, row := range rows {
		parent := ""
		if row.ParentID.Valid {
			parent = row.ParentID.String
		}
		children[parent] = append(children[parent], row)
	}
	var build func(string) []MenuItem
	build = func(parent string) []MenuItem {
		items := make([]MenuItem, 0, len(children[parent]))
		for _, row := range children[parent] {
			url, public := publicURL(row)
			if !public {
				continue
			}
			items = append(items, MenuItem{ID: row.ID, Label: row.Label, URL: url, Children: build(row.ID)})
		}
		return items
	}
	return Menu{ID: menu.ID, Name: menu.Name, Items: build("")}, nil
}

func (l *Loader) LoadAdminItems(ctx context.Context, menuID string) ([]AdminItem, error) {
	rows, err := l.queries.ListNavigationItemsByMenu(ctx, menuID)
	if err != nil {
		return nil, err
	}
	children := make(map[string][]db.ListNavigationItemsByMenuRow)
	for _, row := range rows {
		parent := ""
		if row.ParentID.Valid {
			parent = row.ParentID.String
		}
		children[parent] = append(children[parent], row)
	}
	result := make([]AdminItem, 0, len(rows))
	var walk func(string)
	walk = func(parent string) {
		for _, row := range children[parent] {
			_, public := publicURL(row)
			targetName, targetPath := value(row.EntryTitle), value(row.EntryPath)
			if row.TargetType == "url" {
				targetName, targetPath = "Custom URL", value(row.Url)
			} else if row.TargetType == "group" {
				targetName, targetPath = "Dropdown heading", "No link"
			} else if targetName == "" {
				targetName, targetPath = "Missing or untitled page", value(row.EntryID)
			}
			result = append(result, AdminItem{ID: row.ID, ParentID: parent, Position: int(row.Position), Label: row.Label, TargetType: row.TargetType, EntryID: value(row.EntryID), URL: value(row.Url), Public: public, TargetName: targetName, TargetPath: targetPath})
			walk(row.ID)
		}
	}
	walk("")
	return result, nil
}

func publicURL(row db.ListNavigationItemsByMenuRow) (string, bool) {
	if row.TargetType == "group" {
		return "", true
	}
	if row.TargetType == "url" {
		return value(row.Url), row.Url.Valid && row.Url.String != ""
	}
	return value(row.EntryPath), row.EntryStatus.Valid && row.EntryStatus.String == "active" && row.EntryPublishedRevisionID.Valid && row.EntryPath.Valid
}

func value(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}
