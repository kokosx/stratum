package analytics

import (
	"sync"
)

// aggregate is the in-memory aggregation state protected by mutex.
// It is swapped on flush.

type siteHourlyAgg struct {
	Requests            int64
	Views               int64
	HumanViews          int64
	CrawlerViews        int64
	Speculative         int64
	CacheHits           int64
	CacheMisses         int64
	Status2xx           int64
	Status3xx           int64
	Status4xx           int64
	Status5xx           int64
	ResponseBytes       int64
	ResponseCount       int64
	ResponseDurationSum int64 // microseconds
	Latency             [7]int64
}

type pageDailyAgg struct {
	Day           string
	Resource      Resource
	Views         int64
	HumanViews    int64
	CrawlerViews  int64
	Direct        int64
	Internal      int64
	OrganicSearch int64
	OrganicSocial int64
	AIReferral    int64
	Referral      int64
	Campaign      int64
	CacheHits     int64
	CacheMisses   int64
	ResponseCount int64
	ResponseBytes int64
	DurationSum   int64
	Latency       [7]int64
}

type dimAgg struct {
	Day       string
	Dimension string
	Value     string
	Count     int64
}

type transAgg struct {
	Day      string
	FromKey  string
	ToKey    string
	FromPath string
	ToPath   string
	Count    int64
}

type aggregates struct {
	mu         sync.Mutex
	siteHourly map[int64]*siteHourlyAgg
	pageDaily  map[string]*pageDailyAgg // key day|resourceKey
	dimension  map[string]*dimAgg       // key day|dim|value
	transition map[string]*transAgg     // key day|from|to
	counter    int                      // observations since last flush
}

func newAggregates() *aggregates {
	return &aggregates{
		siteHourly: make(map[int64]*siteHourlyAgg),
		pageDaily:  make(map[string]*pageDailyAgg),
		dimension:  make(map[string]*dimAgg),
		transition: make(map[string]*transAgg),
	}
}

