package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

type App struct {
	Database *storage.Database
	Queries  *db.Queries
	Blocks   *blocks.Registry
	Themes   *themes.Runtime
	Media    *media.Service
}

func New(ctx context.Context) (*App, error) {
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	database, err := storage.Open(filepath.Join(dataDir, "stratum.db"))
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}

	if err := database.Migrate(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("migrate storage: %w", err)
	}

	queries := db.New(database.DB)

	storageRoot := filepath.Join(dataDir, "media")
	mediaStore, err := media.NewLocalStorage(storageRoot)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("init media storage: %w", err)
	}
	mediaService := media.NewServiceWithDB(database.DB, queries, mediaStore)

	registry, err := blocks.NewRegistry(ctx, queries, mediaService)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("load block registry: %w", err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("load theme runtime: %w", err)
	}

	return &App{
		Database: database,
		Queries:  queries,
		Blocks:   registry,
		Themes:   themeRuntime,
		Media:    mediaService,
	}, nil
}

func (a *App) Close() error {
	return a.Database.Close()
}
