package revisions

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Diff is the result of comparing two revisions.
type Diff struct {
	Metadata MetadataDiff `json:"metadata"`
	Fields   []FieldDiff  `json:"fields"`
	Content  ContentDiff  `json:"content"`
	Summary  Summary      `json:"summary"`
	Warnings []string     `json:"warnings,omitempty"`
}

type Summary struct {
	TotalChanges int `json:"totalChanges"`
	Added        int `json:"added"`
	Removed      int `json:"removed"`
	Moved        int `json:"moved"`
	Modified     int `json:"modified"`
}

type MetadataDiff struct {
	Title          *ValueDiff `json:"title,omitempty"`
	Slug           *ValueDiff `json:"slug,omitempty"`
	Excerpt        *ValueDiff `json:"excerpt,omitempty"`
	FeaturedMedia  *ValueDiff `json:"featured_media,omitempty"`
	SocialMedia    *ValueDiff `json:"social_media,omitempty"`
	ParentID       *ValueDiff `json:"parent_id,omitempty"`
	MenuOrder      *ValueDiff `json:"menu_order,omitempty"`
	LayoutTemplate *ValueDiff `json:"layout_template,omitempty"`
	Visibility     *ValueDiff `json:"visibility,omitempty"`
	Sticky         *ValueDiff `json:"sticky,omitempty"`
	SEO            *ValueDiff `json:"seo,omitempty"`
}

type ValueDiff struct {
	Old     string `json:"old"`
	New     string `json:"new"`
	Changed bool   `json:"changed"`
}

type FieldDiff struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Old     any    `json:"old"`
	New     any    `json:"new"`
	Changed bool   `json:"changed"`
}

type ContentDiff struct {
	Added     []NodeChange `json:"added"`
	Removed   []NodeChange `json:"removed"`
	Moved     []NodeChange `json:"moved"`
	Modified  []NodeChange `json:"modified"`
	Unchanged []NodeChange `json:"unchanged"`
}

type NodeChange struct {
	ID           string     `json:"id"`
	Block        string     `json:"block"`
	Version      int        `json:"version"`
	ParentA      string     `json:"parentA,omitempty"`
	ParentB      string     `json:"parentB,omitempty"`
	IndexA       int        `json:"indexA"`
	IndexB       int        `json:"indexB"`
	PropDiffs    []PropDiff `json:"propDiffs,omitempty"`
	SettingDiffs []PropDiff `json:"settingDiffs,omitempty"`
	TypeChanged  bool       `json:"typeChanged"`
	Summary      string     `json:"summary"`
}

type PropDiff struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Old   any    `json:"old"`
	New   any    `json:"new"`
}

type CompareOptions struct {
	ContentTypeID string
	FieldSchemas  map[string]FieldSchema // key -> label
	BlockRegistry *blocks.Registry
}

// FieldSchema is minimal for label resolution
type FieldSchema struct {
	Label string
	Type  string
}

// CompareRevisions compares two EntryRevision rows.
func CompareRevisions(a, b db.EntryRevision, opts CompareOptions) (*Diff, error) {
	diff := &Diff{}

	// Metadata
	diff.Metadata = compareMetadata(a, b)
	// Fields
	fields, err := compareFields(a, b, opts)
	if err != nil {
		return nil, fmt.Errorf("comparison unavailable: fields decode failed: %w", err)
	}
	diff.Fields = fields
	// Content
	contentDiff, err := compareContent(a, b, opts)
	if err != nil {
		return nil, fmt.Errorf("comparison unavailable: %w", err)
	}
	diff.Content = *contentDiff

	// Summary
	diff.Summary = Summary{
		Added:    len(contentDiff.Added),
		Removed:  len(contentDiff.Removed),
		Moved:    len(contentDiff.Moved),
		Modified: len(contentDiff.Modified),
	}
	diff.Summary.TotalChanges = diff.Summary.Added + diff.Summary.Removed + diff.Summary.Moved + diff.Summary.Modified
	// Count metadata and fields changes via helper (includes all metadata fields)
	diff.Summary.TotalChanges += countMetadataChanges(diff.Metadata)
	for _, f := range diff.Fields {
		if f.Changed {
			diff.Summary.TotalChanges++
		}
	}
	// Warnings for missing media
	diff.Warnings = collectWarnings(a, b, *contentDiff)

	// Deterministic sort
	sortFieldDiffs(diff.Fields, opts)
	sortContentDiff(diff)

	return diff, nil
}

