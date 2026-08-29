package canonicalredirect

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// SchemePolicy controls scheme canonicalization.
type SchemePolicy int

const (
	SchemeOff   SchemePolicy = iota // no scheme redirect
	SchemeHTTPS                     // force https
	SchemeHTTP                      // force http
)

// WWWPolicy controls www canonicalization.
type WWWPolicy int

const (
	WWWOff       WWWPolicy = iota // no www redirect
	WWWRequired                   // force www
	WWWForbidden                  // force non-www (strip www)
)

// Config holds the canonical redirect policy. CanonicalHost is the public
// host (with port if the Site URL includes one) that www redirects target.
// It is empty when WWW==WWWOff or when no Site URL is configured.
type Config struct {
	Scheme        SchemePolicy
	WWW           WWWPolicy
	TrustProxy    bool
	CanonicalHost string // e.g. "example.com" or "www.example.com:8080" (lowercased)
	// internal: canonical hostname without port/brackets, lowercased
	canonicalHostname string
	canonicalPort     string
	hasPort           bool
}

// ParseScheme parses a scheme flag value (off|https|http).
func ParseScheme(s string) (SchemePolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off":
		return SchemeOff, nil
	case "https":
		return SchemeHTTPS, nil
	case "http":
		return SchemeHTTP, nil
	default:
		return SchemeOff, fmt.Errorf("invalid --redirect-scheme value %q; expected off, https, or http", s)
	}
}

// ParseWWW parses a www flag value (off|www|non-www).
func ParseWWW(s string) (WWWPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off":
		return WWWOff, nil
	case "www":
		return WWWRequired, nil
	case "non-www", "non_www", "nonwww":
		return WWWForbidden, nil
	default:
		return WWWOff, fmt.Errorf("invalid --redirect-www value %q; expected off, www, or non-www", s)
	}
}

// ParseTrustProxy parses a bool flag value for trust-proxy.
func ParseTrustProxy(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf("invalid --trust-proxy value %q; expected true or false", s)
	}
}

// NewConfig builds a Config from raw flag strings and the site URL. siteURL is
// the configured public Site URL (e.g. https://example.com). It may be empty.
func NewConfig(schemeStr, wwwStr string, trustProxy bool, siteURL string) (Config, error) {
	scheme, err := ParseScheme(schemeStr)
	if err != nil {
		return Config{}, err
	}
	www, err := ParseWWW(wwwStr)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Scheme:     scheme,
		WWW:        www,
		TrustProxy: trustProxy,
	}
	// Always derive canonical host from siteURL if present, for logging and open-redirect protection.
	// WWW policy requires a valid public host; scheme-only does not.
	var siteHost, sitePort string
	var siteHasPort bool
	var siteHostnameLower string
	if strings.TrimSpace(siteURL) != "" {
		parsed, err := url.Parse(strings.TrimSpace(siteURL))
		if err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			rawHost := parsed.Host
			hostname, port, hasPort := splitHost(rawHost)
			if hostname != "" {
				lowerHost := strings.ToLower(hostname)
				if !isLocalHost(lowerHost) {
					siteHost = strings.ToLower(rawHost)
					sitePort = port
					siteHasPort = hasPort
					siteHostnameLower = lowerHost
				}
			}
		}
	}
	if www != WWWOff {
		if strings.TrimSpace(siteURL) == "" || siteHost == "" {
			return Config{}, fmt.Errorf("cannot enable --redirect-www: Site URL does not contain a valid public hostname")
		}
		// siteHost already validated non-local; reuse
		lowerHost := siteHostnameLower
		port := sitePort
		hasPort := siteHasPort
		// Derive canonical hostname based on www policy
		var canonicalHostname string
		switch www {
		case WWWForbidden:
			if strings.HasPrefix(lowerHost, "www.") {
				canonicalHostname = strings.TrimPrefix(lowerHost, "www.")
				if canonicalHostname == "" || isLocalHost(canonicalHostname) {
					return Config{}, fmt.Errorf("cannot enable --redirect-www: Site URL does not contain a valid public hostname")
				}
			} else {
				canonicalHostname = lowerHost
			}
		case WWWRequired:
			if strings.HasPrefix(lowerHost, "www.") {
				canonicalHostname = lowerHost
			} else {
				canonicalHostname = "www." + lowerHost
			}
		}
		// Reassemble canonical host with port (if any)
		var canonicalHost string
		if hasPort {
			canonicalHost = net.JoinHostPort(canonicalHostname, port)
		} else {
			if strings.Contains(canonicalHostname, ":") {
				canonicalHost = "[" + canonicalHostname + "]"
			} else {
				canonicalHost = canonicalHostname
			}
		}
		if !strings.Contains(canonicalHostname, ".") && !strings.Contains(canonicalHostname, ":") {
			// lenient
		}
		cfg.CanonicalHost = strings.ToLower(canonicalHost)
		cfg.canonicalHostname = canonicalHostname
		cfg.canonicalPort = port
		cfg.hasPort = hasPort
	}
	return cfg, nil
}

// splitHost extracts hostname and port from a host string. It correctly handles
// IPv6 literals with and without port. Hostname is returned without brackets and
// without port. For hosts without port, port is empty and hasPort is false.
func splitHost(host string) (hostname, port string, hasPort bool) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", "", false
	}
	// Try net.SplitHostPort first (handles IPv6 with port and normal host:port)
	if h, p, err := net.SplitHostPort(host); err == nil {
		return h, p, true
	}
	// No port - handle IPv6 literal in brackets like "[::1]"
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		inner := host[1 : len(host)-1]
		return inner, "", false
	}
	// Could be IPv6 without brackets and without port ("::1") - treat whole as hostname
	// For normal hosts without port, return as is
	// But need to detect case like "example.com:8080" where SplitHostPort failed due to missing brackets for IPv6?
	// net.SplitHostPort fails for "example.com:8080" without error? Actually it succeeds.
	// So fallback is host without port.
	return host, "", false
}

func isLocalHost(host string) bool {
	// host is already lowercased and without port/brackets
	h := strings.ToLower(strings.TrimSpace(host))
	// Trim brackets if any left
	h = strings.Trim(h, "[]")
	switch h {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	}
	return false
}
