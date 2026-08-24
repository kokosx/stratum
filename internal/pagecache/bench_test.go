package pagecache

import "testing"

func BenchmarkPublicPageCacheHit(b *testing.B) {
	b.ReportAllocs()
	c := New()
	e := Entry{HTML: []byte("<html>hello world " + string(make([]byte, 50000)) + "</html>"), Gzip: []byte("gzip"), Brotli: []byte("br"), ETag: `"etag"`, ContentType: "text/html; charset=utf-8"}
	c.Set("key", e, "entry:1", "site")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := c.Get("key"); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkPageCacheTagInvalidate(b *testing.B) {
	b.ReportAllocs()
	c := New()
	for i := 0; i < 100; i++ {
		c.Set("k"+string(rune(i)), Entry{HTML: []byte("x")}, "content-type:post")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.InvalidateTag("content-type:post")
		// refill for next iteration
		for j := 0; j < 100; j++ {
			c.Set("k"+string(rune(j)), Entry{HTML: []byte("x")}, "content-type:post")
		}
	}
}
