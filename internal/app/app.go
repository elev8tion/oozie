package app

import (
	"database/sql"

	"oozie/internal/web/render"
)

type App struct {
	config   Config
	database *sql.DB
	renderer *render.Renderer
}

func New(config Config, database *sql.DB, renderer *render.Renderer) *App {
	return &App{config: config, database: database, renderer: renderer}
}
