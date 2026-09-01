package public

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/analytics"
	"github.com/kokosx/stratum/internal/routing"
)

// AnalyticsRecorder is the minimal analytics interface needed by public handler.
// It avoids importing the concrete service type in handler construction loops.
type AnalyticsRecorder interface {
	Record(analytics.Observation) bool
	Enabled() bool
}

// SetAnalytics wires the analytics service into the public handler.
// Called from serve() composition root.
func (h *Handler) SetAnalytics(a AnalyticsRecorder) {
	h.analytics = a
}

// analyticsResponseWriter captures status and bytes without affecting semantics.
type analyticsResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *analyticsResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *analyticsResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *analyticsResponseWriter) statusCode() int {
	if w.wroteHeader {
		return w.status
	}
	return http.StatusOK
}

func (w *analyticsResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *analyticsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijack not supported")
}

// isPublicPageview determines if request should count as pageview.
// Must match spec 7.
func isPublicPageview(r *http.Request, status int, speculative bool, route *routing.Route) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if speculative {
		return false
	}
	if status != http.StatusOK && status != http.StatusNotModified {
		return false
	}
	// Only 200/304 with known route that is not password/private
	if route != nil {
		if route.Visibility == "password" || route.Visibility == "private" {
			return false
		}
		// If route is redirect/system, not entry/archive/taxonomy pageview
		if route.RouteType == routing.RouteTypeRedirect || route.RouteType == routing.RouteTypeSystem {
			return false
		}
		if route.RouteType != routing.RouteTypeEntry && route.RouteType != routing.RouteTypeArchive {
			// taxonomy routes are stored as archive with taxonomy IDs; treat archive as pageview
			// If Taxonomy present, it's archive anyway.
			// For unknown, don't count.
			if route.TaxonomyID.Valid && route.TermID.Valid {
				// taxonomy archive -> pageview
			} else {
				return false
			}
		}
	}
	// Path exclusions are already handled outside serveCachedPage (media, sitemap, robots, feed, favicon, search)
	// but double-check here for safety if called elsewhere.
	path := routing.NormalizePath(r.URL.Path)
	if path == "/robots.txt" || path == "/sitemap.xml" || path == "/feed.xml" || path == "/favicon.ico" || path == "/search" {
		return false
	}
	if strings.HasPrefix(path, "/media/") || strings.HasPrefix(path, "/admin") || strings.HasPrefix(path, "/_stratum/") {
		return false
	}
	return true
}

// buildObservation creates sanitized observation without persisting raw data.
func (h *Handler) buildObservation(r *http.Request, normalizedPath string, route *routing.Route, status int, bytes int64, duration time.Duration, cacheHit bool) analytics.Observation {
	now := time.Now()
	// Site host for internal referrer detection
	siteHost := ""
	if snap := h.hub.Site.Current(); snap != nil && snap.SiteURL != "" {
		if u, err := url.Parse(snap.SiteURL); err == nil && u.Host != "" {
			siteHost = strings.ToLower(u.Host)
			if idx := strings.Index(siteHost, ":"); idx >= 0 {
				siteHost = siteHost[:idx]
			}
		}
	} else {
		// fallback to request host
		host := strings.ToLower(r.Host)
		if idx := strings.Index(host, ":"); idx >= 0 {
			host = host[:idx]
		}
		siteHost = host
	}

	rawUA := r.Header.Get("User-Agent")
	rawReferer := r.Header.Get("Referer")
	// Also consider Referrer misspelling? Go canonical is Referer
	if rawReferer == "" {
		rawReferer = r.Header.Get("Referrer")
	}
	rawLang := r.Header.Get("Accept-Language")
	secPurpose := r.Header.Get("Sec-Purpose")
	purpose := r.Header.Get("Purpose")
	rawQuery := r.URL.RawQuery

	client := analytics.ClientClass{
		Browser:  analytics.ClassifyBrowser(rawUA),
		OS:       analytics.ClassifyOS(rawUA),
		Device:   analytics.ClassifyDevice(rawUA),
		Language: analytics.ClassifyLanguage(rawLang),
	}
	crawler := analytics.ClassifyCrawler(rawUA)
	speculative := analytics.IsSpeculative(secPurpose, purpose)
	referrerHost := analytics.SanitizeReferrerHost(rawReferer)
	utmSource, utmMedium, utmCampaign := analytics.ParseUTMFromQuery(rawQuery)
	traffic := analytics.ClassifyTraffic(referrerHost, siteHost, utmSource, utmMedium, utmCampaign)

	// Resource identity
	var res analytics.Resource
	if route != nil {
		res = analytics.BuildResource(*route, normalizedPath)
	} else {
		// Try lookup route from snapshot for path (might be cache-hit case where route already looked up)
		if rt, ok := h.hub.Routes.Lookup(routing.NormalizePath(normalizedPath)); ok {
			res = analytics.BuildResource(rt, normalizedPath)
		} else {
			res = analytics.BuildSystemResource(normalizedPath)
		}
	}

	isPageview := isPublicPageview(r, status, speculative, route)

	// Transition: if internal referrer, resolve from resource
	var fromRes *analytics.Resource
	if traffic == analytics.TrafficInternal && rawReferer != "" {
		if u, err := url.Parse(rawReferer); err == nil && u.Path != "" {
			refPath := routing.NormalizePath(u.Path)
			// Use snapshot lookup for referrer path, zero DB
			if rt, ok := h.hub.Routes.Lookup(refPath); ok {
				fr := analytics.BuildResource(rt, refPath)
				// Only if referrer route is public page (not redirect/private)
				if rt.RouteType == routing.RouteTypeEntry || rt.RouteType == routing.RouteTypeArchive || (rt.TaxonomyID.Valid && rt.TermID.Valid) {
					fromRes = &fr
				} else if rt.RouteType == "archive" {
					fromRes = &fr
				}
			} else {
				// Referrer path not known; no transition
			}
		}
	}

	obs := analytics.Observation{
		Time:         now,
		Resource:     res,
		IsPageview:   isPageview,
		Traffic:      traffic,
		Client:       client,
		Crawler:      crawler,
		CacheHit:     cacheHit,
		Status:       status,
		Duration:     duration,
		Bytes:        bytes,
		Speculative:  speculative,
		ReferrerHost: referrerHost,
		UTMSource:    utmSource,
		UTMMedium:    utmMedium,
		UTMCampaign:  utmCampaign,
		FromResource: fromRes,
	}
	return obs
}

// recordPublic handles non-blocking analytics recording. Must not perform DB I/O.
func (h *Handler) recordPublic(r *http.Request, normalizedPath string, route *routing.Route, status int, bytes int64, duration time.Duration, cacheHit bool) {
	if h.analytics == nil || !h.analytics.Enabled() {
		return
	}
	obs := h.buildObservation(r, normalizedPath, route, status, bytes, duration, cacheHit)
	h.analytics.Record(obs)
}
