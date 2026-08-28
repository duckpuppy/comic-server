# Changelog

## [1.11.1](https://github.com/duckpuppy/comic-server/compare/v1.11.0...v1.11.1) (2026-08-28)


### Bug Fixes

* reimport now lets comic-server's live value win a genuine field conflict ([df164f5](https://github.com/duckpuppy/comic-server/commit/df164f5c83a061e0428089a3a645998cc0c4b968))

## [1.11.0](https://github.com/duckpuppy/comic-server/compare/v1.10.9...v1.11.0) (2026-08-28)


### Features

* add Settings UI and REST API for ScanInfo.Scanners/Blacklist ([056e7f6](https://github.com/duckpuppy/comic-server/commit/056e7f674236a81c875f56bbdcd9f59a328a2710))
* manually trigger a sync for an online device from the web UI ([765f9c5](https://github.com/duckpuppy/comic-server/commit/765f9c561e0ed2275b528572bc3be27869491b0a))
* persist sync history to config.db across restarts ([fb3331b](https://github.com/duckpuppy/comic-server/commit/fb3331b92610764ae81db4725d8c6e5e9dec6983))
* surface live per-device sync progress in the web UI ([f887a1e](https://github.com/duckpuppy/comic-server/commit/f887a1e9bab4e3de8cce502f58427e03286a70e9))


### Bug Fixes

* give a device that's still starting up more time before a sync bails out ([675d428](https://github.com/duckpuppy/comic-server/commit/675d428038c9930bfb15ccab22b5718411ea8f89))
* refresh the persistent list tree sidebar's counts on every navigation ([380d3e0](https://github.com/duckpuppy/comic-server/commit/380d3e0a259dac94be65a6e2d9a6c86fa341cab5))

## [1.10.9](https://github.com/duckpuppy/comic-server/compare/v1.10.8...v1.10.9) (2026-08-28)


### Bug Fixes

* flatten .cbz entry names to basename, matching ComicRackCE's writer ([e71c6a6](https://github.com/duckpuppy/comic-server/commit/e71c6a66471578febe4a733f88adb90ef85e5fc8))
* rename .cbz page entries sequentially to match the sidecar's Key ([4fd60e9](https://github.com/duckpuppy/comic-server/commit/4fd60e9012b8cdd37e621cc243516ea310055d7f))

## [1.10.8](https://github.com/duckpuppy/comic-server/compare/v1.10.7...v1.10.8) (2026-08-28)


### Bug Fixes

* normalizeArchiveForSync used a data descriptor instead of writing sizes upfront ([cf97a09](https://github.com/duckpuppy/comic-server/commit/cf97a09bbe8fdd70d909b203c9fe6d2101f63986))

## [1.10.7](https://github.com/duckpuppy/comic-server/compare/v1.10.6...v1.10.7) (2026-08-28)


### Bug Fixes

* also convert all .cbz entries to Stored, not just strip ComicInfo.xml ([0b4ab43](https://github.com/duckpuppy/comic-server/commit/0b4ab43585b665c1464f0bc2061fc1dc01b7fe68))

## [1.10.6](https://github.com/duckpuppy/comic-server/compare/v1.10.5...v1.10.6) (2026-08-28)


### Bug Fixes

* strip embedded ComicInfo.xml/ComicBook.xml before syncing a .cbz ([9c4df89](https://github.com/duckpuppy/comic-server/commit/9c4df89420d536f4b67306d93e14d7c4aa4aff3f))

## [1.10.5](https://github.com/duckpuppy/comic-server/compare/v1.10.4...v1.10.5) (2026-08-28)


### Bug Fixes

* execute() retries were completely invisible in logs, hiding real transfer failures ([7187876](https://github.com/duckpuppy/comic-server/commit/71878760c516f1499f1c7a7d5db03e0e5c96e9a3))

## [1.10.4](https://github.com/duckpuppy/comic-server/compare/v1.10.3...v1.10.4) (2026-08-28)


### Bug Fixes

* WriteFile/DeleteFile never read the device's success/fail acknowledgment ([d1ded94](https://github.com/duckpuppy/comic-server/commit/d1ded9439c9a8175f5728cca225b2334c149c34d))

## [1.10.3](https://github.com/duckpuppy/comic-server/compare/v1.10.2...v1.10.3) (2026-08-28)


### Bug Fixes

* folder items written into sync_information.xml as empty &lt;List&gt; entries ([f884a84](https://github.com/duckpuppy/comic-server/commit/f884a84983bde2fa7005ba2567d2a556ab7225a2))
* on-device filenames kept full Windows path instead of basename ([5f80bca](https://github.com/duckpuppy/comic-server/commit/5f80bca36a2d3e4c0b902a91ae60872b7a7ec681))
* sync_information.xml wrote an empty &lt;List&gt; for 0-book lists ([e48a38a](https://github.com/duckpuppy/comic-server/commit/e48a38ac5d6060c7157a9d395f31e4d8f8d04a20))

## [1.10.2](https://github.com/duckpuppy/comic-server/compare/v1.10.1...v1.10.2) (2026-08-28)


### Bug Fixes

* filterOnlyUnread/filterOnlyRead off-by-one made "only sync unread" a silent no-op ([3489ccc](https://github.com/duckpuppy/comic-server/commit/3489ccca59c39f84954b2d5e2095bf5dc9709157))
* WriteFile/ReadFile used a flat 5s deadline for the whole transfer, not just protocol overhead ([0bb5f73](https://github.com/duckpuppy/comic-server/commit/0bb5f735a2e29454fe660dcbb7ad4274516b956a))

## [1.10.1](https://github.com/duckpuppy/comic-server/compare/v1.10.0...v1.10.1) (2026-08-27)


### Bug Fixes

* non-smart filter list written twice into sync_information.xml, device hides it ([89ae2d6](https://github.com/duckpuppy/comic-server/commit/89ae2d651a7274df6a3bfe5d6cc32c47a6fec972))

## [1.10.0](https://github.com/duckpuppy/comic-server/compare/v1.9.3...v1.10.0) (2026-08-27)


### Features

* retry a whole failed operation with a fresh connection, not just the initial connect ([33612d9](https://github.com/duckpuppy/comic-server/commit/33612d9385897e9a8a955a25acd5d913c618280e))

## [1.9.3](https://github.com/duckpuppy/comic-server/compare/v1.9.2...v1.9.3) (2026-08-27)


### Bug Fixes

* identical lists on device (comic-server-f4i), and stuck sync blocking new attempts (comic-server-2gh) ([bae26a3](https://github.com/duckpuppy/comic-server/commit/bae26a31d08897c72f064120039a736d9ad4a55f))

## [1.9.2](https://github.com/duckpuppy/comic-server/compare/v1.9.1...v1.9.2) (2026-08-27)


### Bug Fixes

* SharedListSettings has no json tags, every web-UI settings save silently discarded ([2f9db03](https://github.com/duckpuppy/comic-server/commit/2f9db035aeb5f78742c40e9f492c7163b788e6d6))

## [1.9.1](https://github.com/duckpuppy/comic-server/compare/v1.9.0...v1.9.1) (2026-08-27)


### Bug Fixes

* abort only checked every 10 operations, and a hung check kept the sync running ([3f6ba4a](https://github.com/duckpuppy/comic-server/commit/3f6ba4a684076507e08928c4a1ab66cf051b1546))
* config.db lands outside any mounted volume in Docker, gets wiped on restart ([d0fd9e2](https://github.com/duckpuppy/comic-server/commit/d0fd9e2df1df7e68060c544005c561b071501d43))
* deploy workflow skips every tagged release, only fires on main pushes ([e35c8c1](https://github.com/duckpuppy/comic-server/commit/e35c8c19518948170794747a2393960234daf7c9))
* hidden Lists Browser action icons still catch taps on real touch devices ([15c1a43](https://github.com/duckpuppy/comic-server/commit/15c1a4355538ddf86d1cb8276cec57482651db9e))
* per-list sync settings silently discarded once a device has 2+ lists ([79e4742](https://github.com/duckpuppy/comic-server/commit/79e474278298d557135418dc4fe1688e65175502))

## [1.9.0](https://github.com/duckpuppy/comic-server/compare/v1.8.3...v1.9.0) (2026-08-26)


### Features

* add always-open config.db foundation for record-shaped config ([1c5ce5f](https://github.com/duckpuppy/comic-server/commit/1c5ce5f8aea61ec18c3772a288ddff3e4f8bed70))
* migrate device registrations and list assignments off config.yaml into config.db ([f64a070](https://github.com/duckpuppy/comic-server/commit/f64a07040731889351bf2a164019fe03fe46a117))
* migrate Komga sync targets off config.yaml into config.db ([92d50b8](https://github.com/duckpuppy/comic-server/commit/92d50b8056e8876717c20175955628b242d48190))


### Bug Fixes

* standardize device settings list button wording to "Assign" ([8f383ff](https://github.com/duckpuppy/comic-server/commit/8f383ffe2ac28ec19a41532ba6ef78abc5042a73))

## [1.8.3](https://github.com/duckpuppy/comic-server/compare/v1.8.2...v1.8.3) (2026-08-26)


### Bug Fixes

* device settings '+ Add List' button, add shared searchable list picker ([06e85cc](https://github.com/duckpuppy/comic-server/commit/06e85cc242e1d559b3047afa86ad1fabb718b6a0))

## [1.8.2](https://github.com/duckpuppy/comic-server/compare/v1.8.1...v1.8.2) (2026-08-26)


### Bug Fixes

* registered devices stay visible when offline, not just discovered ([8ba134b](https://github.com/duckpuppy/comic-server/commit/8ba134be564bb0586c2459a91d9938d5272802ed))

## [1.8.1](https://github.com/duckpuppy/comic-server/compare/v1.8.0...v1.8.1) (2026-08-26)


### Bug Fixes

* device wireless sync now accepts ID lists and reading lists ([f1564d2](https://github.com/duckpuppy/comic-server/commit/f1564d2f7a4c508629047550106fa66963136387))

## [1.8.0](https://github.com/duckpuppy/comic-server/compare/v1.7.0...v1.8.0) (2026-08-26)


### Features

* add trash browser UI with restore (per-item and multi-select) ([27751b9](https://github.com/duckpuppy/comic-server/commit/27751b98a09606a2e87b192adafe4a645803970d))
* restructure list-detail management panels into tabs ([92b852c](https://github.com/duckpuppy/comic-server/commit/92b852c50b9dcb0ac2ef34bc32396b6c4639a815))


### Bug Fixes

* clear Komga last_index_error after a later successful sync ([2819574](https://github.com/duckpuppy/comic-server/commit/2819574daaf136013291b60fd0584527c9177b0a))

## [1.7.0](https://github.com/duckpuppy/comic-server/compare/v1.6.1...v1.7.0) (2026-08-26)


### Features

* disable Convert-to-CBZ button when a list has nothing to convert ([9129ae3](https://github.com/duckpuppy/comic-server/commit/9129ae3ef18ea1ff49167e09a366c274916439ef))
* reimport merges field-by-field instead of overwriting the whole row ([bca1c26](https://github.com/duckpuppy/comic-server/commit/bca1c2672fbd3082924ec6d43a85f6e1e1b30bb0))
* soft-delete books/lists removed from an XML import ([91660c9](https://github.com/duckpuppy/comic-server/commit/91660c975fe88458324df78149e36c738a6dcf8a))


### Bug Fixes

* Komga sync now accepts ID lists and reading lists, not just smart lists ([2e7328c](https://github.com/duckpuppy/comic-server/commit/2e7328c91ce9b533128a5bd31cdb3cc8ff0724b6))
* move comics preview above management panels on list detail page ([0487747](https://github.com/duckpuppy/comic-server/commit/0487747fc86e3f454833f3e4144ec6b5712dc28b))
* SQLiteBackend.UpdateBookFields was silently dropping most fields ([402d0bf](https://github.com/duckpuppy/comic-server/commit/402d0bf0eb6acd36394fadcfec64a8ea266fc103))

## [1.6.1](https://github.com/duckpuppy/comic-server/compare/v1.6.0...v1.6.1) (2026-08-26)


### Bug Fixes

* stamp CBZ entries with conversion time, not the zip zero-value ([2fe1bd6](https://github.com/duckpuppy/comic-server/commit/2fe1bd60e41b466452bf3ffcdb05cf0ca6a7a7a2))

## [1.6.0](https://github.com/duckpuppy/comic-server/compare/v1.5.0...v1.6.0) (2026-08-25)


### Features

* add internal/trash, safe atomic file-replace + quarantine helper ([cc4fd5c](https://github.com/duckpuppy/comic-server/commit/cc4fd5c4a48b2e768cbd6c20843bcf1bb2379f1a))
* implement Convert-to-CBZ feature (comic-server-43b) ([c8452f8](https://github.com/duckpuppy/comic-server/commit/c8452f8057433a1f0cd9da1a46f9252c2b7ad005))

## [1.5.0](https://github.com/duckpuppy/comic-server/compare/v1.4.1...v1.5.0) (2026-08-25)


### Features

* port ScanInformationFromFilename - detect scan-group tag from filename ([0da72f5](https://github.com/duckpuppy/comic-server/commit/0da72f5ec09a8937bc0f62594fb02d75d8401b9a))


### Bug Fixes

* **ci:** stop branch-push builds from racing to publish :latest ([e3d5512](https://github.com/duckpuppy/comic-server/commit/e3d5512358da7f0b0683b66205195197079ee53c))

## [1.4.1](https://github.com/duckpuppy/comic-server/compare/v1.4.0...v1.4.1) (2026-08-24)


### Bug Fixes

* apply library path translation to device-sync file reads too ([acf59ae](https://github.com/duckpuppy/comic-server/commit/acf59ae9353e7ffb49ea25340d9ae77d4490cceb))

## [1.4.0](https://github.com/duckpuppy/comic-server/compare/v1.3.3...v1.4.0) (2026-08-24)


### Features

* one-way push of read/unread status to Komga, opt-in per target ([f42b6c0](https://github.com/duckpuppy/comic-server/commit/f42b6c02c3aa3b716b919d8c71738302397d890d))

## [1.3.3](https://github.com/duckpuppy/comic-server/compare/v1.3.2...v1.3.3) (2026-08-24)


### Bug Fixes

* cover thumbnails never load - display:none blocks native lazy-loading ([7d8f254](https://github.com/duckpuppy/comic-server/commit/7d8f2541145b56ce2d569e66ac5f969d51d1daea))

## [1.3.2](https://github.com/duckpuppy/comic-server/compare/v1.3.1...v1.3.2) (2026-08-24)


### Bug Fixes

* add dedicated library path mapping, independent of Komga's ([378a836](https://github.com/duckpuppy/comic-server/commit/378a8362ffbe36f30eee29efb4107fade26e44dc))

## [1.3.1](https://github.com/duckpuppy/comic-server/compare/v1.3.0...v1.3.1) (2026-08-24)


### Bug Fixes

* apply Komga's local_root/remote_root path mapping to cover extraction ([f16236a](https://github.com/duckpuppy/comic-server/commit/f16236a04f84ebd9bf8aef4b8e5f28dff9b3ee29))
* clarify Komga Collection "Matched" count is series, not issues ([4bbf999](https://github.com/duckpuppy/comic-server/commit/4bbf999249aae8a8321433308b54b427fdd93680))

## [1.3.0](https://github.com/duckpuppy/comic-server/compare/v1.2.1...v1.3.0) (2026-08-23)


### Features

* add up-a-level button to Lists Browser for mobile thumb navigation ([f73bc5f](https://github.com/duckpuppy/comic-server/commit/f73bc5f76f7e480125f4c3a69a0919f8070844ee))


### Bug Fixes

* populate nav Devices badge (was dead code, never called) ([223d684](https://github.com/duckpuppy/comic-server/commit/223d6843eb5eec53e76175a1c20b47a6364987bb))
* populate nav Lists badge on every page load, not just after visiting Lists ([63bbb50](https://github.com/duckpuppy/comic-server/commit/63bbb50f793f26759b86613f2e05f2c652159994))
* stop Lists Browser action buttons from stealing taps on touch screens ([c66e667](https://github.com/duckpuppy/comic-server/commit/c66e667da3427b49ff4e63ae2fe35f8603dda36e))

## [1.2.1](https://github.com/duckpuppy/comic-server/compare/v1.2.0...v1.2.1) (2026-08-22)


### Bug Fixes

* stop relying on Komga's search filter for collection/read-list lookup ([7559dd6](https://github.com/duckpuppy/comic-server/commit/7559dd6be1c2000def1f3fe4048e80decce77070))

## [1.2.0](https://github.com/duckpuppy/comic-server/compare/v1.1.1...v1.2.0) (2026-08-22)


### Features

* add cover-image endpoint for comics ([fed6af3](https://github.com/duckpuppy/comic-server/commit/fed6af3575d6abdde1ff1c3d5fb984c7230bcad3))
* add manual cover-cache invalidation endpoint ([ca49f00](https://github.com/duckpuppy/comic-server/commit/ca49f005ee029ffe2c423abec06fa81f73ec6b06))
* cache resized cover thumbnails on disk ([5d1ef21](https://github.com/duckpuppy/comic-server/commit/5d1ef2186d6e167ae44c7887fcff19ff2240af01))
* **comic-server-1c0:** surface Komga sync status and unmatched books in the web UI ([a829a4a](https://github.com/duckpuppy/comic-server/commit/a829a4a8abe9f8d2ceb171aaa96da01ace2888cf))
* **comic-server-7n7:** resolve smart-list books to Komga IDs via path mapping ([8e06b4c](https://github.com/duckpuppy/comic-server/commit/8e06b4cd4a4bd5f66fa13359efaf3798b3dbc377))
* **comic-server-abg:** add Komga REST API client ([27e6a1d](https://github.com/duckpuppy/comic-server/commit/27e6a1d900f3fd13cf7b23745fb33071cf456b7d))
* **comic-server-bwz:** add real library reload via fsnotify file watcher ([efda618](https://github.com/duckpuppy/comic-server/commit/efda6186f0b62a62d160cb1b305365a3b4e2e4f1))
* **comic-server-f9w:** wire Komga sync into a scheduled background job ([68f1356](https://github.com/duckpuppy/comic-server/commit/68f135696aa933662072c5c8cce2acaf7b8e3cfa))
* **comic-server-wgr:** add config shape for Komga collection/read-list sync ([e83ebee](https://github.com/duckpuppy/comic-server/commit/e83ebee3b5795a341c7ad9bc87fdea7fd084c3bb))
* **comic-server-y4i:** make KeepLastRead count configurable ([7129d9a](https://github.com/duckpuppy/comic-server/commit/7129d9ab4a8fa1dc7f3c4304d1499c7e9c77d94e))
* manage Komga sync targets from the web UI ([320b698](https://github.com/duckpuppy/comic-server/commit/320b69803b86cff666eb230504a2e996b304903c))
* mark read comics in the list detail preview grid ([4324d21](https://github.com/duckpuppy/comic-server/commit/4324d21ac54d303e6fa2667fe8fabe941f7a8f5c))
* replace native browser dialog for list-to-device assignment ([5390708](https://github.com/duckpuppy/comic-server/commit/53907083323a882c8cd1576274f5898533e70088))
* show build commit sha in dashboard header ([0349d69](https://github.com/duckpuppy/comic-server/commit/0349d69495fdb8d9a79e7e3320dc2ddc897c542c))
* show real cover thumbnails in the list detail comics grid ([c22ec69](https://github.com/duckpuppy/comic-server/commit/c22ec69b73d62e5be97be8c2b66f43b283ce5213))
* SQLite backend stays live-synced with its XML source ([4012ef2](https://github.com/duckpuppy/comic-server/commit/4012ef279c3945ef7dfd506af410858a4e3a3f6b))


### Bug Fixes

* ComicVine enrichment now works on the SQLite backend ([0db40a2](https://github.com/duckpuppy/comic-server/commit/0db40a248a440be93cf7e700f1fff42a9696a327))
* nav Lists badge count diverges from dashboard Smart Lists stat ([cc6f10b](https://github.com/duckpuppy/comic-server/commit/cc6f10b16f055b010f0800b682f50b3c4b77e612))
* remove GHCR image pruning that broke every multi-arch push ([dd6c800](https://github.com/duckpuppy/comic-server/commit/dd6c8006cdeeb42284fbd2871493c756d91f6f9e))
* repopulate dashboard panels immediately on back-navigation ([d224382](https://github.com/duckpuppy/comic-server/commit/d2243825000e1e883ea5eccf5cd2128efd533fa7))
* smart list edit fields render dark text on dark background ([cba2c03](https://github.com/duckpuppy/comic-server/commit/cba2c03afecd8438fc2bd320591c4ada651d1373))
* SQLite reverse-sync now clears tags to empty ([fade8c3](https://github.com/duckpuppy/comic-server/commit/fade8c37d62cdd633f9cad0baa88365cffebd59c))
* SQLiteBackend resolves BaseListId smart-list scoping ([85d35a4](https://github.com/duckpuppy/comic-server/commit/85d35a4c3c2977f73019ecf2de3749a1fb684e28))
* use net.JoinHostPort for TCP dial address in protocol client ([1c7245a](https://github.com/duckpuppy/comic-server/commit/1c7245adf00736ef07ac76695a0765b67456fee1))


### Performance Improvements

* serve stale list counts while refreshing in the background ([9d32e0d](https://github.com/duckpuppy/comic-server/commit/9d32e0dfabd0eee7789ed215addd33ca2eab5b5d))
* SQLite backend translates simple matchers to SQL and fixes N+1 loads ([f7d79e9](https://github.com/duckpuppy/comic-server/commit/f7d79e99125c2b20a43915861627470550306015))

## [1.1.1](https://github.com/duckpuppy/comic-server/compare/v1.1.0...v1.1.1) (2026-08-19)


### Bug Fixes

* **deps:** update module github.com/pelletier/go-toml/v2 to v2.4.3 ([672b56e](https://github.com/duckpuppy/comic-server/commit/672b56e7b209ae9342d96a467ecec3e1c5a6d926))
* **deps:** update module github.com/prometheus/client_golang to v1.24.1 ([2e6c638](https://github.com/duckpuppy/comic-server/commit/2e6c638bb8a9cb1e74c004384660d4750635fe90))
* **deps:** update module github.com/prometheus/client_golang to v1.24.1 ([ee2305b](https://github.com/duckpuppy/comic-server/commit/ee2305b09df6374dcd0bccdd0ffce5f843b1fe2a))
* **deps:** update module golang.org/x/net to v0.58.0 ([c1620bf](https://github.com/duckpuppy/comic-server/commit/c1620bf0bb32b2d62fc471f3c39fc048e8c26588))
* **deps:** update module modernc.org/sqlite to v1.57.0 ([27422d3](https://github.com/duckpuppy/comic-server/commit/27422d3d60ec1428b038318ce2248f12a6b04352))

## [1.1.0](https://github.com/duckpuppy/comic-server/compare/v1.0.0...v1.1.0) (2026-08-19)


### Features

* **api:** add REST API endpoints for device configuration management ([39bd779](https://github.com/duckpuppy/comic-server/commit/39bd77993d650b48ce6db030fbed954efd3a1ce8))
* **build:** add run-real and deploy-windows just recipes ([c033d62](https://github.com/duckpuppy/comic-server/commit/c033d62ced8ed753ade2463a429252c6e367d8bf))
* **comic-server-0el:** wire CV scraper into REST API and WebSocket ([909cdf9](https://github.com/duckpuppy/comic-server/commit/909cdf98e435ce7ade648a8e1340cfb823413ab8))
* **comic-server-0g1,comic-server-cqy:** add character/location and content string matchers ([f79622c](https://github.com/duckpuppy/comic-server/commit/f79622c524230b14327a5c5518548791b5486e01))
* **comic-server-1n6:** add series aggregate matchers ([87a0a10](https://github.com/duckpuppy/comic-server/commit/87a0a10f811cb85df6fc61549255c8ce5462d888))
* **comic-server-43s:** add folder management - create, rename, delete, move ([267c30a](https://github.com/duckpuppy/comic-server/commit/267c30aa9c796bd59e12c11c18f98504c8d02b36))
* **comic-server-4hb:** add ComicVine API client with circuit breaker and cache ([a7cff84](https://github.com/duckpuppy/comic-server/commit/a7cff84e7d24a34fc2b96215d83fb48486584f37))
* **comic-server-4hb:** add CV series completeness matchers and docs ([57d9b1e](https://github.com/duckpuppy/comic-server/commit/57d9b1e9aaa709d2e6643e47c39783b180cabb57))
* **comic-server-4hb:** wire ComicVine sync into server startup and add CLI ([2b195b2](https://github.com/duckpuppy/comic-server/commit/2b195b2441b33dc1294137fb470740a306a1151e))
* **comic-server-58d:** add raw matcher endpoint for lossless list editing ([91baafb](https://github.com/duckpuppy/comic-server/commit/91baafb4b3b6962a0b69ca13a628f1675565c9af))
* **comic-server-8t0:** add CV scraper orchestrator and CLI ([fe057b6](https://github.com/duckpuppy/comic-server/commit/fe057b6a25b0f790f7fb84926d29db893140e205))
* **comic-server-axn:** add CBR and CB7 archive support for cover matching ([7f21c19](https://github.com/duckpuppy/comic-server/commit/7f21c197f729078e6bf5fea1affba97ef7e4d9c2))
* **comic-server-b8m:** add duplicate matcher (cross-book comparison) ([2c48062](https://github.com/duckpuppy/comic-server/commit/2c48062a4b88e5e49f3e84a0f18cc448bd48671b))
* **comic-server-bim:** add CV scraper matching engine ([120299d](https://github.com/duckpuppy/comic-server/commit/120299d2aad32a8e08a374745df74c9d70e68471))
* **comic-server-cbq:** add CV scraper metadata writer ([c45c0f1](https://github.com/duckpuppy/comic-server/commit/c45c0f1aa2f8f8987ddd5a68d9fec823eff93086))
* **comic-server-ehi:** add Expression matcher via Starlark ([53f5aa8](https://github.com/duckpuppy/comic-server/commit/53f5aa833bbb6d6a3eedd9be84a65e5ded36a5cf))
* **comic-server-fsy:** add perceptual-hash cover matching (stretch) ([af73c74](https://github.com/duckpuppy/comic-server/commit/af73c74f39fa4f82f418f5b3b829a1cd71b855a7))
* **comic-server-goc:** add ComicVine search and issue detail fetching ([27c43a0](https://github.com/duckpuppy/comic-server/commit/27c43a0b179aa3defff8db5d146fa1be9387a704))
* **comic-server-j4w:** add filename parser for series/issue/year extraction ([067dce5](https://github.com/duckpuppy/comic-server/commit/067dce58e1b7e375eb7f8adc96f661192ad2f42d))
* **comic-server-ltf:** add ReadPercentage numeric matcher ([25db966](https://github.com/duckpuppy/comic-server/commit/25db966655e90a2c01a3a2732ca8d8308364896c))
* **comic-server-na6:** add Manga 4-way enum matcher ([45d6c71](https://github.com/duckpuppy/comic-server/commit/45d6c7175d4b5c3e0800938199b29617b711f1b0))
* **comic-server-nby:** add credits string matchers: Colorist, CoverArtist, Editor, Inker, Letterer, Penciller, Translator ([275f99a](https://github.com/duckpuppy/comic-server/commit/275f99aac7b5ecc8828a12af3897bc754a21deb9))
* **comic-server-v45:** add Yes/No matchers for Checked, BlackAndWhite, HasCustomValues, IsLinked, IsMissing, ModifiedInfo ([e1e3c6e](https://github.com/duckpuppy/comic-server/commit/e1e3c6ee4eedef08d7bf018c3fa19bd8fb45cc56))
* **comic-server-w9k:** add WebP image decode support for cover hashing ([aa9caef](https://github.com/duckpuppy/comic-server/commit/aa9caef1ffe5e81d657c8a1a2959f48996048d42))
* **comic-server-wlz:** add smart list CRUD via API and web UI ([c1398fe](https://github.com/duckpuppy/comic-server/commit/c1398fe04c3b088742c08c42dc92da2f0b087997))
* **comic-server-xyf:** add AllProperties cross-field string matcher ([164c9ba](https://github.com/duckpuppy/comic-server/commit/164c9ba5f861464254430e4ef226172460ec7fde))
* **config:** add database_path config field and COMIC_SERVER_DATABASE_PATH env var ([b85f964](https://github.com/duckpuppy/comic-server/commit/b85f96413e36b04a89d52dfe2b4f021b53b1047b))
* **library:** implement BaseListId scoping for smart lists ([3d110ac](https://github.com/duckpuppy/comic-server/commit/3d110ac7e8eb7911f6044288ddef2ac73341beb9))
* **library:** implement ComicIdListItem support ([607c0b2](https://github.com/duckpuppy/comic-server/commit/607c0b245365e78d596b149a9480f80ec7ade2dc))
* **matchers:** implement P3 matcher batch for parity with ComicRack ([eab6256](https://github.com/duckpuppy/comic-server/commit/eab6256689b90ec10c61495dcaedbc759b93aac5))
* **matchers:** implement P4 matcher batch - Day, Week, NewPages, BookmarkCount, BookPrice ([716063b](https://github.com/duckpuppy/comic-server/commit/716063bdb8406fbe18059d6e4ff9b2ec994085de))
* **storage:** add Backend interface for pluggable library storage ([8f47d8f](https://github.com/duckpuppy/comic-server/commit/8f47d8f8a5492a64b505eb60ff363b8697ab1517))
* **storage:** add missing fields to SQLite schema (v2 migration) ([86eafd0](https://github.com/duckpuppy/comic-server/commit/86eafd05c24b21a1a575cabe5931ba3a32d81bdf))
* **storage:** add SQLite library import with idempotent sync ([9e73e0d](https://github.com/duckpuppy/comic-server/commit/9e73e0de0d5f6f67475725f1b9853f08e26e2295))
* **ui:** replace lists card grid with Explorer-style file browser ([8fd1ffb](https://github.com/duckpuppy/comic-server/commit/8fd1ffbfff9822c32d774b10c0792c78b79a3ad3))
* **ui:** show unread book counts in list browser and tree ([caaa47d](https://github.com/duckpuppy/comic-server/commit/caaa47da47d470703036563c0f3c3856f4185c39))
* **web:** add device sync settings management UI ([6e875ef](https://github.com/duckpuppy/comic-server/commit/6e875ef6984e0b5319a0008cff41abedb654c3d4))
* **web:** refresh dashboard with useful panels and real stats ([4c10e8c](https://github.com/duckpuppy/comic-server/commit/4c10e8cb9da43eb098f1459eaccc15ada7b54dbb))


### Bug Fixes

* **build:** use mise exec for all go commands in justfile ([470ab45](https://github.com/duckpuppy/comic-server/commit/470ab45f52417752e413452c3f246cd6d6cbdc53))
* **comic-server-jzf:** map ComicVine artist/colorer credit roles ([e991fa2](https://github.com/duckpuppy/comic-server/commit/e991fa2f1f29b5d83e21d08763a63eaadfb46259))
* **comic-server-ns7:** add dirty tracking to XMLBackend so Flush/Close are no-ops when clean ([67320d8](https://github.com/duckpuppy/comic-server/commit/67320d87d66c91f3c7e939ada560bc6d58c6bd5c))
* **deps:** update module modernc.org/sqlite to v1.52.0 ([2296965](https://github.com/duckpuppy/comic-server/commit/2296965c441e8a94902b0b56034e3a0caa07a51a))
* **library:** copy MatchValue2 in NewMatcherFromXML for range operators ([a483cc4](https://github.com/duckpuppy/comic-server/commit/a483cc44f75b07380bd0de94d2c26ce51eb2fa4a))
* **library:** correct CustomValues matcher semantics to match ComicRack ([5a316de](https://github.com/duckpuppy/comic-server/commit/5a316deed11855bdce59a454940e85ace2184b43))
* **library:** correct three smart list matcher bugs ([56e87b6](https://github.com/duckpuppy/comic-server/commit/56e87b648c4faba228d99a98383f4bf763611507))
* **library:** count matcher numeric + expandable nested UI matchers ([2e6c674](https://github.com/duckpuppy/comic-server/commit/2e6c67499b50997b475dc909aea9f1da4d61fdd2))
* **library:** use string ops for path extraction to fix Windows cross-compile ([1dc0ea5](https://github.com/duckpuppy/comic-server/commit/1dc0ea556fa395ff95e3f2d920c84ce21c28780d))
* **ui:** folder click navigation and full breadcrumb path in list detail ([2da2ce6](https://github.com/duckpuppy/comic-server/commit/2da2ce65ee5573b6ed4a80f8f05c36076e4b8464))
* **ui:** preserve tree sidebar scroll position across re-renders ([6ff34c2](https://github.com/duckpuppy/comic-server/commit/6ff34c2a7f6bef72a5c5a21eed2564e7a0711442))
* **ui:** prevent stale navigation from rendering over the active page ([d0e0684](https://github.com/duckpuppy/comic-server/commit/d0e0684cea3f612d9bb0f68a865040a2b1597415))
* **ui:** rename hardcoded breadcrumb root from 'Smart Lists' to 'Lists' ([f68911a](https://github.com/duckpuppy/comic-server/commit/f68911ac9c2a735dc6f2bcdf2495871ff5edeb9d))
* **ui:** rename nav tab from 'Smart Lists' to 'Lists' ([7a805f9](https://github.com/duckpuppy/comic-server/commit/7a805f9a62cd6c184a750e84cab50481a2aa3488))
* **web,library:** fix duplicate expand arrows and FileName without extension ([07f762c](https://github.com/duckpuppy/comic-server/commit/07f762cee8769b71855bfd601225a9b9582d91bc))


### Documentation

* Update link to WIRELESS_SYNC_PROTOCOL.md location ([e82b177](https://github.com/duckpuppy/comic-server/commit/e82b1772cb8413909bbff34a5c98f80f88a90ef6))

## [1.1.0](https://github.com/duckpuppy/comic-server/compare/v1.0.0...v1.1.0) (2025-11-18)


### Features

* add API endpoint for updating device smart list settings ([7aeee21](https://github.com/duckpuppy/comic-server/commit/7aeee219aa39fec721a60f5535328a8212c532a2))
* add automated release process with GoReleaser and Release Please ([a3a0a5a](https://github.com/duckpuppy/comic-server/commit/a3a0a5a33091d66f95f313eea8421c83a7209285))
* add client-side router with History API ([d377cf8](https://github.com/duckpuppy/comic-server/commit/d377cf8d0db1d25fea7770e9d67f5ee78ed6cebf))
* add device detail page styling ([ecd9103](https://github.com/duckpuppy/comic-server/commit/ecd910375a0d30d3913f4d46a9ec001b7a73e24d))
* add device history filtering to syncstate.Manager ([b99620d](https://github.com/duckpuppy/comic-server/commit/b99620d74d530f574d6b4b80d9487057c08631b1))
* add device registration persistence to config files ([fa5990f](https://github.com/duckpuppy/comic-server/commit/fa5990fb1ba2dbc1e9e28f7a4350e1bb76dbc45c))
* add DeviceDetail component ([c36b319](https://github.com/duckpuppy/comic-server/commit/c36b319a321818ef7b2fefe4f537c59f73edc601))
* add GET /api/devices/:deviceId endpoint ([3f46bfe](https://github.com/duckpuppy/comic-server/commit/3f46bfe5564bd27b9a4cff04a048bbd8ac646663))
* add GET /api/devices/:deviceId/sync-history endpoint ([bcf0f3c](https://github.com/duckpuppy/comic-server/commit/bcf0f3c2406f1e774584367f329016fd8761934e))
* add GET /api/library/lists endpoint with caching ([a7269df](https://github.com/duckpuppy/comic-server/commit/a7269dff112ff9b2149d833be3653b6aec1ff005))
* add GET /api/library/lists/:listId endpoint ([c6f7f24](https://github.com/duckpuppy/comic-server/commit/c6f7f244af7987f68d0389237f0d92337a08af2a))
* add GET /api/library/lists/:listId/devices endpoint ([555d616](https://github.com/duckpuppy/comic-server/commit/555d6160964c7122cba6186d0d4e15dc4bca735e))
* add GET /api/library/lists/:listId/preview endpoint ([df86c8b](https://github.com/duckpuppy/comic-server/commit/df86c8b076cbaa28f62a3f9849d6b140317f3d71))
* add just build-windows task for WSL2 development ([14cc073](https://github.com/duckpuppy/comic-server/commit/14cc073b0d23eb8ea91b9d1a2da329f9ac072a30))
* add list assignment UI to device detail page ([22107f9](https://github.com/duckpuppy/comic-server/commit/22107f9357bdcaba42c89e1f0b151d6fa0e8f6f6))
* add list count caching system ([c29e976](https://github.com/duckpuppy/comic-server/commit/c29e9768a1c08303b84ee1bb332498b466c0fb97))
* add list detail page with preview ([555cf3c](https://github.com/duckpuppy/comic-server/commit/555cf3c88bc29a27b66f7f2e7803c6a9313e637d))
* add matcher human-readable formatter ([89b26fc](https://github.com/duckpuppy/comic-server/commit/89b26fc975fca71c4e3b0eb2139776890bddf2cf))
* add navigation component with tabs ([ec7fd00](https://github.com/duckpuppy/comic-server/commit/ec7fd00055c22927611bc92f3b818d0d2e351fa7))
* add smart list management API endpoints ([60bc978](https://github.com/duckpuppy/comic-server/commit/60bc978393f41e65a1e7c7c128248eb887b469d9))
* add smart list selection UI to Web dashboard ([15fdd7f](https://github.com/duckpuppy/comic-server/commit/15fdd7fd30fe45a4f5428b8ef48fddb0d0aeed13))
* add smart lists browser page ([2327ae2](https://github.com/duckpuppy/comic-server/commit/2327ae207e01e8226a8333371cf1e196badea25f))
* add test library infrastructure and --ping-device CLI flag ([3eda85e](https://github.com/duckpuppy/comic-server/commit/3eda85e9f8266b5f2e5dd919dbbefd41e0b4cabf))
* enhance direct-ping with auto-registration and periodic pings ([244efc9](https://github.com/duckpuppy/comic-server/commit/244efc9e839017a835c6b0182b32c76fd13158c1))
* implement device registration/unregistration in Web UI ([90c465d](https://github.com/duckpuppy/comic-server/commit/90c465ddd452d6f6312f4aa0237f9f82b8441539))
* implement safe sync operation ordering ([35cac74](https://github.com/duckpuppy/comic-server/commit/35cac74d1551e083d3f7c8b0e75f37b55be79280))
* **library:** add Prometheus metrics for library cache ([bbfa396](https://github.com/duckpuppy/comic-server/commit/bbfa3961cb7778cf53c97c79370399c230689b62))
* **library:** implement in-memory cache with periodic flush ([a9aa1f8](https://github.com/duckpuppy/comic-server/commit/a9aa1f897a88746314e89dcb0081b1c7be4dccf5))
* **protocol:** add device discovery response with CommandClientPong ([b6c67c4](https://github.com/duckpuppy/comic-server/commit/b6c67c41f282cc6df1a356f3d25418b94f4d20a3))
* redesign web UI with dedicated pages for all sections ([babe107](https://github.com/duckpuppy/comic-server/commit/babe1073ead64d7e4b9ec1c4a4e9eb317f010e38))
* register device detail route and navigation ([9be554e](https://github.com/duckpuppy/comic-server/commit/9be554ec6d90ef8c8e16f31b609c193cb00775e8))
* register list API routes ([89403cf](https://github.com/duckpuppy/comic-server/commit/89403cf14a94f8b48843f633569f68415de70ef1))
* **release:** add comprehensive release automation documentation and tools ([7aa53e8](https://github.com/duckpuppy/comic-server/commit/7aa53e8a1b7a255db5b1e764a22bb5958371edae))
* **sync:** add retry logic and delays for individual sidecar reads ([a1c5174](https://github.com/duckpuppy/comic-server/commit/a1c5174123f86fd4082263c7ce39737bfba6bdf0))
* **sync:** add status messages during book deletion ([b2f6d8d](https://github.com/duckpuppy/comic-server/commit/b2f6d8d53cacb3eb03f1c57c5264be41dc9d9693))
* **sync:** implement reverse sync for reading state and user metadata ([d1bb6d0](https://github.com/duckpuppy/comic-server/commit/d1bb6d06242e283b138eda31af7b10f0af8b3c0e))
* **sync:** integrate library cache for batched saves during reverse sync ([48366a5](https://github.com/duckpuppy/comic-server/commit/48366a553be3e3b7bcd7cd26b6d5b3b6261dbc7d))
* **sync:** touch marker file to trigger device library refresh ([4bb301b](https://github.com/duckpuppy/comic-server/commit/4bb301bd14892ceafc7eb725b90fb8efc845132f))
* **ui:** merge smart-list-ui feature with tree navigation and device management ([35fe9bc](https://github.com/duckpuppy/comic-server/commit/35fe9bce81c06982198853e80c639879bcce6aad))
* Web UI redesign with dedicated pages (v0.8) ([#31](https://github.com/duckpuppy/comic-server/issues/31)) ([19284fe](https://github.com/duckpuppy/comic-server/commit/19284fe5745ca636a23f7fd2764487911f0e97e4))
* **web:** add device registration controls to devices browser page ([8549bc8](https://github.com/duckpuppy/comic-server/commit/8549bc814161421730e07266f887d5bcf7abb272))
* **web:** add persistent tree sidebar to list detail page ([1f2e9e0](https://github.com/duckpuppy/comic-server/commit/1f2e9e03fd71e41acb509b34921511ed7f8cf004))
* **web:** add real-time device updates via WebSocket to devices browser ([1867b0e](https://github.com/duckpuppy/comic-server/commit/1867b0ee725c79a9604651c3576f744220a09e06))
* **web:** add real-time sync progress display to device detail page ([d9d8f4d](https://github.com/duckpuppy/comic-server/commit/d9d8f4da6e3d85a6970817ee8cf872ebc3c5c916))
* **web:** add structured matcher display and device assignment UI ([33725f4](https://github.com/duckpuppy/comic-server/commit/33725f443db73baf051030f1bc23f1b25185f96b))
* **web:** add tree sidebar navigation for smart lists ([5bfe5ea](https://github.com/duckpuppy/comic-server/commit/5bfe5ea30b4c0d48b7e21731ea0fd375de89e9f2))
* **web:** persist tree sidebar state across navigation ([c9a2796](https://github.com/duckpuppy/comic-server/commit/c9a2796cf73da47693cf8194f6873ca10637aefa))


### Bug Fixes

* add recursive list search for nested smart lists ([b3be592](https://github.com/duckpuppy/comic-server/commit/b3be5928eb1a4198eb280129e4f4e9f971c316ee))
* add SPA fallback handler for client-side routing ([fee0867](https://github.com/duckpuppy/comic-server/commit/fee0867c873fd5807e0dea27cc5dbd2e1dab7997))
* **api:** use recursive search to find lists in folders when assigning to devices ([556155f](https://github.com/duckpuppy/comic-server/commit/556155f84f7e8f7ebd1cf4783d0d62cbc4db3554))
* correct ComicDb.xml format to match ComicRack schema ([8cb622b](https://github.com/duckpuppy/comic-server/commit/8cb622b43e979818ef3d16011bd81078e84d4fff))
* correct TagsMatcher implementation to match ComicRackCE behavior ([2fe3a0c](https://github.com/duckpuppy/comic-server/commit/2fe3a0cf15cf1af792c47061b7cb2b86d8b99e1e))
* **deps:** update module golang.org/x/net to v0.47.0 ([#32](https://github.com/duckpuppy/comic-server/issues/32)) ([2deed3a](https://github.com/duckpuppy/comic-server/commit/2deed3a5f878bd8b37f7b46803533a218fa25fbb))
* improve SPA fallback handler ([ed58dab](https://github.com/duckpuppy/comic-server/commit/ed58dab6aa66820dca73b6454dcc3624d7582363))
* include smart lists in sync_information.xml for Android UI ([35c2325](https://github.com/duckpuppy/comic-server/commit/35c232550a3ac223738c6e4e486c44ffc1676313))
* **library:** add support for Tags and CustomValues matchers ([2a73161](https://github.com/duckpuppy/comic-server/commit/2a731610e53d05b53ad70a115fd82663f1093a2e))
* resolve critical issues in device detail endpoint ([c27e1a6](https://github.com/duckpuppy/comic-server/commit/c27e1a6ec1f8b77b7bfc412dfc660ac5d52741f9))
* resolve router race condition on initial load ([ee00820](https://github.com/duckpuppy/comic-server/commit/ee00820935356d3b66d41baa072eb9943a416b98))
* **server:** handle sync requests from already-registered devices ([7c6a9c5](https://github.com/duckpuppy/comic-server/commit/7c6a9c5729e9d557665cc872d1e025f86425e8d1))
* **sync:** add fallback to read sidecars individually if batch read fails ([aee7bfc](https://github.com/duckpuppy/comic-server/commit/aee7bfc8e952f9598f0d058b63ab0b22573e738c))
* **sync:** remove final progress update after CommandCompleted ([66a7c03](https://github.com/duckpuppy/comic-server/commit/66a7c0350090966341115155607fe8739ec6c1fa))
* **sync:** send progress update to 100% before CommandCompleted ([c3aa4b6](https://github.com/duckpuppy/comic-server/commit/c3aa4b682a84a8e945056a8aa7f976a2d21919f8))
* **sync:** use actual filenames instead of GUIDs for device storage ([e8826ff](https://github.com/duckpuppy/comic-server/commit/e8826ff151b416c4a560b7dd72f5ed0aa2831771))
* **sync:** use book GUID from sidecar as device book key ([8ca06af](https://github.com/duckpuppy/comic-server/commit/8ca06afdd1ccee8f9b5355f17b0e123f06d99bec))
* **sync:** use recursive search for smart lists in nested folders ([5ad452c](https://github.com/duckpuppy/comic-server/commit/5ad452ceef9646dc0047fc1524dd06c2c70124cf))
* **tests:** correct test expectations for sparse page metadata and sidecar XML format ([2f982da](https://github.com/duckpuppy/comic-server/commit/2f982daad0577195e2191af0bfc4cee8c5706ae6))
* update test to match MatcherInfo struct type ([9e05a9d](https://github.com/duckpuppy/comic-server/commit/9e05a9da4655dd2dd92c37cdb71d2532d4056048))
* use PageCount instead of len(Pages) for page comparison ([787fbab](https://github.com/duckpuppy/comic-server/commit/787fbabd245a924c70ddf07ebd0a39953ffcdfe8))
* use proper logging instead of fmt.Printf for debug output ([70e3e48](https://github.com/duckpuppy/comic-server/commit/70e3e4831848712919575ba27e978effdad173a6))
* **web,api:** fix device registration UI and list assignment issues ([fc992ab](https://github.com/duckpuppy/comic-server/commit/fc992ab8ac7ab19c0f58f7e68a11a741a5de3dfd))
* **web:** add missing listsTree.js script and CSS to HTML ([4ad7a48](https://github.com/duckpuppy/comic-server/commit/4ad7a48043fa43414e0e8845b1890c76b278676c))
* **web:** add null checks to prevent errors on non-dashboard pages ([9037245](https://github.com/duckpuppy/comic-server/commit/9037245b5df803e378531d7487b7b10ed3e34964))
* **web:** define missing CSS variables and fix border hover effect ([438fec1](https://github.com/duckpuppy/comic-server/commit/438fec16969ad05acbeebdef3a74701459a32300))
* **web:** make deviceDetail global to fix onclick handlers ([05435a9](https://github.com/duckpuppy/comic-server/commit/05435a9bd19a60e6ed94b7c79197a31fafacf035))


### Performance Improvements

* **library:** add comprehensive performance benchmarks and analysis ([f285eda](https://github.com/duckpuppy/comic-server/commit/f285eda5eb1629b57a94279a1087af15063accb3))
* **library:** complete Phase 3 - large library profiling ([39bc539](https://github.com/duckpuppy/comic-server/commit/39bc539807bf6a8c64aa1690d45b4104cc586dbd))


### Documentation

* add comprehensive ARCHITECTURE.md documentation ([5265375](https://github.com/duckpuppy/comic-server/commit/5265375fae8ac47213d8b69d1e3eefa3c7a9fc81))
* add device detail API endpoints ([6907cd7](https://github.com/duckpuppy/comic-server/commit/6907cd7e8c5412f55a4643301402c587cc3737cc))
* add manual testing guide for reverse sync functionality ([dae02da](https://github.com/duckpuppy/comic-server/commit/dae02dac0b3b26c0ec103d1acf8a2456026c0507))
* add test library and ping-device documentation to CLAUDE.md ([872d24e](https://github.com/duckpuppy/comic-server/commit/872d24e021f1c04e1f737d8aabea99e67f95ffe6))
* document reverse sync feature in README, CLAUDE.md, and new FEATURES.md ([95c83e0](https://github.com/duckpuppy/comic-server/commit/95c83e087dd09c583709d0ff604bc36973671031))
* document smart list UI and API endpoints ([4afbf12](https://github.com/duckpuppy/comic-server/commit/4afbf122730ada77fef95799080f8db6a3bc09d5))
* mark Phase 2 performance optimizations as complete ([d11c9e8](https://github.com/duckpuppy/comic-server/commit/d11c9e8786dbfbdcce658a0bd60a8326ddc5ff31))
* update Web UI documentation for smart list features ([0d8a5c6](https://github.com/duckpuppy/comic-server/commit/0d8a5c6943d756d7814fe2779f8b7338d3f09685))


### Code Refactoring

* convert session.go to use zerolog for consistent logging ([3ea8017](https://github.com/duckpuppy/comic-server/commit/3ea801706bd00fe02baf147bfa0498ffed38132e))

## [0.8.0](https://github.com/duckpuppy/comic-server/compare/v0.7.1...v0.8.0) (2025-11-08)


### Features

* add automated release process with GoReleaser and Release Please ([a3a0a5a](https://github.com/duckpuppy/comic-server/commit/a3a0a5a33091d66f95f313eea8421c83a7209285))
* add device registration persistence to config files ([fa5990f](https://github.com/duckpuppy/comic-server/commit/fa5990fb1ba2dbc1e9e28f7a4350e1bb76dbc45c))
* add smart list management API endpoints ([60bc978](https://github.com/duckpuppy/comic-server/commit/60bc978393f41e65a1e7c7c128248eb887b469d9))
* add smart list selection UI to Web dashboard ([15fdd7f](https://github.com/duckpuppy/comic-server/commit/15fdd7fd30fe45a4f5428b8ef48fddb0d0aeed13))
* implement device registration/unregistration in Web UI ([90c465d](https://github.com/duckpuppy/comic-server/commit/90c465ddd452d6f6312f4aa0237f9f82b8441539))
* Web UI redesign with dedicated pages (v0.8) ([#31](https://github.com/duckpuppy/comic-server/issues/31)) ([19284fe](https://github.com/duckpuppy/comic-server/commit/19284fe5745ca636a23f7fd2764487911f0e97e4))


### Documentation

* add comprehensive ARCHITECTURE.md documentation ([5265375](https://github.com/duckpuppy/comic-server/commit/5265375fae8ac47213d8b69d1e3eefa3c7a9fc81))

## [0.7.1](https://github.com/duckpuppy/comic-server/compare/comic-server-v0.7.0...comic-server-v0.7.1) (2025-10-31)


### Features

* add automated release process with GoReleaser and Release Please ([a3a0a5a](https://github.com/duckpuppy/comic-server/commit/a3a0a5a33091d66f95f313eea8421c83a7209285))


### Bug Fixes

* correct Release Please config file naming ([9b65072](https://github.com/duckpuppy/comic-server/commit/9b65072103dcf61eea9a4f47cba70c4ba5a40b08))
* disable SHA-based Docker tags for pull requests ([5a879b7](https://github.com/duckpuppy/comic-server/commit/5a879b736e4e3e4bd40ca63b34e1c8594c23f663))
* migrate Release Please to v4 configuration format ([867edcc](https://github.com/duckpuppy/comic-server/commit/867edcc788ff430d78e065c652e6d67360b7b257))

## [Unreleased]

### Added

- Web UI for real-time server monitoring (v0.7)
  - Real-time dashboard with WebSocket updates
  - Device management (registration/unregistration)
  - Sync progress monitoring with progress bars
  - Sync history with file statistics
  - Responsive design with vanilla JavaScript
  - Static file serving with Go embed package

### Changed

- Improved release process with GoReleaser and Release Please automation

## Previous Releases

For details on releases prior to v0.7, see the milestone documentation in CLAUDE.md:

- v0.6 - Graceful device disconnect handling, pagination, device filtering
- v0.5 - Rate limiting and Prometheus metrics
- v0.4 - Per-device sync configuration and CLI commands
- v0.3 - Structured logging with zerolog
- v0.2 - Core sync implementation with smart list filtering
