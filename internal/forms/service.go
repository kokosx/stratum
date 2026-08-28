package forms

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/mailer"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

var (
	ErrNotFound       = errors.New("form not found")
	ErrDisabled       = errors.New("form is disabled")
	ErrInvalid        = errors.New("invalid form submission")
	ErrHoneypot       = errors.New("honeypot submission")
	ErrRateLimited    = errors.New("too many submissions")
	ErrHasReferences  = errors.New("form is referenced by content")
	ErrHasSubmissions = errors.New("form has submissions")
)

var fieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Service struct {
	db         *sql.DB
	queries    *db.Queries
	mailer     mailer.Mailer
	from       string
	limiter    *rateLimiter
	invalidate func()
	now        func() time.Time
	id         func() (string, error)
}

func NewService(database *sql.DB, queries *db.Queries, mailer mailer.Mailer, from string) *Service {
	return &Service{db: database, queries: queries, mailer: mailer, from: from, limiter: newRateLimiter(), now: time.Now, id: randomID}
}

func (s *Service) SetInvalidator(fn func()) { s.invalidate = fn }

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func ValidateDefinition(name string, def Definition) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > MaxFormName {
		return errors.New("name must be between 1 and 200 characters")
	}
	if len(def.Fields) == 0 || len(def.Fields) > MaxFields {
		return fmt.Errorf("form must contain between 1 and %d fields", MaxFields)
	}
	if strings.TrimSpace(def.SubmitLabel) == "" || len(def.SubmitLabel) > 100 {
		return errors.New("submit label is required")
	}
	if strings.TrimSpace(def.SuccessMessage) == "" || len(def.SuccessMessage) > 1000 {
		return errors.New("success message is required")
	}
	if def.NotificationEmail != "" {
		if strings.ContainsAny(def.NotificationEmail, "\r\n") {
			return errors.New("invalid notification email")
		}
		addr, err := mail.ParseAddress(def.NotificationEmail)
		if err != nil || addr.Address != def.NotificationEmail {
			return errors.New("invalid notification email")
		}
	}
	keys, ids := map[string]bool{}, map[string]bool{}
	for i, f := range def.Fields {
		if f.ID == "" || ids[f.ID] {
			return fmt.Errorf("field %d has an invalid or duplicate ID", i+1)
		}
		ids[f.ID] = true
		if !fieldKeyPattern.MatchString(f.Key) || keys[f.Key] {
			return fmt.Errorf("field %d has an invalid or duplicate key", i+1)
		}
		keys[f.Key] = true
		if strings.TrimSpace(f.Label) == "" || len(f.Label) > 200 || len(f.Placeholder) > 500 {
			return fmt.Errorf("field %q has invalid text", f.Key)
		}
		switch f.Type {
		case FieldText, FieldEmail, FieldTextarea, FieldCheckbox:
			if len(f.Options) != 0 {
				return fmt.Errorf("field %q cannot have options", f.Key)
			}
		case FieldSelect:
			if len(f.Options) == 0 || len(f.Options) > MaxOptions {
				return fmt.Errorf("select %q must contain options", f.Key)
			}
		default:
			return fmt.Errorf("field %q has invalid type", f.Key)
		}
		seenOptions := map[string]bool{}
		for _, option := range f.Options {
			if option == "" || len(option) > 500 || seenOptions[option] {
				return fmt.Errorf("select %q has invalid options", f.Key)
			}
			seenOptions[option] = true
		}
	}
	return nil
}

func StarterDefinition(id func() (string, error)) (Definition, error) {
	fields := []Field{{Key: "name", Type: FieldText, Label: "Name", Required: true}, {Key: "email", Type: FieldEmail, Label: "Email", Required: true}, {Key: "message", Type: FieldTextarea, Label: "Message", Required: true}}
	for i := range fields {
		value, err := id()
		if err != nil {
			return Definition{}, err
		}
		fields[i].ID = value
	}
	return Definition{Fields: fields, SubmitLabel: "Send message", SuccessMessage: "Thanks! Your message has been sent."}, nil
}

