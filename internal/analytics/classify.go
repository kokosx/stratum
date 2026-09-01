package analytics

import (
	"net/url"
	"strings"
)

// Known hosts for classification (lowercased, without www).
var searchHosts = map[string]bool{
	"google.com": true, "google.pl": true, "google.de": true, "google.co.uk": true, "google.fr": true, "google.es": true, "google.it": true, "bing.com": true, "duckduckgo.com": true, "yahoo.com": true, "yahoo.co.jp": true, "baidu.com": true, "yandex.ru": true, "yandex.com": true, "ecosia.org": true, "brave.com": true, "search.brave.com": true, "ask.com": true,
}

var socialHosts = map[string]bool{
	"facebook.com": true, "fb.com": true, "instagram.com": true, "tiktok.com": true, "linkedin.com": true, "x.com": true, "twitter.com": true, "youtube.com": true, "youtu.be": true, "pinterest.com": true, "reddit.com": true, "threads.net": true, "snapchat.com": true, "whatsapp.com": true, "telegram.org": true, "t.me": true, "discord.com": true, "medium.com": true,
}

var aiHosts = map[string]bool{
	"chatgpt.com": true, "chat.openai.com": true, "openai.com": true,
	"perplexity.ai": true, "perplexity.com": true,
	"claude.ai": true, "anthropic.com": true,
	"gemini.google.com": true, "bard.google.com": true,
	"copilot.microsoft.com": true, "bing.com": false, // bing is search, not AI
	"you.com": true, "phind.com": true, "poe.com": true,
	"meta.ai": true,
}

// ClassifyTraffic determines TrafficSource from sanitized inputs.
// Order: UTM campaign present => campaign; same-origin => internal; known search/social/AI else referral/direct.
// referrerHost is already sanitized hostname lowercased "" if none.
// siteHost is lowercased hostname of this stratum site (without port)
func ClassifyTraffic(referrerHost, siteHost, utmSource, utmMedium, utmCampaign string) TrafficSource {
	if utmSource != "" || utmMedium != "" || utmCampaign != "" {
		return TrafficCampaign
	}
	if referrerHost == "" {
		return TrafficDirect
	}
	if siteHost != "" && normalizeHost(referrerHost) == normalizeHost(siteHost) {
		return TrafficInternal
	}
	// check AI first (some AI hosts also could be search? prioritize AI per spec)
	if isAIHost(referrerHost) {
		return TrafficAIReferral
	}
	if isSearchHost(referrerHost) {
		return TrafficOrganicSearch
	}
	if isSocialHost(referrerHost) {
		return TrafficOrganicSocial
	}
	return TrafficReferral
}

func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimPrefix(h, "www.")
	return h
}

func isSearchHost(host string) bool {
	h := normalizeHost(host)
	if searchHosts[h] {
		return true
	}
	// also match subdomains: e.g., www.google.com already normalized, but also maps.google.com -> google.com?
	for k := range searchHosts {
		if strings.HasSuffix(h, "."+k) {
			return true
		}
	}
	return false
}

func isSocialHost(host string) bool {
	h := normalizeHost(host)
	if socialHosts[h] {
		return true
	}
	for k := range socialHosts {
		if strings.HasSuffix(h, "."+k) {
			return true
		}
	}
	return false
}

func isAIHost(host string) bool {
	h := normalizeHost(host)
	if v, ok := aiHosts[h]; ok {
		return v
	}
	for k, v := range aiHosts {
		if !v {
			continue
		}
		if strings.HasSuffix(h, "."+k) {
			return true
		}
	}
	return false
}

// ClassifyBrowser reduces UA to coarse bucket without heavy dependency.
func ClassifyBrowser(ua string) string {
	l := strings.ToLower(ua)
	if l == "" {
		return "Other"
	}
	// Edge contains Chrome, so check Edge first (Edg/)
	if strings.Contains(l, "edg/") || strings.Contains(l, "edge") {
		return "Edge"
	}
	if strings.Contains(l, "chrome") && !strings.Contains(l, "chromium") {
		// Chrome includes Safari string; check Chrome before Safari
		return "Chrome"
	}
	if strings.Contains(l, "firefox") || strings.Contains(l, "fxios") {
		return "Firefox"
	}
	if strings.Contains(l, "safari") && !strings.Contains(l, "chrome") {
		return "Safari"
	}
	return "Other"
}

// ClassifyOS coarse.
func ClassifyOS(ua string) string {
	l := strings.ToLower(ua)
	if l == "" {
		return "Other"
	}
	if strings.Contains(l, "windows") {
		return "Windows"
	}
	if strings.Contains(l, "android") {
		return "Android"
	}
	if strings.Contains(l, "iphone") || strings.Contains(l, "ipad") {
		return "iOS"
	}
	if strings.Contains(l, "mac os") || strings.Contains(l, "macos") || strings.Contains(l, "macintosh") {
		return "macOS"
	}
	if strings.Contains(l, "linux") {
		return "Linux"
	}
	return "Other"
}

// ClassifyDevice coarse.
func ClassifyDevice(ua string) string {
	l := strings.ToLower(ua)
	if l == "" {
		return "other"
	}
	if strings.Contains(l, "ipad") || strings.Contains(l, "tablet") {
		return "tablet"
	}
	if strings.Contains(l, "mobi") || strings.Contains(l, "iphone") || strings.Contains(l, "android") && strings.Contains(l, "mobile") {
		return "mobile"
	}
	if strings.Contains(l, "android") {
		// Android without mobile token often tablet, but fallback mobile
		return "mobile"
	}
	return "desktop"
}

