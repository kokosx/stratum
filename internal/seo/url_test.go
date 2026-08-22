package seo

import "testing"

func TestCanonicalNormalization(t *testing.T) {
	cases := []struct {
		name    string
		siteURL string
		origin  string
		path    string
		want    string
	}{
		{"site url preferred over origin", "https://example.com", "https://other.example", "/about", "https://example.com/about"},
		{"site url trailing slash trimmed", "https://example.com/", "", "/about", "https://example.com/about"},
		{"path trailing slash stripped", "https://example.com", "", "/about/", "https://example.com/about"},
		{"root keeps single slash", "https://example.com", "", "/", "https://example.com/"},
		{"empty path becomes root", "https://example.com", "", "", "https://example.com/"},
		{"origin fallback when no site url", "", "http://localhost:8080", "/x", "http://localhost:8080/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Canonical(c.siteURL, c.origin, c.path, ""); got != c.want {
				t.Fatalf("Canonical() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCanonicalOverride(t *testing.T) {
	cases := []struct {
		name     string
		siteURL  string
		path     string
		override string
		want     string
	}{
		// External canonicals are allowed: an absolute override wins verbatim.
		{"external absolute passes through", "https://example.com", "/about", "https://external.example/post", "https://external.example/post"},
		{"absolute override normalised host", "https://example.com", "/about", "https://External.Example/Post/", "https://external.example/Post"},
		{"root-relative override joined to site url", "https://example.com", "/about", "/preferred", "https://example.com/preferred"},
		{"root-relative override trailing slash", "https://example.com", "/about", "/preferred/", "https://example.com/preferred"},
		{"override root", "https://example.com", "/about", "/", "https://example.com/"},
		{"no override falls back to path", "https://example.com", "/about", "", "https://example.com/about"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Canonical(c.siteURL, "", c.path, c.override); got != c.want {
				t.Fatalf("Canonical(override=%q) = %q, want %q", c.override, got, c.want)
			}
		})
	}
}

func TestBaseURLPrefersSiteURL(t *testing.T) {
	if got := BaseURL("https://prod.example", "https://request.example"); got != "https://prod.example" {
		t.Fatalf("BaseURL = %q, want configured site URL", got)
	}
	if got := BaseURL("", "https://request.example"); got != "https://request.example" {
		t.Fatalf("BaseURL = %q, want request origin fallback", got)
	}
	if got := BaseURL("https://prod.example/", ""); got != "https://prod.example" {
		t.Fatalf("BaseURL = %q, want trimmed site URL", got)
	}
}

func TestPaginatedPathSelfCanonical(t *testing.T) {
	cases := []struct {
		path string
		page int
		want string
	}{
		{"/blog", 1, "/blog"},
		{"/blog", 2, "/blog/page/2"},
		{"/blog", 10, "/blog/page/10"},
		{"/", 2, "/page/2"},
		{"/blog/", 2, "/blog/page/2"},
	}
	for _, c := range cases {
		got := PaginatedPath(c.path, c.page)
		if got != c.want {
			t.Fatalf("PaginatedPath(%q, %d) = %q, want %q", c.path, c.page, got, c.want)
		}
	}
	// A paginated page is its own canonical entity: it must never collapse
	// onto the first page.
	if PaginatedPath("/blog", 2) == PaginatedPath("/blog", 1) {
		t.Fatal("page 2 canonical must differ from page 1")
	}
}
