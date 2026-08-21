package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/navigation"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
	webassets "github.com/kokosx/stratum/internal/web"
)

type Handler struct {
	database           *sql.DB
	queries            *db.Queries
	auth               *auth.Service
	blocks             *blocks.Registry
	media              *media.Service
	dashboardTemplate  *template.Template
	entriesTemplate    *template.Template
	entryTemplate      *template.Template
	setupTemplate      *template.Template
	loginTemplate      *template.Template
	menusTemplate      *template.Template
	mediaTemplate      *template.Template
	appearanceTemplate *template.Template
	settingsTemplate   *template.Template
	navigation         *navigation.Service
	navigationLoader   *navigation.Loader
	themes             *themes.Runtime
	previewRenderer    func(context.Context, string, string, map[string]any, string) ([]byte, error)
}

type LayoutData struct {
	Title      string
	ActiveMenu string
	Flash      string
	CSRFToken  string
	Content    any
}

func NewHandler(database *sql.DB, queries *db.Queries, authService *auth.Service, blockRegistry *blocks.Registry, themeRuntime *themes.Runtime, mediaService *media.Service) (*Handler, error) {
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
	entryTemplate, err := template.ParseFS(templateFS, "layout.html", "entry_form.html")
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
	menusTemplate, err := template.ParseFS(templateFS, "layout.html", "menus.html")
	if err != nil {
		return nil, err
	}
	appearanceTemplate, err := template.ParseFS(templateFS, "layout.html", "appearance.html")
	if err != nil {
		return nil, err
	}
	settingsTemplate, err := template.ParseFS(templateFS, "layout.html", "settings.html")
	if err != nil {
		return nil, err
	}
	mediaTemplate, err := template.ParseFS(templateFS, "layout.html", "media.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		database:           database,
		queries:            queries,
		auth:               authService,
		blocks:             blockRegistry,
		media:              mediaService,
		dashboardTemplate:  dashboardTemplate,
		entriesTemplate:    entriesTemplate,
		entryTemplate:      entryTemplate,
		setupTemplate:      setupTemplate,
		loginTemplate:      loginTemplate,
		menusTemplate:      menusTemplate,
		mediaTemplate:      mediaTemplate,
		appearanceTemplate: appearanceTemplate,
		settingsTemplate:   settingsTemplate,
		navigation:         navigation.NewService(database, queries),
		navigationLoader:   navigation.NewLoader(queries),
		themes:             themeRuntime,
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
	mux.HandleFunc("GET /admin/pages/new", h.requireAuth(h.newPage))
	mux.HandleFunc("POST /admin/pages", h.requireAuth(h.createPage))
	mux.HandleFunc("GET /admin/pages/{id}/edit", h.requireAuth(h.editPage))
	mux.HandleFunc("POST /admin/pages/{id}", h.requireAuth(h.savePage))
	mux.HandleFunc("POST /admin/pages/{id}/publish", h.requireAuth(h.publishPage))
	mux.HandleFunc("POST /admin/editor/preview", h.requireAuth(h.previewDocument))
	mux.HandleFunc("GET /admin/posts", h.requireAuth(h.listPosts))
	mux.HandleFunc("GET /admin/posts/new", h.requireAuth(h.newPost))
	mux.HandleFunc("POST /admin/posts", h.requireAuth(h.createPost))
	mux.HandleFunc("GET /admin/posts/{id}/edit", h.requireAuth(h.editPost))
	mux.HandleFunc("POST /admin/posts/{id}", h.requireAuth(h.savePost))
	mux.HandleFunc("POST /admin/posts/{id}/publish", h.requireAuth(h.publishPost))
	mux.HandleFunc("GET /admin/menus", h.requireAuth(h.listMenus))
	mux.HandleFunc("POST /admin/menus", h.requireAuth(h.createMenu))
	mux.HandleFunc("POST /admin/menus/{id}", h.requireAuth(h.updateMenu))
	mux.HandleFunc("POST /admin/menus/{id}/delete", h.requireAuth(h.deleteMenu))
	mux.HandleFunc("GET /admin/appearance", h.requireAuth(h.appearance))
	mux.HandleFunc("POST /admin/appearance", h.requireAuth(h.saveAppearance))
	mux.HandleFunc("POST /admin/appearance/preview", h.requireAuth(h.previewAppearance))
	mux.HandleFunc("GET /admin/settings", h.requireAuth(h.settings))
	mux.HandleFunc("POST /admin/settings", h.requireAuth(h.saveSettings))
	mux.HandleFunc("POST /admin/settings/robots-preview", h.requireAuth(h.robotsPreview))
	mux.HandleFunc("GET /admin/media", h.requireAuth(h.mediaLibrary))
	mux.HandleFunc("GET /admin/media.json", h.requireAuth(h.mediaListJSON))
	mux.HandleFunc("POST /admin/media/upload", h.requireAuth(h.uploadMedia))
	mux.HandleFunc("GET /admin/media/{id}/json", h.requireAuth(h.mediaDetailJSON))
	mux.HandleFunc("POST /admin/media/{id}", h.requireAuth(h.updateMedia))
	mux.HandleFunc("POST /admin/media/{id}/delete", h.requireAuth(h.deleteMedia))
	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		panic(fmt.Sprintf("admin static files: %v", err))
	}
	mux.Handle("GET /admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.FS(staticFS))))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, maxAdminRequestBody)
		}
		mux.ServeHTTP(w, r)
	})
}

const csrfCookieName = "stratum_csrf"
const maxAdminRequestBody = 1 << 20
const maxUploadBytes = 12 << 20

func (h *Handler) csrfToken(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: token, Path: "/admin", HttpOnly: true, Secure: h.auth.SecureCookies(), SameSite: http.SameSiteStrictMode, MaxAge: 60 * 60 * 8})
	return token, nil
}

func (h *Handler) validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		token = r.FormValue("csrf_token")
	}
	if err != nil || cookie.Value == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) == 1
}

func (h *Handler) SetPreviewRenderer(renderer func(context.Context, string, string, map[string]any, string) ([]byte, error)) {
	h.previewRenderer = renderer
}

const flashCookieName = "stratum_flash"

func (h *Handler) setFlash(w http.ResponseWriter, message string) {
	value := base64.RawURLEncoding.EncodeToString([]byte(message))
	http.SetCookie(w, &http.Cookie{Name: flashCookieName, Value: value, Path: "/admin", HttpOnly: true, Secure: h.auth.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 60})
}

func (h *Handler) consumeFlash(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookieName, Value: "", Path: "/admin", HttpOnly: true, Secure: h.auth.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	message, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	return string(message)
}

type authPageData struct {
	Title     string
	Error     string
	CSRFToken string
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
		if !h.validCSRF(r) {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}
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
		h.renderAuth(w, r, h.setupTemplate, "Install Stratum", err.Error())
		return
	}
	h.renderAuth(w, r, h.setupTemplate, "Install Stratum", "")
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
		if !h.validCSRF(r) {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}
		token, err := h.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
		if err == nil {
			h.setSessionCookie(w, token)
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		h.renderAuth(w, r, h.loginTemplate, "Sign in", "Invalid email or password.")
		return
	}
	h.renderAuth(w, r, h.loginTemplate, "Sign in", "")
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
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

func (h *Handler) renderAuth(w http.ResponseWriter, r *http.Request, page *template.Template, title, message string) {
	token, err := h.csrfToken(w, r)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := page.ExecuteTemplate(w, "auth.html", authPageData{Title: title, Error: message, CSRFToken: token}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
