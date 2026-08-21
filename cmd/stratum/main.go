package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/kokosx/stratum/internal/app"
	"github.com/kokosx/stratum/internal/auth"
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
		log.Println("Development seed data is ready at http://localhost:8080/")
	case "serve":
		serve(application)
	}
}

func serve(application *app.App) {
	authService, err := auth.NewService(application.Database.DB, application.Queries, os.Getenv("STRATUM_ENV") == "production")
	if err != nil {
		log.Fatal(err)
	}
	if setupCode := authService.SetupCode(); setupCode != "" {
		log.Printf("StratumCMS is not configured. Open http://localhost:8080/admin/setup with setup code: %s", setupCode)
	}
	adminHandler, err := adminweb.NewHandler(
		application.Database.DB,
		application.Queries,
		authService,
	)
	if err != nil {
		log.Fatal(err)
	}

	publicHandler, err := publicweb.NewHandler(
		application.Queries,
		application.Blocks,
	)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/admin", adminHandler.Routes())
	mux.Handle("/admin/", adminHandler.Routes())
	mux.Handle("/", publicHandler)

	server := &http.Server{Addr: ":8080", Handler: mux}
	log.Println("Stratum running on http://localhost:8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
