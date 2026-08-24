package admin

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/content"
)

func TestCustomFieldControlsAndPostedValuesAreGeneric(t *testing.T) {
	definition := content.ContentTypeDefinition{ID: "case-study", Fields: []content.FieldDefinition{
		{Key: "client", Label: "Client", Type: content.FieldText, Required: true},
		{Key: "featured", Label: "Featured", Type: content.FieldBoolean},
		{Key: "format", Label: "Format", Type: content.FieldSelect, Validation: content.FieldValidation{Options: []string{"web", "print"}}},
	}}
	form := url.Values{"field_client": {"Acme"}, "field_featured_present": {"true"}, "field_format": {"print"}}
	req := httptest.NewRequest("POST", "/admin/case-studies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	values := rawFieldValues(req, definition)
	controls := customFieldControls(definition, values)
	if len(controls) != 3 || controls[0].Value != "Acme" || controls[1].Checked || controls[2].Selected != "print" {
		t.Fatalf("submitted values were not preserved: %#v", controls)
	}
}
