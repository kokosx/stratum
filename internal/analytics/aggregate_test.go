package analytics

import (
	"testing"
	"time"
)

func TestAggregatePageviewCounters(t *testing.T) {
	aggs := newAggregates()
	obs := Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/hello", RouteType: "entry", EntryID: "e1", RevisionID: "r1", ContentTypeID: "page"},
		IsPageview: true,
		Traffic:    TrafficDirect,
		Client:     ClientClass{Browser: "Chrome", OS: "Windows", Device: "desktop", Language: "en"},
		Status:     200,
		Duration:   30 * time.Millisecond,
		Bytes:      1234,
	}
	aggs.add(obs)
	aggs.add(obs) // second same
	if len(aggs.pageDaily) != 1 {
		t.Fatalf("expected 1 page row, got %d", len(aggs.pageDaily))
	}
	for _, v := range aggs.pageDaily {
		if v.Views != 2 || v.HumanViews != 2 {
			t.Fatalf("views %d human %d", v.Views, v.HumanViews)
		}
		if v.Direct != 2 {
			t.Fatalf("direct %d", v.Direct)
		}
		if v.Latency[2] != 2 { // 30ms -> lt50 bucket index 2
			t.Fatalf("latency bucket %v", v.Latency)
		}
	}
	// site hourly
	if len(aggs.siteHourly) != 1 {
		t.Fatalf("site hourly len %d", len(aggs.siteHourly))
	}
	for _, v := range aggs.siteHourly {
		if v.Views != 2 || v.HumanViews != 2 {
			t.Fatalf("site views %d", v.Views)
		}
		if v.Status2xx != 2 {
			t.Fatalf("2xx %d", v.Status2xx)
		}
	}
	// dimensions
	if len(aggs.dimension) == 0 {
		t.Fatal("expected dimensions")
	}
	// check browser dimension
	found := false
	for _, d := range aggs.dimension {
		if d.Dimension == "browser" && d.Value == "Chrome" && d.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("browser dimension not found")
	}
}

func TestAggregateCrawlerSeparation(t *testing.T) {
	aggs := newAggregates()
	human := Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry"},
		IsPageview: true,
		Client:     ClientClass{Browser: "Chrome"},
		Status:     200, Duration: 10 * time.Millisecond,
	}
	crawler := Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry"},
		IsPageview: true,
		Crawler:    "Googlebot",
		Client:     ClientClass{Browser: "Other"},
		Status:     200, Duration: 10 * time.Millisecond,
	}
	aggs.add(human)
	aggs.add(crawler)
	for _, v := range aggs.pageDaily {
		if v.HumanViews != 1 || v.CrawlerViews != 1 || v.Views != 2 {
			t.Fatalf("human %d crawler %d views %d", v.HumanViews, v.CrawlerViews, v.Views)
		}
	}
	for _, v := range aggs.siteHourly {
		if v.HumanViews != 1 || v.CrawlerViews != 1 {
			t.Fatalf("site human %d crawler %d", v.HumanViews, v.CrawlerViews)
		}
	}
}

func TestAggregateTransitions(t *testing.T) {
	aggs := newAggregates()
	from := Resource{Key: "entry/e1/revision/r1", Path: "/"}
	to := Resource{Key: "entry/e2/revision/r2", Path: "/about"}
	obs := Observation{
		Time:       time.Now(),
		Resource:   to,
		IsPageview: true,
		Traffic:    TrafficInternal,
		Status:     200, Duration: 5 * time.Millisecond,
		FromResource: &from,
		Client:       ClientClass{Browser: "Chrome"},
	}
	aggs.add(obs)
	aggs.add(obs)
	if len(aggs.transition) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(aggs.transition))
	}
	for _, v := range aggs.transition {
		if v.Count != 2 {
			t.Fatalf("count %d", v.Count)
		}
	}
}

func TestSpeculativeNotPageview(t *testing.T) {
	aggs := newAggregates()
	obs := Observation{
		Time:        time.Now(),
		Resource:    Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry"},
		IsPageview:  true,
		Speculative: true,
		Status:      200, Duration: 10 * time.Millisecond,
		Client: ClientClass{Browser: "Chrome"},
	}
	// But speculative should not be counted as pageview in aggs.add logic - we mark IsPageview true but speculative true -> should not increment page?
	// Actually add() checks speculative: if speculative then not page daily. We set IsPageview true but add will treat as not pageview because Speculative flag.
	aggs.add(obs)
	if len(aggs.pageDaily) != 0 {
		t.Fatalf("speculative should not create page row, got %d", len(aggs.pageDaily))
	}
	for _, v := range aggs.siteHourly {
		if v.Speculative != 1 {
			t.Fatalf("speculative count %d", v.Speculative)
		}
		if v.Views != 0 {
			t.Fatalf("views should be 0 for speculative, got %d", v.Views)
		}
	}
}
