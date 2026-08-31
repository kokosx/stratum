package main

import (
	"context"
	"flag"
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
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/datalock"
	wordpress "github.com/kokosx/stratum/internal/importer/wordpress"
	"github.com/kokosx/stratum/internal/mcpserver"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/version"
	adminweb "github.com/kokosx/stratum/internal/web/admin"
	"github.com/kokosx/stratum/internal/web/canonicalredirect"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func printHelp() {
	fmt.Fprintln(os.Stdout, "usage: stratum [serve|migrate|seed|backup|search|import|doctor|version]")
	fmt.Fprintln(os.Stdout, "  serve                    start server (default)")
	fmt.Fprintln(os.Stdout, "  migrate                  run migrations")
	fmt.Fprintln(os.Stdout, "  seed                     seed development data")
	fmt.Fprintln(os.Stdout, "  backup create [--output path]")
	fmt.Fprintln(os.Stdout, "  backup verify <archive>")
	fmt.Fprintln(os.Stdout, "  backup restore <archive>")
	fmt.Fprintln(os.Stdout, "  doctor [--production] [--json]")
	fmt.Fprintln(os.Stdout, "  search rebuild")
	fmt.Fprintln(os.Stdout, "  import wordpress <file.xml> [--dry-run]")
	fmt.Fprintln(os.Stdout, "  version                  print version")
}

func main() {
	ctx := context.Background()
	// Version/help must exit before DB open or lock.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-V":
			fmt.Fprintln(os.Stdout, version.Version)
			os.Exit(0)
		case "--help", "-h", "help":
			printHelp()
			os.Exit(0)
		}
		// also handle global --version/--help without subcommand position
		for _, a := range os.Args[1:] {
			if a == "--version" || a == "-V" {
				fmt.Fprintln(os.Stdout, version.Version)
				os.Exit(0)
			}
			if a == "--help" || a == "-h" {
				printHelp()
				os.Exit(0)
			}
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		if err := runDoctor(ctx, os.Args[2:]); err != nil {
			// runDoctor already handles exit codes; this is fallback
			log.Fatal(err)
		}
		return
	}
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
	if len(os.Args) > 1 && os.Args[1] == "import" {
		if err := runImport(ctx, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" && command != "seed" {
		fmt.Fprintln(os.Stderr, "usage: stratum [serve|migrate|seed|backup|search|import|doctor]")
		fmt.Fprintln(os.Stderr, "  backup create [--output path]")
		fmt.Fprintln(os.Stderr, "  backup verify <archive>")
		fmt.Fprintln(os.Stderr, "  backup restore <archive>")
		fmt.Fprintln(os.Stderr, "  doctor [--production] [--json]")
		os.Exit(2)
	}

	var serveCfg ServeConfig
	if command == "serve" {
		var parseErr error
		var serveArgs []string
		if len(os.Args) > 2 && os.Args[1] == "serve" {
			serveArgs = os.Args[2:]
		} else if len(os.Args) > 1 && os.Args[1] == "serve" {
			serveArgs = []string{}
		} else {
			serveArgs = []string{}
		}
		serveCfg, parseErr = parseServeOptions(serveArgs)
		if parseErr != nil {
			fmt.Fprintln(os.Stderr, parseErr)
			os.Exit(2)
		}
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
		if err := serve(application, serveCfg); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}

func runImport(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "wordpress" {
		return fmt.Errorf("usage: stratum import wordpress <file.xml> [--dry-run] [--download-media=true|false] [--author=email] [--on-conflict=skip]")
	}
	// Help handling
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Fprintln(os.Stderr, "usage: stratum import wordpress <file.xml> [--dry-run] [--download-media=true|false] [--author=email] [--on-conflict=skip]")
			return nil
		}
	}
	fs := flag.NewFlagSet("wordpress", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "validate without writing")
	downloadMedia := fs.Bool("download-media", true, "download WordPress attachments")
	author := fs.String("author", "", "fallback Stratum author email")
	onConflict := fs.String("on-conflict", "skip", "conflict strategy")
	if len(args) < 2 {
		return fmt.Errorf("usage: stratum import wordpress <file.xml> [flags]")
	}
	filename := args[1]
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: stratum import wordpress <file.xml> [flags]")
	}
	dataDir := os.Getenv("STRATUM_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	if *dryRun {
		database, err := storage.OpenReadOnly(filepath.Join(dataDir, "stratum.db"))
		if err != nil {
			return fmt.Errorf("open database for dry run: %w", err)
		}
		defer database.Close()
		queries := db.New(database.DB)
		// Dry-run must validate with the SAME block registry/schema validation
		// as a real import. The registry only READS block_definitions, so it is
		// safe against the read-only handle; media provider stays nil (image
		// blocks validate structurally without resolving assets).
		registry, err := blocks.NewRegistry(ctx, queries)
		if err != nil {
			return fmt.Errorf("load block registry for dry run: %w", err)
		}
		im := wordpress.New(database.DB, queries, registry, nil, dataDir)
		report, _, err := im.Import(ctx, filename, wordpress.Options{DryRun: true, DownloadMedia: *downloadMedia, Author: *author, OnConflict: *onConflict, DataDir: dataDir})
		if err != nil {
			return err
		}
		fmt.Println(report.String())
		return nil
	}
	// Acquire data lock explicitly at process boundary (spec §19).
	lock, err := datalock.Acquire(dataDir)
	if err != nil {
		return fmt.Errorf("cannot import while Stratum is serving this data directory: %w", err)
	}
	defer lock.Close()
	application, err := app.New(ctx)
	if err != nil {
		return err
	}
	defer application.Close()
	im := wordpress.New(application.Database.DB, application.Queries, application.Blocks, application.Media, dataDir)
	report, backupPath, err := im.Execute(ctx, filename, wordpress.Options{DryRun: *dryRun, DownloadMedia: *downloadMedia, Author: *author, OnConflict: *onConflict, DataDir: dataDir})
	if backupPath != "" {
		fmt.Printf("Pre-import backup created: %s\n", backupPath)
	}
	if err != nil {
		return err
	}
	fmt.Println(report.String())
	return nil
}

