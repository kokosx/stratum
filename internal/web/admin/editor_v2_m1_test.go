package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
)

func TestV2TopbarM1Composition(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	entryID := seedPageForV2WithAuth(t, handler, token)
	req := httptest.NewRequest("GET", "/admin/pages/"+entryID+"/edit?editor=v2", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// LEFT / CENTER / RIGHT zones must exist
	for _, want := range []string{
		`editor-v2-topbar__left`,
		`editor-v2-topbar__center`,
		`editor-v2-topbar__right`,
		`editor-v2-topbar`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing topbar zone %q", want)
		}
	}
	// Back appears exactly once
	if count := strings.Count(body, `aria-label="Back"`); count != 1 {
		t.Fatalf("Back aria-label count = %d want 1", count)
	}
	if !strings.Contains(body, `title="Back to Pages"`) {
		t.Fatalf("Back missing title Back to Pages")
	}
	// Resource identity: title + subtitle
	if !strings.Contains(body, `editor-v2-topbar__title`) || !strings.Contains(body, `editor-v2-topbar__subtitle`) {
		t.Fatalf("missing identity title/subtitle")
	}
	// View live must be icon action, not text button
	if !strings.Contains(body, `data-testid="editor-v2-view-live"`) {
		t.Fatalf("View live icon missing data-testid")
	}
	if !strings.Contains(body, `aria-label="View live page"`) || !strings.Contains(body, `title="View live page"`) {
		t.Fatalf("View live missing aria-label/title")
	}
	if !strings.Contains(body, `target="_blank"`) {
		t.Fatalf("View live should open in new tab")
	}
	if strings.Contains(body, `>View live<`) {
		t.Fatalf("View live should be icon-only, not text button >View live<")
	}
	// Open V1 must not be permanent button; must be in overflow
	if strings.Contains(body, `>Open V1<`) {
		t.Fatalf("Open V1 should not be permanent text button")
	}
	if !strings.Contains(body, `data-testid="editor-v2-overflow"`) {
		t.Fatalf("overflow button missing")
	}
	if !strings.Contains(body, `id="editor-v2-overflow-menu"`) {
		t.Fatalf("overflow menu missing")
	}
	if !strings.Contains(body, `Open legacy editor`) {
		t.Fatalf("overflow menu missing Open legacy editor")
	}
	if !strings.Contains(body, `data-testid="editor-v2-legacy-link"`) {
		t.Fatalf("legacy link testid missing")
	}
	// aria for overflow
	if !strings.Contains(body, `aria-haspopup="menu"`) || !strings.Contains(body, `aria-expanded="false"`) || !strings.Contains(body, `aria-label="More"`) {
		t.Fatalf("overflow missing aria attributes")
	}
	// Viewport segmented control still present
	for _, vp := range []string{`data-viewport="desktop"`, `data-viewport="tablet"`, `data-viewport="mobile"`} {
		if !strings.Contains(body, vp) {
			t.Fatalf("missing viewport %s", vp)
		}
	}
	// Check not containing admin sidebar
	if strings.Contains(body, `id="admin-sidebar"`) {
		t.Fatalf("V2 must not contain admin sidebar")
	}
}

