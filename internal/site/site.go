package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// languagePattern accepts simple BCP-47-ish language tags such as "en", "pl" or
// "en-US". It is deliberately strict: a full locale parser is out of scope.
var languagePattern = regexp.MustCompile(`^[a-zA-Z]{2}(-[a-zA-Z]{2,3})?$`)

const maxRobotsCustomBytes = 32 * 1024

// ValidateSiteURL normalises a canonical public origin. An empty value is
// allowed (the admin shows a warning where an absolute URL is required); a
// non-empty value must be an absolute http(s) URL with a host and no query
// string or fragment. The trailing slash is trimmed so callers can join paths
// without producing double slashes.
func ValidateSiteURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid site URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("site URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return "", errors.New("site URL must include a host")
	}
	if parsed.Fragment != "" {
		return "", errors.New("site URL must not contain a fragment")
	}
	if parsed.RawQuery != "" {
		return "", errors.New("site URL must not contain a query string")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// ValidateLanguage checks the basic shape of a language tag.
func ValidateLanguage(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("language is required")
	}
	if !languagePattern.MatchString(value) {
		return errors.New("language must look like 'en' or 'en-US'")
	}
	return nil
}

// ValidateTimezone requires a real IANA timezone location.
func ValidateTimezone(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(value); err != nil {
		return fmt.Errorf("invalid IANA timezone: %w", err)
	}
	return nil
}

// ValidateRobotsSize ensures a custom robots.txt stays within the 32 KB limit.
func ValidateRobotsSize(value string) error {
	if len(value) > maxRobotsCustomBytes {
		return fmt.Errorf("custom robots.txt exceeds the %d byte limit", maxRobotsCustomBytes)
	}
	return nil
}

// ValidSpeculationMode reports whether mode is one of the supported enum values.
func ValidSpeculationMode(mode string) bool {
	switch mode {
	case "off", "prefetch", "prerender":
		return true
	}
	return false
}

// ValidSpeculationEagerness reports whether eagerness is a supported value.
func ValidSpeculationEagerness(eagerness string) bool {
	switch eagerness {
	case "conservative", "moderate", "eager":
		return true
	}
	return false
}

// RobotsInput carries the site settings needed to render robots.txt.
type RobotsInput struct {
	Mode            string
	IndexingEnabled bool
	SitemapEnabled  bool
	SiteURL         string
	Custom          string
}

// BuildRobots returns the exact robots.txt body for the given settings. In
// custom mode it returns the administrator-provided text verbatim. In managed
// mode it emits a safe default that depends on the indexing setting, and only
// advertises the sitemap when both sitemap and a canonical site URL are set.
func BuildRobots(input RobotsInput) string {
	if input.Mode == "custom" {
		return input.Custom
	}
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	if input.IndexingEnabled {
		b.WriteString("Allow: /\n")
		b.WriteString("\n")
		b.WriteString("Disallow: /admin/\n")
		if input.SitemapEnabled && input.SiteURL != "" {
			b.WriteString("\n")
			b.WriteString("Sitemap: " + strings.TrimRight(input.SiteURL, "/") + "/sitemap.xml\n")
		}
		return b.String()
	}
	b.WriteString("Disallow: /\n")
	return b.String()
}

type speculationRule struct {
	Source    string           `json:"source"`
	Where     speculationWhere `json:"where"`
	Eagerness string           `json:"eagerness"`
}

type speculationWhere struct {
	HrefMatches     string                 `json:"href_matches"`
	SelectorMatches string                 `json:"selector_matches"`
	Not             []speculationCondition `json:"not"`
}

type speculationCondition struct {
	HrefMatches     []string `json:"href_matches,omitempty"`
	PathnameMatches string   `json:"pathname_matches,omitempty"`
}

type speculationRules struct {
	Prefetch  []speculationRule `json:"prefetch,omitempty"`
	Prerender []speculationRule `json:"prerender,omitempty"`
}

// BuildSpeculationRules generates the Speculation Rules JSON document for the
// given mode and eagerness. The JSON is always produced with encoding/json so
// it can never contain injected script. An empty string is returned when the
// feature is disabled (mode "off"); callers treat that as "no script".
func BuildSpeculationRules(mode, eagerness string) (string, error) {
	if !ValidSpeculationMode(mode) {
		return "", fmt.Errorf("unsupported speculation mode %q", mode)
	}
	if mode == "off" {
		return "", nil
	}
	if !ValidSpeculationEagerness(eagerness) {
		return "", fmt.Errorf("unsupported speculation eagerness %q", eagerness)
	}
	rule := speculationRule{
		Source: "document",
		Where: speculationWhere{
			HrefMatches:     "/*",
			SelectorMatches: "a:not([data-no-speculate]):not([download]):not([href^='mailto:']):not([href^='tel:']):not([href^='javascript:'])",
			Not: []speculationCondition{
				{HrefMatches: []string{"/admin/*", "/admin/login", "/admin/logout", "/admin/setup"}},
				{PathnameMatches: "/stratum/*"},
			},
		},
		Eagerness: eagerness,
	}
	doc := speculationRules{}
	if mode == "prefetch" {
		doc.Prefetch = []speculationRule{rule}
	} else {
		doc.Prerender = []speculationRule{rule}
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("encode speculation rules: %w", err)
	}
	return string(encoded), nil
}
