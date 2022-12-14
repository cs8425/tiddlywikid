package tiddlywikid

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// put in db
type Attachment struct {
	// Token string `json:"token"` // unique

	OriginalName string    `json:"on"`
	Size         int64     `json:"sz"`
	UploadTime   time.Time `json:"time"` // also for Last-Modified
	Checksum     string    `json:"hash"` // also for ETag

	SaveName string `json:"sn"` // time + random + hash

	Hide bool `json:"hide,omitempty"` // mark as delete

	//	Gzipped bool `json:"gzip,omitempty"` // gzipped on disk
	//	GzSize int64 `json:"gzsz,omitempty"`
	//	GzChecksum string `json:"gzhash,omitempty"`
}

func (a *Attachment) DelFromFS(baseDir string) error {
	saveFp := filepath.Join(baseDir, a.SaveName)
	return os.Remove(saveFp)
}

func (a *Attachment) ServeContent(w http.ResponseWriter, r *http.Request, baseDir string) {
	saveName := filepath.Clean("/" + a.SaveName)[1:] // clean again for SaveName in db tamper by other program
	saveFp := filepath.Join(baseDir, saveName)

	fi, err := os.Stat(saveFp)
	if err != nil {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	szCk := a.Size
	//	if a.Gzipped {
	//		szCk = a.GzSize
	//	}
	if fi.Size() != szCk {
		http.Error(w, "419 Checksum failed", 419)
		return
	}

	fd, err := os.OpenFile(saveFp, os.O_RDONLY, 0400)
	if err != nil {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	defer fd.Close()

	// checksum check
	sha256, ok := sha256fd(fd)
	if !ok || sha256 != a.Checksum {
		http.Error(w, "419 Checksum failed", 419)
		return
	}
	w.Header().Set("Etag", `"`+a.Checksum+`"`)
	http.ServeContent(w, r, a.OriginalName, a.UploadTime, fd)
}

func NewAttachment(name string, size int64) *Attachment {
	now := time.Now()
	token, _ := genRang() // TODO: handle error
	sname := formatTimestamp(now) + "-" + token
	a := &Attachment{
		OriginalName: filepath.Clean("/" + name)[1:], // remove '../'
		Size:         size,
		UploadTime:   now,
		SaveName:     sname,
	}
	return a
}

func formatTimestamp(t time.Time) string {
	return t.Format("20060102T150405") // "2006-01-02T15:04:05Z07:00"
}

func sha256fd(f io.Reader) (string, bool) {
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err.Error(), false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func cpAndHashFd(fo io.Writer, fi io.Reader) (string, error) {
	sha1h := sha256.New()
	w := io.MultiWriter(fo, sha1h)
	if _, err := io.Copy(w, fi); err != nil {
		return "", err
	}
	return hex.EncodeToString(sha1h.Sum(nil)), nil
}
