package forms

import "time"

const (
	SchemaVersion      = 1
	MaxFields          = 50
	MaxOptions         = 100
	MaxFormName        = 200
	MaxFieldKey        = 64
	MaxText            = 500
	MaxEmail           = 320
	MaxTextarea        = 10000
	MaxPublicBodyBytes = 256 << 10
)

type FieldType string

const (
	FieldText     FieldType = "text"
	FieldEmail    FieldType = "email"
	FieldTextarea FieldType = "textarea"
	FieldSelect   FieldType = "select"
	FieldCheckbox FieldType = "checkbox"
)

type Field struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Type        FieldType `json:"type"`
	Label       string    `json:"label"`
	Placeholder string    `json:"placeholder,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Options     []string  `json:"options,omitempty"`
}

type Definition struct {
	Fields            []Field `json:"fields"`
	SubmitLabel       string  `json:"submitLabel"`
	SuccessMessage    string  `json:"successMessage"`
	NotificationEmail string  `json:"notificationEmail,omitempty"`
}

type Form struct {
	ID            string
	Name          string
	SchemaVersion int
	Definition
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FormView struct {
	ID             string
	Fields         []Field
	SubmitLabel    string
	SuccessMessage string
}

type SubmissionStatus string

const (
	StatusNew      SubmissionStatus = "new"
	StatusRead     SubmissionStatus = "read"
	StatusSpam     SubmissionStatus = "spam"
	StatusArchived SubmissionStatus = "archived"
)

type FormSnapshot struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

type Submission struct {
	ID             string
	FormID         string
	Status         SubmissionStatus
	Values         map[string]string
	SchemaSnapshot FormSnapshot
	CreatedAt      time.Time
}

type FormSummary struct {
	Form
	SubmissionCount int64
	NewCount        int64
}

type SubmitInput struct {
	Values   map[string][]string
	Honeypot string
	ClientIP string
	Now      time.Time
}
