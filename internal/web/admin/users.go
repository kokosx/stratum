package admin

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type usersData struct {
	Users     []userData
	CSRFToken string
}

type userData struct {
	ID        string
	Email     string
	Role      string
	Status    string
	CreatedAt string
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.queries.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := usersData{CSRFToken: token, Users: make([]userData, 0, len(rows))}
	for _, row := range rows {
		data.Users = append(data.Users, userData{ID: row.ID, Email: row.Email, Role: row.Role, Status: row.Status, CreatedAt: time.Unix(row.CreatedAt, 0).Format("2 Jan 2006, 15:04")})
	}
	state := ResolveNav(r.URL.Path)
	if err := h.usersTemplate.ExecuteTemplate(w, "layout.html", LayoutData{Title: "Users", ActiveMenu: state.ActiveSection, ActiveSection: state.ActiveSection, ActiveItem: state.ActiveItem, Nav: h.navForUser(r), Flash: h.consumeFlash(w, r), CSRFToken: token, Content: data}); err != nil {
		log.Printf("render users: %v", err)
	}
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := h.auth.CreateUser(r.Context(), r.FormValue("email"), r.FormValue("password"), r.FormValue("role")); err != nil {
		h.setFlash(w, err.Error())
	} else {
		h.setFlash(w, "User created.")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	current, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	role := strings.TrimSpace(r.FormValue("role"))
	status := strings.TrimSpace(r.FormValue("status"))
	if current.ID == id && (role != current.Role || status != "active") {
		http.Error(w, "You cannot change your own role or disable your account", http.StatusBadRequest)
		return
	}
	if err := h.auth.UpdateUser(r.Context(), id, role, status); err != nil {
		h.setFlash(w, "Could not update user.")
	} else {
		h.setFlash(w, "User updated. Their sessions were revoked.")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := h.auth.ResetPassword(r.Context(), r.PathValue("id"), r.FormValue("password")); err != nil {
		h.setFlash(w, err.Error())
	} else {
		h.setFlash(w, "Password reset. Their sessions were revoked.")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}