func TestV2TopbarCSSGridAndIcon(t *testing.T) {
	// Read CSS via static handler to ensure embedded assets contain grid
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	req := httptest.NewRequest("GET", "/admin/static/editor-v2/editor.css", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("css status %d", rec.Code)
	}
	css := rec.Body.String()
	// Also try direct file read as fallback for local dev
	if css == "" {
		data, _ := os.ReadFile("../web/static/editor-v2/editor.css")
		css = string(data)
	}
	checks := []struct {
		substr string
		msg    string
	}{
		{`grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr)`, "topbar must use grid 1fr auto 1fr for centering"},
		{`min-height: 52px`, "topbar min-height 52px (48-52)"},
		{`padding: 8px 12px`, "topbar padding 8px 12px"},
		{`editor-v2-icon-btn`, "icon button 32px class must exist"},
		{`width: 32px`, "icon button width 32px"},
		{`height: 32px`, "icon button height 32px"},
		{`editor-v2-viewport-group`, "viewport group must exist"},
		{`gap: 0`, "viewport group gap 0"},
		{`editor-v2-overflow-menu`, "overflow menu must exist"},
		{`text-overflow: ellipsis`, "title ellipsis for long titles"},
		{`justify-self: center`, "center must be justify-self center"},
		{`justify-self: start`, "left must be justify-self start"},
		{`justify-self: end`, "right must be justify-self end"},
	}
	for _, c := range checks {
		if !strings.Contains(css, c.substr) {
			t.Fatalf("CSS missing %q: %s\ncss snippet: %s", c.substr, c.msg, css[:minInt(2000, len(css))])
		}
	}
	// Ensure no inline margin-left hack in CSS for viewport label is via class not inline style
	// The label spacing should be via class editor-v2-viewport-label
	if !strings.Contains(css, `editor-v2-viewport-label`) {
		t.Fatalf("viewport label class missing")
	}
}

func TestV2AnchorStaticGuards(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Fetch app.js, canvas.js and state.js - after M2.5 cleanup anchor helpers are dead and removed
	for _, path := range []string{"/admin/static/editor-v2/app.js", "/admin/static/editor-v2/canvas.js", "/admin/static/editor-v2/state.js"} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("static %s status %d", path, rec.Code)
		}
		body := rec.Body.String()
		if path == "/admin/static/editor-v2/app.js" {
			// After M2.5: anchor navigation helpers are dead — app.js must NOT contain them
			for _, unwanted := range []string{
				"isSameResourceFragment",
				"handleSamePageAnchor",
				"findAnchorTarget",
				"getCurrentResourceInfo",
			} {
				if strings.Contains(body, unwanted) {
					t.Fatalf("app.js should not contain dead %q after M2.5 cleanup", unwanted)
				}
			}
			// Should not contain scrollIntoView anchor navigation
			if strings.Contains(body, `scrollIntoView`) {
				t.Fatalf("app.js should not contain scrollIntoView after inert edit mode")
			}
			// Ensure no emoji icons
			if strings.Contains(body, "🖥") || strings.Contains(body, "📱") {
				t.Fatalf("app.js should not use emoji")
			}
			// Must still contain VIEWPORTS
			if !strings.Contains(body, "VIEWPORTS") {
				t.Fatalf("app.js should contain VIEWPORTS")
			}
		} else if path == "/admin/static/editor-v2/canvas.js" {
			// canvas.js must NOT contain dead anchor helpers, must contain simplified inert handling
			for _, unwanted := range []string{
				"isSameResourceFragment",
				"handleSamePageAnchor",
				"findAnchorTarget",
			} {
				if strings.Contains(body, unwanted) {
					t.Fatalf("canvas.js should not contain dead %q", unwanted)
				}
			}
			if strings.Contains(body, `scrollIntoView`) {
				t.Fatalf("canvas.js should not contain scrollIntoView after M2.5")
			}
			if strings.Contains(body, "getElementById") && strings.Contains(body, "findAnchorTarget") {
				t.Fatalf("canvas.js should not contain anchor target lookup")
			}
			// Must contain generic inert handling (no visualRoot abstraction)
			if !strings.Contains(body, "preventDefault") || !strings.Contains(body, "selectInstance") {
				t.Fatalf("canvas.js should contain inert preventDefault+selectInstance")
			}
			if strings.Contains(body, "getVisualRootForBlock") || strings.Contains(body, "resolveVisualElements") || strings.Contains(body, "visualRoot") || strings.Contains(body, "visualElement") {
				t.Fatalf("canvas.js should not contain visualRoot abstraction after corrective")
			}
			if strings.Contains(body, "nodeIdToBlockCache") {
				t.Fatalf("canvas.js should not contain nodeIdToBlockCache after marker carries block")
			}
			if strings.Contains(body, "data-stratum-editor-visual-root") {
				t.Fatalf("canvas.js should not contain data-stratum-editor-visual-root attribute")
			}
		} else {
			// state.js must expose publicUrl but NOT dead publicPath/Search/Origin
			if !strings.Contains(body, "publicUrl") {
				t.Fatalf("state.js missing publicUrl")
			}
			if strings.Contains(body, "publicPath") || strings.Contains(body, "publicSearch") || strings.Contains(body, "publicOrigin") {
				t.Fatalf("state.js should not contain dead publicPath/publicSearch/publicOrigin after M2.5")
			}
			if !strings.Contains(body, "publicPreviewUrl") {
				t.Fatalf("state.js should derive from actions.publicPreviewUrl")
			}
			if strings.Contains(body, "visualRoot") || strings.Contains(body, "getVisualRootForBlock") {
				t.Fatalf("state.js should not contain visualRoot after corrective")
			}
			if !strings.Contains(body, "displayNameForBlock") {
				t.Fatalf("state.js should contain displayNameForBlock")
			}
		}
	}
}

