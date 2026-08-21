package web

import "embed"

// Assets contains the templates and static files required by the web handlers.
//
//go:embed templates/admin/*.html static/admin.css static/admin/* static/editor/* static/appearance/* templates/public/layout.html
var Assets embed.FS
