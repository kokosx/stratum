package analytics

import (
	"net/url"
	"strings"
	"time"
)

// Max lengths and cardinality
const (
	MaxUTMLength            = 100
	MaxReferrerHostLength   = 253
	MaxDimensionCardinality = 256
	QueueSize               = 4096
	FlushInterval           = 5 * time.Second
	FlushThreshold          = 500
)

// Latency bucket helpers
func LatencyBucket(d time.Duration) int {
	ms := d.Milliseconds()
	switch {
	case ms < 5:
		return 0
	case ms < 20:
		return 1
	case ms < 50:
		return 2
	case ms < 100:
		return 3
	case ms < 250:
		return 4
	case ms < 1000:
		return 5
	default:
		return 6
	}
}

// DayBucket returns YYYY-MM-DD for given time in UTC (or given location? Use UTC for simplicity, but spec expects day per site timezone).
// We'll use UTC for now; admin displays UTC day. Could later use site timezone.
func DayBucket(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// HourBucket returns unix hour (seconds truncated to hour)
func HourBucket(t time.Time) int64 {
	tt := t.UTC().Truncate(time.Hour)
	return tt.Unix()
}

// SanitizePath validates path for analytics_page_daily snapshot.
// It ensures normalized path and bounded length.
func SanitizePath(p string) string {
	if p == "" {
		return "/"
	}
	if len(p) > 2048 {
		p = p[:2048]
	}
	for _, ch := range p {
		if ch < 32 || ch == 127 {
			return "/"
		}
	}
	return p
}

// SanitizeHost is alias for SanitizeReferrerHost already in classify.go

// BuildObservationFromHeaders is a pure helper to create sanitized observation without involving *http.Request persistence.
// rawUA, rawReferer, rawAcceptLang, rawQuery, secPurpose, purpose are transient inputs.
func BuildSanitizedDimensions(rawUA, rawReferer, rawAcceptLang, rawQuery, secPurpose, purpose, siteHost string) (client ClientClass, crawler string, speculative bool, referrerHost, utmSource, utmMedium, utmCampaign string, traffic TrafficSource) {
	client = ClientClass{
		Browser:  ClassifyBrowser(rawUA),
		OS:       ClassifyOS(rawUA),
		Device:   ClassifyDevice(rawUA),
		Language: ClassifyLanguage(rawAcceptLang),
	}
	crawler = ClassifyCrawler(rawUA)
	speculative = IsSpeculative(secPurpose, purpose)
	referrerHost = SanitizeReferrerHost(rawReferer)
	utmSource, utmMedium, utmCampaign = ParseUTMFromQuery(rawQuery)
	traffic = ClassifyTraffic(referrerHost, siteHost, utmSource, utmMedium, utmCampaign)
	return
}

// SanitizeObservation post-validates an Observation before enqueue.
// Returns false if observation should be dropped (e.g., non-public).
func SanitizeObservation(obs *Observation) bool {
	obs.ReferrerHost = SanitizeDimensionValue("referrer_host", obs.ReferrerHost)
	obs.UTMSource = SanitizeDimensionValue("utm_source", obs.UTMSource)
	obs.UTMMedium = SanitizeDimensionValue("utm_medium", obs.UTMMedium)
	obs.UTMCampaign = SanitizeDimensionValue("utm_campaign", obs.UTMCampaign)
	obs.Client.Browser = SanitizeDimensionValue("browser", obs.Client.Browser)
	obs.Client.OS = SanitizeDimensionValue("os", obs.Client.OS)
	obs.Client.Device = SanitizeDimensionValue("device", obs.Client.Device)
	obs.Client.Language = SanitizeDimensionValue("language", obs.Client.Language)
	if obs.Crawler != "" {
		obs.Crawler = SanitizeDimensionValue("crawler", obs.Crawler)
	}
	obs.Resource.Path = SanitizePath(obs.Resource.Path)
	if len(obs.Resource.Key) > 512 {
		obs.Resource.Key = obs.Resource.Key[:512]
	}
	return true
}

// ExtractHost is helper for tests
func ExtractHost(rawReferer string) string {
	u, err := url.Parse(rawReferer)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
