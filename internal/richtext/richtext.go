// Package richtext defines the safe, portable inline content format used by
// text-bearing blocks. It stores content as JSON data, never as HTML.
package richtext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strings"
)

const Version = 1

const (
	maxChars       = 10000
	maxRuns        = 200
	maxMarksPerRun = 5
	maxHrefLength  = 2048
)

type RichText struct {
	Version int   `json:"version"`
	Content []Run `json:"content"`
}

type Run struct {
	Text  string `json:"text"`
	Marks []Mark `json:"marks,omitempty"`
}

type Mark struct {
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
}

func Parse(value any) (RichText, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return RichText{}, fmt.Errorf("encode rich text: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var text RichText
	if err := decoder.Decode(&text); err != nil {
		return RichText{}, fmt.Errorf("decode rich text: %w", err)
	}
	if text.Version != Version {
		return RichText{}, fmt.Errorf("unsupported rich text version %d", text.Version)
	}
	return Normalize(text)
}

func Normalize(text RichText) (RichText, error) {
	if text.Version != Version {
		return RichText{}, fmt.Errorf("unsupported rich text version %d", text.Version)
	}
	if len(text.Content) > maxRuns {
		return RichText{}, fmt.Errorf("too many runs: %d", len(text.Content))
	}
	totalChars := 0
	for _, run := range text.Content {
		totalChars += len(run.Text)
		if totalChars > maxChars {
			return RichText{}, fmt.Errorf("rich text too large: %d characters", totalChars)
		}
		if len(run.Marks) > maxMarksPerRun {
			return RichText{}, fmt.Errorf("too many marks in run: %d", len(run.Marks))
		}
	}
	result := RichText{Version: Version, Content: make([]Run, 0, len(text.Content))}
	for _, run := range text.Content {
		if run.Text == "" {
			continue
		}
		marks, err := normalizeMarks(run.Marks)
		if err != nil {
			return RichText{}, err
		}
		run.Marks = marks
		if len(result.Content) > 0 && sameMarks(result.Content[len(result.Content)-1].Marks, run.Marks) {
			result.Content[len(result.Content)-1].Text += run.Text
			continue
		}
		result.Content = append(result.Content, run)
	}
	return result, nil
}

func (text RichText) PlainText() string {
	var out strings.Builder
	for _, run := range text.Content {
		out.WriteString(run.Text)
	}
	return out.String()
}

func (text RichText) Links() []string {
	links := make([]string, 0)
	seen := make(map[string]bool)
	for _, run := range text.Content {
		for _, mark := range run.Marks {
			if mark.Type == "link" && !seen[mark.Href] {
				seen[mark.Href] = true
				links = append(links, mark.Href)
			}
		}
	}
	return links
}

func Render(value any) (template.HTML, error) {
	text, err := Parse(value)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, run := range text.Content {
		content := template.HTMLEscapeString(run.Text)
		for _, mark := range run.Marks {
			switch mark.Type {
			case "bold":
				content = "<strong>" + content + "</strong>"
			case "italic":
				content = "<em>" + content + "</em>"
			case "strike":
				content = "<s>" + content + "</s>"
			case "code":
				content = "<code>" + content + "</code>"
			case "link":
				content = `<a href="` + template.HTMLEscapeString(mark.Href) + `">` + content + "</a>"
			}
		}
		out.WriteString(content)
	}
	return template.HTML(out.String()), nil
}

func normalizeMarks(marks []Mark) ([]Mark, error) {
	if len(marks) > maxMarksPerRun {
		return nil, fmt.Errorf("too many marks: %d", len(marks))
	}
	unique := make(map[string]Mark, len(marks))
	linkCount := 0
	for _, mark := range marks {
		if len(mark.Href) > maxHrefLength {
			return nil, fmt.Errorf("href too long: %d", len(mark.Href))
		}
		switch mark.Type {
		case "bold", "italic", "strike", "code":
			if mark.Href != "" {
				return nil, fmt.Errorf("%s mark cannot have href", mark.Type)
			}
		case "link":
			if !safeLink(mark.Href) {
				return nil, fmt.Errorf("unsafe link %q", mark.Href)
			}
			linkCount++
		default:
			return nil, fmt.Errorf("unsupported mark %q", mark.Type)
		}
		key := mark.Type + "\x00" + mark.Href
		unique[key] = mark
	}
	// Enforce max one link per run; conflicting links are invalid.
	if linkCount > 1 {
		// Count unique links after dedup
		uniqueLinks := 0
		for _, m := range unique {
			if m.Type == "link" {
				uniqueLinks++
			}
		}
		if uniqueLinks > 1 {
			return nil, fmt.Errorf("multiple links in one run")
		}
	}
	result := make([]Mark, 0, len(unique))
	for _, mark := range unique {
		result = append(result, mark)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type == result[j].Type {
			return result[i].Href < result[j].Href
		}
		return result[i].Type < result[j].Type
	})
	return result, nil
}

func sameMarks(left, right []Mark) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func safeLink(href string) bool {
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "/") {
		return href != "" && !strings.HasPrefix(href, "//")
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto", "tel":
		return true
	default:
		return false
	}
}
