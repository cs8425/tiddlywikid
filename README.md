# tiddlywikid

A server implementation in goling for [TiddlyWiki](http://tiddlywiki.com/), base on [TiddlyWeb](http://tiddlyweb.com/) plugin for tiddler store (for saved tiddlers).

## features

* no dependency, one executable file with a base image of wiki will work.
* use browser cache by default, tiddlers won't send again if there were no update
	* the build-in server of TiddlyWiki will calculate the `md5` of tiddler every time when browser request that tiddler, it will wasting CPU and let latency longer.
* auto generated tiddler
	* `$:/sync-time`: return last time of fetching tiddlers
	* `$:/client-ip`: return IP of wiki user
* provide attachment api (with plugin) for binary files/large tiddlers, avoiding slowing down whole wiki.
* more permission control: multiple users, allow/block by IP range.

## build

get source

	$ git clone git@github.com:cs8425/tiddlywikid.git
	$ cd tiddlywikid

build:

	$ cd cmd
	$ go build -o tiddlywikid -ldflags="-s -w" -trimpath .

## config

hash password with salt: `./tiddlywikid -hash <pwd123456>`

user and access control: `user.json`

```json
{
	"allow-anonymous": {
		"def": true
	},
	"static-file": {
		"def": true
	},
	"allow-anonymous-edit": {
		"def": false
	},
	"users": [
		{
			"id": "test",
			"name": "",
			"hash": "1vm953mjdu+u8t9I:Xm0ZJOrbz+G4B1SClnN27SHW6hJ3hBrSXBf4pBemYEQ="
		}
	]
}
```

## run

CLI:

	./tiddlywikid -l :8080 -v 4 -store path/to/store -db bitcask

parameters:

* `-l :8080` - listen on port 8080 (by default port 4040 on localhost)
* `-base wiki.html` - base TiddlyWiki file with TiddlyWeb plugin
* `-d ./static/files` - path for plugin upload attachments and serve them
* `-db bitcask` - database type: json, bitcask; json will keep all tiddlers in memory! 
* `-store path/to/store` - explicitly specify which file/directory to use for the database (by default `tiddlersDb.json` in the current directory)
* `-auth auth.json` - json file for access control and user login info
* `-gz 5` - gzip compress level (1~9), 0 for disable, -1 for golang default level
* `-crt <crt.pem>`, `-key <key.pem>` - PEM encoded certificate file and private key file for HTTPS server, fill empty (default) for HTTP server
* `-sync-story-sequence` - save `$:/StoryList` and `$:/HistoryList`, will cause some issue when multi-user/multi-window
* `-hash` - hash password with salt, print it, and exit
* `-upload-limit` - size limit for file uploading
* `-tiddler-size-limit` - size limit for a tiddler
* `-parse-limit` - limit the memory usage for parsing when file uploading


## base image

Base image is a html file of TiddlyWiki with `TiddlyWeb` plugin installed.
Plugins must be pre-baked into the TiddlyWiki file, they can not be load via lazily loaded Tiddlers currently.
The `cmd/index.html` is `5.2.6-prerelease` (wiki core patch for file MIME issue) with the `TiddlyWeb`, `Highlight`, `Internals`, `TiddlyWeb External Attachments` plugins added.
There are several ways to build/get the base image.

### by cli
* `npm install tiddlywiki`
* `npx tiddlywiki srv --init server` : init client-server version in `srv` directory
* edit `srv/tiddlywiki.info`
	* `build` >> `index`
		* change `"$:/plugins/tiddlywiki/tiddlyweb/save/offline"` to `"$:/core/save/all"`
	* `plugins`: add more plugins if you want
* `npx tiddlywiki srv --build index` : build `index.html` with plugin `tiddlywiki/tiddlyweb`
	* copy `srv/output/index.html`

### by manually
* open a empty tiddlywiki (can download from [TiddlyWiki website](https://tiddlywiki.com/#GettingStarted)) in web browser
* click the control panel (gear) icon
* click the `Plugins` tab
* click `Get more plugins`
* click `Open plugin library`
* find `TiddlyWeb` plugin and click `install`
* A bar at the top of the page should say `Please save and reload to allow changes to JavaScript plugins to take effect`
* save the new wiki file
* if more plugins is need to be added, open the downloaded file in the web browser and repeat.
* copy the final wiki file to `cmd/index.html`

## TODO

* [x] CSRF token
* [x] file struct of code
* [x] auto reload config
* [ ] more backend DB
	* [x] bitcask
		* [ ] call `Bitcask.Merge()` periodically to reclaim disk space
* [ ] static file upload UI (by html, by tiddler, by plugin)
	* [x] plugin, big file via `$:/Import` will use POST upload
		* [ ] auto inject plugin without change base wiki file
		* [ ] plugin source code and docs
	* [ ] another upload page
	* [ ] tiddler with upload UI
