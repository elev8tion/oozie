package app

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"oozie"
	"oozie/internal/db"
	"oozie/internal/web/render"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "smoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	migrationsFS, _ := fs.Sub(oozie.Assets, "migrations")
	if err := db.RunMigrations(database, migrationsFS); err != nil {
		t.Fatal(err)
	}
	templatesFS, _ := fs.Sub(oozie.Assets, "templates")
	staticFS, _ := fs.Sub(oozie.Assets, "static")
	renderer, err := render.New(templatesFS, "test")
	if err != nil {
		t.Fatal(err)
	}
	return New(Config{}, database, renderer, staticFS).Routes()
}

func TestPagesRender(t *testing.T) {
	handler := newTestServer(t)
	cases := []struct {
		path string
		want int
	}{
		{"/projects", 200},
		{"/projects/new", 200},
		{"/store", 200},
		{"/installed-apps", 200},
		{"/publishing/jobs", 200},
		{"/settings", 200},
		{"/onboarding", 200},
		{"/static/js/htmx.min.js", 200},
		{"/static/css/app.css", 200},
		{"/projects/999", 404},
		{"/store/apps/999", 404},
		{"/projects/banana", 400},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("GET %s = %d, want %d", c.path, rec.Code, c.want)
		}
		if c.want != 200 && !strings.Contains(rec.Body.String(), "Back to Projects") {
			t.Errorf("GET %s error page is not styled", c.path)
		}
	}
}

func TestCreateProjectFlow(t *testing.T) {
	handler := newTestServer(t)
	form := strings.NewReader("name=Smoke+Test&project_path_display=~/Projects/smoke-test&trusted=on")
	req := httptest.NewRequest("POST", "/projects", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 303 {
		t.Fatalf("create project = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest("GET", rec.Header().Get("Location"), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Smoke Test") {
		t.Fatalf("project page = %d, contains name: %v", rec.Code, strings.Contains(rec.Body.String(), "Smoke Test"))
	}
}