func serve(application *app.App, serveCfg ServeConfig) error {
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

	siteURL := ""
	if snap := hub.Site.Current(); snap != nil {
		siteURL = snap.SiteURL
	}
	canonicalCfg, err := canonicalredirect.NewConfig(serveCfg.RedirectScheme, serveCfg.RedirectWWW, serveCfg.TrustProxy, siteURL)
	if err != nil {
		return err
	}

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
	// MCP endpoint (Stratum system namespace, single binary)
	mcpSrv := mcpserver.New(application.Database.DB, application.Queries, adminHandler.Agents(), adminHandler.EntryOps(), application.Blocks)
	mux.Handle("/stratum/mcp", mcpSrv.Handler())
	mux.Handle("/stratum/mcp/", mcpSrv.Handler())
	mux.Handle("/admin", adminHandler.Routes())
	mux.Handle("/admin/", adminHandler.Routes())
	mux.Handle("/", publicHandler)

	// Apply canonical redirect middleware (health bypass inside middleware)
	handler := canonicalredirect.Middleware(mux, canonicalCfg)

	addr := serveCfg.Addr

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
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

	logStartup(addr, canonicalCfg)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	log.Println("Stopped")
	return nil
}

func logStartup(addr string, cfg canonicalredirect.Config) {
	log.Printf("Stratum listening on %s", addr)
	if cfg.Scheme == canonicalredirect.SchemeOff && cfg.WWW == canonicalredirect.WWWOff {
		log.Printf("Canonical redirects: disabled")
		return
	}
	schemeStr := "off"
	switch cfg.Scheme {
	case canonicalredirect.SchemeHTTPS:
		schemeStr = "https"
	case canonicalredirect.SchemeHTTP:
		schemeStr = "http"
	}
	log.Printf("Canonical scheme: %s", schemeStr)
	if cfg.WWW != canonicalredirect.WWWOff {
		wwwDesc := ""
		switch cfg.WWW {
		case canonicalredirect.WWWForbidden:
			wwwDesc = "non-www"
		case canonicalredirect.WWWRequired:
			wwwDesc = "www"
		}
		if cfg.CanonicalHost != "" {
			log.Printf("Canonical host: %s (%s)", wwwDesc, cfg.CanonicalHost)
		} else {
			log.Printf("Canonical host: %s", wwwDesc)
		}
	} else {
		log.Printf("Canonical host: off")
	}
	trust := "no"
	if cfg.TrustProxy {
		trust = "yes"
	}
	log.Printf("Trusted proxy headers: %s", trust)
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
