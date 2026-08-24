package compress

import (
	"bytes"
	"compress/gzip"
	"strings"

	"github.com/andybalholm/brotli"
)

// Gzip compresses src at level 6 (stronger than BestSpeed, off hot hit path).
func Gzip(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, 6)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(src); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Brotli compresses src at quality 6 (good size/CPU trade-off, publish-time cost).
func Brotli(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, 6)
	if _, err := w.Write(src); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Artifact holds precompressed representations for a cacheable body.
type Artifact struct {
	Raw    []byte
	Gzip   []byte
	Brotli []byte
	ETag   string
}

// CompressArtifact builds both compressed forms from raw.
func CompressArtifact(raw []byte, etag string) Artifact {
	gz, _ := Gzip(raw)
	br, _ := Brotli(raw)
	return Artifact{Raw: raw, Gzip: gz, Brotli: br, ETag: etag}
}

// NegotiateEncoding parses Accept-Encoding and returns the best supported
// encoding: "br", "gzip", or "" (identity). It respects q values (q=0 means
// not acceptable) and prefers br over gzip on tie.
func NegotiateEncoding(header string) string {
	if header == "" {
		return ""
	}
	// Split by comma into tokens.
	parts := strings.Split(header, ",")
	type enc struct {
		name string
		q    float64
	}
	var encs []enc
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// split token; encoding; q=...
		segments := strings.Split(p, ";")
		name := strings.TrimSpace(strings.ToLower(segments[0]))
		q := 1.0
		for _, seg := range segments[1:] {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "q=") {
				val := strings.TrimPrefix(seg, "q=")
				// parse float
				if val == "0" || val == "0." || val == "0.0" {
					q = 0
				} else {
					// simple parse: handle 0.xxx, 1, 1.0
					// Use manual parse to avoid strconv overhead? Use general.
					switch val {
					case "1", "1.", "1.0":
						q = 1
					default:
						// try parse with strings – fallback to 0 if invalid
						// small fast parse
						if len(val) > 0 && val[0] == '0' {
							// 0.xxx
							if len(val) >= 2 && val[1] == '.' {
								// parse fractional part
								q = 0
								div := 1.0
								for _, ch := range val[2:] {
									if ch < '0' || ch > '9' {
										q = 0
										break
									}
									div *= 10
									q += float64(ch-'0') / div
								}
							} else {
								q = 0
							}
						} else if val == "1" {
							q = 1
						} else {
							q = 0
						}
					}
				}
			}
		}
		encs = append(encs, enc{name: name, q: q})
	}
	// Determine best among br, gzip, * for each.
	bestBR := -1.0
	bestGzip := -1.0
	hasStar := false
	starQ := 0.0
	for _, e := range encs {
		switch e.name {
		case "br":
			if e.q > bestBR {
				bestBR = e.q
			}
		case "gzip", "x-gzip":
			if e.q > bestGzip {
				bestGzip = e.q
			}
		case "*":
			hasStar = true
			if e.q > starQ {
				starQ = e.q
			}
		case "identity":
			// identity never compressed; we return "" for it
		}
	}
	// If * present, it implies both if not explicitly listed.
	if bestBR < 0 && hasStar {
		bestBR = starQ
	}
	if bestGzip < 0 && hasStar {
		bestGzip = starQ
	}
	// Respect q=0 (not acceptable)
	if bestBR == 0 {
		bestBR = -1
	}
	if bestGzip == 0 {
		bestGzip = -1
	}
	// Prefer higher q; on tie prefer br.
	if bestBR > bestGzip {
		return "br"
	}
	if bestGzip > bestBR {
		return "gzip"
	}
	if bestBR >= 0 && bestGzip >= 0 && bestBR == bestGzip {
		// tie -> br
		return "br"
	}
	if bestBR >= 0 {
		return "br"
	}
	if bestGzip >= 0 {
		return "gzip"
	}
	return ""
}
