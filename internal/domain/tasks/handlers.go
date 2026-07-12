package tasks

import (
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

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.service.List(r.Context())
	if err != nil {
		http.Error(w, "list tasks", http.StatusInternalServerError)
		return
	}

	h.renderer.HTML(w, http.StatusOK, "layouts/base", render.ViewData{
		Title:   "Tasks",
		Content: "pages/tasks/index-content",
		Data: map[string]any{
			"Tasks": tasks,
		},
	})
}

func (h *Handlers) ListFragment(w http.ResponseWriter, r *http.Request) {
	h.renderList(w, r)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}

	_, err := h.service.Create(r.Context(), r.FormValue("title"))
	if errors.Is(err, ErrBlankTitle) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderer.HTML(w, http.StatusUnprocessableEntity, "partials/tasks/flash", render.ViewData{Flash: "Task title cannot be blank."})
		return
	}
	if err != nil {
		http.Error(w, "create task", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		h.renderList(w, r)
		return
	}
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (h *Handlers) Complete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	task, err := h.service.Complete(r.Context(), id)
	if err != nil {
		http.Error(w, "complete task", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		h.renderer.HTML(w, http.StatusOK, "partials/tasks/row", render.ViewData{Data: map[string]any{"Task": task}})
		return
	}
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		http.Error(w, "delete task", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		h.renderList(w, r)
		return
	}
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (h *Handlers) renderList(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.service.List(r.Context())
	if err != nil {
		http.Error(w, "list tasks", http.StatusInternalServerError)
		return
	}
	h.renderer.HTML(w, http.StatusOK, "partials/tasks/list", render.ViewData{Data: map[string]any{"Tasks": tasks}})
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
