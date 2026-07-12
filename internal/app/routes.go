package app

import (
	"net/http"

	"oozie/internal/agent/pi"
	"oozie/internal/domain/projects"
)

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	repo := projects.NewRepo(a.database)
	service := projects.NewService(repo)
	catalog := pi.LoadCatalog()
	service.SetAgent(pi.NewManager(catalog, service), catalog)
	h := projects.NewHandlers(service, a.renderer)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /", h.Home)
	mux.HandleFunc("GET /onboarding", h.Onboarding)

	mux.HandleFunc("GET /projects", h.Projects)
	mux.HandleFunc("GET /projects/new", h.NewProject)
	mux.HandleFunc("POST /projects", h.CreateProject)
	mux.HandleFunc("GET /projects/{id}", h.ShowProject)
	mux.HandleFunc("POST /projects/{id}/archive", h.ArchiveProject)
	mux.HandleFunc("GET /fragments/projects/list", h.ProjectsList)

	mux.HandleFunc("GET /projects/{id}/agent", h.Agent)
	mux.HandleFunc("GET /projects/{id}/agent/timeline", h.AgentTimeline)
	mux.HandleFunc("POST /projects/{id}/agent/model", h.SelectAgentModel)
	mux.HandleFunc("POST /projects/{id}/agent/requests", h.AgentRequest)
	mux.HandleFunc("POST /projects/{id}/agent/requests/{requestID}/cancel", h.CancelAgentRequest)
	mux.HandleFunc("POST /projects/{id}/agent/questions/{toolUseID}/answer", h.AnswerQuestion)
	mux.HandleFunc("POST /projects/{id}/agent/questions/{toolUseID}/dismiss", h.DismissQuestion)
	mux.HandleFunc("POST /projects/{id}/agent/plan-approvals/{requestID}", h.PlanApproval)
	mux.HandleFunc("POST /projects/{id}/agent/permissions/{requestID}", h.Permission)
	mux.HandleFunc("POST /projects/{id}/feedback", h.Feedback)

	mux.HandleFunc("GET /store", h.Store)
	mux.HandleFunc("GET /store/apps/{id}", h.StoreApp)
	mux.HandleFunc("POST /store/apps/{id}/install", h.InstallApp)
	mux.HandleFunc("GET /installed-apps", h.InstalledApps)
	mux.HandleFunc("GET /fragments/store/results", h.StoreResults)

	mux.HandleFunc("GET /publishing/jobs", h.PublishingJobs)
	mux.HandleFunc("GET /fragments/publishing/jobs", h.PublishingJobsList)
	mux.HandleFunc("GET /projects/{id}/publish", h.PublishPage)
	mux.HandleFunc("POST /projects/{id}/publish/draft", h.SaveDraft)
	mux.HandleFunc("POST /projects/{id}/publish", h.Publish)

	mux.HandleFunc("GET /settings", h.Settings)
	mux.HandleFunc("GET /settings/appearance", h.SettingsAppearance)
	mux.HandleFunc("GET /settings/agent", h.SettingsAgent)
	mux.HandleFunc("POST /settings", h.SaveSettings)

	return mux
}
