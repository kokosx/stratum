package content

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
