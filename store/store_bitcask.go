package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"git.mills.io/prologic/bitcask"
)

type BitcaskStore struct {
	mx sync.RWMutex
	db *bitcask.Bitcask
}

func (s *BitcaskStore) List(ap []*TiddlyWebJSON) []byte {
	var b bytes.Buffer

	s.mx.RLock()
	defer s.mx.RUnlock()

	// need to put `revision` back
	hasOutput := false
	b.WriteByte('[')
	for keyBuf := range s.db.Keys() {
		td := s.get(keyBuf)
		if td == nil {
			continue // ???
		}

		if td.HasMacro {
			var js map[string]interface{}
			err := json.Unmarshal(td.Meta, &js)
			if err != nil {
				continue
			}
			js["text"] = td.Text
			buf, _ := json.Marshal(js)

			if hasOutput {
				b.WriteByte(',')
			}
			b.Write(buf)
		} else {
			if hasOutput {
				b.WriteByte(',')
			}
			b.Write(td.Meta)
			// js["revision"] = fmt.Sprintf("%v", td.rev)
		}

		// flag for ','
		hasOutput = true
	}

	// for system generated data
	for _, td := range ap {
		buf, err := json.Marshal(td)
		if err != nil {
			hasOutput = false
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

func (s *BitcaskStore) Get(key string) (*TiddlyWebJSON, string) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	td := s.get(([]byte)(key))
	if td == nil {
		return nil, ""
	}

	tiddler := &TiddlyWebJSON{}
	err := json.Unmarshal(td.Meta, tiddler)
	if err != nil {
		// ????
		return nil, ""
	}

	// set default value & add text back
	tiddler.Rev = td.Rev
	tiddler.Text = td.Text
	return tiddler, td.Hash
}

func (s *BitcaskStore) get(key []byte) *StoreTiddler {
	tdBuf, err := s.db.Get(key)
	if err != nil {
		return nil
	}

	r := bytes.NewReader(tdBuf)
	dec := gob.NewDecoder(r)

	td := &StoreTiddler{}
	err = dec.Decode(td)
	if err != nil {
		// decode error
		return nil
	}
	return td
}

// need to do:
// Remove any revision field
// Remove `_is_skinny` field, and keep old text
// Extract `text` field
func (s *BitcaskStore) Put(key string, tiddler *TiddlyWebJSON, hasMacro bool) (rev uint64, hash string) {
	keyBuf := ([]byte)(key)

	s.mx.Lock()
	defer s.mx.Unlock()

	td := s.get(keyBuf)
	if td == nil {
		td = &StoreTiddler{}
	}
	rev, hash = s.putExist(td, tiddler, hasMacro)

	// write back
	err := s.put(keyBuf, td)
	if err != nil {
		// ???
		// fmt.Println("[put]err", key, td, err)
		return
	}
	return
}

func (s *BitcaskStore) put(key []byte, td *StoreTiddler) error {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	err := enc.Encode(td)
	if err != nil {
		// ???
		return err
	}
	err = s.db.Put(key, b.Bytes())
	return err
}

func (s *BitcaskStore) putExist(td *StoreTiddler, tiddler *TiddlyWebJSON, hasMacro bool) (rev uint64, hash string) {
	// Remove `_is_skinny` field, and keep old text
	if tiddler.IsSkinny != nil {
		tiddler.IsSkinny = nil
		tiddler.Text = td.Text
	}

	rev = atomic.AddUint64(&td.Rev, 1)

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
	td.Meta = meta
	td.Text = text
	td.HasMacro = hasMacro
	td.Hash = hash

	return rev, hash
}

// remove attachment file at api
func (s *BitcaskStore) Del(key string) (bool, string) {
	keyBuf := ([]byte)(key)

	s.mx.Lock()
	defer s.mx.Unlock()
	td := s.get(keyBuf)
	if td == nil {
		return false, ""
	}
	err := s.db.Delete(keyBuf)
	if err != nil {
		return false, td.File
	}
	return true, td.File
}

func (s *BitcaskStore) AttachAttachment(key string, file string) bool {
	keyBuf := ([]byte)(key)

	s.mx.Lock()
	defer s.mx.Unlock()
	td := s.get(keyBuf)
	if td == nil {
		return false
	}
	td.File = file

	err := s.put(keyBuf, td)
	if err != nil {
		// ???
		return false
	}
	return err == nil
}

func (s *BitcaskStore) Close() error {
	return s.db.Close()
}

func NewBitcaskStore(dir string) (*BitcaskStore, error) {
	db, err := bitcask.Open(
		dir,
		bitcask.WithMaxDatafileSize(64*1024*1024), // 64 MB
		bitcask.WithMaxKeySize(8192),              // for tiddler title
		bitcask.WithMaxValueSize(0),               // remove value size limit
	)
	if err != nil {
		return nil, err
	}
	return &BitcaskStore{
		db: db,
	}, nil
}

// make sure BitcaskStore implement Store
var _ Store = (*BitcaskStore)(nil)
