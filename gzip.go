package tiddlywikid

import (
	"compress/gzip"
	"flag"
	"net/http"
	"strings"
	"sync"
)

var (
	gzipLv = flag.Int("gz", 2, "gzip disable = 0, DefaultCompression = -1, BestSpeed = 1, BestCompression = 9")

	gzWiterPool sync.Pool
)

type GzipResponseWriter struct {
	http.ResponseWriter
	gzip *gzip.Writer
}

func (w *GzipResponseWriter) Write(p []byte) (int, error) {
	if w.gzip == nil {
		return w.ResponseWriter.Write(p)
	}

	return w.gzip.Write(p)
}

func (w *GzipResponseWriter) Close() error {
	if w.gzip != nil {
		err := w.gzip.Close()
		gzWiterPool.Put(w.gzip)
		return err
	}
	return nil
}

func CanAcceptsGzip(r *http.Request) bool {
	s := strings.ToLower(r.Header.Get("Accept-Encoding"))
	for _, ss := range strings.Split(s, ",") {
		if strings.HasPrefix(ss, "gzip") {
			return true
		}
	}
	return false
}

func TryGzipResponse(w http.ResponseWriter, r *http.Request) *GzipResponseWriter {
	if !CanAcceptsGzip(r) || *gzipLv == 0 {
		return nil
	}

	gw, ok := gzWiterPool.Get().(*gzip.Writer)
	if ok {
		gw.Reset(w)
	} else {
		var err error
		gw, err = gzip.NewWriterLevel(w, *gzipLv)
		if err != nil {
			gw = gzip.NewWriter(w)
		}
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Del("Content-Length")

	return &GzipResponseWriter{w, gw}
}
