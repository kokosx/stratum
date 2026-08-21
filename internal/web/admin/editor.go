package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/kokosx/stratum/internal/document"
)

type editorBootstrap struct {
	Document    json.RawMessage `json:"document"`
	Catalog     any             `json:"catalog"`
	Definitions any             `json:"definitions"`
	PreviewURL  string          `json:"previewUrl"`
}

func (h *Handler) previewDocument(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	doc, err := document.Decode([]byte(postedDocument(r)))
	if err == nil {
		err = h.blocks.ValidateDocument(doc)
	}
	var rendered template.HTML
	if err == nil {
		rendered, err = h.blocks.RenderDocument(doc)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	// Render a fully themed, self-contained document. The editor preview is
	// shown inside an isolated iframe so the public theme globals (typography,
	// colors, :root variables) apply to the preview without restyling the
	// admin UI.
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/stratum/theme.css">
<link rel="stylesheet" href="/stratum/blocks.css">
</head>
<body>
<main class="site-main">
<div class="st-container st-container--content">%s</div>
</main>
</body>
</html>`, string(rendered))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(page))
}
