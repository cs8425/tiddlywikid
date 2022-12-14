package store

type Store interface {
	List(ap []*TiddlyWebJSON) []byte
	// ListStream(w io.Writer, ap []*TiddlyWebJSON)) error

	// string Revision set by api handler
	Get(key string) (tiddler *TiddlyWebJSON, hash string)

	// Need to do:
	// Remove any revision field
	// Remove `_is_skinny` field, and keep old text
	// Extract `text` field
	Put(key string, tiddler *TiddlyWebJSON, hasMacro bool) (rev uint64, hash string)

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
