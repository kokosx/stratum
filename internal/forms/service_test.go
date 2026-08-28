package forms

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/mailer"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func validDefinition() Definition {
	return Definition{Fields: []Field{
		{ID: "f_name", Key: "name", Type: FieldText, Label: "Name", Required: true},
		{ID: "f_email", Key: "email", Type: FieldEmail, Label: "Email", Required: true},
		{ID: "f_kind", Key: "kind", Type: FieldSelect, Label: "Kind", Options: []string{"One", "Two"}},
		{ID: "f_ok", Key: "ok", Type: FieldCheckbox, Label: "Agree", Required: true},
	}, SubmitLabel: "Send", SuccessMessage: "Thanks"}
}

func TestValidateDefinitionRejectsDuplicateKeysAndInvalidTypes(t *testing.T) {
	def := validDefinition()
	def.Fields[1].Key = def.Fields[0].Key
	if err := ValidateDefinition("Contact", def); err == nil {
		t.Fatal("duplicate key accepted")
	}
	def = validDefinition()
	def.Fields[0].Type = "phone"
	if err := ValidateDefinition("Contact", def); err == nil {
		t.Fatal("invalid type accepted")
	}
}

func TestValidateValuesRequiredEmailSelectCheckboxAndLimits(t *testing.T) {
	fields := validDefinition().Fields
	valid := map[string][]string{"name": {"Jane"}, "email": {"jane@example.com"}, "kind": {"One"}, "ok": {"1"}}
	if _, err := validateValues(fields, valid); err != nil {
		t.Fatalf("valid values: %v", err)
	}
	for name, mutate := range map[string]func(map[string][]string){
		"required": func(v map[string][]string) { delete(v, "name") },
		"email":    func(v map[string][]string) { v["email"] = []string{"bad"} },
		"select":   func(v map[string][]string) { v["kind"] = []string{"Other"} },
		"checkbox": func(v map[string][]string) { v["ok"] = []string{"on"} },
		"unknown":  func(v map[string][]string) { v["extra"] = []string{"x"} },
		"limit":    func(v map[string][]string) { v["name"] = []string{strings.Repeat("x", MaxText+1)} },
	} {
		t.Run(name, func(t *testing.T) {
			copy := map[string][]string{}
			for k, v := range valid {
				copy[k] = append([]string(nil), v...)
			}
			mutate(copy)
			if _, err := validateValues(fields, copy); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestReturnPathSafety(t *testing.T) {
	for _, value := range []string{"/", "/contact", "/products/a?from=quote"} {
		if !ValidateReturnPath(value) {
			t.Errorf("safe path rejected: %q", value)
		}
	}
	for _, value := range []string{"", "https://evil.test", "//evil.test", "contact", "javascript:alert(1)", "\\evil.test", "/\\evil.test", "/ok\r\nLocation: https://evil.test", "/%5cevil.test", "/ok?x=%0d%0aLocation"} {
		if ValidateReturnPath(value) {
			t.Errorf("unsafe path accepted: %q", value)
		}
	}
}

func TestRateLimiterHasMinuteAndHourWindows(t *testing.T) {
	r := newRateLimiter()
	now := time.Unix(10000, 0)
	for i := 0; i < 5; i++ {
		if !r.AllowAndRecord("key", now) {
			t.Fatalf("request %d denied", i)
		}
	}
	if r.AllowAndRecord("key", now) {
		t.Fatal("sixth minute request accepted")
	}
	if !r.AllowAndRecord("key", now.Add(time.Minute+time.Second)) {
		t.Fatal("minute window did not reset")
	}

	hourly := newRateLimiter()
	for i := 0; i < 30; i++ {
		at := now.Add(time.Duration(i) * 2 * time.Minute)
		if !hourly.AllowAndRecord("key", at) {
			t.Fatalf("hour request %d denied", i)
		}
	}
	if hourly.AllowAndRecord("key", now.Add(59*time.Minute)) {
		t.Fatal("31st hourly request accepted")
	}
	if !hourly.AllowAndRecord("key", now.Add(2*time.Hour)) {
		t.Fatal("hour window did not reset")
	}
}

func TestRateLimiterConcurrentRequestsAreAtomic(t *testing.T) {
	r := newRateLimiter()
	now := time.Unix(10000, 0)
	var accepted atomic.Int64
	var wait sync.WaitGroup
	for i := 0; i < 100; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if r.AllowAndRecord("same", now) {
				accepted.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := accepted.Load(); got != 5 {
		t.Fatalf("accepted=%d want 5", got)
	}
}

func TestRateLimiterOpportunisticallyCleansStaleKeys(t *testing.T) {
	r := newRateLimiter()
	old := time.Unix(10000, 0)
	for i := 0; i < 1000; i++ {
		if !r.AllowAndRecord("old-"+strconv.Itoa(i), old) {
			t.Fatal("unique key rejected")
		}
	}
	for i := 0; i < 128; i++ {
		r.AllowAndRecord("new-"+strconv.Itoa(i), old.Add(2*time.Hour))
	}
	for key := range r.events {
		if strings.HasPrefix(key, "old-") {
			t.Fatalf("stale key retained: %s", key)
		}
	}
}

type fakeMailer struct {
	err      error
	messages []mailer.Message
}

func (f *fakeMailer) Send(_ context.Context, m mailer.Message) error {
	f.messages = append(f.messages, m)
	return f.err
}

func newFormService(t *testing.T, mail mailer.Mailer) (*Service, *db.Queries) {
	t.Helper()
	database, err := storage.Open(filepath.Join(t.TempDir(), "forms.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	service := NewService(database.DB, queries, mail, "site@example.com")
	next := 0
	service.id = func() (string, error) { next++; return "id_" + string(rune('a'+next)), nil }
	service.now = func() time.Time { return time.Unix(1000, 0) }
	return service, queries
}

func TestSubmitSnapshotsSchemaAndMailFailureDoesNotLoseSubmission(t *testing.T) {
	mail := &fakeMailer{err: errors.New("smtp down")}
	service, queries := newFormService(t, mail)
	ctx := context.Background()
	form, err := service.Create(ctx, "Contact")
	if err != nil {
		t.Fatal(err)
	}
	form.NotificationEmail = "owner@example.com"
	if err := service.Update(ctx, form); err != nil {
		t.Fatal(err)
	}
	values := map[string][]string{"name": {"Jane"}, "email": {"jane@example.com"}, "message": {"Hello"}}
	sub, err := service.Submit(ctx, form.ID, SubmitInput{Values: values, ClientIP: "127.0.0.1", Now: time.Unix(2000, 0)})
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[2].Label = "Project details"
	if err := service.Update(ctx, form); err != nil {
		t.Fatal(err)
	}
	stored, err := service.GetSubmission(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SchemaSnapshot.Fields[2].Label != "Message" {
		t.Fatalf("snapshot mutated: %#v", stored.SchemaSnapshot.Fields[2])
	}
	count, err := queries.CountFormSubmissions(ctx, form.ID)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if len(mail.messages) != 1 {
		t.Fatalf("mail calls = %d", len(mail.messages))
	}
}

func TestHoneypotDoesNotPersistAndDisabledFormRejects(t *testing.T) {
	service, queries := newFormService(t, nil)
	ctx := context.Background()
	form, _ := service.Create(ctx, "Contact")
	_, err := service.Submit(ctx, form.ID, SubmitInput{Honeypot: "bot", ClientIP: "ip"})
	if !errors.Is(err, ErrHoneypot) {
		t.Fatalf("error=%v", err)
	}
	count, _ := queries.CountFormSubmissions(ctx, form.ID)
	if count != 0 {
		t.Fatalf("persisted %d", count)
	}
	form.Active = false
	if err := service.Update(ctx, form); err != nil {
		t.Fatal(err)
	}
	_, err = service.Submit(ctx, form.ID, SubmitInput{Values: map[string][]string{}, ClientIP: "ip2"})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("error=%v", err)
	}
}

func TestCSVFormulaInjectionEscaped(t *testing.T) {
	for _, value := range []string{"=SUM(A1:A2)", "+cmd", "-1", "@x", " =SUM(A1:A2)", "\t+cmd", "\r-1"} {
		if got := safeCSVCell(value); got != "'"+value {
			t.Errorf("safeCSVCell(%q)=%q", value, got)
		}
	}
	if got := safeCSVCell("normal"); got != "normal" {
		t.Fatal(got)
	}
}

func TestFormReferenceTraversalIsExact(t *testing.T) {
	doc := []document.Node{{ID: "outer", Block: "core/section", Children: []document.Node{{ID: "form", Block: "core/form", Settings: json.RawMessage(`{"formId":"target"}`)}}}}
	if !hasFormReference(doc, "target") {
		t.Fatal("nested exact reference missed")
	}
	if hasFormReference(doc, "tar") {
		t.Fatal("substring reference matched")
	}
}
