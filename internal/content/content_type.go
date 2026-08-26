package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ContentTypeID is a typed identifier for content types to avoid stringly-typed bugs.
// It is intentionally a string underneath for storage compatibility.
type ContentTypeID string

const (
	ContentTypePage ContentTypeID = "page"
	ContentTypePost ContentTypeID = "post"
)

// Capabilities describes what a content type can do. Each flag replaces a
// previous `if contentType == "post"` branch in generic code.
type Capabilities struct {
	Hierarchical     bool
	HasExcerpt       bool
	HasFeatured      bool
	HasSEO           bool
	Public           bool
	HasArchive       bool
	SupportsSticky   bool
	SupportsComments bool
}

// RoutingPolicy describes how entries of this type are addressed on the public
// site. It is the only place that knows posts live under /{postsBase}/{slug}.
type RoutingPolicy struct {
	// Archive indicates whether this type has a dedicated archive route.
	Archive bool
	// ArchiveContentType is the type for archive routes (e.g. post for /blog)
	ArchiveContentType ContentTypeID
	// BasePath is the effective prefix for custom public entries and archives.
	// Pages deliberately leave it empty; posts retain their settings-backed adapter.
	BasePath string
}

// ContentTypeConfig is the persisted, typed portion of content_types.config_json.
// It intentionally contains only user-editable configuration; core policies for
// Page and Post remain code-owned and are merged by a Catalog on read.
type ContentTypeConfig struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Fields        []FieldDefinition   `json:"fields,omitempty"`
	Features      ContentTypeFeatures `json:"features,omitempty"`
	Routing       ContentTypeRouting  `json:"routing,omitempty"`
}

type ContentTypeFeatures struct {
	Excerpt       bool `json:"excerpt,omitempty"`
	FeaturedMedia bool `json:"featuredMedia,omitempty"`
	SEO           bool `json:"seo,omitempty"`
}

type ContentTypeRouting struct {
	BasePath string `json:"basePath,omitempty"`
	Archive  bool   `json:"archive,omitempty"`
}

const maxContentTypeConfigBytes = 64 << 10

var contentTypeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// DecodeContentTypeConfig parses a bounded, backwards-compatible config JSON.
// Empty and historical {} values mean the default version-one configuration.
func DecodeContentTypeConfig(raw string) (ContentTypeConfig, error) {
	if len(raw) > maxContentTypeConfigBytes {
		return ContentTypeConfig{}, fmt.Errorf("content type config exceeds %d bytes", maxContentTypeConfigBytes)
	}
	config := ContentTypeConfig{SchemaVersion: 1}
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "{}" {
		return config, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ContentTypeConfig{}, fmt.Errorf("invalid content type config: %w", err)
	}
	if config.SchemaVersion == 0 {
		config.SchemaVersion = 1
	}
	if err := ValidateContentTypeConfig(config); err != nil {
		return ContentTypeConfig{}, err
	}
	return config, nil
}

// EncodeContentTypeConfig produces the canonical persisted representation.
func EncodeContentTypeConfig(config ContentTypeConfig) (string, error) {
	if config.SchemaVersion == 0 {
		config.SchemaVersion = 1
	}
	if err := ValidateContentTypeConfig(config); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxContentTypeConfigBytes {
		return "", fmt.Errorf("content type config exceeds %d bytes", maxContentTypeConfigBytes)
	}
	return string(encoded), nil
}

func ValidateContentTypeConfig(config ContentTypeConfig) error {
	if config.SchemaVersion < 1 {
		return fmt.Errorf("content type config schemaVersion must be positive")
	}
	if len(config.Fields) > 64 {
		return fmt.Errorf("content type has too many fields")
	}
	seen := make(map[string]struct{}, len(config.Fields))
	for _, field := range config.Fields {
		if err := validateDefinition(field); err != nil {
			return err
		}
		if _, exists := seen[field.Key]; exists {
			return fmt.Errorf("duplicate field key %q", field.Key)
		}
		seen[field.Key] = struct{}{}
	}
	if config.Routing.BasePath != "" {
		if err := ValidateRouteBase(config.Routing.BasePath); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRouteBase is deliberately storage-neutral so Catalog can validate
// persisted configs without depending on routing (which already depends on content).
func ValidateRouteBase(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" || !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return fmt.Errorf("URL base must be an absolute non-root path without a trailing slash")
	}
	if strings.ContainsAny(path, "?# ") || strings.Contains(path, "//") || strings.Contains(strings.ToLower(path), "%2f") {
		return fmt.Errorf("URL base contains invalid characters")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if segment == "" || strings.HasPrefix(segment, "-") || strings.HasSuffix(segment, "-") {
			return fmt.Errorf("URL base contains an invalid segment")
		}
		for _, ch := range segment {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return fmt.Errorf("URL base may contain only lowercase letters, numbers and hyphens")
			}
		}
	}
	for _, reserved := range []string{"/admin", "/stratum", "/media", "/sitemap.xml", "/robots.txt", "/feed.xml", "/favicon.ico"} {
		if path == reserved || strings.HasPrefix(path, reserved+"/") {
			return fmt.Errorf("URL base conflicts with reserved path %s", reserved)
		}
	}
	return nil
}

