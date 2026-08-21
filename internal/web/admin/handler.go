package admin

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/auth"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	webassets "github.com/kokosx/stratum/internal/web"
)

type Handler struct {
	queries           *db.Queries
	auth              *auth.Service
	dashboardTemplate *template.Template
	entriesTemplate   *template.Template
	setupTemplate     *template.Template
	loginTemplate     *template.Template
}

type LayoutData struct {
	Title      string
	ActiveMenu string
	Content    any
}

func NewHandler(queries *db.Queries, authService *auth.Service) (*Handler, error) {
	templateFS, err := fs.Sub(webassets.Assets, "templates/admin")
	if err != nil {
		return nil, fmt.Errorf("admin templates: %w", err)
	}

	dashboardTemplate, err := template.ParseFS(templateFS, "layout.html", "dashboard.html")
	if err != nil {
		return nil, err
	}

	entriesTemplate, err := template.ParseFS(templateFS, "layout.html", "entries.html")
	if err != nil {
		return nil, err
	}
	setupTemplate, err := template.ParseFS(templateFS, "auth.html", "setup.html")
	if err != nil {
		return nil, err
	}
	loginTemplate, err := template.ParseFS(templateFS, "auth.html", "login.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		queries:           queries,
		auth:              authService,
		dashboardTemplate: dashboardTemplate,
		entriesTemplate:   entriesTemplate,
		setupTemplate:     setupTemplate,
		loginTemplate:     loginTemplate,
	}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", h.adminHome)
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /admin/setup", h.setup)
	mux.HandleFunc("POST /admin/setup", h.setup)
	mux.HandleFunc("GET /admin/login", h.login)
	mux.HandleFunc("POST /admin/login", h.login)
	mux.HandleFunc("POST /admin/logout", h.logout)
	mux.HandleFunc("GET /admin/pages", h.requireAuth(h.listPages))
	mux.HandleFunc("GET /admin/posts", h.requireAuth(h.listPosts))
	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(fmt.Sprintf("admin static files: %v", err))
	}
	mux.Handle("GET /admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.FS(staticFS))))

	return mux
}

type authPageData struct {
	Title string
	Error string
}

func (h *Handler) adminHome(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := h.auth.HasAdmin(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !hasAdmin {
		http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
		return
	}
	if !h.isAuthenticated(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	h.dashboard(w, r)
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := h.auth.HasAdmin(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if hasAdmin {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		token, err := h.auth.Setup(r.Context(), r.FormValue("setup_code"), r.FormValue("site_title"), r.FormValue("email"), r.FormValue("password"))
		if err == nil {
			h.setSessionCookie(w, token)
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		if errors.Is(err, auth.ErrSetupUnavailable) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		h.renderAuth(w, h.setupTemplate, "Install Stratum", err.Error())
		return
	}
	h.renderAuth(w, h.setupTemplate, "Install Stratum", "")
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := h.auth.HasAdmin(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !hasAdmin {
		http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		token, err := h.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
		if err == nil {
			h.setSessionCookie(w, token)
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		h.renderAuth(w, h.loginTemplate, "Sign in", "Invalid email or password.")
		return
	}
	h.renderAuth(w, h.loginTemplate, "Sign in", "")
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.CookieName); err == nil {
		_ = h.auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.auth.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.isAuthenticated(r) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (h *Handler) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	_, err = h.auth.UserForToken(r.Context(), cookie.Value)
	return err == nil
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.auth.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 60 * 60 * 24 * 30})
}

func (h *Handler) renderAuth(w http.ResponseWriter, page *template.Template, title, message string) {
	if err := page.ExecuteTemplate(w, "auth.html", authPageData{Title: title, Error: message}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
