package rendering

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

// iconPaths holds the inner SVG markup for the curated Stratum icon set. The
// markup is author-controlled (never end-user input), so it is safe to emit as
// template.HTML. Icons use a 24x24 viewBox with stroke-based geometry so they
// inherit color via currentColor.
var iconPaths = map[string]string{
	"arrow-right":  `<line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/>`,
	"check":        `<polyline points="20 6 9 17 4 12"/>`,
	"x":            `<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>`,
	"info":         `<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="8"/>`,
	"warning":      `<path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12" y2="17"/>`,
	"star":         `<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>`,
	"menu":         `<line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/>`,
	"search":       `<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>`,
	"plus":         `<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>`,
	"chevron-down": `<polyline points="6 9 12 15 18 9"/>`,
	"phone":        `<path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.13.96.36 1.9.7 2.81a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.91.34 1.85.57 2.81.7A2 2 0 0 1 22 16.92z"/>`,
	"mail":         `<rect x="2" y="4" width="20" height="16" rx="2"/><path d="m22 7-10 5L2 7"/>`,
	"location":     `<path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/>`,
	"link":         `<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>`,
	"external":     `<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/>`,
	"heart":        `<path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>`,
}

func iconFunc(name string) template.HTML {
	inner, ok := iconPaths[name]
	if !ok {
		return ""
	}
	return template.HTML(fmt.Sprintf(`<svg viewBox="0 0 24 24" aria-hidden="true">%s</svg>`, inner))
}

// lines splits a newline-separated string into trimmed, non-empty items. It is
// used by blocks like the List to let editors author repeated content with a
// simple textarea while the renderer iterates the result.
func linesFunc(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "\n")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// split divides value on a literal separator and trims every resulting cell.
// Blocks like Table and Gallery author structured content in a textarea (rows on
// newlines, cells on a separator) and rely on this to turn it into iterables.
func splitFunc(sep, value string) []string {
	if value == "" || sep == "" {
		return nil
	}
	parts := strings.Split(value, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}

// youtubeID extracts an 11-character YouTube video id from common URL shapes
// (watch, youtu.be, embed, shorts). It returns "" when the URL is not a
// recognizable YouTube link so the Video block can fall back gracefully.
func youtubeIDFunc(url string) string {
	url = strings.TrimSpace(url)
	switch {
	case strings.Contains(url, "youtu.be/"):
		return lastPathSegment(url, "youtu.be/")
	case strings.Contains(url, "youtube.com/shorts/"):
		return lastPathSegment(url, "youtube.com/shorts/")
	case strings.Contains(url, "youtube.com/embed/"):
		return lastPathSegment(url, "youtube.com/embed/")
	case strings.Contains(url, "youtube.com/watch?v="):
		return queryValue(url, "v")
	case strings.Contains(url, "youtube.com/v/"):
		return lastPathSegment(url, "youtube.com/v/")
	}
	return ""
}

// vimeoID extracts the numeric Vimeo video id from common URL shapes.
func vimeoIDFunc(url string) string {
	url = strings.TrimSpace(url)
	if strings.Contains(url, "vimeo.com/") {
		return lastPathSegment(url, "vimeo.com/")
	}
	return ""
}

func lastPathSegment(url, marker string) string {
	rest := url[strings.Index(url, marker)+len(marker):]
	if i := strings.IndexAny(rest, "?#&/"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return ""
	}
	return rest
}

func queryValue(url, key string) string {
	start := strings.Index(url, key+"=")
	if start < 0 {
		return ""
	}
	rest := url[start+len(key)+1:]
	if i := strings.IndexAny(rest, "&#"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// tagFor returns an HTML heading tag name ("h1".."h6") for a numeric level. It
// normalizes the JSON number types that arrive from decoded documents so block
// templates can emit semantic heading levels without fragile printf formatting.
func tagForFunc(value any) string {
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("h%d", int(v))
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return fmt.Sprintf("h%d", i)
		}
	case int:
		return fmt.Sprintf("h%d", v)
	case int64:
		return fmt.Sprintf("h%d", v)
	}
	return "h2"
}

// tagOpenFunc and tagCloseFunc emit the raw opening/closing heading tag for a
// tag name. html/template escapes literal angle brackets around a dynamic tag
// name (e.g. "<{{ $tag }}>"), so templates must splice these pre-escaped
// fragments in instead.
func tagOpenFunc(tag string) template.HTML {
	return template.HTML("<" + tag)
}

func tagCloseFunc(tag string) template.HTML {
	return template.HTML("</" + tag + ">")
}
