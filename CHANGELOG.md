# Changelog

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
