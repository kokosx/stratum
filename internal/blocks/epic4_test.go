package blocks

import (
	"context"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/rendering"
)

type fakeFormReader struct {
	view   forms.FormView
	active bool
}

func (f fakeFormReader) GetActiveForm(_ context.Context, id string) (forms.FormView, bool) {
	if f.active && id == f.view.ID {
		return f.view, true
	}
	return forms.FormView{}, false
}

func TestCoreFormRendersSemanticAccessibleFields(t *testing.T) {
	reg := newMigratedRegistry(t)
	doc := decodeDoc(t, `{"version":1,"nodes":[{"id":"instance-a","block":"core/form","version":1,"props":{},"settings":{"formId":"contact"}}]}`)
	view := forms.FormView{ID: "contact", SubmitLabel: "Send", SuccessMessage: "Thanks", Fields: []forms.Field{{ID: "name-id", Key: "name", Type: forms.FieldText, Label: "Name", Required: true}, {ID: "email-id", Key: "email", Type: forms.FieldEmail, Label: "Email", Required: true}, {ID: "select-id", Key: "topic", Type: forms.FieldSelect, Label: "Topic", Options: []string{"Sales & <Support>"}}, {ID: "check-id", Key: "agree", Type: forms.FieldCheckbox, Label: "Agree", Required: true}}}
	html := renderWithRegistry(t, reg, doc, rendering.RenderContext{Mode: rendering.ModePublic, Route: rendering.RouteContext{Path: "/contact"}, FormReader: fakeFormReader{view: view, active: true}, FormCache: map[string]forms.FormView{}})
	for _, want := range []string{`method="post"`, `action="/_stratum/forms/contact"`, `name="return_to" value="/contact"`, `for="form-instance-a-field-name-id"`, `id="form-instance-a-field-name-id"`, `type="email"`, `required`, `Sales &amp; &lt;Support&gt;`, `type="checkbox" value="1"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
}

func TestCoreFormMissingPublicEmptyPreviewWarningAndSuccess(t *testing.T) {
	reg := newMigratedRegistry(t)
	doc := decodeDoc(t, `{"version":1,"nodes":[{"id":"x","block":"core/form","version":1,"props":{},"settings":{"formId":"missing"}}]}`)
	public := renderWithRegistry(t, reg, doc, rendering.RenderContext{Mode: rendering.ModePublic, FormReader: fakeFormReader{}})
	if public != "" {
		t.Fatalf("public=%q", public)
	}
	preview := renderWithRegistry(t, reg, doc, rendering.RenderContext{Mode: rendering.ModePreview, IsPreview: true, FormReader: fakeFormReader{}})
	if !strings.Contains(preview, "Form unavailable") {
		t.Fatal(preview)
	}
	reader := fakeFormReader{active: true, view: forms.FormView{ID: "missing", SuccessMessage: "Done"}}
	success := renderWithRegistry(t, reg, doc, rendering.RenderContext{Mode: rendering.ModePublic, FormReader: reader, FormResult: rendering.FormResultContext{SuccessFormID: "missing"}})
	if !strings.Contains(success, "Done") || strings.Contains(success, "<form") {
		t.Fatal(success)
	}
}

func TestCoreFormSameFormTwiceHasUniqueDOMIDs(t *testing.T) {
	reg := newMigratedRegistry(t)
	doc := decodeDoc(t, `{"version":1,"nodes":[{"id":"one","block":"core/form","version":1,"props":{},"settings":{"formId":"f"}},{"id":"two","block":"core/form","version":1,"props":{},"settings":{"formId":"f"}}]}`)
	reader := fakeFormReader{active: true, view: forms.FormView{ID: "f", SubmitLabel: "Go", Fields: []forms.Field{{ID: "field", Key: "value", Type: forms.FieldText, Label: "Value"}}}}
	html := renderWithRegistry(t, reg, doc, rendering.RenderContext{FormReader: reader, FormCache: map[string]forms.FormView{}})
	if !strings.Contains(html, `id="form-one-field-field"`) || !strings.Contains(html, `id="form-two-field-field"`) {
		t.Fatal(html)
	}
}

func TestCoreSearchFormIsFixedSemanticGET(t *testing.T) {
	reg := newMigratedRegistry(t)
	doc := decodeDoc(t, `{"version":1,"nodes":[{"id":"search-one","block":"core/search-form","version":1,"props":{},"settings":{"placeholder":"Find","buttonLabel":"Go","showLabel":false}}]}`)
	html := renderWithRegistry(t, reg, doc, rendering.RenderContext{})
	for _, want := range []string{`method="get"`, `action="/search"`, `name="q"`, `type="search"`, `>Go</button>`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
}