func countMetadataChanges(md MetadataDiff) int {
	c := 0
	if md.Title != nil && md.Title.Changed {
		c++
	}
	if md.Slug != nil && md.Slug.Changed {
		c++
	}
	if md.Excerpt != nil && md.Excerpt.Changed {
		c++
	}
	if md.FeaturedMedia != nil && md.FeaturedMedia.Changed {
		c++
	}
	if md.SocialMedia != nil && md.SocialMedia.Changed {
		c++
	}
	if md.ParentID != nil && md.ParentID.Changed {
		c++
	}
	if md.MenuOrder != nil && md.MenuOrder.Changed {
		c++
	}
	if md.LayoutTemplate != nil && md.LayoutTemplate.Changed {
		c++
	}
	if md.Visibility != nil && md.Visibility.Changed {
		c++
	}
	if md.Sticky != nil && md.Sticky.Changed {
		c++
	}
	if md.SEO != nil && md.SEO.Changed {
		c++
	}
	return c
}

func compareMetadata(a, b db.EntryRevision) MetadataDiff {
	md := MetadataDiff{}
	if a.Title != b.Title {
		md.Title = &ValueDiff{Old: a.Title, New: b.Title, Changed: true}
	}
	if a.Slug != b.Slug {
		md.Slug = &ValueDiff{Old: a.Slug, New: b.Slug, Changed: true}
	}
	exA := stringValue(a.Excerpt)
	exB := stringValue(b.Excerpt)
	if exA != exB {
		md.Excerpt = &ValueDiff{Old: exA, New: exB, Changed: true}
	}
	fA := stringValue(a.FeaturedMediaID)
	fB := stringValue(b.FeaturedMediaID)
	if fA != fB {
		md.FeaturedMedia = &ValueDiff{Old: fA, New: fB, Changed: true}
	}
	sA := stringValue(a.SocialMediaID)
	sB := stringValue(b.SocialMediaID)
	if sA != sB {
		md.SocialMedia = &ValueDiff{Old: sA, New: sB, Changed: true}
	}
	pA := stringValue(a.ParentEntryID)
	pB := stringValue(b.ParentEntryID)
	if pA != pB {
		md.ParentID = &ValueDiff{Old: pA, New: pB, Changed: true}
	}
	if a.MenuOrder != b.MenuOrder {
		md.MenuOrder = &ValueDiff{Old: fmt.Sprintf("%d", a.MenuOrder), New: fmt.Sprintf("%d", b.MenuOrder), Changed: true}
	}
	ltA := stringValue(a.LayoutTemplateID)
	ltB := stringValue(b.LayoutTemplateID)
	if ltA != ltB {
		md.LayoutTemplate = &ValueDiff{Old: ltA, New: ltB, Changed: true}
	}
	visA := fmt.Sprintf("%v", a.Visibility)
	visB := fmt.Sprintf("%v", b.Visibility)
	if visA != visB {
		md.Visibility = &ValueDiff{Old: visA, New: visB, Changed: true}
	}
	if a.Sticky != b.Sticky {
		md.Sticky = &ValueDiff{Old: fmt.Sprintf("%d", a.Sticky), New: fmt.Sprintf("%d", b.Sticky), Changed: true}
	}
	// SEO: compare SeoTitle, SeoDescription, CanonicalUrl, Robots, SchemaMode
	seoA := strings.Join([]string{
		stringValue(a.SeoTitle),
		stringValue(a.SeoDescription),
		stringValue(a.CanonicalUrl),
		fmt.Sprintf("%v", a.SeoRobotsIndex),
		fmt.Sprintf("%v", a.SeoRobotsFollow),
		a.SchemaMode,
	}, "|")
	seoB := strings.Join([]string{
		stringValue(b.SeoTitle),
		stringValue(b.SeoDescription),
		stringValue(b.CanonicalUrl),
		fmt.Sprintf("%v", b.SeoRobotsIndex),
		fmt.Sprintf("%v", b.SeoRobotsFollow),
		b.SchemaMode,
	}, "|")
	if seoA != seoB {
		md.SEO = &ValueDiff{Old: seoA, New: seoB, Changed: true}
	}
	return md
}

