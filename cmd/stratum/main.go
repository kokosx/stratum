package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kokosx/stratum/internal/app"
	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/runtimehub"
	adminweb "github.com/kokosx/stratum/internal/web/admin"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func main() {
	ctx := context.Background()
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" && command != "seed" {
		fmt.Fprintln(os.Stderr, "usage: stratum [serve|migrate|seed]")
		os.Exit(2)
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
