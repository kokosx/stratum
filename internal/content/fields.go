package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var fieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

const (
	maxFieldKeyLength  = 64
	maxFieldTextLength = 16 << 10
	maxFieldsJSONBytes = 64 << 10
)

type FieldType string

const (
	FieldText     FieldType = "text"
	FieldTextarea FieldType = "textarea"
	FieldNumber   FieldType = "number"
	FieldBoolean  FieldType = "boolean"
	FieldDate     FieldType = "date"
	FieldDateTime FieldType = "datetime"
	FieldURL      FieldType = "url"
	FieldEmail    FieldType = "email"
	FieldMedia    FieldType = "media"
	FieldSelect   FieldType = "select"
)

// FieldDefinition is the stable, semantic schema for one content-type field.
// Version is reserved for a compatible decoder when field migrations arrive.
type FieldDefinition struct {
	Key        string          `json:"key"`
	Label      string          `json:"label"`
	Type       FieldType       `json:"type"`
	Required   bool            `json:"required,omitempty"`
	Default    any             `json:"default,omitempty"`
	HelpText   string          `json:"helpText,omitempty"`
	Validation FieldValidation `json:"validation,omitempty"`
	UI         FieldUI         `json:"ui,omitempty"`
	Version    int             `json:"version,omitempty"`
}

type FieldValidation struct {
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Options []string `json:"options,omitempty"`
}

type FieldUI struct {
	Placeholder string `json:"placeholder,omitempty"`
}

type FieldValidationOptions struct {
	MediaExists func(string) bool
}

// ValidateFields normalizes current-schema input into a JSON-safe typed object.
// Callers store its result verbatim in a revision snapshot, never raw form data.
func ValidateFields(definition ContentTypeDefinition, raw map[string]any, options FieldValidationOptions) (map[string]any, error) {
	byKey := make(map[string]FieldDefinition, len(definition.Fields))
	for _, field := range definition.Fields {
		if err := validateDefinition(field); err != nil {
			return nil, err
		}
		if _, exists := byKey[field.Key]; exists {
			return nil, fmt.Errorf("duplicate field key %q", field.Key)
		}
		byKey[field.Key] = field
	}
	for key := range raw {
		if _, exists := byKey[key]; !exists {
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}

	normalized := make(map[string]any, len(definition.Fields))
	for _, field := range definition.Fields {
		value, present := raw[field.Key]
		if !present && field.Default != nil {
			value, present = field.Default, true
		}
		if !present || isEmpty(value) {
			if field.Required {
				return nil, fmt.Errorf("%s is required", field.Label)
			}
			continue
		}
		value, err := normalizeFieldValue(field, value, options)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Label, err)
		}
		normalized[field.Key] = value
	}
	return normalized, nil
}

// DecodeFieldSnapshot decodes an immutable revision object without applying the
// current schema. Historical revisions may legitimately contain removed fields.
func DecodeFieldSnapshot(raw string) (map[string]any, error) {
	if len(raw) > maxFieldsJSONBytes {
		return nil, fmt.Errorf("field snapshot exceeds %d bytes", maxFieldsJSONBytes)
	}
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, errors.New("invalid field snapshot")
	}
	if fields == nil {
		return nil, errors.New("field snapshot must be an object")
	}
	return fields, nil
}