func compareFields(a, b db.EntryRevision, opts CompareOptions) ([]FieldDiff, error) {
	var fieldsA, fieldsB map[string]any
	rawA := stringValueRaw(a.FieldsJson)
	rawB := stringValueRaw(b.FieldsJson)
	if strings.TrimSpace(rawA) != "" {
		if err := json.Unmarshal([]byte(rawA), &fieldsA); err != nil {
			return nil, fmt.Errorf("fields A decode: %w", err)
		}
	}
	if strings.TrimSpace(rawB) != "" {
		if err := json.Unmarshal([]byte(rawB), &fieldsB); err != nil {
			return nil, fmt.Errorf("fields B decode: %w", err)
		}
	}
	if fieldsA == nil {
		fieldsA = map[string]any{}
	}
	if fieldsB == nil {
		fieldsB = map[string]any{}
	}
	keys := make(map[string]bool)
	for k := range fieldsA {
		keys[k] = true
	}
	for k := range fieldsB {
		keys[k] = true
	}
	var diffs []FieldDiff
	for k := range keys {
		valA, okA := fieldsA[k]
		valB, okB := fieldsB[k]
		changed := !equalJSON(valA, valB) || okA != okB
		label := k
		if schema, ok := opts.FieldSchemas[k]; ok && schema.Label != "" {
			label = schema.Label
		} else {
			label = prettifyKey(k)
		}
		diffs = append(diffs, FieldDiff{
			Key: k, Label: label, Old: valA, New: valB, Changed: changed,
		})
	}
	return diffs, nil
}

func sortFieldDiffs(diffs []FieldDiff, opts CompareOptions) {
	// Schema order if available, then alphabetical
	order := map[string]int{}
	i := 0
	for k := range opts.FieldSchemas {
		order[k] = i
		i++
	}
	sort.Slice(diffs, func(a, b int) bool {
		oa, hasA := order[diffs[a].Key]
		ob, hasB := order[diffs[b].Key]
		if hasA && hasB {
			return oa < ob
		}
		if hasA != hasB {
			return hasA
		}
		return diffs[a].Key < diffs[b].Key
	})
}

func sortContentDiff(diff *Diff) {
	sort.Slice(diff.Content.Added, func(i, j int) bool { return diff.Content.Added[i].ID < diff.Content.Added[j].ID })
	sort.Slice(diff.Content.Removed, func(i, j int) bool { return diff.Content.Removed[i].ID < diff.Content.Removed[j].ID })
	sort.Slice(diff.Content.Moved, func(i, j int) bool { return diff.Content.Moved[i].ID < diff.Content.Moved[j].ID })
	sort.Slice(diff.Content.Modified, func(i, j int) bool { return diff.Content.Modified[i].ID < diff.Content.Modified[j].ID })
	sort.Slice(diff.Content.Unchanged, func(i, j int) bool { return diff.Content.Unchanged[i].ID < diff.Content.Unchanged[j].ID })
	// Also sort prop diffs deterministically
	for i := range diff.Content.Added {
		sortPropDiffs(diff.Content.Added[i].PropDiffs)
		sortPropDiffs(diff.Content.Added[i].SettingDiffs)
	}
	for i := range diff.Content.Removed {
		sortPropDiffs(diff.Content.Removed[i].PropDiffs)
		sortPropDiffs(diff.Content.Removed[i].SettingDiffs)
	}
	for i := range diff.Content.Moved {
		sortPropDiffs(diff.Content.Moved[i].PropDiffs)
		sortPropDiffs(diff.Content.Moved[i].SettingDiffs)
	}
	for i := range diff.Content.Modified {
		sortPropDiffs(diff.Content.Modified[i].PropDiffs)
		sortPropDiffs(diff.Content.Modified[i].SettingDiffs)
	}
}

