package tiddlywikid

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"tiddlywikid/auth"
	"tiddlywikid/session"
	"tiddlywikid/store"
	"tiddlywikid/utils"
)

const (
	TIDDLIYWIKI_VERSION = "5.2.3"

	STORYLIST_PATH   = `$:/StoryList`
	HISTORYLIST_PATH = `$:/HistoryList`
	TAGS_MACRO       = `$:/tags/Macro`

	DefaultUploadFileSizeLimit = 1024 * 1024 * 256 // 256MB
	DefaultParseMemoryLimit    = 1024 * 1024 * 64  // 64MB stored in memory, with the remainder stored on disk in temporary files
	DefaultTiddlerSizeLimit    = 1024 * 1024 * 8   // 8MB

	COOKIE_CSRF = "csrf_token"
)

var (
	_FIRST_LOAD_COOKIE = "_tiddly"

	_SESSION_COOKIE = "tiddlywiki"
	_COOKIE_TTL     = session.SESSION_TTL
	_LOGIN_DELAY    = 500 * time.Millisecond
)

type Wiki struct {
	*Mux
	Store               store.Store
	AuthHandler         auth.Auth
	Sess                session.SessionStore
	Files               string // path for static file
	Base                string // base html file with plugin "tiddlywiki/tiddlyweb"
	Recipe              string // recipe, default: "default"
	SyncStoryList       bool   // save and put `$:/StoryList` and `$:/HistoryList` (cause some issue when multi-user/multi-window)
	UploadFileSizeLimit int64
	ParseMemoryLimit    int64
	TiddlerSizeLimit    int64

	staticFileHandle http.Handler
	fileRe           *regexp.Regexp
}

func (wiki *Wiki) SetupMux(mux *Mux) *Mux {
	if mux == nil {
		mux = NewRootMux()
		wiki.Mux = mux
	}

	mux.HandleFunc("/", wiki.index)
	mux.HandleFunc("/status", wiki.status)

	// wiki.Recipe == "default"
	basePath := fmt.Sprintf("/recipes/%v/", wiki.Recipe)
	mux.HandleFunc(basePath+"tiddlers.json", wiki.list)                                                      // list
	mux.Handle(basePath+"tiddlers/", mux.StripPrefix(basePath+"tiddlers/", http.HandlerFunc(wiki.tiddlers))) // get & put tiddler

	bagPath := fmt.Sprintf("/bags/%v/tiddlers/", wiki.Recipe)
	mux.Handle(bagPath, mux.StripPrefix(bagPath, http.HandlerFunc(wiki.delTiddler))) // delete tiddler

	// TODO: use Attachment.ServeContent()
	// attachments plugin
	wiki.staticFileHandle = http.FileServer(http.Dir(wiki.Files))
	mux.HandleFunc("/upload/", wiki.upload)
	mux.Handle("/files/", mux.StripPrefix("/files/", http.HandlerFunc(wiki.serveFile))) // static files
	wiki.fileRe = regexp.MustCompile(`files/(\d){8}T(\d){6}-([A-Za-z0-9\-_]){16}`)

	// for login
	mux.HandleFunc("/challenge/tiddlywebplugins.tiddlyspace.cookie_form", wiki.login)
	mux.HandleFunc("/logout", wiki.logout)
	return mux
}

// TODO: method OPTIONS
func (wiki *Wiki) index(w http.ResponseWriter, r *http.Request) {
	hdr := w.Header()
	hdr.Set("Cache-Control", "max-age=0, must-revalidate")

	switch r.Method {
	case http.MethodOptions:
		w.Header().Add("Allow", "GET, HEAD, OPTIONS")
		return
	case http.MethodHead: // handle by http.ServeContent()
		fallthrough
	default:
	}

	fd, err := os.Open(wiki.Base)
	if err != nil {
		http.NotFoundHandler().ServeHTTP(w, r)
		return
	}
	info, err := fd.Stat()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// buf, err := io.ReadAll(fd)
	// if err != nil {
	// 	http.Error(w, "internal server error", http.StatusInternalServerError)
	// 	return
	// }
	// buf = append(buf, NewPluginBuf()...)
	// buf = append(NewPluginBuf(), buf...)

	// TODO: better etag
	etag := fmt.Sprintf(`"%d-%d"`, info.ModTime().Unix(), info.Size())
	hdr.Set("Etag", etag)

	// http.ServeContent(w, r, "", info.ModTime(), bytes.NewReader(buf))
	http.ServeContent(w, r, "", info.ModTime(), fd)
	// http.ServeFile(w, r, "index.html")
}

