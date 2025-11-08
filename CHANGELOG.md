# Changelog

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
