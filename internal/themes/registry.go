package themes

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Migration struct {
	ThemeID     string
	FromVersion int
	ToVersion   int
	Migrate     func(settings map[string]any) (map[string]any, error)
}

type Registry struct {
	definitions map[string]*Definition
	migrations  map[string][]Migration
}

func NewRegistry() (*Registry, error) {
	defaultTheme, err := loadDefaultDefinition()
	if err != nil {
		return nil, err
	}
	return &Registry{
		definitions: map[string]*Definition{defaultTheme.ID: defaultTheme},
		migrations:  map[string][]Migration{},
	}, nil
}

func (r *Registry) RegisterMigration(themeID string, migration Migration) {
	r.migrations[themeID] = append(r.migrations[themeID], migration)
}

func (r *Registry) MigrateSettings(themeID string, fromVersion, toVersion int, settingsJSON string) (map[string]any, error) {
	// No migration needed: stored and definition versions match.
	if fromVersion == toVersion {
		var result map[string]any
		decoder := json.NewDecoder(strings.NewReader(settingsJSON))
		decoder.UseNumber()
		if err := decoder.Decode(&result); err != nil {
			return nil, fmt.Errorf("decode settings: %w", err)
		}
		return result, nil
	}
	// A stored version newer than the definition has no forward migration
	// available, so the caller should fall back to defaults rather than
	// silently reusing the newer settings.
	if fromVersion > toVersion {
		return nil, fmt.Errorf("no migration from version %d to %d for theme %s", fromVersion, toVersion, themeID)
	}

	migrations := r.migrations[themeID]
	currentJSON := settingsJSON
	currentVersion := fromVersion

	for currentVersion < toVersion {
		var next Migration
		found := false
		for _, m := range migrations {
			if m.FromVersion == currentVersion {
				next = m
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no migration from version %d for theme %s", currentVersion, themeID)
		}
		decoder := json.NewDecoder(strings.NewReader(currentJSON))
		decoder.UseNumber()
		var current map[string]any
		if err := decoder.Decode(&current); err != nil {
			return nil, fmt.Errorf("decode settings for migration: %w", err)
		}
		migrated, err := next.Migrate(current)
		if err != nil {
			return nil, fmt.Errorf("migrate %s v%d->v%d: %w", themeID, currentVersion, next.ToVersion, err)
		}
		encoded, err := json.Marshal(migrated)
		if err != nil {
			return nil, fmt.Errorf("encode migrated settings: %w", err)
		}
		currentJSON = string(encoded)
		currentVersion = next.ToVersion
	}

	decoder := json.NewDecoder(strings.NewReader(currentJSON))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode final migrated settings: %w", err)
	}
	return result, nil
}

func (r *Registry) Exact(id string, version int) (*Definition, error) {
	if id == "default" { // Compatibility with the original site_settings seed.
		id = "stratum/default"
	}
	definition := r.definitions[id]
	if definition == nil || definition.Version != version {
		return nil, fmt.Errorf("theme %s@%d is not registered", id, version)
	}
	return definition, nil
}

func (r *Registry) Current(id string) (*Definition, error) {
	if id == "default" {
		id = "stratum/default"
	}
	definition := r.definitions[id]
	if definition == nil {
		return nil, fmt.Errorf("theme %s is not registered", id)
	}
	return definition, nil
}
