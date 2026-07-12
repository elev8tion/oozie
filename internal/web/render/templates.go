package render

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
)

type Renderer struct {
	templates *template.Template
}

func New(root string) (*Renderer, error) {
	renderer := &Renderer{}

	patterns := []string{
		filepath.Join(root, "layouts", "*.html"),
		filepath.Join(root, "pages", "*", "*.html"),
		filepath.Join(root, "partials", "*", "*.html"),
		filepath.Join(root, "components", "*.html"),
	}

	tmpl := template.New("app").Funcs(template.FuncMap{
		"dict":    dict,
		"include": renderer.include,
	})
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			continue
		}
		if _, err := tmpl.ParseFiles(matches...); err != nil {
			return nil, err
		}
	}

	renderer.templates = tmpl
	return renderer, nil
}

func (r *Renderer) HTML(w http.ResponseWriter, status int, name string, data ViewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := r.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
}

// include renders a named template to HTML so layouts can embed a
// per-request page body (html/template has no dynamic template calls).
func (r *Renderer) include(name string, data any) (template.HTML, error) {
	if name == "" {
		return "", fmt.Errorf("include: empty template name")
	}
	var buf bytes.Buffer
	if err := r.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func dict(values ...any) map[string]any {
	result := map[string]any{}
	for i := 0; i+1 < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			continue
		}
		result[key] = values[i+1]
	}
	return result
}