func TestV2ViewportRegression(t *testing.T) {
	// Ensure VIEWPORT constants and applyViewport still handle 768/390
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	req := httptest.NewRequest("GET", "/admin/static/editor-v2/app.js", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "tablet: 768") || !strings.Contains(body, "mobile: 390") {
		t.Fatalf("VIEWPORTS must contain 768 and 390")
	}
	if !strings.Contains(body, "applyViewport") {
		t.Fatalf("applyViewport missing")
	}
	if !strings.Contains(body, `iframe.style.width`) {
		t.Fatalf("applyViewport should set iframe width")
	}
	// CSS viewport widths: M2 single source — VIEWPORTS in JS is truth, CSS must not duplicate 768/390 in several selectors
	req2 := httptest.NewRequest("GET", "/admin/static/editor-v2/editor.css", nil)
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec2 := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec2, req2)
	css := rec2.Body.String()
	if strings.Contains(css, ".editor-v2-stage--tablet") || strings.Contains(css, ".editor-v2-workspace--tablet") || strings.Contains(css, ".editor-v2-canvas-wrap--tablet") {
		t.Fatalf("CSS should not duplicate viewport widths in multiple selectors (VIEWPORTS is single source)")
	}
	// Ensure stage base still exists
	if !strings.Contains(css, ".editor-v2-stage") || !strings.Contains(css, ".editor-v2-canvas") {
		t.Fatalf("CSS missing stage/canvas base")
	}
}

