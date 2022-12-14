
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


## TODO

* [x] CSRF token
* [x] file struct of code
* [x] auto reload config
* [ ] more backend DB
	* [x] bitcask
* [ ] static file upload UI (by html, by tiddler, by plugin)
	* [x] plugin, big file via `$:/Import` will use POST upload
		* [ ] auto inject plugin without change base wiki file
	* [ ] another upload page
	* [ ] tiddler with upload UI