// return status by auth state
// resp: `{"username":"","anonymous":true,"read_only":false,"space":{"recipe":"default"},"tiddlywiki_version":"5.2.3"}`
func (wiki *Wiki) status(w http.ResponseWriter, r *http.Request) {

	// set flag for full tildder @ first fetch
	setFirstLoad(w, r)

	isAnno, isLogin, user, sd := wiki.checkAuth(w, r)
	if !isLogin {
		user = "GUEST"
	}

	// no anno && not login
	if !isAnno && !isLogin {
		status := &WikiStatus{
			Username:  "GUEST",
			Anonymous: false,
			ReadOnly:  true,
			Recipe:    wiki.Recipe,
			Version:   TIDDLIYWIKI_VERSION,
		}
		JsonRes(w, status, false)
		return
	}

	// login:
	// Username:  "GUEST",
	// Anonymous: false,
	// ReadOnly:  true,

	// status := &WikiStatus{
	// 	Username:  "",
	// 	Anonymous: true,
	// 	ReadOnly:  false,
	// 	Recipe:    wiki.Recipe,
	// 	Version:   TIDDLIYWIKI_VERSION,
	// }

	readOnly := true
	if isLogin || wiki.AuthHandler.AllowAnonymousEdit(r) {
		readOnly = false
	}

	// update CSRF
	wiki.updateCSRF(w, r, sd)

	status := &WikiStatus{
		Username:  user,
		Anonymous: isAnno,
		ReadOnly:  readOnly,
		Recipe:    wiki.Recipe,
		Version:   TIDDLIYWIKI_VERSION,
	}
	JsonRes(w, status, false)
}

func (wiki *Wiki) checkAuth(w http.ResponseWriter, r *http.Request) (isAnno bool, isLogin bool, user string, sd *session.SessionData) {
	isLogin = false
	isAnno = wiki.AuthHandler.AllowAnonymous(r)
	sd = getSess(wiki.Sess, w, r)
	if sd != nil {
		uid, ok := sd.Get("acc")
		if ok {
			isLogin = true
			user = uid.(string)
		}
	}
	return
}

func (wiki *Wiki) checkAuthEdit(w http.ResponseWriter, r *http.Request) (isAnno bool, isLogin bool, user string, sd *session.SessionData) {
	isLogin = false
	isAnno = wiki.AuthHandler.AllowAnonymousEdit(r)
	sd = getSess(wiki.Sess, w, r)
	if sd != nil {
		uid, ok := sd.Get("acc")
		if ok {
			isLogin = true
			user = uid.(string)
		}
	}
	return
}

func (wiki *Wiki) checkAuthStatic(w http.ResponseWriter, r *http.Request) (isAnno bool, isLogin bool, user string, sd *session.SessionData) {
	isLogin = false
	isAnno = wiki.AuthHandler.AllowAnonymousAccessStaticFile(r)
	sd = getSess(wiki.Sess, w, r)
	if sd != nil {
		uid, ok := sd.Get("acc")
		if ok {
			isLogin = true
			user = uid.(string)
		}
	}
	return
}

func (wiki *Wiki) updateCSRF(w http.ResponseWriter, r *http.Request, sd *session.SessionData) {
	// sd := getSess(wiki.Sess, w, r, false)
	if sd == nil {
		return // no session
	}
	csrfToken, err := genRang()
	if err != nil {
		utils.Vln(4, "[CSRF]err", r.URL.Path, err)
		return
	}
	sd.Set("csrf", csrfToken)

	cookie := &http.Cookie{
		Name:     COOKIE_CSRF,
		Value:    csrfToken,
		Path:     "/",   // TODO: by sub path
		HttpOnly: false, // need to read by js
		SameSite: http.SameSiteStrictMode,
		Expires:  utils.Now().Add(_COOKIE_TTL),
		MaxAge:   int(_COOKIE_TTL.Seconds()),
	}
	http.SetCookie(w, cookie)

	utils.Vln(4, "[CSRF]", r.URL.Path, csrfToken)
}

