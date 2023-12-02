package tiddlywikid

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"tiddlywikid/store"
)

/*

build plugin via node.js:

node tiddlywiki.js srv --render '.' 'packed-plugin.json' 'text/plain' '$:/core/templates/exporters/JsonFile' 'exportFilter' '[[$:/plugins/tiddlywiki/tiddlyweb-external-attachments]]'


build wiki with plugin:

node tiddlywiki.js srv --render '.' 'index.html' 'text/plain' '$:/core/save/all'
# node tiddlywiki.js srv --rendertiddler "$:/core/save/all" "index.html" "text/plain"

*/

const (
	GENERATED_PLUGIN = "$:/plugins/tiddlywiki/tiddlyweb-external-attachments"
)

var (

	//go:embed plugin.js
	pluginBuf []byte

	pluginFields = map[string]interface{}{
		"name":        "TiddlyWeb External Attachments",
		"description": "External attachments for TiddlyWeb",
		"list":        "readme settings",
		"version":     "5.2.5",
		"plugin-type": "plugin",
		"dependents":  "",
	}

	pluginFiles = []*TiddlyJSON{
		{
			Title: "$:/config/TiddlyWebExternalAttachments/Enable",
			Text:  "yes",
		},
		{
			Title: "$:/config/TiddlyWebExternalAttachments/OnlyBinary",
			Text:  "yes",
		},
		{
			Title: "$:/config/TiddlyWebExternalAttachments/ExternalAttachmentsPath",
			Text:  "/files/", // TODO: subpath by config
		},
		{
			Title: "$:/config/TiddlyWebExternalAttachments/SizeForExternal",
			Text:  "16384",
		},
		{
			Title: "$:/config/TiddlyWebExternalAttachments/Debug",
			Text:  "no",
		},
		{
			Title: "$:/plugins/tiddlywiki/tiddlyweb-external-attachments/readme",
			Text: `! Introduction

This plugin provides support for importing tiddlers as external attachments. That means that instead of importing binary files as self-contained tiddlers, they are imported as "skinny" tiddlers that reference the original file via the ''_canonical_uri'' field. This reduces the size of the wiki and thus improves performance. However, it does mean that the wiki is no longer fully self-contained.

! Compatibility

This plugin only works when using TiddlyWiki with platforms such as tiddlywikid that support the ''attachment api'' for imported/dragged files.

`,
		},
		{
			Title: "$:/plugins/tiddlywiki/tiddlyweb-external-attachments/settings",
			Text: `When used on platforms that provide the necessary support (such as ~TiddlyDesktop), you can optionally import binary files as external tiddlers that reference the original file via the ''_canonical_uri'' field.

By default, a relative path is used to reference the file. Optionally, you can specify that an absolute path is used instead. You can do this separately for "descendent" attachments -- files that are contained within the directory containing the wiki -- vs. "non-descendent" attachments.

<$checkbox tiddler="$:/config/TiddlyWebExternalAttachments/Enable" field="text" checked="yes" unchecked="no" default="yes"> <$link to="$:/config/TiddlyWebExternalAttachments/Enable">Enable importing files as external attachments</$link> </$checkbox>

<$checkbox tiddler="$:/config/TiddlyWebExternalAttachments/OnlyBinary" field="text" checked="yes" unchecked="no" default="yes"> <$link to="$:/config/TiddlyWebExternalAttachments/OnlyBinary">only binary files as external attachments</$link> </$checkbox>

<$link to="$:/config/TiddlyWebExternalAttachments/ExternalAttachmentsPath">External Attachments Path</$link>: base path for external files <$edit-text tiddler="$:/config/TiddlyWebExternalAttachments/ExternalAttachmentsPath" field="text" tag="input" default="files/"></$edit-text>

<$link to="$:/config/TiddlyWebExternalAttachments/SizeForExternal">Size For External</$link>: use external attachment if file size large than <$edit-text tiddler="$:/config/TiddlyWebExternalAttachments/SizeForExternal" field="text" tag="input" default="16384"></$edit-text> Bytes

`,
		},
		{
			Title: "$:/plugins/tiddlywiki/tiddlyweb-external-attachments/startup.js",
			Type:  "application/javascript",
			Text:  (string)(pluginBuf),
		},
	}

	// marshal all plugin files into string
	pluginPack string

	// //go:embed tiddlyweb-external-attachments.json
	// pluginBuf2 []byte
)

func init() {

	tiddlers := make(map[string]*TiddlyJSON, len(pluginFiles))
	for _, td := range pluginFiles {
		tiddlers[td.Title] = td
	}

	// {"tiddlers": {}}
	aux := &struct {
		Tiddlers map[string]*TiddlyJSON `json:"tiddlers"`
	}{
		Tiddlers: tiddlers,
	}
	buf, err := json.Marshal(aux)
	if err == nil {
		pluginPack = (string)(buf)
	}
}

func NewPlugin(r *http.Request) *store.TiddlyWebJSON {
	// return &store.TiddlyWebJSON{
	// 	Title: GENERATED_PLUGIN,
	// 	// Tags:     autoGenTags,
	// 	Type:     "application/json",
	// 	Fields:   (*store.TiddlerFields)(&pluginFields),
	// 	Text:     (string)(pluginBuf2),
	// 	Revision: "0",
	// }

	return &store.TiddlyWebJSON{
		Title: GENERATED_PLUGIN,
		// Tags:     autoGenTags,
		Type:     "application/json",
		Fields:   (*store.TiddlerFields)(&pluginFields),
		Text:     pluginPack,
		Revision: "0",
	}
}

func NewPluginBuf() []byte {
	// td := &TiddlyJSON{
	// 	Title: GENERATED_PLUGIN,
	// 	// Tags:     autoGenTags,
	// 	Type:     "application/json",
	// 	// Fields:   (*store.TiddlerFields)(&pluginFields),
	// 	Text:     pluginPack,
	// 	Revision: "0",
	// }

	td := make(map[string]interface{})
	td["title"] = GENERATED_PLUGIN
	td["type"] = "application/json"
	td["text"] = pluginPack
	td["revision"] = "0"
	for k, v := range pluginFields {
		td[k] = v
	}

	buf, err := json.Marshal(td)
	if err != nil {
		return nil
	}
	out := make([]byte, 0, len(buf)+64)
	out = append(out, ([]byte)(`<script class="tiddlywiki-tiddler-store" type="application/json">[`)...)
	out = append(out, buf...)
	out = append(out, ([]byte)(`]</script>`)...)
	return out
}

// for plugin use, not fully implement
type TiddlyJSON struct {
	Title    string `json:"title"` // key
	Created  string `json:"created,omitempty"`
	Modified string `json:"modified,omitempty"`
	Type     string `json:"type,omitempty"` // default: "text/vnd.tiddlywiki"
	Text     string `json:"text,omitempty"`
	// Fields   *TiddlerFields `json:"fields,omitempty"`
	// Tags     *TiddlerTags   `json:"tags,omitempty"`
	Revision string `json:"revision,omitempty"`
}
