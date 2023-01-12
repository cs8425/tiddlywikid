/*\
title: $:/plugins/tiddlywiki/tiddlyweb-external-attachments/startup.js
type: application/javascript
module-type: startup

Startup initialisation

\*/
(function () {

	/*jslint node: true, browser: true */
	/*global $tw: false */
	"use strict";


	const CONFIG_HOST_TIDDLER = "$:/config/tiddlyweb/host";
	const DEFAULT_HOST_TIDDLER = "$protocol$//$host$/";

	const ENABLE_EXTERNAL_ATTACHMENTS_TITLE = "$:/config/TiddlyWebExternalAttachments/Enable";
	const EXTERNAL_ATTACHMENTS_PATH_TITLE = '$:/config/TiddlyWebExternalAttachments/ExternalAttachmentsPath';
	const ONLY_BINARY_TITLE = '$:/config/TiddlyWebExternalAttachments/OnlyBinary';
	const SIZE_TITLE = '$:/config/TiddlyWebExternalAttachments/SizeForExternal';
	const DEBUG_TITLE = '$:/config/TiddlyWebExternalAttachments/Debug';


	const WIKITEXT_TYPE = 'text/vnd.tiddlywiki';

	// Export name and synchronous status
	exports.name = "tiddlyweb-external-attachments";
	exports.platforms = ["browser"];
	exports.after = ["startup"];
	exports.synchronous = true;

	exports.startup = function () {
		const log = (...args) => {
			if ($tw.wiki.getTiddlerText(DEBUG_TITLE, "") !== "yes") return;
			console.log.apply(console, args)
		};

		log("[startup]", exports, this);

		const refTable = {};

		// add file reference
		$tw.hooks.addHook("th-importing-file", function (info) {
			if ($tw.wiki.getTiddlerText(ENABLE_EXTERNAL_ATTACHMENTS_TITLE, "") !== "yes") return false; // skip
			log("[th-importing-file]", exports, info);

			const {
				file,
				type,
				isBinary,
				callback,
				deserializer,
				...other
			} = info;

			// Figure out if we're reading a binary file
			let contentTypeInfo = $tw.config.contentTypeInfo[type];
			// let isBinary = contentTypeInfo ? contentTypeInfo.encoding === "base64" : false;
			let isBin = (contentTypeInfo && contentTypeInfo.encoding === "base64") || !contentTypeInfo || isBinary;

			// $tw.wiki.readFileContent(file, type, isBin, options.deserializer, callback);
			$tw.wiki.readFileContent(file, type, isBin, deserializer, (data) => {
				log("[readFileContent]cb", info, file, data);
				if (data.length && isBin) {
					const token = `${Date.now()}-${makeid(8)}`;
					data[0]['ref'] = token;
					refTable[token] = file;
				}
				callback(data);
			});
			return true;
		});

		// remove unused reference
		$tw.hooks.addHook("th-before-importing", function (importTiddler) {
			if ($tw.wiki.getTiddlerText(ENABLE_EXTERNAL_ATTACHMENTS_TITLE, "") !== "yes") return importTiddler; // skip

			let tiddlers = JSON.parse(importTiddler.fields.text || '{}').tiddlers || {};
			for (let title in tiddlers) {
				const tiddler = tiddlers[title];
				if (importTiddler.fields["selection-" + title] === "unchecked") {
					const token = tiddler.ref;
					delete refTable[token]; // remove reference
				}
			}
			log("[th-before-importing]", exports, importTiddler, tiddlers, refTable);

			return importTiddler;
		});

		$tw.hooks.addHook("th-importing-tiddler", (tiddler) => {
			if ($tw.wiki.getTiddlerText(ENABLE_EXTERNAL_ATTACHMENTS_TITLE, "") !== "yes") return tiddler; // skip

			log("[th-importing-tiddler]0", exports, tiddler, refTable); // debug
			const {
				title,
				type = WIKITEXT_TYPE,
				text = null,
				ref = null, // file reference
				...other
			} = tiddler.fields || {};

			const file = refTable[ref] || null;
			if (ref !== null) { // remove reference
				tiddler = new $tw.Tiddler(tiddler, { ref: null });
				// delete tiddler.fields[ref];
				delete refTable[ref];
			}
			log("[th-importing-tiddler]", tiddler?.fields, file, refTable); // debug

			// check binary and size
			let contentTypeInfo = $tw.config.contentTypeInfo[type]
			let isBinary = (contentTypeInfo && contentTypeInfo.encoding === "base64") || !contentTypeInfo;
			if (!isBinary && $tw.wiki.getTiddlerText(ONLY_BINARY_TITLE, "yes") === "yes") return tiddler; // skip text file

			const size = Number.parseInt($tw.wiki.getTiddlerText(SIZE_TITLE, "")) || 16384;
			if (text && text.length < size) return tiddler; // skip small file

			// do upload, and update Tiddler when finish
			return doUpload(tiddler.fields, file);
		});

		const doUpload = (tiddlerFields, file) => {
			const {
				title,
				type = WIKITEXT_TYPE,
				text = null,
				...other
			} = tiddlerFields;

			// const convFn = $tw.syncadaptor.convertTiddlerToTiddlyWebFormat || JSON.stringify;
			const convFn = JSON.stringify;
			const meta = {
				title,
				type,
				...other
			};
			if (!file) { // use base64 string in text to build blob
				const textU8 = Uint8Array.from(atob(text), (c) => c.charCodeAt(0));
				file = new Blob([textU8], { type: 'application/octet-stream' });
			}
			const data = new FormData();
			data.append('meta', convFn(meta));
			data.append('text', file, 'text');

			log("[upload]", title, tiddlerFields, data, meta, file);

			// $tw.utils.httpRequest() can not use FormData
			fetch(`${getHost()}upload/`, {
				method: 'POST',
				mode: 'cors',
				// headers: {
				// 	'Content-type': 'multipart/form-data',
				// },
				credentials: 'same-origin',
				body: data,
			}).then(async (resp) => {
				if (!resp.ok) throw new Error(resp.statusText);

				// TODO: use json?
				const token = await resp.text();

				// TODO: update tiddler
				const base_path = $tw.wiki.getTiddlerText(EXTERNAL_ATTACHMENTS_PATH_TITLE, "/");
				const tiddler = new $tw.Tiddler(tiddlerFields, {
					type: type, // set type back
					text: null, // remove text
					'_canonical_uri': `${base_path}${token}`,
				});
				$tw.wiki.addTiddler(tiddler);
			}).catch((err) => {
				log("[upload]err", title, tiddlerFields, err);
				// show error as text
				$tw.wiki.addTiddler(new $tw.Tiddler(tiddlerFields, {
					type: WIKITEXT_TYPE,
					text: `! upload \`${title}\` error
\`\`\`
${err}
\`\`\``,
				}));
			})

			// show uploading
			return new $tw.Tiddler(tiddlerFields, {
				type: WIKITEXT_TYPE,
				text: `uploading.... \`${title}\``,
			});
		};
	};

	const getHost = () => {
		var text = $tw.wiki.getTiddlerText(CONFIG_HOST_TIDDLER, DEFAULT_HOST_TIDDLER),
			substitutions = [
				{ name: "protocol", value: document.location.protocol },
				{ name: "host", value: document.location.host }
			];
		for (var t = 0; t < substitutions.length; t++) {
			var s = substitutions[t];
			text = $tw.utils.replaceString(text, new RegExp("\\$" + s.name + "\\$", "mg"), s.value);
		}
		return text;
	};

	const makeid = (length) => {
		var result = '';
		var characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
		var charactersLength = characters.length;
		for (var i = 0; i < length; i++) {
			result += characters.charAt(Math.floor(Math.random() * charactersLength));
		}
		return result;
	}

	console.log("[init]", exports, this)

})();
