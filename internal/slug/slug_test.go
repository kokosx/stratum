package slug

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"polish", "Zażółć gęślą jaźń", "zazolc-gesla-jazn"},
		{"latin accents", "Café Déjà Vu", "cafe-deja-vu"},
		{"spaces trim", " Hello   World ", "hello-world"},
		{"only punctuation", "---", ""},
		{"underscore", "hello_world", "hello-world"},
		{"multiple separators", "a   --__  b", "a-b"},
		{"empty", "", ""},
		{"only spaces", "   ", ""},
		{"uppercase", "HELLO WORLD", "hello-world"},
		{"already good", "hello-world", "hello-world"},
		{"punctuation", "Hello, World! How are you?", "hello-world-how-are-you"},
		{"numbers", "post 123", "post-123"},
		{"mixed diacritics", "ÀÁÂÃÄÅ Æ", "aaaaaa-ae"},
		{"polish chars", "ąćęłńóśźż", "acelnoszz"},
		{"ß", "Straße", "strasse"},
		{"slashes", "a/b/c", "a-b-c"},
		{"dots", "a.b.c", "a-b-c"},
		{"trailing dash trim", "-hello-", "hello"},
		{"collapse dashes", "a---b", "a-b"},
		{"non-latin chinese", "你好世界", ""},
		{"emoji", "hello 🌍 world", "hello-world"},
		{"mixed non-latin + latin", "hello 你好 world", "hello-world"},
		{"long input truncated", strings.Repeat("a", 150), strings.Repeat("a", 100)},
		{"long with dash boundary", strings.Repeat("a", 99) + "-b", strings.Repeat("a", 99)},
		{"preserve numbers and hyphens", "2024-01-01", "2024-01-01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Slugify(tc.in)
			if got != tc.want {
				t.Fatalf("Slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSlugifyDeterministic(t *testing.T) {
	in := "Zażółć gęślą jaźń Café"
	a := Slugify(in)
	b := Slugify(in)
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
}

func TestSlugifyNoTrailingDashAfterTruncate(t *testing.T) {
	// Input that after Slugify would exceed max and cut on a dash.
	in := strings.Repeat("a", 99) + " " + strings.Repeat("b", 20)
	got := Slugify(in)
	if strings.HasSuffix(got, "-") {
		t.Fatalf("trailing dash: %q", got)
	}
	if len(got) > MaxLength {
		t.Fatalf("exceeds MaxLength: %d > %d: %q", len(got), MaxLength, got)
	}
}

func TestSlugifyMaxLength(t *testing.T) {
	if MaxLength != 100 {
		t.Fatalf("MaxLength = %d, want 100", MaxLength)
	}
}

func TestNonLatinFallbackPolicy(t *testing.T) {
	// Server returns "" for pure non-Latin; callers must provide a fallback
	// (e.g., "item"/"term"/unique slug) so we never produce a meaningless empty slug silently.
	if got := Slugify("你好"); got != "" {
		t.Fatalf("expected empty for non-latin, got %q", got)
	}
	if got := Slugify("---"); got != "" {
		t.Fatalf("expected empty for punctuation, got %q", got)
	}
}
