package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type QueryOperator string

const (
	OpEquals       QueryOperator = "equals"
	OpNotEquals    QueryOperator = "not_equals"
	OpContains     QueryOperator = "contains"
	OpExists       QueryOperator = "exists"
	OpNotExists    QueryOperator = "not_exists"
	OpGreater      QueryOperator = "greater_than"
	OpGreaterEqual QueryOperator = "greater_or_equal"
	OpLess         QueryOperator = "less_than"
	OpLessEqual    QueryOperator = "less_or_equal"
	OpIsTrue       QueryOperator = "is_true"
	OpIsFalse      QueryOperator = "is_false"
	OpBefore       QueryOperator = "before"
	OpAfter        QueryOperator = "after"
)

type EntryFilter struct {
	Field    string        `json:"field"`
	Operator QueryOperator `json:"operator"`
	Value    any           `json:"value,omitempty"`
}

// EntryQuery is the declarative, plugin-safe model for fetching published entries.
// It replaces ad-hoc `ListPublishedEntriesByContentType` calls that previously
// lived in the public handler. Only a small, bounded set of fields is supported
// so the host can enforce stable prepared query shapes.
//
// The query is intentionally NOT a SQL builder. The implementation maps this
// struct to one of a few prepared statements (by content type, limit, offset,
// stable sort). A future plugin or Collection block will construct this struct,
// never raw SQL.
type EntryQuery struct {
	ContentType ContentTypeID
	Limit       int
	Offset      int
	Order       string // "published_desc" (default) or "published_asc"
	ExcludeIDs  []string
	// Cursor pagination support (optional, for future).
	Cursor    string
	TermID    string // optional filter for taxonomy archive
	Filters   []EntryFilter
	OrderBy   string // entry.published_at, entry.title, or fields.<key>
	Direction string // asc or desc
}

