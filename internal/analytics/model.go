package analytics

import (
	"time"

	"github.com/kokosx/stratum/internal/routing"
)

// Resource describes the content identity behind a request.
// It is derived from the immutable route snapshot, not from URL parsing.
type Resource struct {
	Key           string
	Path          string // normalized, as in route snapshot (human readable snapshot)
	RouteType     string // entry, archive, taxonomy, system
	EntryID       string
	RevisionID    string
	ContentTypeID string
	TaxonomyID    string
	TermID        string
}

// BuildResource creates a Resource from a routing.Route and the normalized path.
// It never branches on concrete content type names; routing helpers are used where needed.
func BuildResource(route routing.Route, normalizedPath string) Resource {
	rtype := route.RouteType
	if rtype == "" {
		rtype = "system"
	}
	switch rtype {
	case "entry":
	case "archive":
		if route.TaxonomyID.Valid && route.TermID.Valid {
			rtype = "taxonomy"
		} else {
			rtype = "archive"
		}
	case "taxonomy":
		rtype = "taxonomy"
	default:
		rtype = "system"
	}

	key := ""
	switch rtype {
	case "entry":
		entryID := ""
		revID := ""
		if route.EntryID.Valid {
			entryID = route.EntryID.String
		}
		if route.PublishedRevisionID.Valid {
			revID = route.PublishedRevisionID.String
		}
		if entryID == "" {
			entryID = "unknown"
		}
		if revID == "" {
			revID = "unknown"
		}
		key = "entry/" + entryID + "/revision/" + revID
	case "archive":
		ct := ""
		if route.ContentTypeID.Valid {
			ct = route.ContentTypeID.String
		}
		if ct == "" {
			ct = "unknown"
		}
		np := routing.NormalizePath(normalizedPath)
		key = "archive/" + ct + np
	case "taxonomy":
		tax := ""
		term := ""
		if route.TaxonomyID.Valid {
			tax = route.TaxonomyID.String
		}
		if route.TermID.Valid {
			term = route.TermID.String
		}
		np := routing.NormalizePath(normalizedPath)
		key = "taxonomy/" + tax + "/" + term + np
	default:
		np := routing.NormalizePath(normalizedPath)
		key = "system" + np
	}

	en := ""
	if route.EntryID.Valid {
		en = route.EntryID.String
	}
	rev := ""
	if route.PublishedRevisionID.Valid {
		rev = route.PublishedRevisionID.String
	}
	ct := ""
	if route.ContentTypeID.Valid {
		ct = route.ContentTypeID.String
	}
	tax := ""
	if route.TaxonomyID.Valid {
		tax = route.TaxonomyID.String
	}
	term := ""
	if route.TermID.Valid {
		term = route.TermID.String
	}

	return Resource{
		Key:           key,
		Path:          routing.NormalizePath(normalizedPath),
		RouteType:     rtype,
		EntryID:       en,
		RevisionID:    rev,
		ContentTypeID: ct,
		TaxonomyID:    tax,
		TermID:        term,
	}
}

// BuildSystemResource creates a resource for system paths (robots, sitemap, etc.) or unknown routes.
func BuildSystemResource(normalizedPath string) Resource {
	np := routing.NormalizePath(normalizedPath)
	return Resource{
		Key:       "system" + np,
		Path:      np,
		RouteType: "system",
	}
}

// Observation is the sanitized, typed data passed to the analytics worker.
// It contains NO raw IP, raw UA, raw referrer, cookies, visitor IDs, etc.
type Observation struct {
	Time        time.Time
	Resource    Resource
	IsPageview  bool // true if this represents a human or crawler pageview (GET 200/304)
	Traffic     TrafficSource
	Client      ClientClass
	Crawler     string // empty for human, else normalized crawler family
	CacheHit    bool
	Status      int
	Duration    time.Duration
	Bytes       int64
	Speculative bool

	ReferrerHost string // only hostname, e.g., "example.com" or ""
	UTMSource    string
	UTMMedium    string
	UTMCampaign  string

	FromResource *Resource
}

// TrafficSource enumerates acquisition categories.
type TrafficSource string

const (
	TrafficDirect        TrafficSource = "direct"
	TrafficInternal      TrafficSource = "internal"
	TrafficOrganicSearch TrafficSource = "organic_search"
	TrafficOrganicSocial TrafficSource = "organic_social"
	TrafficAIReferral    TrafficSource = "ai_referral"
	TrafficReferral      TrafficSource = "referral"
	TrafficCampaign      TrafficSource = "campaign"
)

// ClientClass holds coarse browser/os/device/language.
type ClientClass struct {
	Browser  string // Chrome, Safari, Firefox, Edge, Other
	OS       string // Windows, macOS, iOS, Android, Linux, Other
	Device   string // desktop, mobile, tablet, other
	Language string // e.g., "en", "pl", "other"
}

// AllowedDimensions is the controlled set for analytics_dimension_daily.
var AllowedDimensions = map[string]bool{
	"referrer_host": true,
	"utm_source":    true,
	"utm_medium":    true,
	"utm_campaign":  true,
	"browser":       true,
	"os":            true,
	"device":        true,
	"language":      true,
	"crawler":       true,
}
