package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// for early stage dev only
type MemStore struct {
	mx      sync.RWMutex
	kv      map[string]*memTiddler
	fileRef map[string]int
}

// meta should contain `title` (the key)
type dumpTiddler struct {
	Meta     string `json:"meta,omitempty"`
	Text     string `json:"text,omitempty"`
	Rev      uint64 `json:"rev,omitempty"`
	HasMacro bool   `json:"macro,omitempty"`
	Hash     string `json:"hash,omitempty"`
	File     string `json:"file,omitempty"`
}

type memTiddler struct {
	rev      uint64
	meta     []byte
	text     string
	hash     string // md5? sha1? sha256?
	file     string // external attachment
	hasMacro bool   // `$:/tags/Macro` tag need to send `text` in skinny tiddler
}

func (s *MemStore) List(ap []*TiddlyWebJSON) []byte {
	var b bytes.Buffer

	s.mx.RLock()
	defer s.mx.RUnlock()

	// need to put `revision` back
	hasOutput := false
	b.WriteByte('[')
	for _, td := range s.kv {
		if td.hasMacro {
			var js map[string]interface{}
			err := json.Unmarshal(td.meta, &js)
			if err != nil {
				continue
			}
			js["text"] = td.text
			buf, _ := json.Marshal(js)

			if hasOutput {
				b.WriteByte(',')
			}
			b.Write(buf)
		} else {

			if hasOutput {
				b.WriteByte(',')
			}
			b.Write(td.meta)
			// js["revision"] = fmt.Sprintf("%v", td.rev)
		}

		// flag for ','
		hasOutput = true
	}

	// for system generated data
	for _, td := range ap {
		buf, err := json.Marshal(td)
		if err != nil {
			continue
		}

		if hasOutput {
			b.WriteByte(',')
		}
		b.Write(buf)

		// flag for ','
		hasOutput = true
	}

	b.WriteByte(']')
	return b.Bytes()
}

func (s *MemStore) Get(key string) (*TiddlyWebJSON, string) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	td, ok := s.kv[key]
	if !ok {
		return nil, ""
	}

	tiddler := &TiddlyWebJSON{}
	err := json.Unmarshal(td.meta, tiddler)
	if err != nil {
		// ????
		return nil, ""
	}

	// set default value & add text back
	tiddler.Rev = td.rev
	tiddler.Text = td.text
	// if tiddler.Bag == "" {
	// 	tiddler.Bag = "default"
	// }
	// if tiddler.Type == "" {
	// 	tiddler.Type = "text/vnd.tiddlywiki"
	// }

	return tiddler, td.hash
}

// need to do:
// Remove any revision field
// Remove `_is_skinny` field, and keep old text
// Extract `text` field
func (s *MemStore) Put(key string, tiddler *TiddlyWebJSON, hasMacro bool, filePath string) (rev uint64, hash string) {
	s.mx.RLock()
	if td, ok := s.kv[key]; ok {
		rev, hash = s.putExist(td, tiddler, hasMacro, filePath, 0)
		s.mx.RUnlock()
		return
	}
	s.mx.RUnlock()

	s.mx.Lock()
	// double check
	ref := 0
	td, ok := s.kv[key]
	if !ok {
		td = &memTiddler{}
		s.kv[key] = td
		ref = 1 // file ref += 1
	}
	rev, hash = s.putExist(td, tiddler, hasMacro, filePath, ref)
	s.mx.Unlock()
	return
}

func (s *MemStore) putExist(td *memTiddler, tiddler *TiddlyWebJSON, hasMacro bool, fp string, ref int) (rev uint64, hash string) {
	// Remove `_is_skinny` field, and keep old text
	if tiddler.IsSkinny != nil {
		tiddler.IsSkinny = nil
		tiddler.Text = td.text
	}

	rev = atomic.AddUint64(&td.rev, 1)

	// TODO: check revision?
	// Remove any revision field
	// tiddler.Revision = ""

	// set revision
	tiddler.Revision = fmt.Sprintf("%v", rev)

	// Extract `text` field
	text := tiddler.Text
	tiddler.Text = ""

	// build meta
	meta, _ := json.Marshal(tiddler)

	// calc hash
	h := sha256.New()
	h.Write(meta)
	h.Write([]byte(text))
	// hash = hex.EncodeToString(h.Sum(nil))
	hash = base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	// set back
	td.meta = meta
	td.text = text
	td.hasMacro = hasMacro
	td.hash = hash
	td.file = fp

	// update file ref
	if ref != 0 && fp != "" {
		s.fileRef[fp] += ref
	}

	return rev, hash
}

