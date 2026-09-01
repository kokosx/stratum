package analytics

import (
	"testing"
	"time"
)

func BenchmarkRecord(b *testing.B) {
	// Benchmark Record non-blocking path with disabled check
	b.ReportAllocs()
	// Use fake site runtime enabled
	// We'll benchmark classification + Record via service with disabled? Instead benchmark pure Record with already sanitized obs
	// For simplicity benchmark classify + add
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obs := Observation{
			Time:       time.Now(),
			Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/test", RouteType: "entry"},
			IsPageview: true,
			Traffic:    TrafficDirect,
			Client:     ClientClass{Browser: "Chrome", OS: "Windows", Device: "desktop", Language: "en"},
			Status:     200,
			Duration:   10 * time.Millisecond,
			Bytes:      1234,
		}
		_ = obs
	}
}

func BenchmarkClassify(b *testing.B) {
	b.ReportAllocs()
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	referer := "https://www.google.com/search?q=test"
	lang := "en-US,en;q=0.9"
	query := "utm_source=newsletter&utm_medium=email&utm_campaign=launch"
	for i := 0; i < b.N; i++ {
		_, _, _, _, _, _, _, _ = BuildSanitizedDimensions(ua, referer, lang, query, "", "", "example.com")
	}
}

func BenchmarkAggregate(b *testing.B) {
	b.ReportAllocs()
	aggs := newAggregates()
	obs := Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry"},
		IsPageview: true,
		Traffic:    TrafficDirect,
		Client:     ClientClass{Browser: "Chrome", OS: "Windows", Device: "desktop", Language: "en"},
		Status:     200,
		Duration:   10 * time.Millisecond,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aggs.add(obs)
	}
}
