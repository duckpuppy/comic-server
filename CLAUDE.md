# Comic Server - Claude Code Guide

This guide helps Claude Code instances work effectively in the comic-server repository. It holds what every session needs; subsystem detail is in `docs/dev/` (see Developer Reference below).

## Project Overview

**comic-server** is a headless standalone wireless sync server for ComicRack Android/iOS clients, written in Go. It implements the ComicRack wireless synchronization protocol to allow devices to sync comic books without requiring the full ComicRack desktop application.

This is a standalone project with its own git repository, located alongside the [ComicRackCE](https://github.com/maforget/ComicRackCE) project (`../ComicRackCE`).

## Important Rules

⚠️ **CRITICAL**: This is a separate project from ComicRackCE.

- **DO NOT** modify any files in the ComicRackCE directory (`../ComicRackCE`)
- **DO NOT** commit changes to the ComicRackCE repository
- All work should be confined to the `comic-server/` directory
- This project **MUST** remain fully compatible with ComicRack libraries created by ComicRackCE
- The ComicRackCE source code in `../ComicRackCE` is the ultimate reference for library format and protocol behavior

## Quick Start

### Prerequisites

- [mise](https://mise.jdx.dev) for tool version management
- Go 1.25.1 (managed by mise)
- just task runner (managed by mise)

### Setup

```bash
# Install tools
mise install

# Build and test
just dev

# Run the server
just run-dev  # Runs with production tablet ignored
```

### Common Commands

```bash
just --list          # Show all available tasks
just build           # Build the binary
just test            # Run all tests
just test-coverage   # Generate coverage report
just run-dev         # Run with production device ignored
just lint            # Format and lint code
just ci              # Run all checks (lint + test + build)
```

## Development Workflow

1. Create/modify code
2. Run tests: `just test` (`just test-verbose`, `just test-match PATTERN`, `just test-coverage`)
3. Format and lint: `just lint`
4. Build: `just build`
5. Test manually: `just run-dev`

All tests must pass before committing: `just ci` (lint + test + build).

Contributing: keep changes focused and well-tested, update tests when adding features, keep coverage above 80%, follow existing style (`just lint`). When adding a major feature, document it in the matching `docs/dev/` file below (or a new one, added to the index), not in this file.

## Developer Reference (read on demand)

Feature and subsystem notes live under `docs/dev/`. Read a file when the task touches its area; they are not needed every turn.

| File | Read this when... |
|------|-------------------|
| [docs/dev/CODEBASE.md](docs/dev/CODEBASE.md) | you need the package layout, the wire protocol basics (discovery, binary types, device validation), `--ignore-device` / `--ping-device`, or the device/protocol/library-loading packages. |
| [docs/dev/CONFIG_AND_LOGGING.md](docs/dev/CONFIG_AND_LOGGING.md) | you touch `internal/config` (config file, env vars, per-device sync settings, device/list resolution, `config` CLI) or `internal/log`. |
| [docs/dev/TESTING.md](docs/dev/TESTING.md) | you write or run tests, smoke-test the web UI with Playwright, use the test library, or debug multicast/discovery/test failures. |
| [docs/dev/STORAGE.md](docs/dev/STORAGE.md) | you touch the SQLite backend (`import`/`export`, schema), backups and `restore`, or the audit log and rollback. |
| [docs/dev/ARCHIVE_FILES.md](docs/dev/ARCHIVE_FILES.md) | anything writes, replaces or reads a comic archive: `internal/trash`, Convert to CBZ, ComicInfo.xml/ComicBook.xml writes, archive sniffing, variant covers, Page Manager, Reader. |
| [docs/dev/LIBRARY_TOOLS.md](docs/dev/LIBRARY_TOOLS.md) | you work on Needs Attention flags, relink/remove of missing files, duplicate review, format detection, count fill, the book edit form, workflow hero, paper copies, manual lists, or Browse search/facets/bulk edit. |
| [docs/dev/COMICVINE.md](docs/dev/COMICVINE.md) | you touch `internal/comicvine`: enrichment sync, rate limiting and circuit breaker, cache, scrape hold-back and publisher filter, Not in ComicVine / match by link, Missing Issues. |
| [docs/dev/SMART_LISTS.md](docs/dev/SMART_LISTS.md) | you work on smart list evaluation or sync filtering, or on the ComicVine, Expression, Virtual Tag or Duplicate matchers. |
| [docs/dev/AUTH.md](docs/dev/AUTH.md) | you touch login, sessions, API tokens, CSRF, trusted proxies, OIDC single sign-on, or add a route that must stay open. |
| [docs/dev/WEB_UI.md](docs/dev/WEB_UI.md) | you change `internal/api/web/`: routes, navigation groups, the book page, WebSocket events. |
| [docs/dev/HISTORY.md](docs/dev/HISTORY.md) | you need what shipped in the v0.2 to v1.0 milestones. Historical only; the tracker holds current work. |

User-facing docs are the other files in `docs/` (API.md, CONFIGURATION.md, ARCHITECTURE.md, RELEASE_PROCESS.md, WIRELESS_SYNC_PROTOCOL.md...).

## Hard Conventions and Gotchas

These bite across features; the detail is in the file named.

- **Never `os.Remove` a comic file.** Replacing or retiring an archive goes through `internal/trash` (`Replace`, `WriteNew` + `Quarantine`). See ARCHIVE_FILES.md.
- **No bulk scans that read file data.** The production library sits on shared disks that also stream media: background passes stat or list only, with pauses (disk check, archive scan). Record things lazily from reads that already happen (`corrupted_archive`). See LIBRARY_TOOLS.md and ARCHIVE_FILES.md.
- **Unknown is not zero.** Unsynced ComicVine volumes, untagged books and uncached issues are never flagged, counted as missing or matched by numeric CV matchers. See COMICVINE.md.
- **`comic_server_` custom values are comic-server state** (auto-written Format/Count, accepted flags, written hash, not-in-ComicVine marker...) and are excluded from the write fingerprint.
- **Fileless books**: a book with no `FilePath` is a Wanted placeholder unless `BookCollectionStatus` is "Physical" (a paper copy, counted as owned). See LIBRARY_TOOLS.md.
- **`.cbw` is not an archive** (it is an XML web-comic pointer). Check a file's bytes before assuming its format.
- **Read-modify-write through the audit wrapper.** The XML backend's `GetBook` returns a pointer into the live library; `audit.Backend.GetBook` returns a copy. See STORAGE.md.
- **Effective trash path**: config.yaml's `trash_path` is cleared in memory after the config.db migration; read `effectiveTrashConfig`, not `cfg.Server.TrashPath`.
- **New HTTP routes need login automatically** once a password is set (`TestEveryRouteNeedsLoginOnceEnabled`); add to `openAPIPaths` only on purpose. See AUTH.md.
- **ComicRack quirks are deliberate.** Caption format / virtual tags copy ComicRackCE's behaviour; check `../ComicRackCE` source before "fixing" one. See SMART_LISTS.md.
- **Browser smoke tests**: point `--library`/`--config` at a scratch copy of `testdata/library/`, never the committed fixture, and see the `FilePath` note under Standing Notes. See TESTING.md.

## Release Process

**GoReleaser** + **Release Please**, fully automated. Full detail: [docs/RELEASE_PROCESS.md](docs/RELEASE_PROCESS.md).

1. Commit with [Conventional Commits](https://www.conventionalcommits.org/): `feat:` (minor bump), `fix:` (patch), `perf:`, `refactor:`, `docs:`, `revert:` (all shown in CHANGELOG.md); `chore:`, `test:`, `ci:`, `build:`, `style:` (hidden from it, see `release-please-config.json`). Breaking change: `feat!:` / `fix!:` or a `BREAKING CHANGE:` footer (major bump).
2. Release Please keeps a release PR on `main` (version bump, CHANGELOG.md, version files).
3. Merging it tags (e.g. `v0.8.0`) and creates the GitHub Release.
4. GoReleaser builds Linux/macOS/Windows amd64/arm64 archives (`comic-server_VERSION_OS_ARCH.tar.gz`, .zip on Windows) with `checksums.txt`; archives include README.md, CLAUDE.md, CHANGELOG.md and docs/.
5. The Docker workflow publishes `ghcr.io/duckpuppy/comic-server:VERSION` on the new tag.

Local check: `goreleaser check`, `goreleaser release --snapshot --clean --skip=publish`. Emergency manual release: update CHANGELOG.md, `git tag -a vX.Y.Z -m "Release vX.Y.Z"`, `git push origin vX.Y.Z`.

Config files: `.goreleaser.yml`, `release-please-config.json` (v4 format), `.release-please-manifest.json`, `.github/workflows/release-please.yml`, `.github/workflows/release.yml`, `.github/workflows/docker-publish.yml`, `CHANGELOG.md` (generated).

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Task Tracker (GitHub Issues)

Open work is tracked as GitHub Issues in the **private** repo `duckpuppy/comic-server-tracker` (moved from beads on 2026-10-16: cloud threads cannot push beads' Dolt data ref). This repo is public, so keep prod host names, paths and other private detail in the tracker or the handoff notes, not in commits, PR bodies or code. Use `gh` (or the GitHub MCP tools in cloud threads); do NOT use TodoWrite, TaskCreate or markdown TODO lists.

- **Find work**: `gh issue list --repo duckpuppy/comic-server-tracker --label P1` (or `P2`...); `gh issue view N --repo ...`.
- **Priority** is the label `P0` (critical) to `P4` (backlog). **Type** is the label `bug`, `feature`, `task` or `epic`.
- **Create**: `gh issue create --repo duckpuppy/comic-server-tracker --title ... --body ... --label P2,feature`. Put design, acceptance criteria and notes in the body; there is no length pressure.
- **Epics**: make children native sub-issues (cloud MCP: `issue_write` create with `parent_issue_number`, or `sub_issue_write`; CLI: `gh api -X POST repos/duckpuppy/comic-server-tracker/issues/PARENT/sub_issues -F sub_issue_id=<child database id>`).
- **Dependencies**: `gh api -X POST repos/duckpuppy/comic-server-tracker/issues/N/dependencies/blocked_by -F issue_id=<blocker database id>` (get the id with `gh api repos/.../issues/M --jq .id`). No MCP tool exists for this.
- **Close from a PR**: put `Closes duckpuppy/comic-server-tracker#N` in the PR body; merging closes the issue even though the repo differs. The line reveals the private repo name and number in a public PR, nothing else.
- **Old IDs**: the 14 beads still open on 2026-10-16 became issues #1-#14 and keep their `comic-server-xxxx` id in the title. The ~278 closed beads stay in the local beads database (`bd show <id>`) as a read-only archive. Older docs, commits and memory notes cite those ids.
- **Not in the tracker**: durable facts about the code belong in this file (rules and gotchas) or the matching `docs/dev/` file (feature detail); `bd remember` is no longer used.

## Standing Notes

Carried over from `bd remember` entries when beads was retired:

- **Smoke-testing anything that opens `book.FilePath`** (covers, sync, cbzconvert...) against a scratch copy of `testdata/library`: `FilePath` entries in `ComicDb.xml` are RELATIVE (for example `testdata/library/comics/...`) and resolve against the SERVER PROCESS's cwd, not the library file's directory. Pointing `--library`/config at a scratch copy is not enough: also run the server with its cwd inside a scratch tree that mirrors the relative path, or it silently reads and writes the REAL fixture files. Those files are gitignored, so a clean `git status` does NOT prove nothing was touched; verify by content or hash. This once overwrote four committed comic files' ComicInfo.xml during the CBZ conversion work.
- **Untagged books** (about 1,650 of 66K) are mostly hand-collected web comics with no ComicVine id. Do not fuzzy-match them for enrichment.
- **ComicRack matcher compatibility**: existing `ComicBookXxxMatcher` types must stay compatible with ComicRackCE smart lists. New comic-server-only matcher types are fine; ComicRack just will not recognise them.
- **ComicVine scraper reference**: https://github.com/cbanack/comic-vine-scraper (the ComicRack plugin this scraper replaces).

## Session Completion

**When ending a work session**, complete ALL steps below. Work is NOT complete until `git push` succeeds.

1. **File issues for remaining work** in the tracker above.
2. **Run quality gates** (if code changed): `just ci` (tests, lint, build).
3. **Update issue status**: close finished issues (or let the PR's `Closes` line do it), comment on in-progress ones.
4. **Push**:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up**: clear stashes, prune merged branches.
6. **Hand off**: give the next session the context it needs.

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds.
- NEVER stop before pushing; that leaves work stranded locally.
- NEVER say "ready to push when you are"; push it yourself.
- If push fails, resolve and retry until it succeeds.
