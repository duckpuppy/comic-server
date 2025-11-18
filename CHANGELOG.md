# Changelog

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
