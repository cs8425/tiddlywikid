package tiddlywikid

import (
	"net/http"
	"time"

	"tiddlywikid/store"
	"tiddlywikid/utils"
)

const (
	AUTO_GENERATED_NOW = "$:/sync-time"
	AUTO_GENERATED_IP  = "$:/client-ip"
)

var (
	autoGen     = make(map[string]AutoGenFn)
	autoGenTags = (*store.TiddlerTags)(&[]string{
		"auto-generated",
	})
)

type AutoGenFn func(r *http.Request) *store.TiddlyWebJSON

func init() {
	autoGen = map[string]AutoGenFn{
		AUTO_GENERATED_NOW: NewNow,
		AUTO_GENERATED_IP:  NewSrcIP,
	}
}

func NewNow(r *http.Request) *store.TiddlyWebJSON {
	return &store.TiddlyWebJSON{
		Title: AUTO_GENERATED_NOW,
		Text:  utils.Now().Format(time.RFC3339Nano),
		Tags:  autoGenTags,
		Type:  "text/vnd.tiddlywiki",
	}
}

func NewSrcIP(r *http.Request) *store.TiddlyWebJSON {
	return &store.TiddlyWebJSON{
		Title: AUTO_GENERATED_IP,
		Text:  r.RemoteAddr,
		Tags:  autoGenTags,
		Type:  "text/vnd.tiddlywiki",
	}
}
