package tiddlywikid

import (
	"net/http"
	"path"
)

type Mux struct {
	base string
	mu   *http.ServeMux
}

func NewRootMux() *Mux {
	return &Mux{
		base: "",
		mu:   http.NewServeMux(),
	}
}

func NewMux(base string) *Mux {
	url := path.Clean(path.Join("/", base) + "/")
	return &Mux{
		base: url,
		mu:   http.NewServeMux(),
	}
}

func (mux *Mux) NewSubMux(pattern string) *Mux {
	url := path.Clean(path.Join(mux.base, pattern) + "/")
	return &Mux{
		base: url,
		mu:   mux.mu,
	}
}

func (mux *Mux) Handle(pattern string, handler http.Handler) {
	mux.mu.Handle(mux.base+pattern, handler)
}

func (mux *Mux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.mu.HandleFunc(mux.base+pattern, handler)
}

func (mux *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux.mu.ServeHTTP(w, r)
}

func (mux *Mux) StripPrefix(prefix string, h http.Handler) http.Handler {
	return http.StripPrefix(mux.base+prefix, h)
}
