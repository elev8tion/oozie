package projects

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"oozie/internal/web/render"
)

type Handlers struct {
	service  *Service
	renderer *render.Renderer
}

func NewHandlers(service *Service, renderer *render.Renderer) *Handlers {
	return &Handlers{service: service, renderer: renderer}
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}
func (h *Handlers) Onboarding(w http.ResponseWriter, r *http.Request) {
	h.renderer.HTML(w, http.StatusOK, "layouts/base", render.ViewData{Title: "Welcome to oozie", Content: "pages/projects/onboarding-content"})
}
func (h *Handlers) Projects(w http.ResponseWriter, r *http.Request) {
	ps, err := h.service.ListProjects(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("filter"))
	if err != nil {
		http.Error(w, "projects", 500)
		return
	}
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Projects · oozie", Content: "pages/projects/index-content", Data: map[string]any{"Projects": ps, "Q": r.URL.Query().Get("q"), "Filter": r.URL.Query().Get("filter")}})
}
func (h *Handlers) ProjectsList(w http.ResponseWriter, r *http.Request) {
	ps, err := h.service.ListProjects(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("filter"))
	if err != nil {
		http.Error(w, "projects", 500)
		return
	}
	h.renderer.HTML(w, 200, "partials/projects/list", render.ViewData{Data: map[string]any{"Projects": ps}})
}
func (h *Handlers) NewProject(w http.ResponseWriter, r *http.Request) {
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "New Project · oozie", Content: "pages/projects/new-content"})
}
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	p, err := h.service.CreateProject(r.Context(), r.FormValue("name"), r.FormValue("project_path_display"), r.FormValue("trusted") == "on")
	if err != nil {
		h.renderer.HTML(w, 422, "partials/projects/flash", render.ViewData{Flash: err.Error()})
		return
	}
	if isHTMX(r) {
		ps, _ := h.service.ListProjects(r.Context(), "", "")
		h.renderer.HTML(w, 200, "partials/projects/list", render.ViewData{Flash: "Project created.", Data: map[string]any{"Projects": ps}})
		return
	}
	http.Redirect(w, r, "/projects/"+strconv.FormatInt(p.ID, 10), 303)
}
func (h *Handlers) ShowProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := h.service.GetProject(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: p.Name + " · oozie", Content: "pages/projects/show-content", Data: map[string]any{"Project": p}})
}
func (h *Handlers) ArchiveProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = h.service.ArchiveProject(r.Context(), id)
	if isHTMX(r) {
		ps, _ := h.service.ListProjects(r.Context(), "", "")
		h.renderer.HTML(w, 200, "partials/projects/list", render.ViewData{Flash: "Project archived.", Data: map[string]any{"Projects": ps}})
		return
	}
	http.Redirect(w, r, "/projects", 303)
}

func (h *Handlers) Agent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	page, err := h.service.AgentPage(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Agent · " + page.Project.Name, Content: "pages/agents/show-content", Data: map[string]any{"Agent": page}})
}
func (h *Handlers) AgentRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := h.service.SendAgentMessage(r.Context(), id, r.FormValue("mode"), r.FormValue("message"))
	page, _ := h.service.AgentPage(r.Context(), id)
	if err != nil {
		page.Error = err.Error()
	}
	h.renderer.HTML(w, 200, "partials/agents/live", render.ViewData{Data: map[string]any{"Agent": page}})
}
func (h *Handlers) AgentTimeline(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	page, err := h.service.AgentPage(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.HTML(w, 200, "partials/agents/live", render.ViewData{Data: map[string]any{"Agent": page}})
}
func (h *Handlers) SelectAgentModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := h.service.SelectModel(r.Context(), id, r.FormValue("model"))
	page, _ := h.service.AgentPage(r.Context(), id)
	flash := "Model set to " + page.Model + "."
	if err != nil {
		flash = err.Error()
	}
	h.renderer.HTML(w, 200, "partials/agents/form", render.ViewData{Flash: flash, Data: map[string]any{"Agent": page}})
}
func (h *Handlers) CancelAgentRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	rid, ok := pathID(w, r, "requestID")
	if !ok {
		return
	}
	_ = h.service.CancelRequest(r.Context(), id, rid)
	page, _ := h.service.AgentPage(r.Context(), id)
	h.renderer.HTML(w, 200, "partials/agents/live", render.ViewData{Flash: "Request cancelled.", Data: map[string]any{"Agent": page}})
}
func (h *Handlers) AnswerQuestion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	qid, ok := pathID(w, r, "toolUseID")
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := h.service.AnswerQuestion(r.Context(), qid, r.FormValue("answer"))
	page, _ := h.service.AgentPage(r.Context(), id)
	flash := "Answer sent to pi."
	if err != nil {
		flash = err.Error()
	}
	h.renderer.HTML(w, 200, "partials/agents/pending", render.ViewData{Flash: flash, Data: map[string]any{"Agent": page}})
}
func (h *Handlers) DismissQuestion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	qid, ok := pathID(w, r, "toolUseID")
	if !ok {
		return
	}
	_ = h.service.DismissQuestion(r.Context(), qid)
	page, _ := h.service.AgentPage(r.Context(), id)
	h.renderer.HTML(w, 200, "partials/agents/pending", render.ViewData{Flash: "Question dismissed.", Data: map[string]any{"Agent": page}})
}
func (h *Handlers) PlanApproval(w http.ResponseWriter, r *http.Request) {
	h.renderer.HTML(w, 200, "partials/agents/pending", render.ViewData{Flash: "Plan decision recorded."})
}
func (h *Handlers) Permission(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	rid, ok := pathID(w, r, "requestID")
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := h.service.ResolvePermission(r.Context(), rid, r.FormValue("decision") != "deny")
	page, _ := h.service.AgentPage(r.Context(), id)
	flash := "Permission decision sent to pi."
	if err != nil {
		flash = err.Error()
	}
	h.renderer.HTML(w, 200, "partials/agents/pending", render.ViewData{Flash: flash, Data: map[string]any{"Agent": page}})
}
func (h *Handlers) Feedback(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := h.service.SaveFeedback(r.Context(), id, r.FormValue("feedback_type"), r.FormValue("reason"), r.FormValue("additional_feedback"))
	if err != nil {
		h.renderer.HTML(w, 422, "partials/projects/flash", render.ViewData{Flash: err.Error()})
		return
	}
	h.renderer.HTML(w, 200, "partials/projects/flash", render.ViewData{Flash: "Feedback sent."})
}

