package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func caseStudyDefinition() ContentTypeDefinition {
	return ContentTypeDefinition{
		ID: "case-study", Name: "Case Study", PluralName: "Case Studies", SchemaVersion: 1,
		Fields: []FieldDefinition{
			{Key: "client", Label: "Client", Type: FieldText, Required: true},
			{Key: "project_url", Label: "Project URL", Type: FieldURL},
			{Key: "launch_date", Label: "Launch Date", Type: FieldDate},
			{Key: "featured", Label: "Featured", Type: FieldBoolean},
			{Key: "price", Label: "Price", Type: FieldNumber},
			{Key: "hero_media", Label: "Hero Media", Type: FieldMedia},
			{Key: "contact", Label: "Contact", Type: FieldEmail},
			{Key: "format", Label: "Format", Type: FieldSelect, Validation: FieldValidation{Options: []string{"web", "print"}}},
		},
	}
}

func TestRevisionFieldSnapshotsRemainIndependent(t *testing.T) {
	_, _, queries := newTestRepository(t)
	ctx := context.Background()
	if err := queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "case-study", DisplayName: "Case Study", PluralName: "Case Studies", Public: 1, ConfigJson: `{}`, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "case-1", ContentTypeID: "case-study", Slug: "case-1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []struct {
		id, fields string
		number     int64
	}{
		{"case-r1", `{"price":100}`, 1},
		{"case-r2", `{"price":200}`, 2},
	} {
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revision.id, EntryID: "case-1", RevisionNumber: revision.number, Title: "Case", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: revision.fields, CreatedAt: revision.number}); err != nil {
			t.Fatal(err)
		}
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: "case-1", PublishedRevisionID: sql.NullString{String: "case-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	published, err := queries.GetPublishedEntryByID(ctx, "case-1")
	if err != nil || published.FieldsJson != `{"price":100}` {
		t.Fatalf("published snapshot leaked draft: %#v, %v", published, err)
	}
	draft, err := queries.GetLatestEntryRevision(ctx, "case-1")
	if err != nil || draft.FieldsJson != `{"price":200}` {
		t.Fatalf("latest draft fields = %#v, %v", draft, err)
	}
	// Restoring creates another immutable revision containing the historical object.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "case-r3", EntryID: "case-1", RevisionNumber: 3, Title: "Case", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: published.FieldsJson, CreatedAt: 3}); err != nil {
		t.Fatal(err)
	}
	restored, err := queries.GetLatestEntryRevision(ctx, "case-1")
	if err != nil || restored.FieldsJson != `{"price":100}` {
		t.Fatalf("restored snapshot = %#v, %v", restored, err)
	}
}

func TestValidateFieldsNormalizesTypedValues(t *testing.T) {
	fields, err := ValidateFields(caseStudyDefinition(), map[string]any{
		"client": " Acme ", "project_url": "HTTPS://EXAMPLE.COM/work", "launch_date": "2026-08-24",
		"featured": "true", "price": "129.99", "hero_media": "media-1", "contact": "HELLO@EXAMPLE.COM", "format": "web",
	}, FieldValidationOptions{MediaExists: func(id string) bool { return id == "media-1" }})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := fields["featured"].(bool); !ok || !got {
		t.Fatalf("boolean was not retained: %#v", fields["featured"])
	}
	if got, ok := fields["price"].(float64); !ok || got != 129.99 {
		t.Fatalf("number was not retained: %#v", fields["price"])
	}
	if fields["project_url"] != "https://example.com/work" || fields["contact"] != "hello@example.com" {
		t.Fatalf("URLs/emails were not normalized: %#v", fields)
	}
	encoded, err := EncodeFieldSnapshot(fields)
	if err != nil {
		t.Fatal(err)
	}
	var jsonFields map[string]any
	if err := json.Unmarshal([]byte(encoded), &jsonFields); err != nil {
		t.Fatal(err)
	}
	if _, ok := jsonFields["featured"].(bool); !ok {
		t.Fatalf("encoded boolean became non-boolean: %s", encoded)
	}
}

func TestValidateFieldsRejectsInvalidAndUnknownValues(t *testing.T) {
	tests := []map[string]any{
		{"client": ""},
		{"client": "Acme", "unknown": "no"},
		{"client": "Acme", "project_url": "ftp://example.com"},
		{"client": "Acme", "contact": "not-an-email"},
		{"client": "Acme", "hero_media": "missing"},
		{"client": "Acme", "format": "video"},
	}
	for _, raw := range tests {
		if _, err := ValidateFields(caseStudyDefinition(), raw, FieldValidationOptions{}); err == nil {
			t.Fatalf("expected validation error for %#v", raw)
		}
	}
}

func TestHistoricalFieldSnapshotSurvivesSchemaRemoval(t *testing.T) {
	historical, err := DecodeFieldSnapshot(`{"client":"Acme","removed_field":"kept"}`)
	if err != nil {
		t.Fatal(err)
	}
	if historical["removed_field"] != "kept" {
		t.Fatalf("historical field lost: %#v", historical)
	}
	current := caseStudyDefinition()
	current.Fields = current.Fields[:1]
	if _, err := ValidateFields(current, map[string]any{"client": "Acme"}, FieldValidationOptions{}); err != nil {
		t.Fatal(err)
	}
}
