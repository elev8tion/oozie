package app

import (
	"context"
	"database/sql"
	"io/fs"

	"oozie/internal/agent/pi"
	"oozie/internal/domain/projects"
	"oozie/internal/web/render"
)

type App struct {
	config   Config
	database *sql.DB
	renderer *render.Renderer
	static   fs.FS
	agent    *pi.Manager
	service  *projects.Service
}

func New(config Config, database *sql.DB, renderer *render.Renderer, static fs.FS) *App {
	repo := projects.NewRepo(database)
	service := projects.NewService(repo)
	catalog := pi.LoadCatalog()
	agent := pi.NewManager(catalog, service)
	service.SetAgent(agent, catalog)
	service.RecoverOrphanedJobs(context.Background())
	return &App{config: config, database: database, renderer: renderer, static: static, agent: agent, service: service}
}

// Shutdown stops all running pi agent subprocesses.
func (a *App) Shutdown() {
	a.agent.Shutdown()
}