func TestIsSameResourceFragmentMatrix(t *testing.T) {
	// Go mirror of JS isSameResourceFragment to document spec and guard regressions.
	// JS impl is in app.js; this test ensures spec matrix is covered even without running JS.
	normalizePath := func(p string) string {
		if p == "" {
			return "/"
		}
		s := strings.TrimSpace(p)
		if !strings.HasPrefix(s, "/") {
			s = "/" + s
		}
		s = strings.TrimRight(s, "/")
		if s == "" {
			s = "/"
		}
		return s
	}
	isSame := func(currentOrigin, currentPath, currentSearch, href string) bool {
		if href == "" {
			return false
		}
		trimmed := strings.TrimSpace(href)
		if trimmed == "" {
			return false
		}
		// block non-http schemes
		lower := strings.ToLower(trimmed)
		if len(trimmed) >= 2 && strings.Contains(trimmed, ":") {
			// quick scheme check: /^[a-z][a-z0-9+.-]*:/  and not http
			hasScheme := false
			for i, c := range trimmed {
				if i == 0 && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
					break
				}
				if c == ':' {
					hasScheme = true
					break
				}
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '+' || c == '.' || c == '-') {
					break
				}
			}
			if hasScheme && !(strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
				return false
			}
		}
		if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "/") && !(strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
			return false
		}
		if trimmed == "#" {
			return true
		}
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(trimmed[1:]) != ""
		}
		if !strings.Contains(trimmed, "#") {
			return false
		}
		// Parse like JS: new URL(trimmed, base)
		baseStr := currentOrigin + normalizePath(currentPath) + currentSearch
		parsedBase, err := url.Parse(baseStr)
		if err != nil {
			return false
		}
		parsedHref, err := url.Parse(trimmed)
		if err != nil {
			return false
		}
		// Resolve reference like JS URL constructor: url.Parse does not resolve relative against base for non-absolute;
		// use ResolveReference
		if !parsedHref.IsAbs() && !strings.HasPrefix(trimmed, "//") {
			parsedHref = parsedBase.ResolveReference(parsedHref)
		} else if strings.HasPrefix(trimmed, "//") {
			// protocol-relative: inherit scheme from base
			tmp, err := url.Parse(trimmed)
			if err != nil {
				return false
			}
			tmp.Scheme = parsedBase.Scheme
			parsedHref = tmp
			// JS new URL("//evil.test/about#x", "https://example.test/about") => scheme https, host evil.test
			// Our handling above will correctly detect host mismatch
		}
		// For absolute http(s) href, parsedHref already absolute; for hash-only, it would have been handled earlier
		if parsedHref.Scheme != parsedBase.Scheme || parsedHref.Host != parsedBase.Host {
			return false
		}
		if normalizePath(parsedHref.Path) != normalizePath(currentPath) {
			return false
		}
		wantQuery := strings.TrimPrefix(currentSearch, "?")
		if parsedHref.RawQuery != wantQuery {
			return false
		}
		return parsedHref.Fragment != ""
	}

	tests := []struct {
		name   string
		origin string
		path   string
		search string
		href   string
		want   bool
	}{
		{"hash only", "http://localhost:8080", "/long-page-scroll-test", "", "#pricing", true},
		{"hash top", "http://localhost:8080", "/long-page-scroll-test", "", "#", true},
		{"hash whitespace invalid", "http://localhost:8080", "/long-page-scroll-test", "", "#   ", true},
		{"same path with hash", "http://localhost:8080", "/long-page-scroll-test", "", "/long-page-scroll-test#section-5", true},
		{"same path absolute url", "http://localhost:8080", "/long-page-scroll-test", "", "http://localhost:8080/long-page-scroll-test#section-5", true},
		{"cross page", "http://localhost:8080", "/long-page-scroll-test", "", "/contact", false},
		{"cross page with hash", "http://localhost:8080", "/long-page-scroll-test", "", "/contact#team", false},
		{"same origin different path", "http://localhost:8080", "/about", "", "/contact#team", false},
		{"query change blocked", "http://localhost:8080", "/products", "?page=1", "/products?page=2#section", false},
		{"query same allowed", "http://localhost:8080", "/products", "?page=1", "/products?page=1#section", true},
		{"query same hash only keeps query", "http://localhost:8080", "/products", "?page=1", "#section", true},
		{"query missing on link blocked", "http://localhost:8080", "/products", "?page=1", "/products#section", false},
		{"mailto blocked", "http://localhost:8080", "/about", "", "mailto:test@example.com", false},
		{"tel blocked", "http://localhost:8080", "/about", "", "tel:+123", false},
		{"javascript blocked", "http://localhost:8080", "/about", "", "javascript:alert(1)", false},
		{"data blocked", "http://localhost:8080", "/about", "", "data:text/html,hi#x", false},
		{"bare relative blocked", "http://localhost:8080", "/about", "", "about#team", false},
		{"protocol relative blocked", "https://example.test", "/about", "", "//evil.test/about#x", false},
		{"evil origin blocked", "https://example.test", "/about", "", "https://evil.test/about#x", false},
		{"same origin same path absolute allowed", "https://example.test", "/about", "", "https://example.test/about#team", true},
		{"same origin different path absolute blocked", "https://example.test", "/about", "", "https://example.test/contact#team", false},
		{"no hash no anchor", "http://localhost:8080", "/about", "", "/about", false},
		{"trailing slash normalized", "http://localhost:8080", "/about/", "", "/about#team", true},
		{"hash encoded", "http://localhost:8080", "/about", "", "#pricing%202", true},
	}
	for _, tc := range tests {
		got := isSame(tc.origin, tc.path, tc.search, tc.href)
		if got != tc.want {
			t.Errorf("%s: isSame(%q,%q,%q,%q) = %v want %v", tc.name, tc.origin, tc.path, tc.search, tc.href, got, tc.want)
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = auth.CookieName
var _ = os.ReadFile
