package compress

import (
	"bytes"
	"compress/gzip"
	"strconv"
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
// encoding: "br", "gzip", or "" (identity) and whether the result is acceptable.
// It respects q values, uses strconv.ParseFloat, and prefers br over gzip on tie.
// Identity is considered acceptable with q=1 unless explicitly listed or covered by *.
func NegotiateEncoding(header string) (string, bool) {
	if header == "" {
		return "", true
	}
	parts := strings.Split(header, ",")
	const notSet = -1.0
	qBr, qGzip, qIdentity, qStar := notSet, notSet, notSet, notSet
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		segments := strings.Split(p, ";")
		name := strings.TrimSpace(strings.ToLower(segments[0]))
		q := 1.0
		for _, seg := range segments[1:] {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "q=") {
				val := strings.TrimSpace(strings.TrimPrefix(seg, "q="))
				parsed, err := parseQ(val)
				if err != nil {
					q = 0
				} else {
					if parsed < 0 {
						parsed = 0
					}
					if parsed > 1 {
						parsed = 1
					}
					q = parsed
				}
			}
		}
		switch name {
		case "br":
			if q > qBr {
				qBr = q
			}
		case "gzip", "x-gzip":
			if q > qGzip {
				qGzip = q
			}
		case "identity":
			if q > qIdentity {
				qIdentity = q
			}
		case "*":
			if q > qStar {
				qStar = q
			}
		}
	}
	// Apply wildcard where explicit not set
	if qBr == notSet && qStar != notSet {
		qBr = qStar
	}
	if qGzip == notSet && qStar != notSet {
		qGzip = qStar
	}
	if qIdentity == notSet && qStar != notSet {
		qIdentity = qStar
	}
	qStarOrig := qStar
	qIdentityOrig := qIdentity
	// q=0 means not acceptable
	if qBr == 0 {
		qBr = notSet
	}
	if qGzip == 0 {
		qGzip = notSet
	}
	if qIdentity == 0 {
		qIdentity = notSet
	}
	if qStar == 0 {
		qStar = notSet
	}
	// If none acceptable, check fallback to identity (when no star/identity explicitly 0)
	if qBr == notSet && qGzip == notSet && qIdentity == notSet {
		if qStarOrig == 0 || qIdentityOrig == 0 {
			return "", false
		}
		// Fallback to identity for cases like br;q=0 or br;q=garbage where no other acceptable but identity not explicitly denied
		return "", true
	}
	// Pick highest q, tie br > gzip > identity
	bestEnc := ""
	bestQ := notSet
	if qBr != notSet && qBr > bestQ {
		bestEnc = "br"
		bestQ = qBr
	}
	if qGzip != notSet && qGzip > bestQ {
		bestEnc = "gzip"
		bestQ = qGzip
	} else if qGzip != notSet && qGzip == bestQ && bestEnc == "br" {
		// tie br wins, keep br
	}
	if qIdentity != notSet && qIdentity > bestQ {
		bestEnc = ""
		bestQ = qIdentity
	}
	// On tie between br/gzip/identity with same q, br wins per spec
	if qBr != notSet && qBr == bestQ && bestEnc != "br" {
		// if best is gzip or identity tie with br, prefer br
		if qBr == bestQ {
			bestEnc = "br"
		}
	}
	return bestEnc, true
}

func parseQ(s string) (float64, error) {
	// Use strconv.ParseFloat for standards compliance
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// NegotiateEncodingString is a legacy wrapper returning only encoding, for callers that don't need 406 handling.
func NegotiateEncodingString(header string) string {
	enc, _ := NegotiateEncoding(header)
	return enc
}
