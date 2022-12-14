package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	api "tiddlywikid"
	authpkg "tiddlywikid/auth"
	session "tiddlywikid/session"
	storepkg "tiddlywikid/store"
)

var (
	readTimeout  = flag.Int("rt", 5, "http ReadTimeout (Second), <= 0 disable")
	writeTimeout = flag.Int("wt", 0, "http WriteTimeout (Second), <= 0 disable")

	verbosity = flag.Int("v", 3, "verbosity")
	port      = flag.String("l", ":4040", "bind port")
	dir       = flag.String("d", "./files", "static file directory")
	wikiBase  = flag.String("base", "index.html", "base TiddlyWiki file")

	// json for dev
	dbType    = flag.String("db", "json", "store type (json, bitcask)")
	dbStore   = flag.String("store", "tiddlersDb.json", "store path")
	authStore = flag.String("auth", "user.json", "all user in a json file (for dev)")

	syncStoryList = flag.Bool("sync-story-sequence", false, "save and put $:/StoryList and $:/HistoryList, will cause some issue when multi-user/multi-window")

	crtFile = flag.String("crt", "", "https certificate file")
	keyFile = flag.String("key", "", "https private key file")

	// for dev
	doHash = flag.String("hash", "", "hash a password")
)

func reqAtom(atomNext *atomic.Value) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next, ok := atomNext.Load().(http.Handler)
		if !ok {
			http.Error(w, "handler error!", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func reqlog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Vln(3, r.Method, r.URL, r.RemoteAddr, r.Host)
		if *verbosity >= 6 {
			for i, hdr := range r.Header {
				Vln(6, "---", i, len(hdr), hdr)
			}
			Vln(6, "")
		}
		// w.Header().Add("Service-Worker-Allowed", "/")
		gzw := api.TryGzipResponse(w, r)
		if gzw != nil {
			defer gzw.Close()
			next.ServeHTTP(gzw, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

func main() {
	flag.Parse()

	if *doHash != "" {
		u := &authpkg.User{}
		u.SetPwd(*doHash)
		Vln(0, "[hash]", u.Hash)
		return
	}

	// store not reload
	var store storepkg.Store
	var shoutdownFn func()
	switch *dbType {
	default:
		fallthrough
	case "json":
		storeJson := storepkg.NewMemStore()
		storeJson.Load(*dbStore)
		shoutdownFn = func() {
			storeJson.Dump(*dbStore)
		}
		store = storeJson
	case "bitcask":
		storeBitcask, err := storepkg.NewBitcaskStore(*dbStore)
		if err != nil {
			Vln(1, "[db]err", err)
			return
		}
		shoutdownFn = func() {
			storeBitcask.Close()
		}
		store = storeBitcask
	}

	// session not reload
	sess := session.NewMemSession()

	var wikiHandler atomic.Value // hold handler for current config

	buildHandler := func() (func(), error) {
		auth := authpkg.NewAuthCustom()
		if err := auth.Load(*authStore); err != nil {
			Vln(0, "[acl]load err", err)
			return nil, err
		}
		// Vln(0, "[acl]", auth.UserDB, auth.AnonymousEdit)

		wiki := api.NewWiki(nil, store, sess)
		wiki.SyncStoryList = *syncStoryList
		wiki.Files = *dir
		wiki.Base = *wikiBase
		// wiki.Store = store
		wiki.AuthHandler = auth
		wiki.SetupMux(nil) // bind handler

		wikiHandler.Store(wiki)
		return nil, nil
	}

	// watch for config file chnage
	go func() {
		clean, _ := buildHandler()

		for {
			if err := watchFile(*authStore); err == nil {
				Vln(2, "[config]has been changed")
				if clr, err := buildHandler(); err == nil {
					if clean != nil {
						clean()
					}
					clean = clr
				}
			}

			time.Sleep(2 * time.Second)
		}
	}()

	// http.Handle("/", reqlog(wiki(http.FileServer(http.Dir(*dir)))))
	srv := &http.Server{
		ReadTimeout:  time.Duration(*readTimeout) * time.Second,
		WriteTimeout: time.Duration(*writeTimeout) * time.Second,
		Addr:         *port,
		Handler:      reqlog(reqAtom(&wikiHandler)),
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		// We received an interrupt signal, shut down.
		if err := srv.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	// log.Printf("srv -> client (TX) limit: %v\n", *txSpd)
	// log.Printf("srv <- client (RX) limit: %v\n", *rxSpd)
	startServer(srv, *crtFile, *keyFile)

	<-idleConnsClosed

	// write back
	shoutdownFn()
}

func startServer(srv *http.Server, crt string, key string) {
	var err error

	// check tls
	if crt != "" && key != "" {
		cfg := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{

				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,

				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // http/2 must
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,   // http/2 must

				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,

				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,

				tls.TLS_RSA_WITH_AES_256_GCM_SHA384, // weak
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,    // waek
			},
		}
		srv.TLSConfig = cfg
		//srv.TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0) // disable http/2

		log.Printf("[server] HTTPS server Listen on: %v", srv.Addr)
		err = srv.ListenAndServeTLS(crt, key)
	} else {
		log.Printf("[server] HTTP server Listen on: %v", srv.Addr)
		err = srv.ListenAndServe()
	}

	if err != http.ErrServerClosed {
		log.Printf("[server] ListenAndServe error: %v", err)
	}
}

func Vf(level int, format string, v ...interface{}) {
	if level <= *verbosity {
		log.Printf(format, v...)
	}
}
func V(level int, v ...interface{}) {
	if level <= *verbosity {
		log.Print(v...)
	}
}
func Vln(level int, v ...interface{}) {
	if level <= *verbosity {
		log.Println(v...)
	}
}

func watchFile(fp string) error {
	stat0, err := os.Stat(fp)
	if err != nil {
		return err
	}

	for {
		stat, err := os.Stat(fp)
		if err != nil {
			return err
		}

		if stat.Size() != stat0.Size() || stat.ModTime() != stat0.ModTime() {
			break
		}

		time.Sleep(1 * time.Second)
	}

	return nil
}
