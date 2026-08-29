package canonicalredirect

import (
	"net/http"
	"net/url"
	"strings"
)

// TargetURL computes the final canonical URL for r. If the request is already
// canonical, it returns (nil, false). Otherwise it returns the single redirect
// target and true.
//
// It is the single decision function for scheme and host canonicalization.
func TargetURL(r *http.Request, cfg Config) (*url.URL, bool) {
	// Health check bypass - exactly /healthz (no query handling needed for path match)
	if r.URL.Path == "/healthz" {
		return nil, false
	}

	effScheme := effectiveScheme(r, cfg.TrustProxy)
	effHost := effectiveHost(r, cfg.TrustProxy)
	if strings.TrimSpace(effHost) == "" {
		return nil, false
	}

	// Scheme decision
	desiredScheme := effScheme
	needScheme := false
	switch cfg.Scheme {
	case SchemeHTTPS:
		if effScheme != "https" {
			needScheme = true
			desiredScheme = "https"
		}
	case SchemeHTTP:
		if effScheme != "http" {
			needScheme = true
			desiredScheme = "http"
		}
	}

	// Host decision
	needWWW := false
	targetHost := effHost

	if cfg.WWW != WWWOff && cfg.CanonicalHost != "" {
		effHostname, effPort, effHasPort := splitHost(effHost)
		effHostnameLower := strings.ToLower(effHostname)
		// Normalize IPv6 without brackets already handled by splitHost

		if isLocalHost(effHostnameLower) {
			// Do not manipulate localhost / loopback hosts
			needWWW = false
		} else {
			switch cfg.WWW {
			case WWWForbidden: // non-www
				canonicalLower := cfg.canonicalHostname // already lower
				if effHostnameLower == "www."+canonicalLower {
					needWWW = true
					targetHost = cfg.CanonicalHost
				} else if effHostnameLower == canonicalLower {
					// Host matches canonical, but check port mismatch
					if effHasPort != cfg.hasPort || effPort != cfg.canonicalPort {
						needWWW = true
						targetHost = cfg.CanonicalHost
					}
				}
				// else: arbitrary subdomain like blog.example.com -> no redirect
			case WWWRequired: // www
				canonicalLower := cfg.canonicalHostname
				baseHost := strings.TrimPrefix(canonicalLower, "www.")
				if effHostnameLower == baseHost {
					needWWW = true
					targetHost = cfg.CanonicalHost
				} else if effHostnameLower == canonicalLower {
					if effHasPort != cfg.hasPort || effPort != cfg.canonicalPort {
						needWWW = true
						targetHost = cfg.CanonicalHost
					}
				}
				// else: other hosts like blog.example.com -> no redirect
			}
		}
	}

	if !needScheme && !needWWW {
		return nil, false
	}

	// If host needs canonicalization, targetHost already set to canonical.
	// If only scheme needs change and www is off, targetHost remains effective host (preserve).
	// If both need change, host is already canonical and scheme is desiredScheme.

	finalHost := targetHost
	// When only scheme needs redirect and www is off, we preserve effective host.
	// When www needs redirect, we already have canonical host.
	// Edge: if scheme needs redirect but host also needs canonicalization (combined), finalHost is canonical host.
	if needWWW {
		finalHost = targetHost
	} else if needScheme {
		finalHost = effHost
	}

	u := &url.URL{
		Scheme:   desiredScheme,
		Host:     finalHost,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	return u, true
}

func effectiveScheme(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			first := strings.Split(proto, ",")[0]
			first = strings.TrimSpace(strings.ToLower(first))
			if first == "https" || first == "http" {
				return first
			}
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func effectiveHost(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if host := r.Header.Get("X-Forwarded-Host"); host != "" {
			first := strings.Split(host, ",")[0]
			first = strings.TrimSpace(first)
			if first != "" {
				return first
			}
		}
	}
	return r.Host
}

// Middleware returns a handler that performs canonical scheme/host redirects
// with a single 308, bypassing /healthz. It uses TargetURL for the decision.
func Middleware(next http.Handler, cfg Config) http.Handler {
	// Fast path: if no redirects are configured, return next directly
	if cfg.Scheme == SchemeOff && cfg.WWW == WWWOff {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if target, ok := TargetURL(r, cfg); ok {
			http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}