// TODO: remove attachment file
func (s *MemStore) Del(key string) (bool, string) {
	s.mx.Lock()
	defer s.mx.Unlock()
	td, ok := s.kv[key]
	if !ok {
		return false, ""
	}
	delete(s.kv, key)

	// skip if no file
	if td.file == "" {
		return true, ""
	}

	// calc file ref
	if _, ok := s.fileRef[td.file]; ok {
		s.fileRef[td.file] -= 1

		if s.fileRef[td.file] == 0 {
			delete(s.fileRef, td.file)
			return true, td.file
		}
	}
	return true, ""
}

func (s *MemStore) AttachAttachment(key string, file string) bool {
	s.mx.Lock()
	defer s.mx.Unlock()
	td, ok := s.kv[key]
	if !ok {
		return false
	}
	td.file = file

	// count file
	s.fileRef[file] += 1

	return true
}

func (s *MemStore) MarshalJSON() ([]byte, error) {
	aux := make([]*dumpTiddler, 0, len(s.kv))
	s.mx.RLock()
	for _, td := range s.kv {
		dtd := &dumpTiddler{
			Meta:     string(td.meta),
			Text:     td.text,
			Rev:      td.rev,
			HasMacro: td.hasMacro,
			Hash:     td.hash,
			File:     td.file,
		}
		aux = append(aux, dtd)
	}
	s.mx.RUnlock()

	// v, _ := json.Marshal(aux)
	// fmt.Println("[dump]", len(aux), (string)(v))
	return json.Marshal(aux)
}

func (s *MemStore) UnmarshalJSON(buf []byte) error {
	type getKey struct {
		Title string `json:"title"`
	}

	aux := make([]*dumpTiddler, 0, 128)
	err := json.Unmarshal(buf, &aux)
	if err != nil {
		return err
	}

	getk := &getKey{}
	nkv := make(map[string]*memTiddler)
	fRef := make(map[string]int)
	for _, dtd := range aux {
		meta := []byte(dtd.Meta)
		err := json.Unmarshal(meta, &getk)
		if err != nil {
			continue
		}

		key := getk.Title
		if key == "" {
			continue
		}

		td := &memTiddler{
			meta:     meta,
			text:     dtd.Text,
			rev:      dtd.Rev,
			hasMacro: dtd.HasMacro,
			hash:     dtd.Hash,
			file:     dtd.File,
		}
		nkv[key] = td

		// count file
		if td.file != "" {
			fRef[td.file] += 1
		}
	}

	s.mx.Lock()
	s.kv = nkv
	s.fileRef = fRef
	s.mx.Unlock()

	return nil
}

func (s *MemStore) Load(fp string) error {
	fd, err := os.Open(fp)
	if err != nil {
		return err
	}
	defer fd.Close()

	ns := &MemStore{}
	dec := json.NewDecoder(fd)
	err = dec.Decode(&ns)
	if err != nil {
		return err
	}

	s.mx.Lock()
	s.kv = ns.kv
	s.fileRef = ns.fileRef
	s.mx.Unlock()

	return nil
}

func (s *MemStore) Dump(fp string) error {
	fd, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()

	enc := json.NewEncoder(fd)
	enc.SetIndent("", "\t")
	err = enc.Encode(s)
	if err != nil {
		return err
	}

	return nil
}

func NewMemStore() *MemStore {
	return &MemStore{
		kv:      make(map[string]*memTiddler),
		fileRef: make(map[string]int),
	}
}

// make sure MemStore implement Store
var _ Store = (*MemStore)(nil)