func sortPropDiffs(d []PropDiff) {
	sort.Slice(d, func(i, j int) bool { return d[i].Key < d[j].Key })
}

func compareContent(a, b db.EntryRevision, opts CompareOptions) (*ContentDiff, error) {
	var docA, docB *document.Document
	var err error
	if strings.TrimSpace(a.DocumentJson) == "" {
		docA = &document.Document{Version: 1, Nodes: []document.Node{}}
	} else {
		docA, err = document.Decode([]byte(a.DocumentJson))
		if err != nil {
			return nil, fmt.Errorf("invalid document A: %w", err)
		}
	}
	if strings.TrimSpace(b.DocumentJson) == "" {
		docB = &document.Document{Version: 1, Nodes: []document.Node{}}
	} else {
		docB, err = document.Decode([]byte(b.DocumentJson))
		if err != nil {
			return nil, fmt.Errorf("invalid document B: %w", err)
		}
	}
	// Flatten both
	mapA := flattenDocument(docA)
	mapB := flattenDocument(docB)

	diff := &ContentDiff{}

	// Track all IDs - deterministic iteration via sorted keys
	allIDs := make(map[string]bool)
	for id := range mapA {
		allIDs[id] = true
	}
	for id := range mapB {
		allIDs[id] = true
	}
	// Collect sorted IDs for deterministic order
	ids := make([]string, 0, len(allIDs))
	for id := range allIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		infoA, okA := mapA[id]
		infoB, okB := mapB[id]
		if !okA && okB {
			// Added
			diff.Added = append(diff.Added, NodeChange{
				ID: id, Block: infoB.Node.Block, Version: infoB.Node.Version,
				ParentB: infoB.ParentID, IndexB: infoB.Index,
				Summary: fmt.Sprintf("%s added", blockLabel(infoB.Node.Block, opts.BlockRegistry)),
			})
		} else if okA && !okB {
			diff.Removed = append(diff.Removed, NodeChange{
				ID: id, Block: infoA.Node.Block, Version: infoA.Node.Version,
				ParentA: infoA.ParentID, IndexA: infoA.Index,
				Summary: fmt.Sprintf("%s removed", blockLabel(infoA.Node.Block, opts.BlockRegistry)),
			})
		} else if okA && okB {
			// Exists in both - check moved, modified, unchanged
			moved := infoA.ParentID != infoB.ParentID || infoA.Index != infoB.Index
			modified, propDiffs, settingDiffs, typeChanged := diffNode(infoA.Node, infoB.Node, opts.BlockRegistry)
			if moved && !modified && !typeChanged {
				diff.Moved = append(diff.Moved, NodeChange{
					ID: id, Block: infoB.Node.Block, Version: infoB.Node.Version,
					ParentA: infoA.ParentID, ParentB: infoB.ParentID, IndexA: infoA.Index, IndexB: infoB.Index,
					Summary: fmt.Sprintf("%s moved", blockLabel(infoB.Node.Block, opts.BlockRegistry)),
				})
			} else if modified || typeChanged || moved {
				// If both moved and modified, classify as modified (with move info)
				nc := NodeChange{
					ID: id, Block: infoB.Node.Block, Version: infoB.Node.Version,
					ParentA: infoA.ParentID, ParentB: infoB.ParentID, IndexA: infoA.Index, IndexB: infoB.Index,
					PropDiffs: propDiffs, SettingDiffs: settingDiffs, TypeChanged: typeChanged,
				}
				if typeChanged {
					nc.Summary = fmt.Sprintf("%s type changed %s → %s", blockLabel(infoA.Node.Block, opts.BlockRegistry), infoA.Node.Block, infoB.Node.Block)
				} else if moved && modified {
					nc.Summary = fmt.Sprintf("%s moved and modified", blockLabel(infoB.Node.Block, opts.BlockRegistry))
				} else if moved {
					nc.Summary = fmt.Sprintf("%s moved", blockLabel(infoB.Node.Block, opts.BlockRegistry))
				} else {
					nc.Summary = fmt.Sprintf("%s modified", blockLabel(infoB.Node.Block, opts.BlockRegistry))
				}
				diff.Modified = append(diff.Modified, nc)
			} else {
				diff.Unchanged = append(diff.Unchanged, NodeChange{
					ID: id, Block: infoB.Node.Block, Version: infoB.Node.Version,
					ParentA: infoA.ParentID, ParentB: infoB.ParentID, IndexA: infoA.Index, IndexB: infoB.Index,
					Summary: fmt.Sprintf("%s unchanged", blockLabel(infoB.Node.Block, opts.BlockRegistry)),
				})
			}
		}
	}

	return diff, nil
}

