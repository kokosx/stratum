package entryops

import (
	"encoding/json"
	"strings"

	"github.com/kokosx/stratum/internal/document"
)

// Optional represents an optional value where Set distinguishes omitted vs explicit.
type Optional[T any] struct {
	Set   bool
	Value T
}

func Opt[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: v} }
func OptClear[T any]() Optional[T] { var zero T; return Optional[T]{Set: true, Value: zero} }

// EntryPatch is the PATCH input for creating/updating entries.
// Nil pointer means omitted (preserve existing); non-nil means explicit change.
// For nullable fields, pointer to "" means clear.
type EntryPatch struct {
	Title             *string `json:"title,omitempty"`
	Slug              *string `json:"slug,omitempty"`
	Excerpt           *string `json:"excerpt,omitempty"`
	SEOTitle          *string `json:"seo_title,omitempty"`
	SEODescription    *string `json:"seo_description,omitempty"`
	CanonicalURL      *string `json:"canonical_url,omitempty"`
	FeaturedMediaID   *string `json:"featured_media_id,omitempty"`
	SocialMediaID     *string `json:"social_media_id,omitempty"`
	RobotsIndex       *bool   `json:"robots_index,omitempty"`
	RobotsFollow      *bool   `json:"robots_follow,omitempty"`
	SchemaMode        *string `json:"schema_mode,omitempty"`
	Document          *document.Document `json:"document,omitempty"`
	DocumentSet       bool               `json:"-"`
	Fields            map[string]any `json:"fields,omitempty"`
	FieldsSet         bool           `json:"-"`
	LayoutTemplateID  *string `json:"layout_template_id,omitempty"`
	LayoutSet         bool    `json:"-"`
	ParentEntryID     *string `json:"parent_entry_id,omitempty"`
	ParentSet         bool    `json:"-"`
	MenuOrder         *int64  `json:"menu_order,omitempty"`
	Visibility        *string `json:"visibility,omitempty"`
	Password          *string `json:"password,omitempty"`
	PasswordSet       bool    `json:"-"`
	Sticky            *bool   `json:"sticky,omitempty"`
	CommentsEnabled   *bool   `json:"comments_enabled,omitempty"`
	ReviewState       *string `json:"review_state,omitempty"`
	TaxonomyValues    map[string][]string `json:"taxonomy_values,omitempty"`
	TaxonomySet       bool                `json:"-"`
}

// UnmarshalJSON custom handling to track presence
func (p *EntryPatch) UnmarshalJSON(data []byte) error {
	type Alias EntryPatch
	aux := &struct {
		*Alias
		DocumentRaw json.RawMessage `json:"document"`
		FieldsRaw   json.RawMessage `json:"fields"`
		LayoutRaw   json.RawMessage `json:"layout_template_id"`
		ParentRaw   json.RawMessage `json:"parent_entry_id"`
		TaxRaw      json.RawMessage `json:"taxonomy_values"`
		PassRaw     json.RawMessage `json:"password"`
	}{Alias: (*Alias)(p)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	// Track document presence
	if len(aux.DocumentRaw) > 0 && string(aux.DocumentRaw) != "null" {
		var doc document.Document
		if err := json.Unmarshal(aux.DocumentRaw, &doc); err != nil {
			return err
		}
		p.Document = &doc
		p.DocumentSet = true
	}
	if len(aux.FieldsRaw) > 0 && string(aux.FieldsRaw) != "null" {
		p.FieldsSet = true
	}
	if len(aux.LayoutRaw) > 0 {
		p.LayoutSet = true
		if string(aux.LayoutRaw) == "null" {
			empty := ""
			p.LayoutTemplateID = &empty
		}
	}
	if len(aux.ParentRaw) > 0 {
		p.ParentSet = true
		if string(aux.ParentRaw) == "null" {
			empty := ""
			p.ParentEntryID = &empty
		}
	}
	if len(aux.TaxRaw) > 0 {
		p.TaxonomySet = true
	}
	if len(aux.PassRaw) > 0 {
		p.PasswordSet = true
		if string(aux.PassRaw) == "null" {
			empty := ""
			p.Password = &empty
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool { return &b }
func int64Ptr(i int64) *int64 { return &i }

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}
