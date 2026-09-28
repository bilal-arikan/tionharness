package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var hashedAsset = regexp.MustCompile(`^assets/.+-[A-Za-z0-9_-]{8,}\.[a-z0-9]+$`)

// Static compression is build-time only. API responses and SSE streams never
// pass through this handler, so their flushing and latency remain unchanged.
func staticHandler(files fs.FS, index []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
			err = nil
		}
		var data []byte
		encoding := ""
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			if compressed, err := fs.ReadFile(files, name+".gz"); err == nil {
				data = compressed
				encoding = "gzip"
			}
		}
		if data == nil {
			if name == "index.html" {
				data = index
			} else {
				data, err = fs.ReadFile(files, name)
			}
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Vary", "Accept-Encoding")
		if encoding != "" {
			w.Header().Set("Content-Encoding", encoding)
		}
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if hashedAsset.MatchString(name) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

func acceptsGzip(header string) bool {
	var wildcard bool
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		name := strings.TrimSpace(fields[0])
		if name != "gzip" && name != "*" {
			continue
		}
		quality := 1.0
		for _, field := range fields[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(field), "=")
			if ok && key == "q" {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		if name == "gzip" {
			return quality > 0
		}
		wildcard = quality > 0
	}
	return wildcard
}
