# Changelog

## [1.1.0](https://github.com/duckpuppy/comic-server/compare/v1.0.0...v1.1.0) (2026-06-19)


### Features

* **api:** add REST API endpoints for device configuration management ([39bd779](https://github.com/duckpuppy/comic-server/commit/39bd77993d650b48ce6db030fbed954efd3a1ce8))
* **build:** add run-real and deploy-windows just recipes ([c033d62](https://github.com/duckpuppy/comic-server/commit/c033d62ced8ed753ade2463a429252c6e367d8bf))
* **comic-server-0g1,comic-server-cqy:** add character/location and content string matchers ([f79622c](https://github.com/duckpuppy/comic-server/commit/f79622c524230b14327a5c5518548791b5486e01))
* **comic-server-1n6:** add series aggregate matchers ([87a0a10](https://github.com/duckpuppy/comic-server/commit/87a0a10f811cb85df6fc61549255c8ce5462d888))
* **comic-server-43s:** add folder management - create, rename, delete, move ([267c30a](https://github.com/duckpuppy/comic-server/commit/267c30aa9c796bd59e12c11c18f98504c8d02b36))
* **comic-server-4hb:** add ComicVine API client with circuit breaker and cache ([a7cff84](https://github.com/duckpuppy/comic-server/commit/a7cff84e7d24a34fc2b96215d83fb48486584f37))
* **comic-server-4hb:** add CV series completeness matchers and docs ([57d9b1e](https://github.com/duckpuppy/comic-server/commit/57d9b1e9aaa709d2e6643e47c39783b180cabb57))
* **comic-server-4hb:** wire ComicVine sync into server startup and add CLI ([2b195b2](https://github.com/duckpuppy/comic-server/commit/2b195b2441b33dc1294137fb470740a306a1151e))
* **comic-server-58d:** add raw matcher endpoint for lossless list editing ([91baafb](https://github.com/duckpuppy/comic-server/commit/91baafb4b3b6962a0b69ca13a628f1675565c9af))
* **comic-server-b8m:** add duplicate matcher (cross-book comparison) ([2c48062](https://github.com/duckpuppy/comic-server/commit/2c48062a4b88e5e49f3e84a0f18cc448bd48671b))
* **comic-server-ltf:** add ReadPercentage numeric matcher ([25db966](https://github.com/duckpuppy/comic-server/commit/25db966655e90a2c01a3a2732ca8d8308364896c))
* **comic-server-na6:** add Manga 4-way enum matcher ([45d6c71](https://github.com/duckpuppy/comic-server/commit/45d6c7175d4b5c3e0800938199b29617b711f1b0))
* **comic-server-nby:** add credits string matchers: Colorist, CoverArtist, Editor, Inker, Letterer, Penciller, Translator ([275f99a](https://github.com/duckpuppy/comic-server/commit/275f99aac7b5ecc8828a12af3897bc754a21deb9))
* **comic-server-v45:** add Yes/No matchers for Checked, BlackAndWhite, HasCustomValues, IsLinked, IsMissing, ModifiedInfo ([e1e3c6e](https://github.com/duckpuppy/comic-server/commit/e1e3c6ee4eedef08d7bf018c3fa19bd8fb45cc56))
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
