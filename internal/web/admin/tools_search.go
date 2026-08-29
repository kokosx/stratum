package admin

import (
	"fmt"
	"log"
	"net/http"
)

func (h *Handler) handleSearchRebuild(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	count, err := h.search.Rebuild(r.Context())
	if err != nil {
		log.Printf("search rebuild: %v", err)
		if isDatastarRequest(r) {
			writeSSE(w, toastEvent("error", fmt.Sprintf("Search rebuild failed: %v", err)))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if h.runtime != nil {
		h.runtime.InvalidateContent()
	}
	msg := fmt.Sprintf("Search index rebuilt: %d entries.", count)
	if isDatastarRequest(r) {
		writeSSE(w, toastEvent("success", msg))
		return
	}
	h.setFlash(w, msg)
	http.Redirect(w, r, "/admin/tools/site-health", http.StatusSeeOther)
}