// add merges one observation into aggregates. Must hold mu or use locked method.
func (a *aggregates) add(obs Observation) {
	// Site hourly (always)
	hour := HourBucket(obs.Time)
	sh := a.siteHourly[hour]
	if sh == nil {
		sh = &siteHourlyAgg{}
		a.siteHourly[hour] = sh
	}
	sh.Requests++
	if obs.Speculative {
		sh.Speculative++
	} else if obs.IsPageview {
		sh.Views++
		if obs.Crawler != "" {
			sh.CrawlerViews++
		} else {
			sh.HumanViews++
		}
	}
	if obs.CacheHit {
		sh.CacheHits++
	} else {
		sh.CacheMisses++
	}
	switch {
	case obs.Status >= 200 && obs.Status < 300:
		sh.Status2xx++
	case obs.Status >= 300 && obs.Status < 400:
		sh.Status3xx++
	case obs.Status >= 400 && obs.Status < 500:
		sh.Status4xx++
	case obs.Status >= 500 && obs.Status < 600:
		sh.Status5xx++
	}
	sh.ResponseBytes += obs.Bytes
	sh.ResponseCount++
	sh.ResponseDurationSum += obs.Duration.Microseconds()
	b := LatencyBucket(obs.Duration)
	if b >= 0 && b < 7 {
		sh.Latency[b]++
	}

	// If not pageview, don't aggregate page/dimension/transition (per spec)
	if !obs.IsPageview || obs.Speculative {
		a.counter++
		return
	}
	// Also, speculative already excluded above, so only human/crawler pageviews continue

	day := DayBucket(obs.Time)
	pageKey := day + "|" + obs.Resource.Key
	pd := a.pageDaily[pageKey]
	if pd == nil {
		pd = &pageDailyAgg{Day: day, Resource: obs.Resource}
		a.pageDaily[pageKey] = pd
	}
	pd.Views++
	if obs.Crawler != "" {
		pd.CrawlerViews++
	} else {
		pd.HumanViews++
	}
	switch obs.Traffic {
	case TrafficDirect:
		pd.Direct++
	case TrafficInternal:
		pd.Internal++
	case TrafficOrganicSearch:
		pd.OrganicSearch++
	case TrafficOrganicSocial:
		pd.OrganicSocial++
	case TrafficAIReferral:
		pd.AIReferral++
	case TrafficReferral:
		pd.Referral++
	case TrafficCampaign:
		pd.Campaign++
	}
	if obs.CacheHit {
		pd.CacheHits++
	} else {
		pd.CacheMisses++
	}
	pd.ResponseCount++
	pd.ResponseBytes += obs.Bytes
	pd.DurationSum += obs.Duration.Microseconds()
	if b >= 0 && b < 7 {
		pd.Latency[b]++
	}

	// Dimensions: 9 allowed dims. For pageview, we record relevant dims.
	// Cardinality protection per dimension handled both in-memory (small batch) and persist-layer DB check.
	addDim := func(dim, val string) {
		if val == "" {
			return
		}
		// In-memory cap: if distinct values for this day/dim already 256 and val not present, map to "other"
		// Count distinct per day/dim in current batch
		// We can approximate by scanning existing dimension keys for this day/dim count
		// If would exceed, use "other"
		if val != "other" {
			distinct := 0
			exists := false
			for k, v := range a.dimension {
				// k format day|dim|value
				// Quick check: if v.Day==day && v.Dimension==dim
				if v.Day == day && v.Dimension == dim {
					distinct++
					if v.Value == val {
						exists = true
					}
				}
				_ = k
			}
			if !exists && distinct >= MaxDimensionCardinality {
				val = "other"
			}
		}
		dimKey := day + "|" + dim + "|" + val
		da := a.dimension[dimKey]
		if da == nil {
			da = &dimAgg{Day: day, Dimension: dim, Value: val}
			a.dimension[dimKey] = da
		}
		da.Count++
	}

	// Always record browser/os/device/language/crawler
	addDim("browser", obs.Client.Browser)
	addDim("os", obs.Client.OS)
	addDim("device", obs.Client.Device)
	addDim("language", obs.Client.Language)
	if obs.Crawler != "" {
		addDim("crawler", obs.Crawler)
	} else {
		// Optionally we could record crawler dimension for human? Not needed.
	}
	// Referrer host only if not internal? But spec says dimension referrer_host for external.
	if obs.ReferrerHost != "" && obs.Traffic != TrafficInternal && obs.Traffic != TrafficDirect {
		addDim("referrer_host", obs.ReferrerHost)
	} else if obs.ReferrerHost != "" && obs.Traffic == TrafficReferral || obs.Traffic == TrafficAIReferral || obs.Traffic == TrafficOrganicSearch || obs.Traffic == TrafficOrganicSocial {
		addDim("referrer_host", obs.ReferrerHost)
	}
	if obs.UTMSource != "" {
		addDim("utm_source", obs.UTMSource)
	}
	if obs.UTMMedium != "" {
		addDim("utm_medium", obs.UTMMedium)
	}
	if obs.UTMCampaign != "" {
		addDim("utm_campaign", obs.UTMCampaign)
	}

	// Transitions
	if obs.FromResource != nil {
		from := *obs.FromResource
		to := obs.Resource
		transKey := day + "|" + from.Key + "|" + to.Key
		tr := a.transition[transKey]
		if tr == nil {
			tr = &transAgg{Day: day, FromKey: from.Key, ToKey: to.Key, FromPath: from.Path, ToPath: to.Path}
			a.transition[transKey] = tr
		}
		tr.Count++
	}

	a.counter++
}

// snapshotAndReset swaps current aggregates with fresh empty ones and returns old.
// Caller holds mu.
func (a *aggregates) snapshotAndReset() (site map[int64]*siteHourlyAgg, page map[string]*pageDailyAgg, dim map[string]*dimAgg, trans map[string]*transAgg) {
	site = a.siteHourly
	page = a.pageDaily
	dim = a.dimension
	trans = a.transition
	a.siteHourly = make(map[int64]*siteHourlyAgg)
	a.pageDaily = make(map[string]*pageDailyAgg)
	a.dimension = make(map[string]*dimAgg)
	a.transition = make(map[string]*transAgg)
	a.counter = 0
	return
}

func (a *aggregates) count() int {
	return a.counter
}

func (a *aggregates) clear() {
	a.siteHourly = make(map[int64]*siteHourlyAgg)
	a.pageDaily = make(map[string]*pageDailyAgg)
	a.dimension = make(map[string]*dimAgg)
	a.transition = make(map[string]*transAgg)
	a.counter = 0
}
