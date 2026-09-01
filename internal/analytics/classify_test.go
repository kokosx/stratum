package analytics

import "testing"

func TestClassifyTraffic(t *testing.T) {
	cases := []struct {
		name      string
		referrer  string
		siteHost  string
		utmSource string
		want      TrafficSource
	}{
		{"google search", "https://www.google.com/search?q=test", "example.com", "", TrafficOrganicSearch},
		{"bing search", "https://www.bing.com/search?q=test", "example.com", "", TrafficOrganicSearch},
		{"facebook social", "https://www.facebook.com/page", "example.com", "", TrafficOrganicSocial},
		{"linkedin social", "https://linkedin.com/in/user", "example.com", "", TrafficOrganicSocial},
		{"chatgpt ai", "https://chatgpt.com/", "example.com", "", TrafficAIReferral},
		{"perplexity ai", "https://www.perplexity.ai/search", "example.com", "", TrafficAIReferral},
		{"claude ai", "https://claude.ai/chat", "example.com", "", TrafficAIReferral},
		{"external unknown referral", "https://example.net/page", "example.com", "", TrafficReferral},
		{"same host internal", "https://example.com/about", "example.com", "", TrafficInternal},
		{"missing referrer direct", "", "example.com", "", TrafficDirect},
		{"utm campaign overrides", "https://www.google.com/", "example.com", "newsletter", TrafficCampaign},
		{"utm medium campaign", "", "example.com", "", TrafficDirect},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := SanitizeReferrerHost(tc.referrer)
			got := ClassifyTraffic(host, tc.siteHost, tc.utmSource, "", "")
			if tc.utmSource != "" {
				got = ClassifyTraffic(host, tc.siteHost, tc.utmSource, "email", "launch")
			} else if tc.name == "utm medium campaign" {
				got = ClassifyTraffic(host, tc.siteHost, "", "email", "launch")
				if got != TrafficCampaign {
					t.Fatalf("want campaign for utm_medium")
				}
				return
			}
			if got != tc.want {
				t.Fatalf("ClassifyTraffic(%q) = %q, want %q", tc.referrer, got, tc.want)
			}
		})
	}
}

func TestClassifyBrowser(t *testing.T) {
	cases := []struct{ ua, want string }{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Chrome"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari"},
		{"Mozilla/5.0 (Windows NT 10.0; rv:121.0) Gecko/20100101 Firefox/121.0", "Firefox"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0", "Edge"},
		{"", "Other"},
		{"completely unknown", "Other"},
	}
	for _, tc := range cases {
		if got := ClassifyBrowser(tc.ua); got != tc.want {
			t.Fatalf("ClassifyBrowser(%q)=%q want %q", tc.ua, got, tc.want)
		}
	}
}

func TestClassifyOS(t *testing.T) {
	cases := []struct{ ua, want string }{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome", "Windows"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari", "macOS"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Version/17.0 Mobile Safari", "iOS"},
		{"Mozilla/5.0 (Linux; Android 13; Pixel) Chrome", "Android"},
		{"Mozilla/5.0 (X11; Linux x86_64) Firefox", "Linux"},
		{"", "Other"},
	}
	for _, tc := range cases {
		if got := ClassifyOS(tc.ua); got != tc.want {
			t.Fatalf("ClassifyOS %q got %q want %q", tc.ua, got, tc.want)
		}
	}
}

func TestClassifyDevice(t *testing.T) {
	if got := ClassifyDevice("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Mobile"); got != "mobile" {
		t.Fatalf("iphone should be mobile got %q", got)
	}
	if got := ClassifyDevice("Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)"); got != "tablet" {
		t.Fatalf("ipad should be tablet got %q", got)
	}
	if got := ClassifyDevice("Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome"); got != "desktop" {
		t.Fatalf("windows should be desktop got %q", got)
	}
}

func TestClassifyLanguage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"pl", "pl"},
		{"en-US,en;q=0.9", "en"},
		{"de-DE", "de"},
		{"", "other"},
		{"*", "other"},
		{"invalid header !!!", "other"},
		{"en-GB,en;q=0.8,pl;q=0.6", "en"},
	}
	for _, tc := range cases {
		if got := ClassifyLanguage(tc.in); got != tc.want {
			t.Fatalf("ClassifyLanguage %q got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestClassifyCrawler(t *testing.T) {
	cases := []struct{ ua, want string }{
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "Googlebot"},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", "Bingbot"},
		{"GPTBot/1.0", "GPTBot"},
		{"ClaudeBot/1.0", "ClaudeBot"},
		{"PerplexityBot/1.0", "PerplexityBot"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome", ""},
		{"some generic crawler", "generic"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ClassifyCrawler(tc.ua); got != tc.want {
			t.Fatalf("ClassifyCrawler %q got %q want %q", tc.ua, got, tc.want)
		}
	}
}

func TestSanitizeReferrerHost(t *testing.T) {
	if got := SanitizeReferrerHost("https://example.com/private/path?q=email@example.com"); got != "example.com" {
		t.Fatalf("host got %q", got)
	}
	if got := SanitizeReferrerHost("https://example.com:8080/path"); got != "example.com" {
		t.Fatalf("port stripped got %q", got)
	}
	if got := SanitizeReferrerHost("https://user:pass@example.com/"); got != "example.com" {
		t.Fatalf("userinfo host should be example.com, got %q", got)
	}
	if got := SanitizeReferrerHost(""); got != "" {
		t.Fatalf("empty should be empty")
	}
	if got := SanitizeReferrerHost("not a url"); got != "" {
		t.Fatalf("invalid url should be empty got %q", got)
	}
}

func TestSanitizeUTM(t *testing.T) {
	if got := SanitizeUTM("newsletter"); got != "newsletter" {
		t.Fatalf("utm got %q", got)
	}
	if got := SanitizeUTM("a\x00b"); got != "" {
		t.Fatalf("control char should reject got %q", got)
	}
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	if got := SanitizeUTM(string(long)); len(got) != 100 {
		t.Fatalf("long utm should be truncated to 100 got %d", len(got))
	}
}

func TestParseUTMFromQuery(t *testing.T) {
	s, m, c := ParseUTMFromQuery("utm_source=newsletter&utm_medium=email&utm_campaign=launch&utm_term=alice@example.com&gclid=SUPER-SECRET")
	if s != "newsletter" || m != "email" || c != "launch" {
		t.Fatalf("utm parse got %q %q %q", s, m, c)
	}
}

func TestIsSpeculative(t *testing.T) {
	if !IsSpeculative("prefetch", "") {
		t.Fatal("prefetch should be speculative")
	}
	if !IsSpeculative("prerender", "") {
		t.Fatal("prerender should be speculative")
	}
	if !IsSpeculative("", "prefetch") {
		t.Fatal("Purpose prefetch should be speculative")
	}
	if IsSpeculative("normal", "") {
		t.Fatal("normal should not be speculative")
	}
}
