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
	Value string    `json:"value"`
	Label string    `json:"label"`
	Type  FieldType `json:"type"`
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
		options = append(options, FieldCatalogOption{Value: "fields." + field.Key, Label: field.Label, Type: field.Type})
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

// ResolveEntryField resolves a ref from a revision snapshot. Missing values are
// represented by ok=false so renderers can omit output without leaking IDs.
func ResolveEntryField(ref FieldRef, title, excerpt, permalink, publishedAt, featuredMedia string, fields map[string]any) (any, bool) {
	if ref.Key != "" {
		value, ok := fields[ref.Key]
		return value, ok
	}
	switch ref.System {
	case "entry.title":
		return title, title != ""
	case "entry.excerpt":
		return excerpt, excerpt != ""
	case "entry.permalink":
		return permalink, permalink != ""
	case "entry.published_at":
		return publishedAt, publishedAt != ""
	case "entry.featured_media":
		return featuredMedia, featuredMedia != ""
	}
	return nil, false
}