func EncodeFieldSnapshot(fields map[string]any) (string, error) {
	if fields == nil {
		fields = map[string]any{}
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxFieldsJSONBytes {
		return "", fmt.Errorf("field snapshot exceeds %d bytes", maxFieldsJSONBytes)
	}
	return string(encoded), nil
}

func validateDefinition(field FieldDefinition) error {
	if len(field.Key) == 0 || len(field.Key) > maxFieldKeyLength || !fieldKeyPattern.MatchString(field.Key) {
		return fmt.Errorf("invalid field key %q", field.Key)
	}
	if field.Label == "" {
		return fmt.Errorf("field %q has no label", field.Key)
	}
	switch field.Type {
	case FieldText, FieldTextarea, FieldNumber, FieldBoolean, FieldDate, FieldDateTime, FieldURL, FieldEmail, FieldMedia, FieldSelect:
	default:
		return fmt.Errorf("field %q has unsupported type %q", field.Key, field.Type)
	}
	if field.Type == FieldSelect && len(field.Validation.Options) == 0 {
		return fmt.Errorf("select field %q has no options", field.Key)
	}
	if field.Validation.Min != nil || field.Validation.Max != nil {
		if field.Type != FieldNumber {
			return fmt.Errorf("field %q: min/max require number type", field.Key)
		}
		if field.Validation.Min != nil && field.Validation.Max != nil && *field.Validation.Min > *field.Validation.Max {
			return fmt.Errorf("field %q: min cannot exceed max", field.Key)
		}
	}
	if field.Default != nil {
		if field.Type == FieldMedia {
			return fmt.Errorf("field %q: media defaults are not supported", field.Key)
		}
		if _, err := normalizeFieldValue(field, field.Default, FieldValidationOptions{}); err != nil {
			return fmt.Errorf("field %q has invalid default: %w", field.Key, err)
		}
	}
	return nil
}

func ValidateFieldDefinition(field FieldDefinition) error { return validateDefinition(field) }

func normalizeFieldValue(field FieldDefinition, value any, options FieldValidationOptions) (any, error) {
	raw, isString := value.(string)
	if isString {
		raw = strings.TrimSpace(raw)
	}
	switch field.Type {
	case FieldText, FieldTextarea:
		if !isString {
			return nil, errors.New("must be text")
		}
		if len(raw) > maxFieldTextLength {
			return nil, fmt.Errorf("must not exceed %d characters", maxFieldTextLength)
		}
		return raw, nil
	case FieldNumber:
		number, err := numberValue(value)
		if err != nil {
			return nil, errors.New("must be a number")
		}
		if field.Validation.Min != nil && number < *field.Validation.Min {
			return nil, fmt.Errorf("must be at least %v", *field.Validation.Min)
		}
		if field.Validation.Max != nil && number > *field.Validation.Max {
			return nil, fmt.Errorf("must be at most %v", *field.Validation.Max)
		}
		return number, nil
	case FieldBoolean:
		if boolean, ok := value.(bool); ok {
			return boolean, nil
		}
		if isString {
			boolean, err := strconv.ParseBool(raw)
			if err == nil {
				return boolean, nil
			}
		}
		return nil, errors.New("must be true or false")
	case FieldDate:
		if !isString {
			return nil, errors.New("must be a date")
		}
		if _, err := time.Parse("2006-01-02", raw); err != nil {
			return nil, errors.New("must use YYYY-MM-DD")
		}
		return raw, nil
	case FieldDateTime:
		if !isString {
			return nil, errors.New("must be a date and time")
		}
		if _, err := time.Parse("2006-01-02T15:04", raw); err != nil {
			return nil, errors.New("must use YYYY-MM-DDTHH:MM")
		}
		return raw, nil
	case FieldURL:
		if !isString {
			return nil, errors.New("must be a URL")
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("must be an absolute http(s) URL")
		}
		u.Host = strings.ToLower(u.Host)
		return u.String(), nil
	case FieldEmail:
		if !isString {
			return nil, errors.New("must be an email address")
		}
		parsed, err := mail.ParseAddress(raw)
		if err != nil || parsed.Address != raw || !strings.Contains(parsed.Address, "@") {
			return nil, errors.New("must be an email address")
		}
		return strings.ToLower(raw), nil
	case FieldMedia:
		if !isString || raw == "" {
			return nil, errors.New("must be a media ID")
		}
		if options.MediaExists == nil || !options.MediaExists(raw) {
			return nil, errors.New("references unknown media")
		}
		return raw, nil
	case FieldSelect:
		if !isString {
			return nil, errors.New("must be a selection")
		}
		for _, option := range field.Validation.Options {
			if raw == option {
				return raw, nil
			}
		}
		return nil, errors.New("contains an invalid option")
	}
	return nil, errors.New("unsupported field type")
}

func numberValue(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, errors.New("not a number")
	}
}

func isEmpty(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}
