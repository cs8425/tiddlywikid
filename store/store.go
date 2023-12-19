package store

import (
	"bytes"
	"encoding/json"
	"sync/atomic"
)

const (
	// TODO: configurable
	// Max size when outputting Tiddler with its Text
	// more than this size will output Meta only
	ListFullTiddlerMaxSize = 32 * 1024 * 1024 // 32MB
)

type Store interface {
	List(ap []*TiddlyWebJSON, full bool) []byte
	// ListStream(w io.Writer, ap []*TiddlyWebJSON)) error

	// string Revision set by api handler
	Get(key string) (tiddler *TiddlyWebJSON, hash string)

	// Need to do:
	// Remove any revision field
	// Remove `_is_skinny` field, and keep old text
	// Extract `text` field
	// Extract external file
	Put(key string, tiddler *TiddlyWebJSON, hasMacro bool, filePath string) (rev uint64, hash string)

	Del(key string) (ok bool, file string)

	// bind external attachment to a tiddler for removing file if tiddler delete
	AttachAttachment(key string, file string) bool
}

type TiddlerFields map[string]interface{}

type TiddlerTags []string

// Field values are represented as strings. Lists (like the tags and list fields) use double square brackets to quote values that contain spaces
// Tags must be passed as an array, not a string <<< ???
// https://github.com/Jermolene/TiddlyWiki5/blob/0b1a4f3a4d80a58729ce5b399cf0d38b5f29a279/plugins/tiddlywiki/tiddlyweb/tiddlywebadaptor.js#L280
// Tiddlers are represented as an object containing any of a fixed set of standard fields, with custom fields being relegated to a special property called fields
// standard fields: "bag", "created", "creator", "modified", "modifier", "permissions", "recipe", "revision", "tags", "text", "title", "type", "uri"
type TiddlyWebJSON struct {
	Title    string         `json:"title"` // key
	Created  string         `json:"created,omitempty"`
	Modified string         `json:"modified,omitempty"`
	Type     string         `json:"type,omitempty"` // default: "text/vnd.tiddlywiki"
	Text     string         `json:"text,omitempty"`
	Fields   *TiddlerFields `json:"fields,omitempty"`
	Tags     *TiddlerTags   `json:"tags,omitempty"`

	Modifier    string `json:"modifier,omitempty"`
	Permissions string `json:"permissions,omitempty"`
	Uri         string `json:"uri,omitempty"`

	Recipe   string      `json:"recipe,omitempty"`
	Bag      string      `json:"bag,omitempty"` // default: "default"
	Revision string      `json:"revision,omitempty"`
	IsSkinny interface{} `json:"_is_skinny,omitempty"` // exist or `undefined`

	Rev uint64 `json:"-"` // internal usage
}

type StoreTiddler struct {
	Rev      uint64
	Meta     []byte
	Text     string
	Hash     string // md5? sha1? sha256?
	File     string // external attachment
	HasMacro bool   // `$:/tags/Macro` tag need to send `text` in skinny tiddler
}

type ListCacheState struct {
	isDirty int32        // 1 for dirty, 0 for clear
	cached  atomic.Value // *TiddlerListCache
}

// if true, then we must update cache
func (cs *ListCacheState) IsDirty() bool {
	return atomic.CompareAndSwapInt32(&cs.isDirty, 1, 0)
}

func (cs *ListCacheState) FlagDirty() {
	atomic.StoreInt32(&cs.isDirty, 1)
}

func (cs *ListCacheState) Set(buf []byte, hasOutput bool) {
	lc := &ListCache{
		buf:       buf,
		hasOutput: hasOutput,
	}
	cs.cached.Store(lc)
	atomic.StoreInt32(&cs.isDirty, 0)
}

func (cs *ListCacheState) Build(ap []*TiddlyWebJSON) ([]byte, bool) {
	c, ok := cs.cached.Load().(*ListCache)
	if !ok {
		return nil, false
	}
	return buildTiddlerList(c.buf, c.hasOutput, ap), true
}

type ListCache struct {
	buf       []byte // `[` + tiddler + ....
	hasOutput bool
}

func buildTiddlerList(buf []byte, hasOutput bool, ap []*TiddlyWebJSON) []byte {
	var b bytes.Buffer
	b.Write(buf)

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
