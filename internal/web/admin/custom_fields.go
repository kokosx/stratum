package admin

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kokosx/stratum/internal/content"
)

// customFieldControl is a presentation-only projection of a FieldDefinition.
// Values come from the submitted form on errors, otherwise from the revision.
type customFieldControl struct {
	Key         string
	Label       string
	Type        content.FieldType
	HelpText    string
	Placeholder string
	Required    bool
	Value       string
	Checked     bool
	Options     []string
	Selected    string
}

func rawFieldValues(r *http.Request, definition content.ContentTypeDefinition) map[string]any {
	_ = r.ParseForm()
	values := make(map[string]any, len(definition.Fields))
	for _, field := range definition.Fields {
		name := "field_" + field.Key
		if field.Type == content.FieldBoolean {
			if r.FormValue(name+"_present") != "" {
				values[field.Key] = r.FormValue(name) != ""
			}
			continue
		}
		if posted, ok := r.Form[name]; ok && len(posted) > 0 {
			values[field.Key] = posted[0]
		}
	}
	return values
}

func fieldValues(snapshot string) map[string]any {
	values, err := content.DecodeFieldSnapshot(snapshot)
	if err != nil {
		return map[string]any{}
	}
	return values
}

func customFieldControls(definition content.ContentTypeDefinition, values map[string]any) []customFieldControl {
	controls := make([]customFieldControl, 0, len(definition.Fields))
	for _, field := range definition.Fields {
		value, exists := values[field.Key]
		if !exists {
			value = field.Default
		}
		control := customFieldControl{
			Key: field.Key, Label: field.Label, Type: field.Type, HelpText: field.HelpText,
			Placeholder: field.UI.Placeholder, Required: field.Required, Options: field.Validation.Options,
		}
		if boolean, ok := value.(bool); ok {
			control.Checked = boolean
		} else if value != nil {
			control.Value = fieldValueString(value)
			control.Selected = control.Value
		}
		controls = append(controls, control)
	}
	return controls
}

func fieldValueString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}