type nodeInfo struct {
	Node     document.Node
	ParentID string
	Index    int
}

func flattenDocument(doc *document.Document) map[string]nodeInfo {
	m := make(map[string]nodeInfo)
	var walk func(nodes []document.Node, parentID string)
	walk = func(nodes []document.Node, parentID string) {
		for idx, n := range nodes {
			m[n.ID] = nodeInfo{Node: n, ParentID: parentID, Index: idx}
			if len(n.Children) > 0 {
				walk(n.Children, n.ID)
			}
		}
	}
	if doc != nil {
		walk(doc.Nodes, "")
	}
	return m
}

func diffNode(a, b document.Node, registry *blocks.Registry) (modified bool, propDiffs []PropDiff, settingDiffs []PropDiff, typeChanged bool) {
	if a.Block != b.Block || a.Version != b.Version {
		typeChanged = true
		modified = true
	}
	// Compare props
	propsA := decodeMap(a.Props)
	propsB := decodeMap(b.Props)
	propDiffs = diffMaps(propsA, propsB, a.Block, registry, "props")
	if len(propDiffs) > 0 {
		modified = true
	}
	settingsA := decodeMap(a.Settings)
	settingsB := decodeMap(b.Settings)
	settingDiffs = diffMaps(settingsA, settingsB, a.Block, registry, "settings")
	if len(settingDiffs) > 0 {
		modified = true
	}
	return
}

func decodeMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

func diffMaps(a, b map[string]any, block string, registry *blocks.Registry, kind string) []PropDiff {
	keys := make(map[string]bool)
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	// Deterministic order
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	var diffs []PropDiff
	for _, k := range sortedKeys {
		valA, okA := a[k]
		valB, okB := b[k]
		if !okA || !okB || !equalJSON(valA, valB) {
			label := prettifyKey(k)
			if blockLabelMap, ok := blockFieldLabels[block]; ok {
				if l, ok := blockLabelMap[k]; ok {
					label = l
				}
			}
			diffs = append(diffs, PropDiff{Key: k, Label: label, Old: valA, New: valB})
		}
	}
	return diffs
}

func equalJSON(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func prettifyKey(k string) string {
	parts := strings.Split(k, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.Title(p)
	}
	return strings.Join(parts, " ")
}

func blockLabel(block string, registry *blocks.Registry) string {
	parts := strings.Split(block, "/")
	if len(parts) == 2 {
		return prettifyKey(parts[1])
	}
	return block
}

func stringValue(ns interface{}) string {
	switch v := ns.(type) {
	case string:
		return v
	case sql.NullString:
		if v.Valid {
			return v.String
		}
		return ""
	default:
		return ""
	}
}

func stringValueRaw(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case json.RawMessage:
		return string(val)
	case []byte:
		return string(val)
	default:
		if val == nil {
			return ""
		}
		b, _ := json.Marshal(val)
		return string(b)
	}
}

func collectWarnings(a, b db.EntryRevision, contentDiff ContentDiff) []string {
	var warnings []string
	return warnings
}

// Known field labels for common blocks
var blockFieldLabels = map[string]map[string]string{
	"core/heading": {
		"text":  "Text",
		"level": "Level",
	},
	"core/text": {
		"text": "Text",
	},
	"core/button": {
		"label": "Label",
		"url":   "URL",
		"href":  "URL",
	},
	"core/image": {
		"mediaId": "Media",
		"alt":     "Alt",
		"caption": "Caption",
	},
}
