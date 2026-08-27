package content

import (
	"fmt"
	"strings"
)

// FieldRef is the small, closed field binding vocabulary used by dynamic blocks.
// It intentionally cannot evaluate arbitrary expressions or reflect over structs.
type FieldRef struct {
	System string
	Key    string
}

type FieldCatalogOption struct {
	Value   string    `json:"value"`
	Label   string    `json:"label"`
	Type    FieldType `json:"type"`
	Options []string  `json:"options,omitempty"`
}

// FieldCatalog is the server-owned list used by editor pickers. Stable refs are
// values; translated/user-controlled labels are presentation only.
func FieldCatalog(definition ContentTypeDefinition) []FieldCatalogOption {
	options := []FieldCatalogOption{{Value: "entry.title", Label: "Title", Type: FieldText}, {Value: "entry.permalink", Label: "Permalink", Type: FieldURL}, {Value: "entry.published_at", Label: "Published Date", Type: FieldDateTime}}
	if definition.Capabilities.HasExcerpt {
		options = append(options, FieldCatalogOption{Value: "entry.excerpt", Label: "Excerpt", Type: FieldTextarea})
	}
	if definition.Capabilities.HasFeatured {
		options = append(options, FieldCatalogOption{Value: "entry.featured_media", Label: "Featured Image", Type: FieldMedia})
	}
	for _, field := range definition.Fields {
		opt := FieldCatalogOption{Value: "fields." + field.Key, Label: field.Label, Type: field.Type}
		if field.Type == FieldSelect {
			opt.Options = append([]string(nil), field.Validation.Options...)
		}
		options = append(options, opt)
	}
	return options
}

func ParseFieldRef(raw string) (FieldRef, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "fields.") {
		key := strings.TrimPrefix(raw, "fields.")
		if !fieldKeyPattern.MatchString(key) {
			return FieldRef{}, fmt.Errorf("invalid custom field reference %q", raw)
		}
		return FieldRef{Key: key}, nil
	}
	switch raw {
	case "entry.title", "entry.excerpt", "entry.permalink", "entry.published_at", "entry.featured_media":
		return FieldRef{System: raw}, nil
	default:
		return FieldRef{}, fmt.Errorf("invalid field reference %q", raw)
	}
}

// ResolvedField is the typed result of a field lookup. It carries the field's
// type so renderers can enforce type-aware safety (e.g. never emit a media
// ID as text) without leaking IDs.
type ResolvedField struct {
	Value  any
	Type   FieldType
	Exists bool
}

// ResolveEntryField resolves a ref from a revision snapshot. Missing values are
// represented by ok=false so renderers can omit output without leaking IDs.
func ResolveEntryField(ref FieldRef, title, excerpt, permalink, publishedAt, featuredMedia string, fields map[string]any) (any, bool) {
	rf := ResolveEntryFieldTyped(ref, ContentTypeDefinition{}, title, excerpt, permalink, publishedAt, featuredMedia, fields)
	return rf.Value, rf.Exists
}

// ResolveEntryFieldTyped is the type-aware variant. When def is supplied, a
// custom field that no longer exists in the definition (deleted/historical)
// is treated as non-existent so future contexts stop resolving it, while
// historical snapshots remain decodable via DecodeFieldSnapshot.
func ResolveEntryFieldTyped(ref FieldRef, def ContentTypeDefinition, title, excerpt, permalink, publishedAt, featuredMedia string, fields map[string]any) ResolvedField {
	if ref.Key != "" {
		// Custom field: need to know its type from the current definition.
		// If definition is empty (no type info) we still resolve but with unknown type.
		var fieldType FieldType
		foundDef := false
		for _, f := range def.Fields {
			if f.Key == ref.Key {
				fieldType = f.Type
				foundDef = true
				break
			}
		}
		value, ok := fields[ref.Key]
		if !ok {
			return ResolvedField{Exists: false, Type: fieldType}
		}
		// If definition is non-empty but field not found, treat as deleted: do not resolve for future contexts.
		if len(def.Fields) > 0 && !foundDef {
			return ResolvedField{Exists: false, Type: fieldType}
		}
		// If we have no definition (empty), infer type via value? Keep as unknown.
		return ResolvedField{Value: value, Type: fieldType, Exists: true}
	}
	switch ref.System {
	case "entry.title":
		return ResolvedField{Value: title, Type: FieldText, Exists: title != ""}
	case "entry.excerpt":
		return ResolvedField{Value: excerpt, Type: FieldTextarea, Exists: excerpt != ""}
	case "entry.permalink":
		return ResolvedField{Value: permalink, Type: FieldURL, Exists: permalink != ""}
	case "entry.published_at":
		return ResolvedField{Value: publishedAt, Type: FieldDateTime, Exists: publishedAt != ""}
	case "entry.featured_media":
		return ResolvedField{Value: featuredMedia, Type: FieldMedia, Exists: featuredMedia != ""}
	}
	return ResolvedField{}
}

// FieldTypeForRef returns the field type for a ref given a definition, or false if unknown.
// It is used for validation of Collection filters and type-aware UI.
func FieldTypeForRef(def ContentTypeDefinition, ref FieldRef) (FieldType, bool) {
	if ref.Key != "" {
		for _, f := range def.Fields {
			if f.Key == ref.Key {
				return f.Type, true
			}
		}
		return "", false
	}
	switch ref.System {
	case "entry.title":
		return FieldText, true
	case "entry.excerpt":
		return FieldTextarea, true
	case "entry.permalink":
		return FieldURL, true
	case "entry.published_at":
		return FieldDateTime, true
	case "entry.featured_media":
		return FieldMedia, true
	}
	return "", false
}
