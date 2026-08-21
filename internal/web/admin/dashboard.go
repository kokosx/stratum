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
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data.CSRFToken = token

	if err := h.dashboardTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render admin dashboard: %v", err)
	}
}
