package app

import (
	"context"
	"fmt"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type App struct {
	Database *storage.Database
	Queries  *db.Queries
}

func New(ctx context.Context) (*App, error) {
	database, err := storage.Open("data/stratum.db")
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}

	if err := database.Migrate(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("migrate storage: %w", err)
	}

	queries := db.New(database.DB)

	return &App{
		Database: database,
		Queries:  queries,
	}, nil
}

func (a *App) Close() error {
	return a.Database.Close()
}
