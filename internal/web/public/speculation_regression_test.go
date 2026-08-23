package public

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSpeculationRegressionLiteralJSON(t *testing.T) {
	handler, queries := setupSite(t)
	// Enable speculation
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SpeculationMode = "prefetch"
		p.SpeculationEagerness = "conservative"
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	body := rec.Body.String()
	// Extract speculation script
	start := strings.Index(body, `<script type="speculationrules">`)
	if start == -1 {
		t.Fatalf("speculation script not found, body=%s", body)
	}
	end := strings.Index(body[start:], `</script>`)
	if end == -1 {
		t.Fatalf("speculation script end not found")
	}
	inner := body[start+len(`<script type="speculationrules">`) : start+end]
	if strings.Contains(inner, "&#34;") {
		t.Fatalf("speculation JSON is escaped (contains &#34;): %s", inner)
	}
	if !strings.Contains(inner, `"prefetch"`) {
		t.Fatalf("speculation JSON missing prefetch: %s", inner)
	}
	if !strings.Contains(inner, `"conservative"`) {
		t.Fatalf("speculation JSON missing eagerness: %s", inner)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(inner), &parsed); err != nil {
		t.Fatalf("speculation JSON invalid: %v, inner=%s", err, inner)
	}
	// Disabled must emit no script
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SpeculationMode = "off"
	})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/about", nil))
	body2 := rec2.Body.String()
	if strings.Contains(body2, "speculationrules") {
		t.Fatalf("disabled speculation should not emit script, got %s", body2)
	}
}
