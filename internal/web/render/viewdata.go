package render

type ViewData struct {
	Title   string
	Content string // template name the layout renders as the page body
	Flash   string
	Data    map[string]any
}
