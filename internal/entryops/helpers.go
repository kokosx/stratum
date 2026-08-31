package entryops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/slug"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/taxonomy"
)

var entrySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var reservedSlugs = map[string]bool{
	"admin":       true,
	"stratum":     true,
	"sitemap.xml": true,
	"robots.txt":  true,
	"sitemap-xml": true,
	"robots-txt":  true,
}

func slugify(title string) string {
	canonical := slug.Slugify(title)
	if canonical == "" {
		return "item"
	}
	return canonical
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "UNIQUE")
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func stringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func normalizeSchemaMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "disabled", "webpage", "aboutpage", "contactpage":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func validCanonicalURL(value string) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return true
	}
	return strings.HasPrefix(value, "/")
}

func taxonomySlugify(s string) string {
	canonical := slug.Slugify(s)
	if canonical == "" {
		return "tag"
	}
	return canonical
}

func validateHierarchyInput(ctx context.Context, q *db.Queries, contentType, entryID, parentEntryID string, menuOrder int64, isPostsPage bool, postsPageID sql.NullString) error {
	def := content.DefinitionFor(contentType)
	if catalogDef, err := content.NewCatalog(q).GetDefinition(ctx, contentType); err == nil {
		def = catalogDef
	}
	if !def.Capabilities.Hierarchical {
		if parentEntryID != "" {
			return errors.New("this content type does not support a parent")
		}
		return nil
	}
	if menuOrder < 0 {
		return errors.New("order must be a non-negative integer")
	}
	if isPostsPage && parentEntryID != "" {
		return errors.New("the Posts Page cannot have a parent")
	}
	if parentEntryID != "" && postsPageID.Valid && parentEntryID == postsPageID.String {
		return errors.New("the Posts Page cannot be selected as a parent")
	}
	rows, err := q.ListLatestHierarchyForContentType(ctx, contentType)
	if err != nil {
		return err
	}
	nodes := make([]content.HierarchyNode, 0, len(rows))
	parentFound := parentEntryID == ""
	for _, row := range rows {
		parent := ""
		if row.ParentEntryID.Valid {
			parent = row.ParentEntryID.String
		}
		if row.EntryID == entryID {
			parent = parentEntryID
		}
		if row.EntryID == parentEntryID {
			if row.Status == "trash" {
				return errors.New("the selected parent is in Trash")
			}
			parentFound = true
		}
		nodes = append(nodes, content.HierarchyNode{EntryID: row.EntryID, Slug: row.Slug, ParentEntryID: parent, MenuOrder: row.MenuOrder, Title: row.Title})
	}
	if !parentFound {
		return errors.New("the selected parent does not exist in this content type")
	}
	_, err = content.NewHierarchy(nodes)
	return err
}

func allocateUniqueSlug(ctx context.Context, qtx *db.Queries, contentType, baseSlug, entryID string) (string, error) {
	baseSlug = strings.TrimSpace(baseSlug)
	if baseSlug == "" {
		baseSlug = "item"
	}
	if len(baseSlug) > 100 {
		baseSlug = baseSlug[:100]
	}
	for i := 0; i < 100; i++ {
		candidate := baseSlug
		if i > 0 {
			suffix := fmt.Sprintf("-%d", i+1)
			if len(baseSlug)+len(suffix) > 100 {
				candidate = baseSlug[:100-len(suffix)] + suffix
			} else {
				candidate = baseSlug + suffix
			}
		}
		existing, err := qtx.GetFlatEntryBySlug(ctx, db.GetFlatEntryBySlugParams{ContentTypeID: contentType, Slug: candidate})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return candidate, nil
			}
			return "", err
		}
		if existing.ID == entryID {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate unique slug for %q", baseSlug)
}

func taxonomyTermIDsForInput(ctx context.Context, q *db.Queries, dbConn *sql.DB, contentType string, values map[string][]string) ([]string, error) {
	taxRows, err := q.ListTaxonomiesByContentType(ctx, contentType)
	if err != nil {
		return nil, nil
	}
	// Need database for taxonomy service creation of missing tags
	svc := taxonomy.New(dbConn, q)
	var out []string
	seen := map[string]bool{}
	for _, tax := range taxRows {
		key := "taxonomy_" + tax.ID
		if tax.Hierarchical != 0 {
			ids := values[key]
			if len(ids) == 0 {
				if v := strings.Join(values[key], ","); v != "" {
					ids = strings.Split(v, ",")
				}
			}
			for _, id := range ids {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if seen[id] {
					continue
				}
				t, err := q.GetTerm(ctx, id)
				if err != nil {
					return nil, fmt.Errorf("invalid term %s", id)
				}
				if t.TaxonomyID != tax.ID {
					return nil, fmt.Errorf("term %s does not belong to %s", id, tax.ID)
				}
				seen[id] = true
				out = append(out, id)
			}
		} else {
			raw := strings.TrimSpace(strings.Join(values[key], ","))
			if raw == "" {
				continue
			}
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				name := strings.TrimSpace(p)
				if name == "" {
					continue
				}
				lower := strings.ToLower(name)
				if seen[lower+"_tag"] {
					continue
				}
				slugVal := taxonomySlugify(name)
				if t, err := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: tax.ID, Slug: slugVal}); err == nil {
					if !seen[t.ID] {
						seen[t.ID] = true
						seen[lower+"_tag"] = true
						out = append(out, t.ID)
					}
					continue
				}
				created, err := svc.CreateTermWithQueries(ctx, q, tax.ID, name, slugVal, "", "")
				if err != nil {
					if t, err2 := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: tax.ID, Slug: slugVal}); err2 == nil {
						if !seen[t.ID] {
							seen[t.ID] = true
							out = append(out, t.ID)
						}
						continue
					}
					return nil, err
				}
				if !seen[created.ID] {
					seen[created.ID] = true
					seen[lower+"_tag"] = true
					out = append(out, created.ID)
				}
			}
		}
	}
	return out, nil
}

func fieldValues(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}
