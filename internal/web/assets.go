package web

import "embed"

// Assets contains the templates and static files required by the web handlers.
//
//go:embed templates/admin/*.html static/admin.css templates/public/layout.html
var Assets embed.FS
