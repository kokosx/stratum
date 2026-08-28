package admin

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/forms"
)

type formsListData struct {
	Forms     []forms.FormSummary
	CSRFToken string
}
type formEditorData struct {
	Form             forms.Form
	CSRFToken, Error string
}
type submissionRowView struct {
	Submission                   forms.Submission
	Primary, Secondary, Received string
}
type submissionsData struct {
	Form      forms.Form
	Rows      []submissionRowView
	CSRFToken string
}
type submissionValueView struct {
	Label, Key, Value string
	Type              forms.FieldType
}
type submissionDetailData struct {
	Form                forms.Form
	Submission          forms.Submission
	Values              []submissionValueView
	Received, CSRFToken string
}

func (h *Handler) listForms(w http.ResponseWriter, r *http.Request) {
	items, err := h.forms.List(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	token, _ := h.csrfToken(w, r)
	data := h.layoutDataWithFlash(w, r, "Forms")
	data.CSRFToken = token
	data.Content = formsListData{Forms: items, CSRFToken: token}
	if err := h.formsTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render forms: %v", err)
	}
}

func (h *Handler) newForm(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrfToken(w, r)
	data := h.layoutData(r, "New Form")
	data.CSRFToken = token
	data.Content = formEditorData{CSRFToken: token}
	if err := h.formNewTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render new form: %v", err)
	}
}

func (h *Handler) createForm(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	form, err := h.forms.Create(r.Context(), r.FormValue("name"))
	if err != nil {
		token, _ := h.csrfToken(w, r)
		data := h.layoutData(r, "New Form")
		data.CSRFToken = token
		data.Content = formEditorData{Form: forms.Form{Name: r.FormValue("name")}, CSRFToken: token, Error: err.Error()}
		_ = h.formNewTemplate.ExecuteTemplate(w, "layout.html", data)
		return
	}
	h.setFlash(w, "Form created.")
	http.Redirect(w, r, "/admin/forms/"+form.ID+"/edit", http.StatusSeeOther)
}

func (h *Handler) editForm(w http.ResponseWriter, r *http.Request) {
	form, err := h.forms.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderFormEditor(w, r, form, "")
}
func (h *Handler) renderFormEditor(w http.ResponseWriter, r *http.Request, form forms.Form, errText string) {
	token, _ := h.csrfToken(w, r)
	data := h.layoutDataWithFlash(w, r, "Edit Form")
	data.CSRFToken = token
	data.Content = formEditorData{Form: form, CSRFToken: token, Error: errText}
	if err := h.formEditorTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render form editor: %v", err)
	}
}

func (h *Handler) saveForm(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	existing, err := h.forms.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	ids, keys, types, labels := r.PostForm["field_id"], r.PostForm["field_key"], r.PostForm["field_type"], r.PostForm["field_label"]
	if len(ids) != len(keys) || len(ids) != len(types) || len(ids) != len(labels) {
		h.renderFormEditor(w, r, existing, "Invalid fields")
		return
	}
	fields := make([]forms.Field, 0, len(ids))
	placeholders := r.PostForm["field_placeholder"]
	options := r.PostForm["field_options"]
	for i := range ids {
		placeholder, opt := "", ""
		if i < len(placeholders) {
			placeholder = placeholders[i]
		}
		if i < len(options) {
			opt = options[i]
		}
		required := r.PostForm.Has("required_" + ids[i])
		field := forms.Field{ID: strings.TrimSpace(ids[i]), Key: strings.TrimSpace(keys[i]), Type: forms.FieldType(types[i]), Label: strings.TrimSpace(labels[i]), Placeholder: placeholder, Required: required}
		if field.Type == forms.FieldSelect {
			for _, line := range strings.Split(strings.ReplaceAll(opt, "\r\n", "\n"), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					field.Options = append(field.Options, line)
				}
			}
		}
		fields = append(fields, field)
	}
	existing.Name = r.FormValue("name")
	existing.Active = r.FormValue("active") == "1"
	existing.Definition = forms.Definition{Fields: fields, SubmitLabel: r.FormValue("submit_label"), SuccessMessage: r.FormValue("success_message"), NotificationEmail: strings.TrimSpace(r.FormValue("notification_email"))}
	if err := h.forms.Update(r.Context(), existing); err != nil {
		h.renderFormEditor(w, r, existing, err.Error())
		return
	}
	h.setFlash(w, "Form saved.")
	http.Redirect(w, r, "/admin/forms/"+existing.ID+"/edit", http.StatusSeeOther)
}

