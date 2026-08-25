package compress

import "testing"

func TestNegotiateEncodingTable(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"", "", true},
		{"br", "br", true},
		{"gzip", "gzip", true},
		{"br, gzip", "br", true},
		{"br;q=1.00", "br", true},
		{"gzip;q=0.7, br;q=0.5", "gzip", true},
		{"identity;q=1, br;q=0.5", "", true},
		{"identity;q=0.1, br;q=0.8", "br", true},
		{"br;q=0,gzip;q=1", "gzip", true},
		{"br;q=0,gzip;q=0", "", true},
		{"br;q=0,gzip;q=0,identity;q=0", "", false},
		{"*", "br", true},
		{"*;q=0", "", false},
		{"gzip;q=0.5,*;q=0.8", "br", true},
		{"br;q=garbage", "", true},           // garbage q=0, so br not acceptable, fallback to identity
		{"br;q=0.5, gzip;q=0.5", "br", true}, // tie br wins
	}
	for _, tc := range tests {
		got, ok := NegotiateEncoding(tc.header)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("header %q => got %q ok=%v want %q ok=%v", tc.header, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNegotiateEncodingQ1(t *testing.T) {
	if enc, ok := NegotiateEncoding("br;q=1.00"); !ok || enc != "br" {
		t.Fatalf("br;q=1.00 => %q %v", enc, ok)
	}
}

func TestNegotiateEncodingIdentityPreferred(t *testing.T) {
	enc, ok := NegotiateEncoding("identity;q=1, br;q=0.5")
	if !ok || enc != "" {
		t.Fatalf("identity preferred => %q %v", enc, ok)
	}
	enc, ok = NegotiateEncoding("identity;q=0.1, br;q=0.8")
	if !ok || enc != "br" {
		t.Fatalf("br should win => %q", enc)
	}
}
