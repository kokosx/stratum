package admin

import (
	"log"
	"net/http"
)

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	data := LayoutData{
		Title:      "Dashboard",
		ActiveMenu: "dashboard",
	}

	if err := h.dashboardTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render admin dashboard: %v", err)
	}
}
