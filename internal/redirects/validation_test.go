package redirects

import "testing"

func TestNormalizeSourceRejectsQueryAndFragment(t *testing.T) {
	if _, err := NormalizeSource("/foo?x=1"); err == nil {
		t.Fatalf("source with query should be rejected")
	}
	if _, err := NormalizeSource("/foo#section"); err == nil {
		t.Fatalf("source with fragment should be rejected")
	}
	if _, err := NormalizeSource("/\\evil"); err == nil {
		t.Fatalf("source with backslash should be rejected")
	}
	if _, err := NormalizeSource("/foo\x01bar"); err == nil {
		t.Fatalf("source with control char should be rejected")
	}
	if _, err := NormalizeSource("/foo\\bar"); err == nil {
		t.Fatalf("source with backslash should be rejected")
	}
	if _, err := NormalizeSource("foo"); err == nil {
		t.Fatalf("source without leading slash should be rejected")
	}
	if _, err := NormalizeSource("//evil"); err == nil {
		t.Fatalf("source with // should be rejected")
	}
	// Valid
	if _, err := NormalizeSource("/valid-path"); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	if _, err := NormalizeSource("/valid/path"); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
}

func TestNormalizeTarget(t *testing.T) {
	// Valid internal
	if _, err := NormalizeTarget("/internal-path"); err != nil {
		t.Fatalf("valid internal target rejected: %v", err)
	}
	if out, err := NormalizeTarget("/internal-path?source=old"); err != nil {
		t.Fatalf("internal with query rejected: %v", err)
	} else {
		if out != "/internal-path?source=old" {
			t.Fatalf("internal with query normalized incorrectly: %q", out)
		}
	}
	if _, err := NormalizeTarget("https://example.com/path"); err != nil {
		t.Fatalf("external https rejected: %v", err)
	}
	if _, err := NormalizeTarget("http://example.com/path"); err != nil {
		t.Fatalf("external http rejected: %v", err)
	}
	// Invalid
	if _, err := NormalizeTarget("//evil.example"); err == nil {
		t.Fatalf("// target should be rejected")
	}
	if _, err := NormalizeTarget("/\\evil.example"); err == nil {
		t.Fatalf("/\\ target should be rejected")
	}
	if _, err := NormalizeTarget("javascript:alert(1)"); err == nil {
		t.Fatalf("javascript: should be rejected")
	}
	if _, err := NormalizeTarget("data:text/html,hi"); err == nil {
		t.Fatalf("data: should be rejected")
	}
	if _, err := NormalizeTarget("vbscript:msgbox"); err == nil {
		t.Fatalf("vbscript: should be rejected")
	}
	if _, err := NormalizeTarget("/path\x02withcontrol"); err == nil {
		t.Fatalf("control char should be rejected")
	}
	if _, err := NormalizeTarget("/path\\withbackslash"); err == nil {
		t.Fatalf("backslash should be rejected")
	}
	if _, err := NormalizeTarget("/path#frag"); err == nil {
		t.Fatalf("fragment should be rejected for internal")
	}
	// Internal target with query should be normalized on path only, not including query in NormalizePath
	if out, err := NormalizeTarget("/blog/?source=old"); err != nil {
		t.Fatalf("target /blog/? rejected: %v", err)
	} else {
		// NormalizePath would strip trailing slash from /blog/ to /blog, but preserve query
		if out != "/blog?source=old" {
			t.Fatalf("unexpected normalized target %q", out)
		}
	}
}