func (h *Handlers) Store(w http.ResponseWriter, r *http.Request) {
	apps, err := h.service.ListStoreApps(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("filter"))
	if err != nil {
		http.Error(w, "store", 500)
		return
	}
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Store · oozie", Content: "pages/store/index-content", Data: map[string]any{"Apps": apps}})
}
func (h *Handlers) StoreResults(w http.ResponseWriter, r *http.Request) {
	apps, _ := h.service.ListStoreApps(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("filter"))
	h.renderer.HTML(w, 200, "partials/store/list", render.ViewData{Data: map[string]any{"Apps": apps}})
}
func (h *Handlers) StoreApp(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	app, err := h.service.GetStoreApp(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: app.Name + " · Store", Content: "pages/store/show-content", Data: map[string]any{"App": app}})
}
func (h *Handlers) InstallApp(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = h.service.InstallApp(r.Context(), id)
	app, _ := h.service.GetStoreApp(r.Context(), id)
	h.renderer.HTML(w, 200, "partials/store/row", render.ViewData{Flash: "App installed.", Data: map[string]any{"App": app}})
}
func (h *Handlers) InstalledApps(w http.ResponseWriter, r *http.Request) {
	apps, _ := h.service.InstalledApps(r.Context())
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Installed Apps · oozie", Content: "pages/store/installed-content", Data: map[string]any{"Apps": apps}})
}

func (h *Handlers) PublishingJobs(w http.ResponseWriter, r *http.Request) {
	jobs, _ := h.service.ListJobs(r.Context(), r.URL.Query().Get("status"))
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Publishing Jobs · oozie", Content: "pages/publishing/index-content", Data: map[string]any{"Jobs": jobs}})
}
func (h *Handlers) PublishingJobsList(w http.ResponseWriter, r *http.Request) {
	jobs, _ := h.service.ListJobs(r.Context(), r.URL.Query().Get("status"))
	h.renderer.HTML(w, 200, "partials/publishing/list", render.ViewData{Data: map[string]any{"Jobs": jobs}})
}
func (h *Handlers) PublishPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, _ := h.service.GetProject(r.Context(), id)
	d, _ := h.service.GetDraft(r.Context(), id)
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Publish · " + p.Name, Content: "pages/publishing/show-content", Data: map[string]any{"Project": p, "Draft": d}})
}
func (h *Handlers) SaveDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = r.ParseForm()
	d := PublishDraft{ProjectID: id, AppName: r.FormValue("app_name"), Headline: r.FormValue("headline"), Description: r.FormValue("description"), Changelog: r.FormValue("changelog"), PublishTarget: r.FormValue("publish_target"), Visibility: r.FormValue("visibility"), ScreenshotManifest: r.FormValue("screenshot_manifest")}
	err := h.service.SaveDraft(r.Context(), d)
	if err != nil {
		h.renderer.HTML(w, 422, "partials/publishing/form", render.ViewData{Flash: err.Error(), Data: map[string]any{"Draft": d}})
		return
	}
	h.renderer.HTML(w, 200, "partials/publishing/form", render.ViewData{Flash: "Draft saved.", Data: map[string]any{"Draft": d}})
}
func (h *Handlers) Publish(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = h.service.Publish(r.Context(), id)
	jobs, _ := h.service.ListJobs(r.Context(), "")
	h.renderer.HTML(w, 200, "partials/publishing/list", render.ViewData{Flash: "Publish succeeded.", Data: map[string]any{"Jobs": jobs}})
}

func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	s, _ := h.service.GetSettings(r.Context())
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Settings · oozie", Content: "pages/settings/index-content", Data: map[string]any{"Settings": s, "Section": "general"}})
}
func (h *Handlers) SettingsAppearance(w http.ResponseWriter, r *http.Request) {
	s, _ := h.service.GetSettings(r.Context())
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Appearance · oozie", Content: "pages/settings/index-content", Data: map[string]any{"Settings": s, "Section": "appearance"}})
}
func (h *Handlers) SettingsAgent(w http.ResponseWriter, r *http.Request) {
	s, _ := h.service.GetSettings(r.Context())
	h.renderer.HTML(w, 200, "layouts/base", render.ViewData{Title: "Agent · oozie", Content: "pages/settings/index-content", Data: map[string]any{"Settings": s, "Section": "agent"}})
}
func (h *Handlers) SaveSettings(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s := Settings{Appearance: r.FormValue("appearance"), StyleProfile: r.FormValue("style_profile"), AgentShortcut: r.FormValue("agent_shortcut"), SendMessageShortcut: r.FormValue("send_message_shortcut")}
	_ = h.service.SaveSettings(r.Context(), s)
	h.renderer.HTML(w, 200, "partials/settings/form", render.ViewData{Flash: "Settings saved.", Data: map[string]any{"Settings": s}})
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid id", 400)
		return 0, false
	}
	return id, true
}
func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }
func ignoreNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
