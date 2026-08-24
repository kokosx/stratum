package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
)

// Schema is the supported Stratum Block Schema v1 contract. It intentionally
// models only the subset understood by both the server and the editor.
type Schema struct {
	SchemaVersion int            `json:"schemaVersion"`
	Props         ValueSchema    `json:"props"`
	Settings      ValueSchema    `json:"settings"`
	Children      ChildrenSchema `json:"children"`
	Editor        EditorSchema   `json:"editor"`
}

type ValueSchema struct {
	Type       string                 `json:"type"`
	Required   []string               `json:"required,omitempty"`
	Properties map[string]ValueSchema `json:"properties,omitempty"`
	Items      *ValueSchema           `json:"items,omitempty"`
	Enum       []any                  `json:"enum,omitempty"`
	Default    any                    `json:"default"`
	Minimum    *float64               `json:"minimum,omitempty"`
	Maximum    *float64               `json:"maximum,omitempty"`
	MinLength  *int                   `json:"minLength,omitempty"`
	MaxLength  *int                   `json:"maxLength,omitempty"`
	Pattern    string                 `json:"pattern,omitempty"`
	pattern    *regexp.Regexp
	hasDefault bool
}

type ChildrenSchema struct {
	Mode   string   `json:"mode"`
	Blocks []string `json:"blocks,omitempty"`
	Min    *int     `json:"min,omitempty"`
	Max    *int     `json:"max,omitempty"`
}

type EditorSchema struct {
	Category         string                 `json:"category,omitempty"`
	Icon             string                 `json:"icon,omitempty"`
	Fields           map[string]EditorField `json:"fields,omitempty"`
	Contexts         []string               `json:"contexts,omitempty"`
	Hidden           bool                   `json:"hidden,omitempty"`
	SummaryFields    []string               `json:"summaryFields,omitempty"`
	LCPCandidate     bool                   `json:"lcpCandidate,omitempty"`
	RequiresFeatured bool                   `json:"requiresFeatured,omitempty"`
}

type EditorField struct {
	Label   string `json:"label,omitempty"`
	Control string `json:"control,omitempty"`
	Group   string `json:"group,omitempty"`
}

var supportedControls = map[string]bool{
	"text": true, "textarea": true, "number": true, "checkbox": true,
	"select": true, "segmented": true, "radio": true, "range": true,
	"media": true, "richtext": true,
}

var blockNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*/[a-z0-9][a-z0-9_-]*$`)

func ParseSchema(data string) (Schema, error) {
	var raw struct {
		SchemaVersion int             `json:"schemaVersion"`
		Props         json.RawMessage `json:"props"`
		Settings      json.RawMessage `json:"settings"`
		Children      ChildrenSchema  `json:"children"`
		Editor        EditorSchema    `json:"editor"`
	}
	if err := strictDecode([]byte(data), &raw); err != nil {
		return Schema{}, fmt.Errorf("decode schema: %w", err)
	}
	if raw.SchemaVersion != 1 {
		return Schema{}, fmt.Errorf("unsupported schemaVersion %d", raw.SchemaVersion)
	}
	props, err := parseValueSchema(raw.Props, "props")
	if err != nil {
		return Schema{}, err
	}
	settings, err := parseValueSchema(raw.Settings, "settings")
	if err != nil {
		return Schema{}, err
	}
	schema := Schema{SchemaVersion: 1, Props: props, Settings: settings, Children: raw.Children, Editor: raw.Editor}
	if err := schema.validateContract(); err != nil {
		return Schema{}, err
	}
	return schema, nil
}

func parseValueSchema(data json.RawMessage, path string) (ValueSchema, error) {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return ValueSchema{}, fmt.Errorf("%s schema is required", path)
	}
	var raw struct {
		Type       string                     `json:"type"`
		Required   []string                   `json:"required,omitempty"`
		Properties map[string]json.RawMessage `json:"properties,omitempty"`
		Items      json.RawMessage            `json:"items,omitempty"`
		Enum       []any                      `json:"enum,omitempty"`
		Default    json.RawMessage            `json:"default,omitempty"`
		Minimum    *float64                   `json:"minimum,omitempty"`
		Maximum    *float64                   `json:"maximum,omitempty"`
		MinLength  *int                       `json:"minLength,omitempty"`
		MaxLength  *int                       `json:"maxLength,omitempty"`
		Pattern    string                     `json:"pattern,omitempty"`
	}
	if err := strictDecode(data, &raw); err != nil {
		return ValueSchema{}, fmt.Errorf("%s: %w", path, err)
	}
	result := ValueSchema{Type: raw.Type, Required: raw.Required, Enum: raw.Enum, Minimum: raw.Minimum, Maximum: raw.Maximum, MinLength: raw.MinLength, MaxLength: raw.MaxLength, Pattern: raw.Pattern}
	if len(raw.Default) > 0 {
		result.hasDefault = true
		if err := json.Unmarshal(raw.Default, &result.Default); err != nil {
			return ValueSchema{}, fmt.Errorf("%s.default: %w", path, err)
		}
	}
	if raw.Properties != nil {
		result.Properties = make(map[string]ValueSchema, len(raw.Properties))
		for name, child := range raw.Properties {
			parsed, err := parseValueSchema(child, path+".properties."+name)
			if err != nil {
				return ValueSchema{}, err
			}
			result.Properties[name] = parsed
		}
	}
	if len(raw.Items) > 0 {
		items, err := parseValueSchema(raw.Items, path+".items")
		if err != nil {
			return ValueSchema{}, err
		}
		result.Items = &items
	}
	if result.Pattern != "" {
		compiled, err := regexp.Compile(result.Pattern)
		if err != nil {
			return ValueSchema{}, fmt.Errorf("%s.pattern: %w", path, err)
		}
		result.pattern = compiled
	}
	return result, nil
}

func strictDecode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON")
		}
		return err
	}
	return nil
}

func (s *Schema) validateContract() error {
	if err := validateValueContract(&s.Props, "props", hasRichTextField(s.Editor, "props.")); err != nil {
		return err
	}
	if err := validateValueContract(&s.Settings, "settings", hasRichTextField(s.Editor, "settings.")); err != nil {
		return err
	}
	if s.Props.Type != "object" || s.Settings.Type != "object" {
		return fmt.Errorf("props and settings must have type object")
	}
	switch s.Children.Mode {
	case "none", "any":
		if len(s.Children.Blocks) != 0 {
			return fmt.Errorf("children.blocks is only valid for mode allowed")
		}
	case "allowed":
		if len(s.Children.Blocks) == 0 {
			return fmt.Errorf("children.blocks is required for mode allowed")
		}
	default:
		return fmt.Errorf("children.mode must be none, any, or allowed")
	}
	for _, block := range s.Children.Blocks {
		if !blockNamePattern.MatchString(block) {
			return fmt.Errorf("children.blocks: invalid block name %q", block)
		}
	}
	if s.Children.Min != nil && *s.Children.Min < 0 || s.Children.Max != nil && *s.Children.Max < 0 {
		return fmt.Errorf("children min/max cannot be negative")
	}
	if s.Children.Min != nil && s.Children.Max != nil && *s.Children.Min > *s.Children.Max {
		return fmt.Errorf("children.min cannot exceed children.max")
	}
	for path, field := range s.Editor.Fields {
		parts := strings.Split(path, ".")
		if len(parts) < 2 || parts[0] != "props" && parts[0] != "settings" {
			return fmt.Errorf("editor.fields.%s: path must start with props. or settings.", path)
		}
		value := s.Props
		if parts[0] == "settings" {
			value = s.Settings
		}
		for _, part := range parts[1:] {
			next, ok := value.Properties[part]
			if !ok {
				return fmt.Errorf("editor.fields.%s: field does not exist", path)
			}
			value = next
		}
		if field.Control != "" && !supportedControls[field.Control] {
			return fmt.Errorf("editor.fields.%s: unsupported control %q", path, field.Control)
		}
		switch field.Control {
		case "select", "segmented", "radio":
			if len(value.Enum) == 0 {
				return fmt.Errorf("editor.fields.%s: control %s requires enum", path, field.Control)
			}
		case "checkbox":
			if value.Type != "boolean" {
				return fmt.Errorf("editor.fields.%s: checkbox requires boolean", path)
			}
		case "number", "range":
			if value.Type != "integer" && value.Type != "number" {
				return fmt.Errorf("editor.fields.%s: control %s requires number", path, field.Control)
			}
		case "text", "textarea":
			if value.Type != "string" {
				return fmt.Errorf("editor.fields.%s: control %s requires string", path, field.Control)
			}
		}
	}
	return nil
}

func hasRichTextField(editor EditorSchema, prefix string) bool {
	for path, field := range editor.Fields {
		if field.Control == "richtext" && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func validateValueContract(schema *ValueSchema, path string, allowComplex bool) error {
	switch schema.Type {
	case "object":
		if schema.Items != nil || len(schema.Enum) > 0 || schema.Minimum != nil || schema.Maximum != nil || schema.MinLength != nil || schema.MaxLength != nil || schema.Pattern != "" {
			return fmt.Errorf("%s: unsupported constraint for object", path)
		}
		required := make(map[string]bool, len(schema.Required))
		for _, name := range schema.Required {
			if _, ok := schema.Properties[name]; !ok {
				return fmt.Errorf("%s.required: unknown property %q", path, name)
			}
			if required[name] {
				return fmt.Errorf("%s.required: duplicate property %q", path, name)
			}
			required[name] = true
		}
		for name, property := range schema.Properties {
			copy := property
			if err := validateValueContract(&copy, path+"."+name, allowComplex); err != nil {
				return err
			}
			schema.Properties[name] = copy
		}
	case "array":
		if len(schema.Enum) > 0 {
			return fmt.Errorf("%s: enum is not supported for array", path)
		}
		if schema.Items == nil {
			return fmt.Errorf("%s.items is required", path)
		}
		if !allowComplex && (schema.Items.Type == "array" || schema.Items.Type == "object" && hasComplexObjects(*schema.Items)) {
			return fmt.Errorf("%s.items: only primitives and simple objects are supported", path)
		}
		if err := validateValueContract(schema.Items, path+"[]", allowComplex); err != nil {
			return err
		}
	case "string":
		if len(schema.Properties) > 0 || schema.Items != nil || schema.Minimum != nil || schema.Maximum != nil || len(schema.Required) > 0 {
			return fmt.Errorf("%s: unsupported constraint for string", path)
		}
	case "integer", "number":
		if len(schema.Properties) > 0 || schema.Items != nil || schema.MinLength != nil || schema.MaxLength != nil || schema.Pattern != "" || len(schema.Required) > 0 {
			return fmt.Errorf("%s: unsupported constraint for number", path)
		}
	case "boolean":
		if len(schema.Properties) > 0 || schema.Items != nil || schema.Minimum != nil || schema.Maximum != nil || schema.MinLength != nil || schema.MaxLength != nil || schema.Pattern != "" || len(schema.Required) > 0 {
			return fmt.Errorf("%s: unsupported constraint for boolean", path)
		}
	default:
		return fmt.Errorf("%s.type: unsupported type %q", path, schema.Type)
	}
	if schema.Minimum != nil && schema.Maximum != nil && *schema.Minimum > *schema.Maximum {
		return fmt.Errorf("%s.minimum cannot exceed maximum", path)
	}
	if schema.MinLength != nil && *schema.MinLength < 0 || schema.MaxLength != nil && *schema.MaxLength < 0 {
		return fmt.Errorf("%s minLength/maxLength cannot be negative", path)
	}
	if schema.MinLength != nil && schema.MaxLength != nil && *schema.MinLength > *schema.MaxLength {
		return fmt.Errorf("%s.minLength cannot exceed maxLength", path)
	}
	if len(schema.Enum) > 0 {
		withoutEnum := *schema
		withoutEnum.Enum = nil
		withoutEnum.hasDefault = false
		for i, option := range schema.Enum {
			if err := validateValue(withoutEnum, option, fmt.Sprintf("%s.enum[%d]", path, i), true, false); err != nil {
				return err
			}
		}
	}
	if schema.hasDefault {
		if err := validateValue(*schema, schema.Default, path+".default", true, false); err != nil {
			return err
		}
	}
	return nil
}

func hasComplexObjects(schema ValueSchema) bool {
	for _, property := range schema.Properties {
		if property.Type == "object" || property.Type == "array" {
			return true
		}
	}
	return false
}

func validateValue(schema ValueSchema, value any, path string, enforceRequired bool, allowUnknown bool) error {
	if !enumContains(schema.Enum, value) {
		return fmt.Errorf("%s: expected one of %s", path, enumText(schema.Enum))
	}
	switch schema.Type {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object", path)
		}
		if enforceRequired {
			for _, name := range schema.Required {
				if _, ok := object[name]; !ok {
					return fmt.Errorf("%s.%s: field is required", path, name)
				}
			}
		}
		for name, item := range object {
			property, ok := schema.Properties[name]
			if !ok {
				if allowUnknown {
					continue
				}
				return fmt.Errorf("%s.%s: unknown field", path, name)
			}
			if err := validateValue(property, item, path+"."+name, true, allowUnknown); err != nil {
				return err
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array", path)
		}
		for i, item := range array {
			if err := validateValue(*schema.Items, item, fmt.Sprintf("%s[%d]", path, i), true, allowUnknown); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: expected string", path)
		}
		length := len([]rune(text))
		if schema.MinLength != nil && length < *schema.MinLength {
			return fmt.Errorf("%s: minimum length is %d", path, *schema.MinLength)
		}
		if schema.MaxLength != nil && length > *schema.MaxLength {
			return fmt.Errorf("%s: maximum length is %d", path, *schema.MaxLength)
		}
		if schema.pattern != nil && !schema.pattern.MatchString(text) {
			return fmt.Errorf("%s: does not match pattern %q", path, schema.Pattern)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean", path)
		}
	case "integer", "number":
		number, ok := numericValue(value)
		if !ok || schema.Type == "integer" && math.Trunc(number) != number {
			return fmt.Errorf("%s: expected %s", path, schema.Type)
		}
		if schema.Minimum != nil && number < *schema.Minimum {
			return fmt.Errorf("%s: minimum is %v", path, *schema.Minimum)
		}
		if schema.Maximum != nil && number > *schema.Maximum {
			return fmt.Errorf("%s: maximum is %v", path, *schema.Maximum)
		}
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case json.Number:
		value, err := number.Float64()
		return value, err == nil
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func enumContains(enum []any, value any) bool {
	if len(enum) == 0 {
		return true
	}
	want, _ := json.Marshal(value)
	for _, option := range enum {
		got, _ := json.Marshal(option)
		if bytes.Equal(want, got) {
			return true
		}
	}
	return false
}

func enumText(enum []any) string {
	values := make([]string, 0, len(enum))
	for _, item := range enum {
		values = append(values, fmt.Sprint(item))
	}
	return strings.Join(values, ", ")
}
