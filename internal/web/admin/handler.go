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
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/search"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
	webassets "github.com/kokosx/stratum/internal/web"
)

type Handler struct {
	database                     *sql.DB
	queries                      *db.Queries
	auth                         *auth.Service
	blocks                       *blocks.Registry
	media                        *media.Service
	dashboardTemplate            *template.Template
	entriesTemplate              *template.Template
	entryTemplate                *template.Template
	setupTemplate                *template.Template
	loginTemplate                *template.Template
	menusTemplate                *template.Template
	mediaTemplate                *template.Template
	appearanceTemplate           *template.Template
	settingsTemplate             *template.Template
	layoutTemplatesTemplate      *template.Template
	layoutTemplateFormTemplate   *template.Template
	layoutTemplateEditorTemplate *template.Template
	taxonomyTemplate             *template.Template
	usersTemplate                *template.Template
	navigation                   *navigation.Service
	navigationLoader             *navigation.Loader
	themes                       *themes.Runtime
	runtime                      *runtimehub.Runtime
	layoutsService               *layouts.Service
	previewRenderer              func(context.Context, string, string, map[string]any, string) ([]byte, error)
	documentPreview              func(context.Context, RenderInput) ([]byte, error)
	publishing                   *publishing.Service
	scheduler                    *publishing.Scheduler
}

type LayoutData struct {
	Title         string
	ActiveMenu    string
	ActiveSection string
	ActiveItem    string
	Nav           []AdminNavItem
	Flash         string
	CSRFToken     string
	Content       any
}

func (h *Handler) navStateFor(r *http.Request) NavState { return ResolveNav(r.URL.Path) }

func (h *Handler) layoutData(r *http.Request, title string) LayoutData {
	state := ResolveNav(r.URL.Path)
	legacy := state.ActiveSection
	return LayoutData{
		Title:         title,
		ActiveMenu:    legacy,
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
	}
}

func (h *Handler) layoutDataWithFlash(w http.ResponseWriter, r *http.Request, title string) LayoutData {
	state := ResolveNav(r.URL.Path)
	legacy := state.ActiveSection
	return LayoutData{
		Title:         title,
		ActiveMenu:    legacy,
		ActiveSection: state.ActiveSection,
		ActiveItem:    state.ActiveItem,
		Nav:           h.navForUser(r),
		Flash:         h.consumeFlash(w, r),
	}
}