// X-Requested-With: TiddlyWiki
func (wiki *Wiki) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t0 := time.Now().Add(_LOGIN_DELAY)

	xReq, ok := r.Header["X-Requested-With"]
	if !ok || len(xReq) != 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if xReq[0] != "TiddlyWiki" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// check already login
	sd := getSess(wiki.Sess, w, r)
	if sd != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user := r.Form.Get("user")
	pwd := r.Form.Get("password")

	utils.Vln(4, "[login]", r.URL.Path, user)
	name, ok := wiki.AuthHandler.Login(user, pwd, r)
	if !ok {
		time.Sleep(time.Until(t0)) // block untill time up
		wiki.errNotLogin(w, r)
		return
	}

	time.Sleep(time.Until(t0)) // block untill time up

	sd = startSess(wiki.Sess, w, r)
	sd.Set("acc", name)

	// update CSRF
	wiki.updateCSRF(w, r, sd)

	w.WriteHeader(http.StatusNoContent)
}

func (wiki *Wiki) logout(w http.ResponseWriter, r *http.Request) {
	xReq, ok := r.Header["X-Requested-With"]
	if !ok || len(xReq) != 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if xReq[0] != "TiddlyWiki" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	sd := getSess(wiki.Sess, w, r)
	if sd == nil {
		wiki.errNotLogin(w, r)
		return
	}

	token, ok := sd.Get("csrf")
	if !ok {
		wiki.errNotLogin(w, r)
		return
	}
	tokenStr, ok := token.(string)
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	csrfToken := r.Form.Get("csrf_token")

	if tokenStr != csrfToken {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	utils.Vln(4, "[logout]", r.URL.Path)
	delSess(wiki.Sess, w, r)
}

func (wiki *Wiki) errNotLogin(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func (wiki *Wiki) list(w http.ResponseWriter, r *http.Request) {
	isAnno, isLogin, _, sd := wiki.checkAuth(w, r)
	if !isAnno && !isLogin { // no anno && not login
		SetHeader(w, false)
		w.Write(([]byte)(`[]`))
		return
	}

	isFirst := checkAndResetFirstLoad(w, r)
	utils.Vln(4, "[list]", r.URL.Path, isFirst)

	// update CSRF
	wiki.updateCSRF(w, r, sd)

	tds := []*store.TiddlyWebJSON{
		NewNow(r),
		NewSrcIP(r),
		// NewPlugin(r),
	}
	SetHeader(w, false)
	w.Header().Set("Content-Type", "application/json")
	w.Write(wiki.Store.List(tds, isFirst))
}

// TODO: check permission
func (wiki *Wiki) tiddlers(w http.ResponseWriter, r *http.Request) {
	utils.Vln(4, "[req]", r.Method, r.URL.Path)

	switch r.Method {
	case http.MethodOptions:
		w.Header().Add("Allow", "GET, PUT, OPTIONS")
		return
	case http.MethodGet:
		wiki.getTiddler(w, r)
	case http.MethodPut:
		wiki.putTiddler(w, r)
	case http.MethodDelete:
		wiki.delTiddler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// TODO: cache by etag & LastModified
func (wiki *Wiki) getTiddler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path
	if wiki.skipTiddler(w, key) {
		return
	}

	isAnno, isLogin, _, sd := wiki.checkAuth(w, r)
	if !isAnno && !isLogin { // no anno && not login
		wiki.errNotLogin(w, r)
		return
	}

	// update CSRF
	wiki.updateCSRF(w, r, sd)

	useCache := true
	td, hash := wiki.Store.Get(key)
	if td == nil {
		// check auto generated
		fn, ok := autoGen[key]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		td = fn(r)
		useCache = false
	}

	// set default value & add text back
	td.Revision = fmt.Sprintf("%v", td.Rev)
	if td.Bag == "" {
		td.Bag = wiki.Recipe // "default"
	}
	if td.Type == "" {
		td.Type = "text/vnd.tiddlywiki"
	}

	// skip title due to not used and escape issue
	etag := fmt.Sprintf(`"%v/%v/%v:%v"`, wiki.Recipe, "", td.Rev, hash) // recipe, title, revision, hash/checksum
	h := w.Header()
	h.Set("Cache-Control", "max-age=0, must-revalidate")
	h.Set("Content-Type", "application/json")
	if useCache {
		h.Set("Etag", etag)

		// TODO: carefully follows RFC 7232 section 6.
		inEtag := r.Header.Get("If-None-Match")
		utils.Vln(6, "[etag]", key, etag, "<->", inEtag)
		if inEtag == etag {
			writeNotModified(w)
			return
		}
	}

	enc := json.NewEncoder(w)
	err := enc.Encode(td)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		utils.Vln(4, "[get]err", key, err)
		return
	}
}

func (wiki *Wiki) putTiddler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path
	if wiki.skipTiddler(w, key) {
		return
	}

	isAnno, isLogin, _, sd := wiki.checkAuthEdit(w, r)
	if !isAnno && !isLogin { // no anno && not login
		wiki.errNotLogin(w, r)
		return
	}

	// update CSRF
	wiki.updateCSRF(w, r, sd)

	// skip auto generated
	_, ok := autoGen[key]
	if ok {
		// cause non-stop error
		// http.Error(w, "conflict", http.StatusConflict)

		// just skip it
		etag := fmt.Sprintf(`"%v/%v/%v:%v"`, wiki.Recipe, "", 0, "") // recipe, title, revision, hash/checksum
		w.Header().Set("Etag", etag)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, wiki.TiddlerSizeLimit)
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	utils.Vln(4, "[put]", key, (string)(buf))

	// need to do:
	// Remove any revision field
	// Remove `_is_skinny` field, and keep old text
	// Extract `text` field
	tiddler, hasMacro, err := parseMeta(buf)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// dbg
	// meta, _ := json.MarshalIndent(tiddler, "", "\t")
	// Vln(5, "[put]2", key, (string)(meta), tiddler.IsSkinny)

	// check has file and file path is like "/files/20220201T104852-VgVjI7W_aR7_nkPT"
	fp := getCanonicalUri(tiddler.Fields)
	if fp != "" {
		if wiki.fileRe.MatchString(fp) {
			fp = path.Base(fp)
		} else {
			fp = ""
		}
	}

	rev, hash := wiki.Store.Put(key, tiddler, hasMacro, fp)
	// skip title due to not used and escape issue
	etag := fmt.Sprintf(`"%v/%v/%v:%v"`, wiki.Recipe, "", rev, hash) // recipe, title, revision, hash/checksum
	w.Header().Set("Etag", etag)
	w.WriteHeader(http.StatusNoContent)
}

func (wiki *Wiki) delTiddler(w http.ResponseWriter, r *http.Request) {
	isAnno, isLogin, _, sd := wiki.checkAuthEdit(w, r)
	if !isAnno && !isLogin { // no anno && not login
		wiki.errNotLogin(w, r)
		return
	}

	// update CSRF
	wiki.updateCSRF(w, r, sd)

	key := r.URL.Path
	utils.Vln(4, "[del]", key)

	ok, file := wiki.Store.Del(key)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// remove external attachment if exist
	if file != "" {
		saveFp := filepath.Join(wiki.Files, file)
		os.Remove(saveFp)
	}

	// http.Error(w, "OK", http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}

// skip `$:/StoryList` and `$:/HistoryList`
// https://tiddlywiki.com/#Hidden%20Setting%3A%20Sync%20System%20Tiddlers%20From%20Server
func (wiki *Wiki) skipTiddler(w http.ResponseWriter, key string) bool {
	// TODO: default value?
	if !wiki.SyncStoryList {
		switch key {
		case STORYLIST_PATH, HISTORYLIST_PATH:
			etag := fmt.Sprintf(`"%v/%v/%v:"`, wiki.Recipe, key, 0) // recipe, title, revision
			w.Header().Set("Etag", etag)
			w.WriteHeader(http.StatusNoContent)
			return true
		}
	}
	return false
}

func (wiki *Wiki) upload(w http.ResponseWriter, r *http.Request) {
	// TODO: session timeout when uploading?
	isAnno, isLogin, _, _ := wiki.checkAuthEdit(w, r)
	if !isAnno && !isLogin { // no anno && not login
		wiki.errNotLogin(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, wiki.UploadFileSizeLimit)
	err := r.ParseMultipartForm(wiki.ParseMemoryLimit)
	if err != nil {
		utils.Vln(0, "[upload]parse multipart-form error", r.RemoteAddr, r.Method, r.URL, r.Referer(), r.UserAgent(), err)
		return
	}

	meta := r.MultipartForm.Value["meta"]
	if len(meta) != 1 {
		// no meta or more than 1 meta...?
		return
	}
	fhs := r.MultipartForm.File["text"]
	if len(fhs) == 0 {
		// only meta...?
		return
	}

	// only read one
	fh := fhs[0]
	file, err := fh.Open()
	if err != nil {
		utils.Vln(3, "[upload]parse file error", r.RemoteAddr, r.Method, r.URL, r.Referer(), r.UserAgent(), err, meta)
		return
	}
	attach, erroeText, errCode := wiki.saveFile(file, fh, r)
	if erroeText != "" && errCode != 0 {
		http.Error(w, erroeText, errCode)
		return
	}

	// buf, err := io.ReadAll(r.Body)
	// if err != nil {
	// 	http.Error(w, "bad request", http.StatusBadRequest)
	// 	return
	// }

	// utils.Vln(0, "[upload]", (string)(buf))

	tiddler, hasMacro, err := parseMeta(([]byte)(meta[0]))
	if err != nil {
		utils.Vln(3, "[upload]meta error", meta, err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// save as tiddler first
	wiki.Store.Put(tiddler.Title, tiddler, hasMacro, "")

	// bind to tiddler store for auto remove
	if !wiki.Store.AttachAttachment(tiddler.Title, attach.SaveName) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	utils.Vln(3, "[upload]", meta, attach)
	w.Write(([]byte)(attach.SaveName))
}

func (wiki *Wiki) saveFile(file multipart.File, handler *multipart.FileHeader, r *http.Request) (attach *Attachment, erroeText string, errCode int) {
	defer file.Close()

	// TODO: check file magic
	attach = NewAttachment(handler.Filename, handler.Size)
	saveFp := filepath.Join(wiki.Files, attach.SaveName)
	fd, err := os.OpenFile(saveFp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		utils.Vln(3, "[upload]open save file error", r.RemoteAddr, r.Method, r.URL, r.Referer(), r.UserAgent(), err)
		return nil, "internal server error", http.StatusInternalServerError
	}
	defer fd.Close()

	// save file
	// if _, err := io.Copy(fd, file); err != nil {
	// 	utils.Vln(3, "[upload]write save file error", r.RemoteAddr, r.Method, r.URL, r.Referer(), r.UserAgent(), err)
	// 	return nil, "internal server error", http.StatusInternalServerError
	// }

	// TODO: base64 decode @ server side?
	// save and calc hash
	hash, err := cpAndHashFd(fd, file)
	if err != nil {
		utils.Vln(3, "[upload]save and hash file error", r.RemoteAddr, r.Method, r.URL, r.Referer(), r.UserAgent(), err)
		return nil, "internal server error", http.StatusInternalServerError
	}
	attach.Checksum = hash
	return
}

func (wiki *Wiki) serveFile(w http.ResponseWriter, r *http.Request) {
	isAnno, isLogin, _, _ := wiki.checkAuthStatic(w, r)
	if !isAnno && !isLogin { // no anno && not login
		wiki.errNotLogin(w, r)
		return
	}
	wiki.staticFileHandle.ServeHTTP(w, r)
}

func NewWiki(mux *Mux, db store.Store, sess session.SessionStore) *Wiki {
	if db == nil {
		db = store.NewMemStore()
	}
	if sess == nil {
		sess = session.NewMemSession()
	}
	wiki := &Wiki{
		Mux:                 mux,
		AuthHandler:         &auth.AuthAllowAll{},
		Sess:                sess,
		Store:               db,
		Base:                "index.html",
		Recipe:              "default",
		Files:               "./files/",
		UploadFileSizeLimit: DefaultUploadFileSizeLimit,
		ParseMemoryLimit:    DefaultParseMemoryLimit,
		TiddlerSizeLimit:    DefaultTiddlerSizeLimit,
	}
	return wiki
}

type WikiStatus struct {
	Username  string `json:"username"` // "GUEST" === not login
	Anonymous bool   `json:"anonymous"`
	ReadOnly  bool   `json:"read_only"`
	Recipe    string `json:"-"`
	// Space   json.RawMessage `json:"space"`
	Version string `json:"tiddlywiki_version,omitempty"`
}

func (ws *WikiStatus) MarshalJSON() ([]byte, error) {
	type Space struct {
		Recipe string `json:"recipe"` // default: "default"
	}
	type aWikiStatus WikiStatus
	aux := &struct {
		// Username  string `json:"username"` // "GUEST" === not login
		// Anonymous bool   `json:"anonymous"`
		// ReadOnly  bool   `json:"read_only"`
		// Version   string `json:"tiddlywiki_version,omitempty"`
		*aWikiStatus
		Space *Space `json:"space"`
	}{
		aWikiStatus: (*aWikiStatus)(ws),
		Space: &Space{
			Recipe: ws.Recipe,
		},
	}
	return json.Marshal(aux)
}

func parseMeta(meta []byte) (*store.TiddlyWebJSON, bool, error) {
	tiddler := &store.TiddlyWebJSON{}
	err := json.Unmarshal(meta, tiddler)
	if err != nil {
		return nil, false, err
	}

	// check `$:/tags/Macro` tag exist or not
	hasMacro := false
	if tiddler.Tags != nil {
		for _, tag := range *tiddler.Tags {
			if tag == TAGS_MACRO {
				hasMacro = true
				break
			}
		}
	}
	return tiddler, hasMacro, err
}

func getCanonicalUri(fields *store.TiddlerFields) string {
	if fields == nil {
		return ""
	}
	fpRaw, ok := (*fields)["_canonical_uri"]
	if !ok {
		return ""
	}
	fpStr, ok := fpRaw.(string)
	if !ok {
		return ""
	}
	return fpStr
}

func SetHeader(w http.ResponseWriter, cors bool) {
	header := w.Header()
	header.Set("Cache-Control", "public, max-age=0, must-revalidate")
	if cors {
		header.Set("Access-Control-Allow-Origin", "*")
		header.Set("Access-Control-Allow-Credentials", "true")
	}
}

func JsonRes(w http.ResponseWriter, value interface{}, cors bool) {
	SetHeader(w, cors)
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err := enc.Encode(value)
	if err != nil {
		utils.Vln(3, "[web]json response err:", err.Error())
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// func cleanPath(fp string) string {
// 	return filepath.FromSlash(path.Clean("/" + fp))
// }

// set in "/status"
// check and reset in "/recipes/default/tiddlers.json"
func setFirstLoad(w http.ResponseWriter, r *http.Request) {
	_, err := r.Cookie(_FIRST_LOAD_COOKIE)
	if err == http.ErrNoCookie {
		// set cookie
		http.SetCookie(w, &http.Cookie{
			Name:     _FIRST_LOAD_COOKIE,
			Value:    "1",
			Path:     "/", // TODO: by sub path
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(_COOKIE_TTL.Seconds()),
			// MaxAge:   10, // better way?
		})
	}
}
func checkAndResetFirstLoad(w http.ResponseWriter, r *http.Request) bool {
	_, err := r.Cookie(_FIRST_LOAD_COOKIE)
	if err == http.ErrNoCookie {
		return false
	}

	// clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     _FIRST_LOAD_COOKIE,
		Value:    "",
		Path:     "/", // TODO: by sub path
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  utils.Now(),
		MaxAge:   -1,
	})
	return true
}

func setSessionCookie(w http.ResponseWriter, token string) {
	// update cookie
	cookie := &http.Cookie{
		Name:     _SESSION_COOKIE,
		Value:    token,
		Path:     "/", // TODO: by sub path
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  utils.Now().Add(_COOKIE_TTL),
		MaxAge:   int(_COOKIE_TTL.Seconds()),
	}
	http.SetCookie(w, cookie)
}

func getSess(sess session.SessionStore, w http.ResponseWriter, r *http.Request) *session.SessionData {
	cookie, err := r.Cookie(_SESSION_COOKIE)
	if err != nil || cookie.Value == "" {
		return nil
	}

	token := cookie.Value
	sd := sess.GetOrRenew(token)
	if sd == nil { // timeout?
		return nil
	}

	// update cookie
	setSessionCookie(w, token)

	return sd
}

func delSess(sess session.SessionStore, w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(_SESSION_COOKIE)
	if err != nil || cookie.Value == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	token := cookie.Value
	sess.Destroy(token)

	// force cookie timeout
	cookieSet := &http.Cookie{
		Name:     _SESSION_COOKIE,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  utils.Now(),
		MaxAge:   -1,
	}
	http.SetCookie(w, cookieSet)
}

func startSess(sess session.SessionStore, w http.ResponseWriter, r *http.Request) *session.SessionData {
	token, sd := sess.NewToken()
	if sd == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil
	}

	// update cookie
	setSessionCookie(w, token)
	return sd
}

func writeNotModified(w http.ResponseWriter) {
	// RFC 7232 section 4.1:
	// a sender SHOULD NOT generate representation metadata other than the
	// above listed fields unless said metadata exists for the purpose of
	// guiding cache updates (e.g., Last-Modified might be useful if the
	// response does not have an ETag field).
	h := w.Header()
	delete(h, "Content-Type")
	delete(h, "Content-Length")
	delete(h, "Content-Encoding")
	delete(h, "Last-Modified")
	w.WriteHeader(http.StatusNotModified)
}

func genRang() (string, error) {
	buf := make([]byte, 12)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}