func (s *Service) Create(ctx context.Context, name string) (Form, error) {
	def, err := StarterDefinition(s.id)
	if err != nil {
		return Form{}, err
	}
	if err := ValidateDefinition(name, def); err != nil {
		return Form{}, err
	}
	id, err := s.id()
	if err != nil {
		return Form{}, err
	}
	encoded, _ := json.Marshal(def)
	now := s.now().Unix()
	if err := s.queries.CreateForm(ctx, db.CreateFormParams{ID: id, Name: strings.TrimSpace(name), SchemaVersion: SchemaVersion, DefinitionJson: string(encoded), Active: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		return Form{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, form Form) error {
	if form.SchemaVersion == 0 {
		form.SchemaVersion = SchemaVersion
	}
	if form.SchemaVersion != SchemaVersion {
		return errors.New("unsupported form schema version")
	}
	if err := ValidateDefinition(form.Name, form.Definition); err != nil {
		return err
	}
	encoded, err := json.Marshal(form.Definition)
	if err != nil {
		return err
	}
	active := int64(0)
	if form.Active {
		active = 1
	}
	if err := s.queries.UpdateForm(ctx, db.UpdateFormParams{Name: strings.TrimSpace(form.Name), SchemaVersion: SchemaVersion, DefinitionJson: string(encoded), Active: active, UpdatedAt: s.now().Unix(), ID: form.ID}); err != nil {
		return err
	}
	if s.invalidate != nil {
		s.invalidate()
	}
	return nil
}

func decodeDefinition(raw string, version int64) (Definition, error) {
	if version != SchemaVersion {
		return Definition{}, fmt.Errorf("unsupported form schema version %d", version)
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var def Definition
	if err := dec.Decode(&def); err != nil {
		return Definition{}, err
	}
	if err := ensureEOF(dec); err != nil {
		return Definition{}, err
	}
	return def, nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func formFromRow(row db.Form) (Form, error) {
	def, err := decodeDefinition(row.DefinitionJson, row.SchemaVersion)
	if err != nil {
		return Form{}, err
	}
	return Form{ID: row.ID, Name: row.Name, SchemaVersion: int(row.SchemaVersion), Definition: def, Active: row.Active == 1, CreatedAt: time.Unix(row.CreatedAt, 0), UpdatedAt: time.Unix(row.UpdatedAt, 0)}, nil
}

func (s *Service) Get(ctx context.Context, id string) (Form, error) {
	row, err := s.queries.GetForm(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Form{}, ErrNotFound
	}
	if err != nil {
		return Form{}, err
	}
	return formFromRow(row)
}

type FormState string

const (
	FormStateMissing  FormState = "missing"
	FormStateDisabled FormState = "disabled"
	FormStateActive   FormState = "active"
)

type FormResolution struct {
	State FormState
	View  FormView
}

func (s *Service) GetActiveForm(ctx context.Context, id string) (FormView, bool) {
	res := s.ResolveForm(ctx, id)
	if res.State != FormStateActive {
		return FormView{}, false
	}
	return res.View, true
}

func (s *Service) ResolveForm(ctx context.Context, id string) FormResolution {
	if id == "" {
		return FormResolution{State: FormStateMissing}
	}
	form, err := s.Get(ctx, id)
	if err != nil {
		return FormResolution{State: FormStateMissing}
	}
	if !form.Active {
		return FormResolution{State: FormStateDisabled}
	}
	return FormResolution{State: FormStateActive, View: FormView{ID: form.ID, Fields: form.Fields, SubmitLabel: form.SubmitLabel, SuccessMessage: form.SuccessMessage}}
}

func (s *Service) List(ctx context.Context) ([]FormSummary, error) {
	rows, err := s.queries.ListForms(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FormSummary, 0, len(rows))
	for _, row := range rows {
		f, err := formFromRow(db.Form{ID: row.ID, Name: row.Name, SchemaVersion: row.SchemaVersion, DefinitionJson: row.DefinitionJson, Active: row.Active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
		if err != nil {
			return nil, err
		}
		out = append(out, FormSummary{Form: f, SubmissionCount: row.SubmissionCount, NewCount: row.NewCount})
	}
	return out, nil
}
func (s *Service) ListActive(ctx context.Context) ([]Form, error) {
	rows, err := s.queries.ListActiveForms(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Form, 0, len(rows))
	for _, row := range rows {
		f, e := formFromRow(row)
		if e != nil {
			return nil, e
		}
		out = append(out, f)
	}
	return out, nil
}

func ValidateReturnPath(value string) bool {
	if value == "" || value[0] != '/' || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") || hasControl(value) {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Opaque != "" || !strings.HasPrefix(parsed.Path, "/") {
		return false
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || strings.Contains(decodedPath, "\\") || hasControl(decodedPath) {
		return false
	}
	decodedQuery, err := url.QueryUnescape(parsed.RawQuery)
	return err == nil && !strings.Contains(decodedQuery, "\\") && !hasControl(decodedQuery)
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func validateValues(fields []Field, submitted map[string][]string) (map[string]string, error) {
	known := make(map[string]Field, len(fields))
	for _, f := range fields {
		known[f.Key] = f
	}
	for key := range submitted {
		if _, ok := known[key]; !ok {
			return nil, ErrInvalid
		}
	}
	values := make(map[string]string, len(fields))
	for _, f := range fields {
		raw := submitted[f.Key]
		if len(raw) > 1 {
			return nil, ErrInvalid
		}
		value := ""
		if len(raw) == 1 {
			value = raw[0]
		}
		if f.Required && (value == "" || f.Type == FieldCheckbox && value != "1") {
			return nil, ErrInvalid
		}
		switch f.Type {
		case FieldText:
			if len(value) > MaxText {
				return nil, ErrInvalid
			}
		case FieldEmail:
			if len(value) > MaxEmail {
				return nil, ErrInvalid
			}
			if value != "" {
				addr, err := mail.ParseAddress(value)
				if err != nil || addr.Address != value {
					return nil, ErrInvalid
				}
			}
		case FieldTextarea:
			if len(value) > MaxTextarea {
				return nil, ErrInvalid
			}
		case FieldSelect:
			allowed := value == ""
			for _, o := range f.Options {
				if value == o {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, ErrInvalid
			}
		case FieldCheckbox:
			if value != "" && value != "1" {
				return nil, ErrInvalid
			}
		default:
			return nil, ErrInvalid
		}
		values[f.Key] = value
	}
	return values, nil
}

func (s *Service) Submit(ctx context.Context, formID string, input SubmitInput) (Submission, error) {
	if strings.TrimSpace(input.Honeypot) != "" {
		return Submission{}, ErrHoneypot
	}
	if input.Now.IsZero() {
		input.Now = s.now()
	}
	form, err := s.Get(ctx, formID)
	if err != nil {
		return Submission{}, err
	}
	if !form.Active {
		return Submission{}, ErrDisabled
	}
	// Count all requests to a real, active form. This deliberately happens before
	// validation so malformed traffic cannot bypass the inexpensive limiter.
	key := input.ClientIP + "\x00" + formID
	if !s.limiter.AllowAndRecord(key, input.Now) {
		return Submission{}, ErrRateLimited
	}
	values, err := validateValues(form.Fields, input.Values)
	if err != nil {
		return Submission{}, err
	}
	snapshot := FormSnapshot{Name: form.Name, Fields: append([]Field(nil), form.Fields...)}
	valuesJSON, _ := json.Marshal(values)
	snapshotJSON, _ := json.Marshal(snapshot)
	id, err := s.id()
	if err != nil {
		return Submission{}, err
	}
	if err := s.queries.CreateFormSubmission(ctx, db.CreateFormSubmissionParams{ID: id, FormID: form.ID, Status: string(StatusNew), ValuesJson: string(valuesJSON), SchemaSnapshotJson: string(snapshotJSON), CreatedAt: input.Now.Unix()}); err != nil {
		return Submission{}, err
	}
	sub := Submission{ID: id, FormID: form.ID, Status: StatusNew, Values: values, SchemaSnapshot: snapshot, CreatedAt: input.Now}
	recipient := form.NotificationEmail
	if recipient == "" {
		if users, listErr := s.queries.ListUsers(ctx); listErr == nil {
			for _, user := range users {
				if user.Role == "admin" && user.Status == "active" {
					recipient = user.Email
					break
				}
			}
		}
	}
	mailerAvailable := s.mailer != nil
	if availability, ok := s.mailer.(interface{ Available() bool }); ok {
		mailerAvailable = availability.Available()
	}
	if recipient != "" && mailerAvailable {
		if err := s.sendNotification(ctx, form, sub, recipient); err != nil {
			log.Printf("form notification delivery failed for form %s: %v", form.ID, err)
		}
	}
	return sub, nil
}

func (s *Service) sendNotification(ctx context.Context, form Form, sub Submission, recipient string) error {
	var body strings.Builder
	fmt.Fprintf(&body, "New submission: %s\n\n", form.Name)
	emails := []string{}
	for _, field := range sub.SchemaSnapshot.Fields {
		value := sub.Values[field.Key]
		fmt.Fprintf(&body, "%s:\n%s\n\n", field.Label, value)
		if field.Type == FieldEmail && value != "" {
			emails = append(emails, value)
		}
	}
	reply := ""
	if len(emails) == 1 {
		reply = emails[0]
	}
	return s.mailer.Send(ctx, mailer.Message{To: recipient, From: s.from, Subject: "New submission: " + form.Name, ReplyTo: reply, Body: body.String()})
}

func submissionFromRow(row db.FormSubmission) (Submission, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(row.ValuesJson), &values); err != nil {
		return Submission{}, err
	}
	var snap FormSnapshot
	dec := json.NewDecoder(strings.NewReader(row.SchemaSnapshotJson))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snap); err != nil {
		return Submission{}, err
	}
	return Submission{ID: row.ID, FormID: row.FormID, Status: SubmissionStatus(row.Status), Values: values, SchemaSnapshot: snap, CreatedAt: time.Unix(row.CreatedAt, 0)}, nil
}
func (s *Service) ListSubmissions(ctx context.Context, formID string, limit, offset int64) ([]Submission, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.queries.ListFormSubmissions(ctx, db.ListFormSubmissionsParams{FormID: formID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	out := make([]Submission, 0, len(rows))
	for _, row := range rows {
		sub, e := submissionFromRow(row)
		if e != nil {
			return nil, e
		}
		out = append(out, sub)
	}
	return out, nil
}
func (s *Service) CountSubmissions(ctx context.Context, formID string) (int64, error) {
	return s.queries.CountFormSubmissions(ctx, formID)
}
func (s *Service) GetSubmission(ctx context.Context, id string) (Submission, error) {
	row, err := s.queries.GetFormSubmission(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, err
	}
	return submissionFromRow(row)
}
func validStatus(v SubmissionStatus) bool {
	return v == StatusNew || v == StatusRead || v == StatusSpam || v == StatusArchived
}
func (s *Service) UpdateSubmissionStatus(ctx context.Context, id string, status SubmissionStatus) error {
	if !validStatus(status) {
		return ErrInvalid
	}
	return s.queries.UpdateFormSubmissionStatus(ctx, db.UpdateFormSubmissionStatusParams{Status: string(status), ID: id})
}
func (s *Service) UpdateSubmissionStatusForForm(ctx context.Context, formID, id string, status SubmissionStatus) error {
	if !validStatus(status) {
		return ErrInvalid
	}
	submission, err := s.GetSubmission(ctx, id)
	if err != nil {
		return err
	}
	if submission.FormID != formID {
		return ErrNotFound
	}
	return s.queries.UpdateFormSubmissionStatus(ctx, db.UpdateFormSubmissionStatusParams{Status: string(status), ID: id})
}

func (s *Service) MailConfigured() bool {
	if s.mailer == nil {
		return false
	}
	if availability, ok := s.mailer.(interface{ Available() bool }); ok {
		return availability.Available()
	}
	return true
}
func (s *Service) DeleteSubmission(ctx context.Context, id string) error {
	return s.queries.DeleteFormSubmission(ctx, id)
}

func hasFormReference(nodes []document.Node, id string) bool {
	for _, node := range nodes {
		if node.Block == "core/form" {
			var settings struct {
				FormID string `json:"formId"`
			}
			if json.Unmarshal(node.Settings, &settings) == nil && settings.FormID == id {
				return true
			}
		}
		if hasFormReference(node.Children, id) {
			return true
		}
	}
	return false
}
func (s *Service) ReferenceCount(ctx context.Context, id string) (int, error) {
	docs, err := s.queries.ListDocumentsForFormReferenceScan(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, raw := range docs {
		doc, err := document.Decode([]byte(raw))
		if err == nil && hasFormReference(doc.Nodes, id) {
			count++
		}
	}
	return count, nil
}
func (s *Service) Delete(ctx context.Context, id string) error {
	refs, err := s.ReferenceCount(ctx, id)
	if err != nil {
		return err
	}
	if refs > 0 {
		return ErrHasReferences
	}
	count, err := s.queries.CountFormSubmissions(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrHasSubmissions
	}
	if err := s.queries.DeleteForm(ctx, id); err != nil {
		return err
	}
	if s.invalidate != nil {
		s.invalidate()
	}
	return nil
}

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n\v\f")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
func (s *Service) ExportCSV(ctx context.Context, formID string) ([]byte, error) {
	rows, err := s.queries.ListAllFormSubmissions(ctx, formID)
	if err != nil {
		return nil, err
	}
	subs := make([]Submission, 0, len(rows))
	keys := map[string]bool{}
	ordered := []string{}
	for _, row := range rows {
		sub, e := submissionFromRow(row)
		if e != nil {
			return nil, e
		}
		subs = append(subs, sub)
		for _, f := range sub.SchemaSnapshot.Fields {
			if !keys[f.Key] {
				keys[f.Key] = true
				ordered = append(ordered, f.Key)
			}
		}
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := append([]string{"submitted_at", "status"}, ordered...)
	_ = w.Write(header)
	for _, sub := range subs {
		record := []string{sub.CreatedAt.UTC().Format(time.RFC3339), string(sub.Status)}
		for _, k := range ordered {
			record = append(record, safeCSVCell(sub.Values[k]))
		}
		_ = w.Write(record)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type rateLimiter struct {
	mu         sync.Mutex
	events     map[string][]time.Time
	operations uint64
}

func newRateLimiter() *rateLimiter { return &rateLimiter{events: map[string][]time.Time{}} }
func (r *rateLimiter) AllowAndRecord(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations++
	if r.operations%128 == 0 {
		r.cleanupLocked(now)
	}
	events := r.events[key][:0]
	hourAgo := now.Add(-time.Hour)
	minuteAgo := now.Add(-time.Minute)
	minute := 0
	for _, event := range r.events[key] {
		if event.After(hourAgo) {
			events = append(events, event)
			if event.After(minuteAgo) {
				minute++
			}
		}
	}
	r.events[key] = events
	if minute >= 5 || len(events) >= 30 {
		return false
	}
	r.events[key] = append(r.events[key], now)
	return true
}

func (r *rateLimiter) cleanupLocked(now time.Time) {
	hourAgo := now.Add(-time.Hour)
	for key, stored := range r.events {
		kept := stored[:0]
		for _, event := range stored {
			if event.After(hourAgo) {
				kept = append(kept, event)
			}
		}
		if len(kept) == 0 {
			delete(r.events, key)
		} else {
			r.events[key] = kept
		}
	}
}
