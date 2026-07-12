package main

import (
	"log"
	"net/http"

	"oozie/internal/app"
	"oozie/internal/db"
	"oozie/internal/web/render"
)

func main() {
	cfg := app.LoadConfig()

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database, cfg.MigrationsDir); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	renderer, err := render.New("templates")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	application := app.New(cfg, database, renderer)

	log.Printf("listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, application.Routes()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
