package site

import (
	"strings"
	"testing"
)

func TestValidateSiteURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty allowed", input: "", want: "", wantErr: false},
		{name: "https trailing slash trimmed", input: "https://example.com/", want: "https://example.com", wantErr: false},
		{name: "https path kept", input: "https://example.com/about", want: "https://example.com/about", wantErr: false},
		{name: "http allowed", input: "http://example.com", want: "http://example.com", wantErr: false},
		{name: "no scheme rejected", input: "example.com", wantErr: true},
		{name: "fragment rejected", input: "https://example.com/#x", wantErr: true},
		{name: "query rejected", input: "https://example.com/?x=1", wantErr: true},
		{name: "no host rejected", input: "https://", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateSiteURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestValidateLanguage(t *testing.T) {
	for _, ok := range []string{"en", "pl", "en-US", "EN"} {
		if err := ValidateLanguage(ok); err != nil {
			t.Fatalf("expected %q valid, got %v", ok, err)
		}
	}
	for _, bad := range []string{"", "english", "e", "en_US", "12"} {
		if err := ValidateLanguage(bad); err == nil {
			t.Fatalf("expected %q invalid", bad)
		}
	}
}

func TestValidateTimezone(t *testing.T) {
	for _, ok := range []string{"UTC", "Europe/Warsaw", "America/New_York"} {
		if err := ValidateTimezone(ok); err != nil {
			t.Fatalf("expected %q valid, got %v", ok, err)
		}
	}
	if err := ValidateTimezone("Mars/Phobos"); err == nil {
		t.Fatal("expected invalid timezone rejected")
	}
	if err := ValidateTimezone(""); err == nil {
		t.Fatal("expected empty timezone rejected")
	}
}

func TestValidateRobotsSize(t *testing.T) {
	if err := ValidateRobotsSize(strings.Repeat("x", 32*1024)); err != nil {
		t.Fatalf("boundary size should be allowed: %v", err)
	}
	if err := ValidateRobotsSize(strings.Repeat("x", 32*1024+1)); err == nil {
		t.Fatal("oversized robots should be rejected")
	}
}

func TestBuildRobotsManagedIndexingOn(t *testing.T) {
	got := BuildRobots(RobotsInput{Mode: "managed", IndexingEnabled: true, SitemapEnabled: true, SiteURL: "https://example.com/"})
	want := "User-agent: *\nAllow: /\n\nDisallow: /admin/\n\nSitemap: https://example.com/sitemap.xml\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBuildRobotsManagedIndexingOnNoSiteURL(t *testing.T) {
	got := BuildRobots(RobotsInput{Mode: "managed", IndexingEnabled: true, SitemapEnabled: true, SiteURL: ""})
	want := "User-agent: *\nAllow: /\n\nDisallow: /admin/\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBuildRobotsManagedIndexingOff(t *testing.T) {
	got := BuildRobots(RobotsInput{Mode: "managed", IndexingEnabled: false, SitemapEnabled: true, SiteURL: "https://example.com"})
	want := "User-agent: *\nDisallow: /\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBuildRobotsCustom(t *testing.T) {
	custom := "User-agent: *\nDisallow: /secret/\n"
	got := BuildRobots(RobotsInput{Mode: "custom", Custom: custom})
	if got != custom {
		t.Fatalf("custom robots must be returned verbatim, got %q", got)
	}
}

func TestBuildSpeculationRulesDisabled(t *testing.T) {
	got, err := BuildSpeculationRules("off", "conservative")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("disabled speculation should return empty JSON, got %q", got)
	}
}

func TestBuildSpeculationRulesValid(t *testing.T) {
	got, err := BuildSpeculationRules("prefetch", "moderate")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"prefetch"`) {
		t.Fatalf("expected prefetch key, got %q", got)
	}
	if !strings.Contains(got, `"eagerness":"moderate"`) {
		t.Fatalf("expected eagerness, got %q", got)
	}
	if !strings.Contains(got, `"source":"document"`) {
		t.Fatalf("expected document source, got %q", got)
	}
	if !strings.Contains(got, "/admin/*") || !strings.Contains(got, "data-no-speculate") {
		t.Fatalf("expected exclusions present, got %q", got)
	}
	// The JSON must be valid and must not contain any raw script injection.
	if strings.Contains(got, "<script") {
		t.Fatalf("speculation JSON must not contain script tags, got %q", got)
	}
}

func TestBuildSpeculationRulesRejectsUnknown(t *testing.T) {
	if _, err := BuildSpeculationRules("prerender", "aggressive"); err == nil {
		t.Fatal("unknown eagerness should be rejected")
	}
	if _, err := BuildSpeculationRules("speculate", "conservative"); err == nil {
		t.Fatal("unknown mode should be rejected")
	}
}
