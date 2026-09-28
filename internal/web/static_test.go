package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticCompressionCacheAndFallback(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	zw.Write([]byte("script"))
	zw.Close()
	files := fstest.MapFS{
		"index.html":                {Data: []byte("shell")},
		"assets/app-abcdefgh.js":    {Data: []byte("script")},
		"assets/app-abcdefgh.js.gz": {Data: compressed.Bytes()},
	}
	h := staticHandler(files, []byte("shell"))
	r := httptest.NewRequest("GET", "/assets/app-abcdefgh.js", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("headers: %+v", w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(zr)
	zr.Close()
	if string(data) != "script" {
		t.Fatal("compressed content changed")
	}
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatalf("conditional: %d", w.Code)
	}
	r = httptest.NewRequest("HEAD", "/chat/SES1", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("shell HEAD: %d %+v", w.Code, w.Header())
	}
	r = httptest.NewRequest("GET", "/assets/missing.js", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("missing asset returned shell: %d", w.Code)
	}
	for _, header := range []string{"gzip;q=0", "gzip;q=0, *;q=1", "br", "gzip;q=invalid"} {
		if acceptsGzip(header) {
			t.Fatalf("gzip disabled: %s", header)
		}
	}
}