func (h *Handler) deleteForm(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	err := h.forms.Delete(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, forms.ErrHasReferences):
		h.setFlash(w, "Cannot delete this form because it is used in content.")
	case errors.Is(err, forms.ErrHasSubmissions):
		h.setFlash(w, "Cannot delete this form while it has submissions. Delete submissions first.")
	case err != nil:
		h.setFlash(w, "Could not delete form.")
	default:
		h.setFlash(w, "Form deleted.")
	}
	http.Redirect(w, r, "/admin/forms", http.StatusSeeOther)
}

func primaryValues(sub forms.Submission) (string, string) {
	values := []string{}
	for _, field := range sub.SchemaSnapshot.Fields {
		value := strings.TrimSpace(sub.Values[field.Key])
		if value != "" && (field.Type == forms.FieldText || field.Type == forms.FieldEmail || field.Type == forms.FieldTextarea) {
			values = append(values, value)
			if len(values) == 2 {
				break
			}
		}
	}
	if len(values) == 0 {
		return "Submission", ""
	}
	if len(values) == 1 {
		return values[0], ""
	}
	return values[0], values[1]
}
func formatReceived(t time.Time) string { return t.Local().Format("2 Jan 2006 15:04") }

func (h *Handler) listFormSubmissions(w http.ResponseWriter, r *http.Request) {
	form, err := h.forms.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	subs, err := h.forms.ListSubmissions(r.Context(), form.ID, 100, 0)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	rows := make([]submissionRowView, 0, len(subs))
	for _, sub := range subs {
		p, s := primaryValues(sub)
		rows = append(rows, submissionRowView{Submission: sub, Primary: p, Secondary: s, Received: formatReceived(sub.CreatedAt)})
	}
	token, _ := h.csrfToken(w, r)
	data := h.layoutDataWithFlash(w, r, "Submissions")
	data.CSRFToken = token
	data.Content = submissionsData{Form: form, Rows: rows, CSRFToken: token}
	if err := h.submissionsTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render submissions: %v", err)
	}
}

func (h *Handler) viewFormSubmission(w http.ResponseWriter, r *http.Request) {
	form, err := h.forms.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sub, err := h.forms.GetSubmission(r.Context(), r.PathValue("submissionID"))
	if err != nil || sub.FormID != form.ID {
		http.NotFound(w, r)
		return
	}
	values := make([]submissionValueView, 0, len(sub.SchemaSnapshot.Fields))
	for _, field := range sub.SchemaSnapshot.Fields {
		values = append(values, submissionValueView{Label: field.Label, Key: field.Key, Value: sub.Values[field.Key], Type: field.Type})
	}
	token, _ := h.csrfToken(w, r)
	data := h.layoutDataWithFlash(w, r, "Submission")
	data.CSRFToken = token
	data.Content = submissionDetailData{Form: form, Submission: sub, Values: values, Received: formatReceived(sub.CreatedAt), CSRFToken: token}
	if err := h.submissionTemplate.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render submission: %v", err)
	}
}

func (h *Handler) updateFormSubmissionStatus(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}
	status := forms.SubmissionStatus(r.FormValue("status"))
	if err := h.forms.UpdateSubmissionStatus(r.Context(), r.PathValue("submissionID"), status); err != nil {
		http.Error(w, "Invalid status", 422)
		return
	}
	h.setFlash(w, "Submission updated.")
	http.Redirect(w, r, fmt.Sprintf("/admin/forms/%s/submissions/%s", r.PathValue("id"), r.PathValue("submissionID")), 303)
}
func (h *Handler) deleteFormSubmission(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(r) {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}
	sub, err := h.forms.GetSubmission(r.Context(), r.PathValue("submissionID"))
	if err != nil || sub.FormID != r.PathValue("id") {
		http.NotFound(w, r)
		return
	}
	if err := h.forms.DeleteSubmission(r.Context(), sub.ID); err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	h.setFlash(w, "Submission deleted.")
	http.Redirect(w, r, "/admin/forms/"+sub.FormID+"/submissions", 303)
}
func (h *Handler) exportFormSubmissions(w http.ResponseWriter, r *http.Request) {
	form, err := h.forms.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := h.forms.ExportCSV(r.Context(), form.ID)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=form-%s-submissions.csv", form.ID))
	_, _ = w.Write(data)
}
