package navigation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/slug"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

var (
	ErrInvalidMenu = errors.New("invalid navigation menu")
)

type ItemInput struct {
	ID         string
	ParentID   string
	Position   int
	Label      string
	TargetType string
	EntryID    string
	URL        string
}

type Service struct {
	database *sql.DB
	queries  *db.Queries
}

func NewService(database *sql.DB, queries *db.Queries) *Service {
	return &Service{database: database, queries: queries}
}

func (s *Service) CreateMenu(ctx context.Context, name string) (db.NavigationMenu, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.NavigationMenu{}, fmt.Errorf("%w: name is required", ErrInvalidMenu)
	}
	id, err := newID()
	if err != nil {
		return db.NavigationMenu{}, err
	}
	slug := menuSlug(name)
	if _, err := s.queries.GetNavigationMenuBySlug(ctx, slug); err == nil {
		slug += "-" + strings.ToLower(id[:6])
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.NavigationMenu{}, fmt.Errorf("check menu slug: %w", err)
	}
	now := time.Now().Unix()
	if err := s.queries.CreateNavigationMenu(ctx, db.CreateNavigationMenuParams{ID: id, Name: name, Slug: slug, CreatedAt: now, UpdatedAt: now}); err != nil {
		return db.NavigationMenu{}, fmt.Errorf("create menu: %w", err)
	}
	return s.queries.GetNavigationMenu(ctx, id)
}

func (s *Service) DeleteMenu(ctx context.Context, menuID string) error {
	if _, err := s.queries.GetNavigationMenu(ctx, menuID); err != nil {
		return err
	}
	return s.queries.DeleteNavigationMenu(ctx, menuID)
}

// SaveMenu replaces a menu's complete structure and location assignments atomically.
func (s *Service) SaveMenu(ctx context.Context, menuID, name string, items []ItemInput, locations []string) error {
	if s.database == nil {
		return errors.New("navigation database is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidMenu)
	}
	if err := validateStructure(items); err != nil {
		return err
	}
	locations, err := cleanLocations(locations)
	if err != nil {
		return err
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin menu save: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	menu, err := qtx.GetNavigationMenu(ctx, menuID)
	if err != nil {
		return fmt.Errorf("get menu: %w", err)
	}
	for _, item := range items {
		if item.TargetType == "entry" {
			if _, err := qtx.GetEntry(ctx, item.EntryID); err != nil {
				return fmt.Errorf("%w: entry target %q does not exist", ErrInvalidMenu, item.EntryID)
			}
		}
	}
	now := time.Now().Unix()
	if err := qtx.UpdateNavigationMenu(ctx, db.UpdateNavigationMenuParams{Name: name, Slug: menu.Slug, UpdatedAt: now, ID: menuID}); err != nil {
		return fmt.Errorf("update menu: %w", err)
	}
	existing, err := qtx.ListNavigationItemsByMenu(ctx, menuID)
	if err != nil {
		return fmt.Errorf("load existing menu items: %w", err)
	}
	createdAt := make(map[string]int64, len(existing))
	for _, item := range existing {
		createdAt[item.ID] = item.CreatedAt
	}
	if err := qtx.DeleteNavigationItemsByMenu(ctx, menuID); err != nil {
		return fmt.Errorf("replace menu items: %w", err)
	}
	children := make(map[string][]ItemInput)
	for _, item := range items {
		children[item.ParentID] = append(children[item.ParentID], item)
	}
	for parentID := range children {
		sort.SliceStable(children[parentID], func(i, j int) bool {
			return children[parentID][i].Position < children[parentID][j].Position
		})
	}
	var insert func(string) error
	insert = func(parentID string) error {
		for position, item := range children[parentID] {
			itemCreatedAt := createdAt[item.ID]
			if itemCreatedAt == 0 {
				itemCreatedAt = now
			}
			parent := nullString(item.ParentID)
			entryID, url := sql.NullString{}, sql.NullString{}
			if item.TargetType == "entry" {
				entryID = nullString(item.EntryID)
			} else {
				url = nullString(item.URL)
			}
			if err := qtx.CreateNavigationItem(ctx, db.CreateNavigationItemParams{
				ID: item.ID, MenuID: menuID, ParentID: parent, Position: int64(position), Label: strings.TrimSpace(item.Label),
				TargetType: item.TargetType, EntryID: entryID, Url: url, CreatedAt: itemCreatedAt, UpdatedAt: now,
			}); err != nil {
				return fmt.Errorf("insert menu item %q: %w", item.ID, err)
			}
			if err := insert(item.ID); err != nil {
				return err
			}
		}
		return nil
	}
	if err := insert(""); err != nil {
		return err
	}
	if err := qtx.DeleteNavigationLocationsForMenu(ctx, menuID); err != nil {
		return fmt.Errorf("replace menu locations: %w", err)
	}
	for _, location := range locations {
		if err := qtx.UpsertNavigationLocation(ctx, db.UpsertNavigationLocationParams{Location: location, MenuID: menuID}); err != nil {
			return fmt.Errorf("assign menu location %q: %w", location, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit menu save: %w", err)
	}
	return nil
}

func validateStructure(items []ItemInput) error {
	byID := make(map[string]ItemInput, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Label) == "" {
			return fmt.Errorf("%w: every item needs an id and label", ErrInvalidMenu)
		}
		if _, exists := byID[item.ID]; exists {
			return fmt.Errorf("%w: duplicate item id %q", ErrInvalidMenu, item.ID)
		}
		switch item.TargetType {
		case "entry":
			if item.EntryID == "" || item.URL != "" {
				return fmt.Errorf("%w: entry item %q has inconsistent target fields", ErrInvalidMenu, item.ID)
			}
		case "url":
			if strings.TrimSpace(item.URL) == "" || item.EntryID != "" {
				return fmt.Errorf("%w: url item %q has inconsistent target fields", ErrInvalidMenu, item.ID)
			}
		case "group":
			if item.EntryID != "" || item.URL != "" {
				return fmt.Errorf("%w: group item %q cannot have a target", ErrInvalidMenu, item.ID)
			}
		default:
			return fmt.Errorf("%w: unsupported target type %q", ErrInvalidMenu, item.TargetType)
		}
		byID[item.ID] = item
	}
	for _, item := range items {
		if item.ParentID != "" {
			if _, ok := byID[item.ParentID]; !ok {
				return fmt.Errorf("%w: parent of %q is not in the same menu", ErrInvalidMenu, item.ID)
			}
		}
		seen := map[string]bool{item.ID: true}
		for parent := item.ParentID; parent != ""; parent = byID[parent].ParentID {
			if seen[parent] {
				return fmt.Errorf("%w: item hierarchy contains a cycle", ErrInvalidMenu)
			}
			seen[parent] = true
		}
	}
	return nil
}

func cleanLocations(locations []string) ([]string, error) {
	result := make([]string, 0, len(locations))
	seen := make(map[string]bool)
	for _, location := range locations {
		location = strings.TrimSpace(location)
		if location == "" {
			return nil, fmt.Errorf("%w: location cannot be empty", ErrInvalidMenu)
		}
		if !seen[location] {
			seen[location] = true
			result = append(result, location)
		}
	}
	return result, nil
}

func menuSlug(name string) string {
	canonical := slug.Slugify(name)
	if canonical == "" {
		return "menu"
	}
	return canonical
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
