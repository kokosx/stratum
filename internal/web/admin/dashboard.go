package admin

import (
	"log"
	"net/http"
)

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	incomplete := false
	if completed, err := h.queries.GetOnboardingCompleted(r.Context()); err == nil {
		incomplete = completed == 0
	}
	state := ResolveNav(r.URL.Path)
	data := LayoutData{
		Title:         "Dashboard",
		ActiveMenu:    state.ActiveSection,
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
		Content:       struct{ OnboardingIncomplete bool }{incomplete},
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