// SEOProfile describes the semantic SEO kind for a type.
type SEOProfile struct {
	// SchemaType maps to structured data: "WebPage", "BlogPosting", etc.
	SchemaType string
	// OpenGraphType: "website" or "article"
	OpenGraphType string
}

// TemplatePolicy describes theme resolution for this type.
type TemplatePolicy struct {
	SinglePatterns  []string // e.g. single-post, single
	ArchivePatterns []string // e.g. archive-post, archive
}

// ContentTypeDefinition is the storage-neutral domain view of a content type.
// It is loaded from the content_types table but enriched with the policies
// above so generic code never branches on the type name.
type ContentTypeDefinition struct {
	ID         ContentTypeID
	Name       string
	PluralName string
	// Fields is the current editing schema. Stored revision snapshots remain
	// readable even when a field is later removed from this definition.
	Fields        []FieldDefinition
	SchemaVersion int
	Capabilities  Capabilities
	Routing       RoutingPolicy
	SEO           SEOProfile
	Templates     TemplatePolicy
}

// KnownDefinitions returns built-in definitions. Custom types will be loaded
// from the DB and merged with these defaults.
func KnownDefinitions() map[ContentTypeID]ContentTypeDefinition {
	return map[ContentTypeID]ContentTypeDefinition{
		ContentTypePage: {
			ID: ContentTypePage, Name: "Page", PluralName: "Pages",
			Capabilities: Capabilities{Hierarchical: true, HasExcerpt: false, HasFeatured: true, HasSEO: true, Public: true, HasArchive: false, SupportsSticky: false, SupportsComments: false},
			Routing:      RoutingPolicy{Archive: false},
			SEO:          SEOProfile{SchemaType: "WebPage", OpenGraphType: "website"},
			Templates:    TemplatePolicy{SinglePatterns: []string{"single-page", "single"}, ArchivePatterns: nil},
		},
		ContentTypePost: {
			ID: ContentTypePost, Name: "Post", PluralName: "Posts",
			Capabilities: Capabilities{Hierarchical: false, HasExcerpt: true, HasFeatured: true, HasSEO: true, Public: true, HasArchive: true, SupportsSticky: true, SupportsComments: true},
			Routing:      RoutingPolicy{Archive: true, ArchiveContentType: ContentTypePost},
			SEO:          SEOProfile{SchemaType: "BlogPosting", OpenGraphType: "article"},
			Templates:    TemplatePolicy{SinglePatterns: []string{"single-post", "single"}, ArchivePatterns: []string{"archive-post", "archive"}},
		},
	}
}

// DefinitionFor returns the definition for id, or a generic fallback if unknown.
// The fallback has safe defaults so a future custom type does not crash generic code.
func DefinitionFor(id string) ContentTypeDefinition {
	if def, ok := KnownDefinitions()[ContentTypeID(id)]; ok {
		return def
	}
	return ContentTypeDefinition{
		ID: ContentTypeID(id), Name: id, PluralName: id,
		Capabilities: Capabilities{HasSEO: true, Public: true},
		SEO:          SEOProfile{SchemaType: "WebPage", OpenGraphType: "website"},
		Templates:    TemplatePolicy{SinglePatterns: []string{"single-" + id, "single"}, ArchivePatterns: []string{"archive-" + id, "archive"}},
	}
}

// IsArchived reports whether this type has an archive.
func (d ContentTypeDefinition) IsArchived() bool {
	return d.Capabilities.HasArchive || d.Routing.Archive
}
