// Package cssmin minifies CSS with a stable pure-Go minifier.
// Callers must hash the returned bytes (never the source) for fingerprints.
package cssmin

import (
	"bytes"
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
)

var (
	once sync.Once
	m    *minify.M
)

func minifier() *minify.M {
	once.Do(func() {
		m = minify.New()
		m.AddFunc("text/css", css.Minify)
	})
	return m
}

// CSS returns minified CSS. On failure it returns the original input so a
// minify error never breaks page rendering.
func CSS(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	var buf bytes.Buffer
	if err := minifier().Minify("text/css", &buf, bytes.NewReader(src)); err != nil {
		return src
	}
	return buf.Bytes()
}

// CSSString is a convenience wrapper around CSS.
func CSSString(src string) string {
	return string(CSS([]byte(src)))
}
