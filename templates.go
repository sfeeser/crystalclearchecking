package main

// CHUNK_START: imports-package-embed-templates-v1-uuid-m7n4p9q2
// BUSINESS_PURPOSE: Declares the package, imports required modules, and embeds all HTML templates from the templates/ directory using go:embed. This is the single source of truth for UI template dependencies and static file inclusion.
// SPEC_LINK: specbook-chapter-5 (UI & Operational Flows) + non-negotiables on minimal dependencies
import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.html"))
// CHUNK_END: imports-package-embed-templates-v1-uuid-m7n4p9q2

// CHUNK_START: render-template-helper-v1-uuid-r2s8t5v1
// BUSINESS_PURPOSE: Provides a centralized helper function to execute and render a named HTML template with provided data, handling errors gracefully with HTTP 500 response per specbook Chapter 5 UI rendering requirements
// SPEC_LINK: specbook-chapter-5
func RenderTemplate(w http.ResponseWriter, name string, data interface{}) {
	err := templates.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
// CHUNK_END: render-template-helper-v1-uuid-r2s8t5v1