func NewHandler(database *sql.DB, queries *db.Queries, authService *auth.Service, blockRegistry *blocks.Registry, themeRuntime *themes.Runtime, mediaService *media.Service, runtimes ...*runtimehub.Runtime) (*Handler, error) {
	var runtime *runtimehub.Runtime
	if len(runtimes) > 0 {
		runtime = runtimes[0]
	}
	if runtime == nil {
		var err error
		runtime, err = runtimehub.New(queries, blockRegistry, themeRuntime, mediaService)
		if err != nil {
			return nil, err
		}
	}
	templateFS, err := fs.Sub(webassets.Assets, "templates/admin")
	if err != nil {
		return nil, fmt.Errorf("admin templates: %w", err)
	}
	adminFuncs := template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"multiply": func(a, b int) int { return a * b },
	}

	dashboardTemplate, err := template.New("dashboard").Funcs(adminFuncs).ParseFS(templateFS, "layout.html", "dashboard.html")
	if err != nil {
		return nil, err
	}

	entriesTemplate, err := template.New("entries").Funcs(adminFuncs).ParseFS(templateFS, "layout.html", "entries.html")
	if err != nil {
		return nil, err
	}
	entryTemplate, err := template.New("entry_form").Funcs(adminFuncs).ParseFS(templateFS, "layout.html", "entry_form.html")
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

	layoutTemplatesTemplate, err := template.ParseFS(templateFS, "layout.html", "layout_templates.html")
	if err != nil {
		return nil, err
	}

	layoutTemplateFormTemplate, err := template.ParseFS(templateFS, "layout.html", "layout_template_form.html")
	if err != nil {
		return nil, err
	}

	layoutTemplateEditorTemplate, err := template.ParseFS(templateFS, "layout.html", "layout_template_editor.html")
	if err != nil {
		return nil, err
	}

	taxonomyTemplate, err := template.New("taxonomy").Funcs(adminFuncs).ParseFS(templateFS, "layout.html", "taxonomy.html")
	if err != nil {
		return nil, err
	}
	usersTemplate, err := template.New("users").Funcs(adminFuncs).ParseFS(templateFS, "layout.html", "users.html")
	if err != nil {
		return nil, err
	}

	publisher := publishing.New(database, queries)
	searchService := search.New(database, blockRegistry)
	publisher.SetSearchRefresh(searchService.RefreshEntry)
	scheduler := publishing.NewScheduler(database, queries)
	scheduler.SetSearchRefresh(searchService.RefreshEntry)
	return &Handler{
		database:                     database,
		queries:                      queries,
		auth:                         authService,
		blocks:                       blockRegistry,
		media:                        mediaService,
		dashboardTemplate:            dashboardTemplate,
		entriesTemplate:              entriesTemplate,
		entryTemplate:                entryTemplate,
		setupTemplate:                setupTemplate,
		loginTemplate:                loginTemplate,
		menusTemplate:                menusTemplate,
		mediaTemplate:                mediaTemplate,
		appearanceTemplate:           appearanceTemplate,
		settingsTemplate:             settingsTemplate,
		taxonomyTemplate:             taxonomyTemplate,
		usersTemplate:                usersTemplate,
		layoutTemplatesTemplate:      layoutTemplatesTemplate,
		layoutTemplateFormTemplate:   layoutTemplateFormTemplate,
		layoutTemplateEditorTemplate: layoutTemplateEditorTemplate,
		navigation:                   navigation.NewService(database, queries),
		navigationLoader:             navigation.NewLoader(queries),
		themes:                       themeRuntime,
		runtime:                      runtime,
		layoutsService:               layouts.NewService(database, queries, blockRegistry),
		publishing:                   publisher,
		scheduler:                    scheduler,
	}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", h.adminHome)
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/" {
			http.Redirect(w, r, "/admin", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /admin/setup", h.setup)
	mux.HandleFunc("POST /admin/setup", h.setup)
	mux.HandleFunc("GET /admin/login", h.login)
	mux.HandleFunc("POST /admin/login", h.login)
	mux.HandleFunc("POST /admin/logout", h.logout)
	mux.HandleFunc("GET /admin/users", h.requireAuth(h.listUsers))
	mux.HandleFunc("POST /admin/users", h.requireAuth(h.createUser))
	mux.HandleFunc("POST /admin/users/{id}", h.requireAuth(h.updateUser))
	mux.HandleFunc("POST /admin/users/{id}/password", h.requireAuth(h.resetUserPassword))
	mux.HandleFunc("GET /admin/pages", h.requireAuth(h.listPages))
	mux.HandleFunc("GET /admin/pages/new", h.requireAuth(h.newPage))
	mux.HandleFunc("POST /admin/pages", h.requireAuth(h.createPage))
	mux.HandleFunc("GET /admin/pages/{id}/edit", h.requireAuth(h.editPage))
	mux.HandleFunc("POST /admin/pages/{id}", h.requireAuth(h.savePage))
	mux.HandleFunc("POST /admin/pages/{id}/publish", h.requireAuth(h.publishPage))
	mux.HandleFunc("POST /admin/pages/{id}/unpublish", h.requireAuth(h.unpublishPage))
	mux.HandleFunc("POST /admin/pages/{id}/schedule", h.requireAuth(h.schedulePage))
	mux.HandleFunc("POST /admin/pages/{id}/cancel-schedule", h.requireAuth(h.cancelSchedulePage))
	mux.HandleFunc("POST /admin/pages/{id}/submit-review", h.requireAuth(h.submitReviewPage))
	mux.HandleFunc("POST /admin/pages/{id}/revisions/{revisionID}/restore", h.requireAuth(h.restorePageRevision))
	mux.HandleFunc("GET /admin/pages/{id}/revisions/{revisionID}/preview", h.requireAuth(h.previewPageRevision))
	mux.HandleFunc("POST /admin/pages/{id}/trash", h.requireAuth(h.trashPage))
	mux.HandleFunc("POST /admin/pages/{id}/restore", h.requireAuth(h.restorePage))
	mux.HandleFunc("POST /admin/pages/{id}/delete", h.requireAuth(h.deletePagePermanently))
	mux.HandleFunc("POST /admin/pages/bulk", h.requireAuth(h.bulkPages))
	mux.HandleFunc("POST /admin/editor/preview", h.requireAuth(h.previewDocument))
	mux.HandleFunc("GET /admin/posts", h.requireAuth(h.listPosts))
	mux.HandleFunc("GET /admin/posts/new", h.requireAuth(h.newPost))
	mux.HandleFunc("POST /admin/posts", h.requireAuth(h.createPost))
	mux.HandleFunc("GET /admin/posts/{id}/edit", h.requireAuth(h.editPost))
	mux.HandleFunc("POST /admin/posts/{id}", h.requireAuth(h.savePost))
	mux.HandleFunc("POST /admin/posts/{id}/publish", h.requireAuth(h.publishPost))
	mux.HandleFunc("POST /admin/posts/{id}/unpublish", h.requireAuth(h.unpublishPost))
	mux.HandleFunc("POST /admin/posts/{id}/schedule", h.requireAuth(h.schedulePost))
	mux.HandleFunc("POST /admin/posts/{id}/cancel-schedule", h.requireAuth(h.cancelSchedulePost))
	mux.HandleFunc("POST /admin/posts/{id}/submit-review", h.requireAuth(h.submitReviewPost))
	mux.HandleFunc("POST /admin/posts/{id}/revisions/{revisionID}/restore", h.requireAuth(h.restorePostRevision))
	mux.HandleFunc("GET /admin/posts/{id}/revisions/{revisionID}/preview", h.requireAuth(h.previewPostRevision))
	mux.HandleFunc("POST /admin/posts/{id}/trash", h.requireAuth(h.trashPost))
	mux.HandleFunc("POST /admin/posts/{id}/restore", h.requireAuth(h.restorePost))
	mux.HandleFunc("POST /admin/posts/{id}/delete", h.requireAuth(h.deletePostPermanently))
	mux.HandleFunc("POST /admin/posts/bulk", h.requireAuth(h.bulkPosts))
	mux.HandleFunc("GET /admin/posts/categories", h.requireAuth(h.listCategories))
	mux.HandleFunc("POST /admin/posts/categories", h.requireAuth(h.createCategory))
	mux.HandleFunc("POST /admin/posts/categories/{id}/update", h.requireAuth(h.updateCategory))
	mux.HandleFunc("POST /admin/posts/categories/{id}/delete", h.requireAuth(h.deleteCategory))
	mux.HandleFunc("GET /admin/posts/tags", h.requireAuth(h.listTags))
	mux.HandleFunc("POST /admin/posts/tags", h.requireAuth(h.createTag))
	mux.HandleFunc("POST /admin/posts/tags/{id}/update", h.requireAuth(h.updateTag))
	mux.HandleFunc("POST /admin/posts/tags/{id}/delete", h.requireAuth(h.deleteTag))
	mux.HandleFunc("GET /admin/menus", h.requireAuth(h.listMenus))
	mux.HandleFunc("POST /admin/menus", h.requireAuth(h.createMenu))
	mux.HandleFunc("POST /admin/menus/{id}", h.requireAuth(h.updateMenu))
	mux.HandleFunc("POST /admin/menus/{id}/delete", h.requireAuth(h.deleteMenu))
	mux.HandleFunc("GET /admin/appearance", h.requireAuth(h.appearance))
	mux.HandleFunc("POST /admin/appearance", h.requireAuth(h.saveAppearance))
	mux.HandleFunc("POST /admin/appearance/preview", h.requireAuth(h.previewAppearance))
	mux.HandleFunc("GET /admin/appearance/templates", h.requireAuth(h.listLayoutTemplates))
	mux.HandleFunc("GET /admin/appearance/templates/new", h.requireAuth(h.newLayoutTemplate))
	mux.HandleFunc("POST /admin/appearance/templates", h.requireAuth(h.createLayoutTemplate))
	mux.HandleFunc("GET /admin/appearance/templates/{id}/edit", h.requireAuth(h.editLayoutTemplate))
	mux.HandleFunc("POST /admin/appearance/templates/{id}", h.requireAuth(h.saveLayoutTemplate))
	mux.HandleFunc("POST /admin/appearance/templates/{id}/publish", h.requireAuth(h.publishLayoutTemplate))
	mux.HandleFunc("POST /admin/appearance/templates/{id}/preview", h.requireAuth(h.previewLayoutTemplate))
	mux.HandleFunc("POST /admin/appearance/templates/{id}/default", h.requireAuth(h.setDefaultLayoutTemplate))
	mux.HandleFunc("GET /admin/settings", h.requireAuth(h.settingsRedirect))
	mux.HandleFunc("POST /admin/settings", h.requireAuth(h.saveSettings))
	mux.HandleFunc("GET /admin/settings/general", h.requireAuth(h.settingsGeneral))
	mux.HandleFunc("POST /admin/settings/general", h.requireAuth(h.saveSettingsGeneral))
	mux.HandleFunc("GET /admin/settings/reading", h.requireAuth(h.settingsReading))
	mux.HandleFunc("POST /admin/settings/reading", h.requireAuth(h.saveSettingsReading))
	mux.HandleFunc("GET /admin/settings/seo", h.requireAuth(h.settingsSEO))
	mux.HandleFunc("POST /admin/settings/seo", h.requireAuth(h.saveSettingsSEO))
	mux.HandleFunc("GET /admin/settings/performance", h.requireAuth(h.settingsPerformance))
	mux.HandleFunc("POST /admin/settings/performance", h.requireAuth(h.saveSettingsPerformance))
	mux.HandleFunc("POST /admin/settings/robots-preview", h.requireAuth(h.robotsPreview))
	mux.HandleFunc("POST /admin/settings/seo/robots-preview", h.requireAuth(h.robotsPreview))
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
		// The admin UI must never be indexed. robots.txt is not a security or
		// indexing guarantee, so every admin response also sends the header.
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Cache-Control", "no-store")
		// The media upload endpoint carries large multipart bodies (~12 MB),
		// so it must not inherit the small global admin POST body limit.
		if r.Method == http.MethodPost && r.URL.Path != "/admin/media/upload" {
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

// SetDocumentPreviewRenderer wires the shared public rendering pipeline into
// the block editor preview so it matches the live frontend exactly.
func (h *Handler) SetDocumentPreviewRenderer(renderer func(context.Context, RenderInput) ([]byte, error)) {
	h.documentPreview = renderer
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
			// Seed starter content on a genuinely fresh installation only.
			// Seed is transactional, idempotent during setup, and never overwrites user content.
			func() {
				d := &storage.Database{DB: h.database}
				if err := d.Seed(r.Context()); err != nil {
					log.Printf("seed starter content: %v", err)
				} else if h.runtime != nil {
					// New routes and settings need a runtime reload.
					_ = h.runtime.ReloadSite(r.Context())
					_ = h.runtime.ReloadNavigation(r.Context())
					_ = h.blocks.Reload(r.Context())
				}
			}()
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
		user, err := h.currentUser(r)
		if err != nil {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		if !h.authorized(r, user) {
			http.Error(w, "Forbidden", http.StatusForbidden)
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

func (h *Handler) authorized(r *http.Request, user auth.User) bool {
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/admin/users"):
		return authz.Allows(user.Role, authz.ManageUsers)
	case strings.HasPrefix(path, "/admin/menus"):
		return authz.Allows(user.Role, authz.ManageNavigation)
	case strings.HasPrefix(path, "/admin/settings"), strings.HasPrefix(path, "/admin/appearance"):
		return authz.Allows(user.Role, authz.ManageSite)
	case strings.HasPrefix(path, "/admin/posts/categories"), strings.HasPrefix(path, "/admin/posts/tags"):
		return authz.Allows(user.Role, authz.ManageTaxonomies)
	case strings.HasPrefix(path, "/admin/media"):
		return authz.Allows(user.Role, authz.ManageMedia)
	case strings.HasPrefix(path, "/admin/pages"):
		return h.authorizeEntryRequest(r, user, pageContentType)
	case strings.HasPrefix(path, "/admin/posts"):
		return h.authorizeEntryRequest(r, user, postContentType)
	case path == "/admin/editor/preview":
		return authz.Allows(user.Role, authz.CreateEntries)
	default:
		return true
	}
}

func (h *Handler) authorizeEntryRequest(r *http.Request, user auth.User, contentType string) bool {
	id := r.PathValue("id")
	action := authz.EntryRead
	if id == "" {
		if r.Method == http.MethodPost || strings.HasSuffix(r.URL.Path, "/new") {
			action = authz.EntryCreate
		}
		if action == authz.EntryRead {
			return contentType == postContentType && authz.Allows(user.Role, authz.ReadEntries) || authz.CanAccessEntry(user.Role, user.ID, "", contentType, action)
		}
		return authz.CanAccessEntry(user.Role, user.ID, "", contentType, action)
	}
	entry, err := h.queries.GetEntry(r.Context(), id)
	if err != nil || entry.ContentTypeID != contentType {
		return false
	}
	if strings.Contains(r.URL.Path, "/revisions/") {
		if strings.Contains(r.URL.Path, "/preview") && r.Method == http.MethodGet {
			action = authz.EntryRead
		} else if strings.Contains(r.URL.Path, "/restore") {
			action = authz.EntryEdit
		} else {
			action = authz.EntryDelete
		}
	} else if strings.Contains(r.URL.Path, "/publish") || strings.Contains(r.URL.Path, "/unpublish") || strings.Contains(r.URL.Path, "/schedule") {
		action = authz.EntryPublish
	} else if strings.Contains(r.URL.Path, "/submit-review") {
		action = authz.EntryEdit
	} else if strings.Contains(r.URL.Path, "/trash") || strings.Contains(r.URL.Path, "/restore") || strings.Contains(r.URL.Path, "/delete") {
		action = authz.EntryDelete
	} else if r.Method == http.MethodPost || strings.HasSuffix(r.URL.Path, "/edit") {
		action = authz.EntryEdit
	}
	return authz.CanAccessEntry(user.Role, user.ID, entry.AuthorID.String, contentType, action)
}

func (h *Handler) navForUser(r *http.Request) []AdminNavItem {
	user, err := h.currentUser(r)
	if err != nil {
		return nil
	}
	return FilterAdminNav(AdminNav(), user.Role)
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