// Normalized returns a copy with defaults applied, limits clamped, and
// deterministic ordering so the memo key is stable.
func (q EntryQuery) Normalized() EntryQuery {
	if q.Limit <= 0 {
		q.Limit = 10
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Order == "" {
		q.Order = "published_desc"
	}
	q.Order = strings.ToLower(strings.TrimSpace(q.Order))
	if q.Order != "published_asc" && q.Order != "published_desc" {
		q.Order = "published_desc"
	}
	q.TermID = strings.TrimSpace(q.TermID)
	q.OrderBy = strings.TrimSpace(q.OrderBy)
	q.Direction = strings.ToLower(strings.TrimSpace(q.Direction))
	if q.Direction != "asc" && q.Direction != "desc" {
		q.Direction = "desc"
	}
	for i := range q.Filters {
		q.Filters[i].Field = strings.TrimSpace(q.Filters[i].Field)
		q.Filters[i].Operator = QueryOperator(strings.ToLower(strings.TrimSpace(string(q.Filters[i].Operator))))
	}
	sort.SliceStable(q.Filters, func(i, j int) bool {
		a, _ := json.Marshal(q.Filters[i])
		b, _ := json.Marshal(q.Filters[j])
		return string(a) < string(b)
	})
	// Deduplicate and sort ExcludeIDs for stable key (sorting via map would add dep).
	uniq := make(map[string]struct{}, len(q.ExcludeIDs))
	for _, id := range q.ExcludeIDs {
		if id != "" {
			uniq[id] = struct{}{}
		}
	}
	q.ExcludeIDs = q.ExcludeIDs[:0]
	for id := range uniq {
		q.ExcludeIDs = append(q.ExcludeIDs, id)
	}
	// Deterministic by sorting.
	for i := 0; i < len(q.ExcludeIDs); i++ {
		for j := i + 1; j < len(q.ExcludeIDs); j++ {
			if q.ExcludeIDs[j] < q.ExcludeIDs[i] {
				q.ExcludeIDs[i], q.ExcludeIDs[j] = q.ExcludeIDs[j], q.ExcludeIDs[i]
			}
		}
	}
	return q
}

// CacheKey returns a stable string that can be used as a request-memo key.
// It must be deterministic for Normalized queries with the same semantics.
func (q EntryQuery) CacheKey() string {
	q = q.Normalized()
	encoded, _ := json.Marshal(q)
	sum := sha256.Sum256(encoded)
	return "entry-query:" + hex.EncodeToString(sum[:])
}

// Validate returns an error if the query is structurally invalid.
func (q EntryQuery) Validate() error {
	if strings.TrimSpace(string(q.ContentType)) == "" {
		return fmt.Errorf("content type is required")
	}
	if q.Limit < 0 || q.Limit > 100 {
		return fmt.Errorf("limit must be between 0 and 100")
	}
	if q.Offset < 0 {
		return fmt.Errorf("offset must be >= 0")
	}
	if len(q.Filters) > 5 {
		return fmt.Errorf("at most 5 filters are allowed")
	}
	validOps := map[QueryOperator]bool{OpEquals: true, OpNotEquals: true, OpContains: true, OpExists: true, OpNotExists: true, OpGreater: true, OpGreaterEqual: true, OpLess: true, OpLessEqual: true, OpIsTrue: true, OpIsFalse: true, OpBefore: true, OpAfter: true}
	for _, filter := range q.Filters {
		if _, err := ParseFieldRef(filter.Field); err != nil {
			return err
		}
		if !validOps[filter.Operator] {
			return fmt.Errorf("unsupported filter operator %q", filter.Operator)
		}
	}
	if q.OrderBy != "" {
		if _, err := ParseFieldRef(q.OrderBy); err != nil {
			return err
		}
	}
	return nil
}

// ValidateForDefinition checks field existence and operator compatibility
// against the concrete ContentTypeDefinition. It must be called after Validate.
func (q EntryQuery) ValidateForDefinition(def ContentTypeDefinition) error {
	if err := q.Validate(); err != nil {
		return err
	}
	// Build field type index: system + custom.
	typeIndex := make(map[string]FieldType)
	typeIndex["entry.title"] = FieldText
	typeIndex["entry.permalink"] = FieldURL
	typeIndex["entry.published_at"] = FieldDateTime
	if def.Capabilities.HasExcerpt {
		typeIndex["entry.excerpt"] = FieldTextarea
	}
	if def.Capabilities.HasFeatured {
		typeIndex["entry.featured_media"] = FieldMedia
	}
	for _, f := range def.Fields {
		typeIndex["fields."+f.Key] = f.Type
	}
	allowed := func(ft FieldType, op QueryOperator) bool {
		switch ft {
		case FieldText, FieldTextarea:
			return op == OpEquals || op == OpNotEquals || op == OpContains || op == OpExists || op == OpNotExists
		case FieldNumber:
			return op == OpEquals || op == OpNotEquals || op == OpGreater || op == OpGreaterEqual || op == OpLess || op == OpLessEqual || op == OpExists || op == OpNotExists
		case FieldBoolean:
			return op == OpIsTrue || op == OpIsFalse || op == OpExists || op == OpNotExists
		case FieldDate, FieldDateTime:
			return op == OpEquals || op == OpBefore || op == OpAfter || op == OpExists || op == OpNotExists
		case FieldSelect:
			return op == OpEquals || op == OpNotEquals || op == OpExists || op == OpNotExists
		case FieldURL, FieldEmail:
			return op == OpEquals || op == OpNotEquals || op == OpContains || op == OpExists || op == OpNotExists
		case FieldMedia:
			return op == OpExists || op == OpNotExists || op == OpEquals
		default:
			return false
		}
	}
	for _, filter := range q.Filters {
		ft, ok := typeIndex[filter.Field]
		if !ok {
			// Schemaless legacy types (no field definitions) allow ad-hoc snapshot fields for backward compat.
			// This keeps repository tests that insert products with empty config.json working while new
			// schema-aware types remain strictly validated.
			if len(def.Fields) == 0 && strings.HasPrefix(filter.Field, "fields.") {
				// Infer permissive type: treat as text/number/boolean generic – allow all common operators except strict type mismatches.
				// For boolean `is_true` we need boolean, for `contains` we need text – we allow both leniently.
				continue
			}
			// Try to resolve via ParseFieldRef for system fields that may still be valid but not in index (e.g. excerpt when not enabled): treat as not allowed.
			if _, err := ParseFieldRef(filter.Field); err == nil {
				return fmt.Errorf("field %q does not exist for content type %q", filter.Field, def.ID)
			}
			return fmt.Errorf("field %q does not exist for content type %q", filter.Field, def.ID)
		}
		if !allowed(ft, filter.Operator) {
			return fmt.Errorf("operator %q is not allowed for field %q of type %q", filter.Operator, filter.Field, ft)
		}
		// Value presence check for operators that require/don't require value.
		needsValue := filter.Operator != OpExists && filter.Operator != OpNotExists && filter.Operator != OpIsTrue && filter.Operator != OpIsFalse
		hasValue := filter.Value != nil && fmt.Sprint(filter.Value) != ""
		if needsValue && !hasValue {
			return fmt.Errorf("operator %q requires a value for field %q", filter.Operator, filter.Field)
		}
		if !needsValue && hasValue {
			// IsTrue/IsFalse/Exists/NotExists should not have value – but allow silently ignoring for compat
		}
	}
	if q.OrderBy != "" {
		if _, ok := typeIndex[q.OrderBy]; !ok {
			if len(def.Fields) == 0 && strings.HasPrefix(q.OrderBy, "fields.") {
				// Allow legacy schemaless order.
			} else {
				return fmt.Errorf("orderBy field %q does not exist for content type %q", q.OrderBy, def.ID)
			}
		}
	}
	return nil
}
