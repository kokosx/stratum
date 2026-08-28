package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	formdomain "github.com/kokosx/stratum/internal/forms"
	"github.com/kokosx/stratum/internal/layouts"
	"github.com/kokosx/stratum/internal/siteparts"
)

func setupEpic4Admin(t *testing.T, client *http.Client, serverURL string, setupCode string) {
	resp := postForm(t, client, serverURL, "/admin/setup", url.Values{
		"setup_code": {setupCode}, "site_title": {"Test Site"}, "email": {"admin@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, client, serverURL, "/admin/setup")},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup status=%d body=%s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func TestEpic4ContactFormVerticalSliceAndSnapshotCacheInvalidation(t *testing.T) {
	server, queries, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := newClient(t)
	setupEpic4Admin(t, client, server.URL, authService.SetupCode())
	create := postForm(t, client, server.URL, "/admin/forms", url.Values{"name": {"Contact Form"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/forms/new")}})
	if create.StatusCode != http.StatusSeeOther {
		t.Fatalf("create form=%d %s", create.StatusCode, bodyString(t, create))
	}
	create.Body.Close()
	rows, err := queries.ListActiveForms(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("forms=%d err=%v", len(rows), err)
	}
	formID := rows[0].ID
	documentJSON := `{"version":1,"nodes":[{"id":"contact-instance","block":"core/form","version":1,"props":{},"settings":{"formId":"` + formID + `"}}]}`
	page := postForm(t, client, server.URL, "/admin/pages", url.Values{"title": {"Contact"}, "slug": {"contact"}, "document_json": {documentJSON}, "publish": {"1"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/pages/new")}})
	if page.StatusCode != http.StatusSeeOther {
		t.Fatalf("create page=%d %s", page.StatusCode, bodyString(t, page))
	}
	page.Body.Close()
	public := getPath(t, client, server.URL, "/contact")
	if public.StatusCode != http.StatusOK {
		t.Fatalf("GET contact=%d", public.StatusCode)
	}
	body := bodyString(t, public)
	for _, want := range []string{"/_stratum/forms/" + formID, `name="name"`, `type="email"`, `name="message"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	deleteAttempt := postForm(t, client, server.URL, "/admin/forms/"+formID+"/delete", url.Values{"csrf_token": {csrfToken(t, client, server.URL, "/admin/forms/"+formID+"/edit")}})
	if deleteAttempt.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete referenced form=%d", deleteAttempt.StatusCode)
	}
	deleteAttempt.Body.Close()
	if _, err := queries.GetForm(context.Background(), formID); err != nil {
		t.Fatalf("referenced form was deleted: %v", err)
	}
	submit := postForm(t, client, server.URL, "/_stratum/forms/"+formID, url.Values{"return_to": {"/contact"}, "name": {"Jane"}, "email": {"jane@example.com"}, "message": {"Hello"}})
	if submit.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit=%d %s", submit.StatusCode, bodyString(t, submit))
	}
	location := submit.Header.Get("Location")
	submit.Body.Close()
	if !strings.Contains(location, "form_success=") || !strings.HasPrefix(location, "/contact?") {
		t.Fatalf("location=%q", location)
	}
	subs, err := queries.ListAllFormSubmissions(context.Background(), formID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("submissions=%d err=%v", len(subs), err)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(subs[0].ValuesJson), &values); err != nil || values["email"] != "jane@example.com" {
		t.Fatalf("values=%v err=%v", values, err)
	}
	success := getPath(t, client, server.URL, location)
	successBody := bodyString(t, success)
	if !strings.Contains(successBody, "Thanks! Your message has been sent.") || strings.Contains(successBody, `<form id="form-contact-instance"`) {
		t.Fatal(successBody)
	}
	if success.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("success cache=%q", success.Header.Get("Cache-Control"))
	}
	service := formdomain.NewService(database.DB, queries, nil, "")
	form, err := service.Get(context.Background(), formID)
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[2].Label = "Project details"
	editValues := url.Values{"name": {form.Name}, "submit_label": {form.SubmitLabel}, "success_message": {form.SuccessMessage}, "notification_email": {form.NotificationEmail}, "active": {"1"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/forms/"+formID+"/edit")}}
	for _, field := range form.Fields {
		editValues.Add("field_id", field.ID)
		editValues.Add("field_key", field.Key)
		editValues.Add("field_type", string(field.Type))
		editValues.Add("field_label", field.Label)
		editValues.Add("field_placeholder", field.Placeholder)
		editValues.Add("field_options", strings.Join(field.Options, "\n"))
		if field.Required {
			editValues.Set("required_"+field.ID, "1")
		}
	}
	edit := postForm(t, client, server.URL, "/admin/forms/"+formID, editValues)
	if edit.StatusCode != http.StatusSeeOther {
		payload, _ := io.ReadAll(edit.Body)
		t.Fatalf("edit=%d %s", edit.StatusCode, payload)
	}
	edit.Body.Close()
	updated := getPath(t, client, server.URL, "/contact")
	updatedBody := bodyString(t, updated)
	if !strings.Contains(updatedBody, "Project details") || strings.Contains(updatedBody, ">Message</label>") {
		t.Fatal(updatedBody)
	}
	stored, err := service.GetSubmission(context.Background(), subs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SchemaSnapshot.Fields[2].Label != "Message" {
		t.Fatalf("old snapshot=%q", stored.SchemaSnapshot.Fields[2].Label)
	}
}

func TestEpic4HoneypotLooksSuccessfulWithoutPersistence(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := newClient(t)
	setupEpic4Admin(t, client, server.URL, authService.SetupCode())
	create := postForm(t, client, server.URL, "/admin/forms", url.Values{"name": {"Contact"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/forms/new")}})
	create.Body.Close()
	forms, _ := queries.ListActiveForms(context.Background())
	id := forms[0].ID
	resp := postForm(t, client, server.URL, "/_stratum/forms/"+id, url.Values{"return_to": {"/"}, "website_confirm": {"bot"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	count, _ := queries.CountFormSubmissions(context.Background(), id)
	if count != 0 {
		t.Fatalf("persisted=%d", count)
	}
}

func TestEpic4FormRendersThroughSitePartAndSingleTemplate(t *testing.T) {
	server, queries, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := newClient(t)
	setupEpic4Admin(t, client, server.URL, authService.SetupCode())
	create := postForm(t, client, server.URL, "/admin/forms", url.Values{"name": {"Shared Form"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/forms/new")}})
	create.Body.Close()
	active, _ := queries.ListActiveForms(context.Background())
	formID := active[0].ID
	registry, err := blocks.NewRegistry(context.Background(), queries)
	if err != nil {
		t.Fatal(err)
	}
	partService := siteparts.NewService(database.DB, queries, registry)
	partID, err := partService.Create(context.Background(), "Newsletter CTA")
	if err != nil {
		t.Fatal(err)
	}
	partDoc := `{"version":1,"nodes":[{"id":"part-form","block":"core/form","version":1,"props":{},"settings":{"formId":"` + formID + `"}}]}`
	if err := partService.Publish(context.Background(), partID, "Newsletter CTA", partDoc, ""); err != nil {
		t.Fatal(err)
	}
	layoutService := layouts.NewService(database.DB, queries, registry)
	templateID, err := layoutService.Create(context.Background(), "Contact Page Template", "page")
	if err != nil {
		t.Fatal(err)
	}
	templateDoc := `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}},{"id":"template-form","block":"core/form","version":1,"props":{},"settings":{"formId":"` + formID + `"}}]}`
	if err := layoutService.Publish(context.Background(), templateID, "Contact Page Template", templateDoc, ""); err != nil {
		t.Fatal(err)
	}
	createPage := func(title, slug, documentJSON, layoutID string) {
		t.Helper()
		values := url.Values{"title": {title}, "slug": {slug}, "document_json": {documentJSON}, "publish": {"1"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/pages/new")}}
		if layoutID != "" {
			values.Set("layout_template_id", layoutID)
		}
		resp := postForm(t, client, server.URL, "/admin/pages", values)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("create %s=%d %s", slug, resp.StatusCode, bodyString(t, resp))
		}
		resp.Body.Close()
	}
	partReference := `{"version":1,"nodes":[{"id":"part-ref","block":"core/site-part","version":1,"props":{},"settings":{"sitePartId":"` + partID + `"}}]}`
	createPage("Site Part A", "site-part-a", partReference, "")
	createPage("Site Part B", "site-part-b", partReference, "")
	createPage("Template A", "template-a", doc("A content"), templateID)
	createPage("Template B", "template-b", doc("B content"), templateID)
	for _, path := range []string{"/site-part-a", "/site-part-b", "/template-a", "/template-b"} {
		resp := getPath(t, client, server.URL, path)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "/_stratum/forms/"+formID) {
			t.Fatalf("%s status=%d body=%s", path, resp.StatusCode, body)
		}
		if !strings.Contains(body, `name="return_to" value="`+path+`"`) {
			t.Fatalf("%s missing return path: %s", path, body)
		}
	}
}
