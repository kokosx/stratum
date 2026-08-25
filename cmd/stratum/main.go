package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kokosx/stratum/internal/app"
	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/backup"
	"github.com/kokosx/stratum/internal/datalock"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	adminweb "github.com/kokosx/stratum/internal/web/admin"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func main() {
	ctx := context.Background()
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		if err := runBackup(ctx, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "search" {
		if len(os.Args) > 2 && os.Args[2] == "rebuild" {
			if err := runSearchRebuild(ctx); err != nil {
				log.Fatal(err)
			}
			return
		}
		fmt.Fprintln(os.Stderr, "usage: stratum search rebuild")
		os.Exit(2)
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" && command != "seed" {
		fmt.Fprintln(os.Stderr, "usage: stratum [serve|migrate|seed|backup|search]")
		fmt.Fprintln(os.Stderr, "  backup create [--output path]")
		fmt.Fprintln(os.Stderr, "  backup verify <archive>")
		fmt.Fprintln(os.Stderr, "  backup restore <archive>")
		os.Exit(2)
	}

	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	var dataLock *datalock.Lock
	var err error
	if command == "serve" {
		dataLock, err = datalock.Acquire(dataDir)
		if err != nil {
			log.Fatal("Stratum data directory is already in use by another process.")
		}
		defer dataLock.Close()
	}

	application, err := app.New(ctx)
	if err != nil {
		log.Fatal(err)
	}

	defer application.Close()

	switch command {
	case "migrate":
		log.Println("Database migrations are up to date")
	case "seed":
		if err := application.Database.Seed(ctx); err != nil {
			log.Fatal(err)
		}
		if err := application.Blocks.Reload(ctx); err != nil {
			log.Fatal(err)
		}
		log.Println("Development seed data is ready")
	case "serve":
		if err := serve(application); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}

func serve(application *app.App) error {
	authService, err := auth.NewService(application.Database.DB, application.Queries, os.Getenv("STRATUM_ENV") == "production")
	if err != nil {
		return fmt.Errorf("auth service: %w", err)
	}
	if setupCode := authService.SetupCode(); setupCode != "" {
		log.Printf("StratumCMS is not configured. Open /admin/setup with setup code: %s", setupCode)
	}
	hub, err := runtimehub.New(
		application.Queries,
		application.Blocks,
		application.Themes,
		application.Media,
	)
	if err != nil {
		return fmt.Errorf("runtime hub: %w", err)
	}

	adminHandler, err := adminweb.NewHandler(
		application.Database.DB,
		application.Queries,
		authService,
		application.Blocks,
		application.Themes,
		application.Media,
		hub,
	)
	if err != nil {
		return fmt.Errorf("admin handler: %w", err)
	}

	publicHandler, err := publicweb.NewHandlerWithHub(hub)
	if err != nil {
		return fmt.Errorf("public handler: %w", err)
	}
	adminHandler.SetPreviewRenderer(publicHandler.RenderPreview)
	adminHandler.SetDocumentPreviewRenderer(publicHandler.RenderEditableDocument)

	// Scheduled publishing runs as part of stratum serve.
	scheduler := publishing.NewSchedulerWithHub(application.Database.DB, application.Queries, hub)
	scheduler.SetSearchRefresh(search.New(application.Database.DB, application.Blocks).RefreshEntry)
	schedCtx, schedCancel := context.WithCancel(context.Background())
	go scheduler.Start(schedCtx)
	defer schedCancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := application.Database.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/admin", adminHandler.Routes())
	mux.Handle("/admin/", adminHandler.Routes())
	mux.Handle("/", publicHandler)

	addr := os.Getenv("STRATUM_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("Received shutdown signal, draining connections...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("Stratum running on http://%s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	log.Println("Stopped")
	return nil
}

func runBackup(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: stratum backup <create|verify|restore> [...]")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		return runBackupCreate(ctx, args[1:])
	case "verify":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: stratum backup verify <archive>")
			os.Exit(2)
		}
		return runBackupVerify(args[1])
	case "restore":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: stratum backup restore <archive>")
			os.Exit(2)
		}
		return runBackupRestore(ctx, args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown backup command %q\n", args[0])
		os.Exit(2)
	}
	return nil
}

func runBackupCreate(ctx context.Context, args []string) error {
	output := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--output" || args[i] == "-o" {
			if i+1 >= len(args) {
				return fmt.Errorf("missing value for %s", args[i])
			}
			output = args[i+1]
			i++
		} else if args[i] == "--help" || args[i] == "-h" {
			fmt.Println("usage: stratum backup create [--output path]")
			return nil
		} else {
			return fmt.Errorf("unknown flag %q", args[i])
		}
	}
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	// Open DB for snapshot
	dbPath := filepath.Join(dataDir, "stratum.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	queries := db.New(database.DB)
	result, err := backup.CreateResult(ctx, database, queries, dataDir, output)
	if err != nil {
		return err
	}
	fmt.Printf("Backup created: %s\nSchema version: %s\nDB size: %d bytes\nMedia: %d files, %d bytes\nArchive size: %d bytes\n", result.Path, result.SchemaVersion, result.DatabaseSize, result.MediaCount, result.MediaBytes, result.ArchiveSize)
	return nil
}

func runBackupVerify(archive string) error {
	if err := backup.Verify(archive); err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}
	fmt.Printf("Backup verified: %s\n", archive)
	return nil
}

func runBackupRestore(ctx context.Context, archive string) error {
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	if err := backup.Restore(ctx, archive, dataDir); err != nil {
		return err
	}
	fmt.Printf("Backup restored from %s\n", archive)
	return nil
}

func runSearchRebuild(ctx context.Context) error {
	application, err := app.New(ctx)
	if err != nil {
		return err
	}
	defer application.Close()
	count, err := search.New(application.Database.DB, application.Blocks).Rebuild(ctx)
	if err != nil {
		return fmt.Errorf("rebuild search index: %w", err)
	}
	fmt.Printf("Search index rebuilt: %d entries\n", count)
	return nil
}
