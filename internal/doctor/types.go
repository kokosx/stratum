package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Status represents check result severity.
type Status string

const (
	StatusPass Status = "PASS"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
)

// Check holds one doctor check result.
type Check struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Status  Status   `json:"status"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
	Hint    string   `json:"hint,omitempty"`
	Action  string   `json:"action,omitempty"`
}

// Report is the full doctor report.
type Report struct {
	SchemaVersion int     `json:"schemaVersion"`
	Status        Status  `json:"status"` // overall: PASS/WARN/FAIL
	Production    bool    `json:"production"`
	Checks        []Check `json:"checks"`
	Summary       Summary `json:"summary"`
}

// Summary counts.
type Summary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failures int `json:"failures"`
}

// JSON returns indented JSON for --json flag.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// Human renders human-readable output.
func (r *Report) Human() string {
	var b strings.Builder
	b.WriteString("Stratum Doctor\n\n")
	for _, c := range r.Checks {
		b.WriteString(fmt.Sprintf("%-4s  %s\n", string(c.Status), c.Title))
		if c.Message != "" {
			b.WriteString(fmt.Sprintf("      %s\n", c.Message))
		}
		for _, d := range c.Details {
			if d != "" {
				b.WriteString(fmt.Sprintf("      %s\n", d))
			}
		}
		if c.Hint != "" {
			b.WriteString(fmt.Sprintf("      %s\n", c.Hint))
		}
		if c.Action != "" {
			b.WriteString(fmt.Sprintf("      %s\n", c.Action))
		}
		b.WriteString("\n")
	}
	b.WriteString("-----------------------------------\n")
	b.WriteString(fmt.Sprintf("%d passed · %d warnings · %d failures\n", r.Summary.Passed, r.Summary.Warnings, r.Summary.Failures))
	return b.String()
}

func (r *Report) computeStatus() {
	// derive overall status and summary
	var p, w, f int
	for _, c := range r.Checks {
		switch c.Status {
		case StatusPass:
			p++
		case StatusWarn:
			w++
		case StatusFail:
			f++
		}
	}
	r.Summary = Summary{Passed: p, Warnings: w, Failures: f}
	if f > 0 {
		r.Status = StatusFail
	} else if w > 0 {
		r.Status = StatusWarn
	} else {
		r.Status = StatusPass
	}
}
