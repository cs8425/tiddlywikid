package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"sync/atomic"

	"git.mills.io/prologic/bitcask"
)

type BitcaskStore struct {
	mx      sync.RWMutex
	db      *bitcask.Bitcask
	fileRef *bitcask.Bitcask

	ListCacheState
}

func (s *BitcaskStore) putText(td *StoreTiddler) ([]byte, error) {
	var js map[string]interface{}
	err := json.Unmarshal(td.Meta, &js)
	if err != nil {
		return nil, err
	}
	js["text"] = td.Text
	return json.Marshal(js)
}

func (s *BitcaskStore) List(ap []*TiddlyWebJSON, full bool) []byte {
	// check cache for full
	if !full {
		fmt.Println("[list]slim")
		buf, hasOutput := s.list(full)
		return buildTiddlerList(buf, hasOutput, ap)
	}

	if s.ListCacheState.IsDirty() {
		// do full and write cache
		buf, hasOutput := s.list(full)
		s.Set(buf, hasOutput)
		fmt.Println("[list]full/dirty")
	}
	fmt.Println("[list]full")

	// just do system generated data and return
	out, ok := s.Build(ap)
	if !ok {
		fmt.Println("[list]full,no-cache")
		// no cache, we need to build one
		buf, hasOutput := s.list(full)
		s.Set(buf, hasOutput)
		out, _ = s.Build(ap)
	}
	return out
}

func (s *BitcaskStore) list(full bool) ([]byte, bool) {
	var b bytes.Buffer

	s.mx.RLock()
	defer s.mx.RUnlock()

	// need to put `revision` back
	hasOutput := false
	outputFull := full
	b.WriteByte('[')
	for keyBuf := range s.db.Keys() {
		td := s.get(keyBuf)
		if td == nil {
			continue // ???
		}

		if b.Len() >= ListFullTiddlerMaxSize {
			outputFull = false
		}
		if td.HasMacro || outputFull {
			buf, err := s.putText(td)
			if err != nil {
				continue
			}

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

	return b.Bytes(), hasOutput
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
func (s *BitcaskStore) Put(key string, tiddler *TiddlyWebJSON, hasMacro bool, filePath string) (rev uint64, hash string) {
	keyBuf := ([]byte)(key)
	ref := 0

	s.mx.Lock()
	defer s.mx.Unlock()

	td := s.get(keyBuf)
	if td == nil {
		td = &StoreTiddler{}
		ref = 1 // file ref += 1
	}
	rev, hash = s.putExist(td, tiddler, hasMacro, filePath, ref)

	// update file ref
	if ref != 0 && filePath != "" {
		s.attachRef(filePath, ref)
		// TODO: error handle
	}

	// write back
	err := s.put(keyBuf, td)
	if err != nil {
		// ???
		// fmt.Println("[put]err", key, td, err)
		return
	}

	// flag dirty
	s.FlagDirty()
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

func (s *BitcaskStore) putExist(td *StoreTiddler, tiddler *TiddlyWebJSON, hasMacro bool, fp string, ref int) (rev uint64, hash string) {
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
	td.File = fp

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
		return false, ""
	}

	// flag dirty
	s.FlagDirty()

	// skip if no file
	if td.File == "" {
		return true, ""
	}

	// calc file ref
	count, _ := s.attachRef(td.File, -1)
	if count == 0 {
		// delete ref key
		s.fileRef.Delete([]byte(td.File))
		return true, td.File
	}
	return true, ""
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

	// calc file ref
	_, err := s.attachRef(file, 1)
	if err != nil {
		// ???
		return false
	}

	err = s.put(keyBuf, td)
	return err == nil
}

func (s *BitcaskStore) attachRef(file string, delta int) (int64, error) {
	fnBuf := ([]byte)(file)
	countBuf, err := s.fileRef.Get(fnBuf)
	if err != nil {
		countBuf = make([]byte, 8)
	}
	count := int64(binary.LittleEndian.Uint64(countBuf))
	count += int64(delta)
	binary.LittleEndian.PutUint64(countBuf, uint64(count))
	err = s.fileRef.Put(fnBuf, countBuf)
	if err != nil {
		return -1, err
	}
	return count, err
}

func (s *BitcaskStore) Merge() error {
	if err := s.db.Merge(); err != nil {
		return err
	}
	return s.fileRef.Merge()
}

func (s *BitcaskStore) Close() error {
	if err := s.db.Close(); err != nil {
		return err
	}
	return s.fileRef.Close()
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

	// file ref count
	fRef, err := bitcask.Open(
		path.Join(dir, "attach"),
		bitcask.WithMaxDatafileSize(64*1024*1024), // 64 MB
		bitcask.WithMaxKeySize(256),               // for attach filename
		bitcask.WithMaxValueSize(8),               // for counter int64
	)
	if err != nil {
		return nil, err
	}

	return &BitcaskStore{
		db:      db,
		fileRef: fRef,
	}, nil
}

// make sure BitcaskStore implement Store
var _ Store = (*BitcaskStore)(nil)