// ClassifyLanguage normalizes Accept-Language coarse language.
// Returns 2-letter lowercased code or "other".
func ClassifyLanguage(acceptLang string) string {
	if acceptLang == "" {
		return "other"
	}
	// Take first language tag before comma or ;
	parts := strings.Split(acceptLang, ",")
	if len(parts) == 0 {
		return "other"
	}
	first := strings.TrimSpace(parts[0])
	if idx := strings.Index(first, ";"); idx >= 0 {
		first = first[:idx]
	}
	first = strings.TrimSpace(strings.ToLower(first))
	if first == "" || first == "*" {
		return "other"
	}
	// Extract primary subtag before -
	if idx := strings.Index(first, "-"); idx >= 0 {
		first = first[:idx]
	}
	if idx := strings.Index(first, "_"); idx >= 0 {
		first = first[:idx]
	}
	if len(first) < 2 || len(first) > 3 {
		return "other"
	}
	for _, ch := range first {
		if ch < 'a' || ch > 'z' {
			return "other"
		}
	}
	return first
}

// ClassifyCrawler detects declared crawlers.
// Returns normalized name e.g., "Googlebot", "Bingbot", "GPTBot", "ClaudeBot", "PerplexityBot", "generic", "".
func ClassifyCrawler(ua string) string {
	l := strings.ToLower(ua)
	if l == "" {
		return ""
	}
	if strings.Contains(l, "googlebot") {
		return "Googlebot"
	}
	if strings.Contains(l, "bingbot") {
		return "Bingbot"
	}
	if strings.Contains(l, "gptbot") {
		return "GPTBot"
	}
	if strings.Contains(l, "chatgpt-user") {
		return "ChatGPT-User"
	}
	if strings.Contains(l, "claudebot") {
		return "ClaudeBot"
	}
	if strings.Contains(l, "perplexitybot") {
		return "PerplexityBot"
	}
	if strings.Contains(l, "ccbot") {
		return "CCBot"
	}
	if strings.Contains(l, "bot") || strings.Contains(l, "crawler") || strings.Contains(l, "spider") || strings.Contains(l, "headlesschrome") && strings.Contains(l, "bot") {
		return "generic"
	}
	return ""
}

// IsSpeculative checks Sec-Purpose / Purpose headers for prerefetch/prerender.
// headers is lowercased value check containing those tokens.
func IsSpeculative(secPurpose, purpose string) bool {
	candidates := []string{strings.ToLower(secPurpose), strings.ToLower(purpose)}
	for _, v := range candidates {
		if strings.Contains(v, "prefetch") || strings.Contains(v, "prerender") {
			return true
		}
	}
	return false
}

// SanitizeReferrerHost extracts only hostname lowercased, bounded, or empty if invalid.
// raw is full Referer header value. siteHost not needed here but host validation is done.
func SanitizeReferrerHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	// strip port
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 253 {
		return ""
	}
	// Reject control chars
	for _, ch := range host {
		if ch < 32 || ch == 127 {
			return ""
		}
	}
	// Basic hostname validation: only allowed chars, no userinfo/path
	if strings.Contains(host, "/") || strings.Contains(host, "?") || strings.Contains(host, "#") || strings.Contains(host, "@") {
		return ""
	}
	return host
}

// SanitizeUTM bounds and rejects control chars, overly long, etc.
func SanitizeUTM(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if len(s) > 100 {
		s = s[:100]
	}
	for _, ch := range s {
		if ch < 32 || ch == 127 {
			return ""
		}
	}
	// Reject if contains control-like? Already.
	// Also reject if contains = or & ? Those are not typical utm but we allow? Keep permissive but no control.
	return s
}

// SanitizeLanguage wrapper used for storage dimension value: already coarse via ClassifyLanguage.
func SanitizeDimensionValue(dim, val string) string {
	switch dim {
	case "browser", "os", "device", "language", "crawler":
		// enums are already controlled, but bound length
		if len(val) > 32 {
			return "other"
		}
		return val
	case "referrer_host", "utm_source", "utm_medium", "utm_campaign":
		if len(val) > 100 {
			val = val[:100]
		}
		if val == "" {
			return ""
		}
		for _, ch := range val {
			if ch < 32 || ch == 127 {
				return "other"
			}
		}
		return val
	default:
		return "other"
	}
}

// ParseUTMFromQuery extracts utm_source, utm_medium, utm_campaign from URL query string.
// rawQuery is r.URL.RawQuery or Query(). It uses net/url parsing and sanitizes.
// It intentionally ignores utm_term, gclid, fbclid, msclkid.
func ParseUTMFromQuery(rawQuery string) (source, medium, campaign string) {
	if rawQuery == "" {
		return "", "", ""
	}
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", "", ""
	}
	source = SanitizeUTM(vals.Get("utm_source"))
	medium = SanitizeUTM(vals.Get("utm_medium"))
	campaign = SanitizeUTM(vals.Get("utm_campaign"))
	return
}

// SiteHostFromURL extracts hostname from siteURL config.
func SiteHostFromURL(siteURL string) string {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return ""
	}
	u, err := url.Parse(siteURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return host
}
