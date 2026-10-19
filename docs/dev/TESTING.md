# Testing

Test layout, browser testing with Playwright, test infrastructure and the small test library.

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

## Quality Gate

All tests must pass before committing:

```bash
just ci  # Runs lint + test + build
```

## Running Tests

```bash
just test              # All tests
just test-verbose      # Verbose output
just test-coverage     # Generate HTML coverage report
just test-match PATTERN  # Run specific tests
```

### Test Organization

**Unit Tests** - Component-level testing:

- `internal/protocol/protocol_test.go`: Binary encoding/decoding (21 tests)
- `internal/device/discovery_test.go`: Message parsing (11 tests)
- `internal/device/info_test.go`: INI parsing and validation (multiple test suites)
- `internal/library/library_test.go`: Library XML parsing (11 test suites, 24 test cases)
- `internal/library/smartlist_test.go`: Smart list matcher evaluation (100+ test cases)
- `internal/sync/sync_test.go`: Sync operation planning and execution
- `internal/sync/smartlist_test.go`: Smart list filtering integration with sync
- `internal/config/config_test.go`: Configuration structures and operations (10 tests)
- `internal/config/loader_test.go`: YAML/TOML loading and saving (9 tests)
- `internal/config/helpers_test.go`: Device and smart list resolution (9 tests)
- `internal/config/xdg_test.go`: XDG directory resolution (2 tests)
- `internal/log/log_test.go`: Logging initialization and output (7 tests)

**Integration Tests** - End-to-end workflow testing:

- `internal/protocol/integration_test.go`: Client-server communication with mock device server
  - Tests file transfer operations (ReadFile, FileExists)
  - Tests device info retrieval (CommandInfo)
  - Tests connection handling, retries, and large file transfers
- `internal/device/integration_test.go`: Device discovery and registration workflows
  - UDP multicast discovery testing with mock broadcasters
  - Full device validation and registry management
  - End-to-end discovery → connect → validate → register workflow
- `internal/library/integration_test.go`: Real library parsing with testdata
  - Uses actual ComicDB.xml file from `testdata/` directory
  - Tests loading, parsing, and querying large comic libraries
  - Benchmarks library loading performance
- `internal/sync/integration_test.go`: Full sync session testing
  - Tests complete sync workflow with mock device
  - Tests error handling and progress reporting
  - Tests sync with smart list filtering

### Browser Testing (Playwright)

For verifying web UI changes (`internal/api/web/`) with a real headless
browser instead of just reading the JS/CSS. Setup is npm-based
(`package.json`, `.gitignore` allows it and ignores `node_modules/`):

```bash
npm install                    # installs the playwright package
npx playwright install chromium  # downloads the actual browser binary (~150-300MB, one-time)
```

`npx playwright install --with-deps` (which also apt-installs system
libraries) needs root and will fail in this sandbox - skip `--with-deps`.
Chromium launches fine headless without it here.

A driver script must live inside this repo (or anywhere under it) for
Node's `require('playwright')` to resolve via `node_modules` - a script
under `/tmp` or the scratchpad directory won't find it. Write one-off
driver scripts as dotfiles in the repo root (e.g. `.dm-click-through.js`),
delete them when done, and confirm `git status` is clean afterward (they're
untracked, not gitignored, so `git status` will show them until removed).

**Point `--library`/`--config` at a scratch copy, never
`testdata/library/ComicDb.xml` directly** - any write path (a rule apply,
a sync, a metadata edit) will silently overwrite the real committed
fixture through a real headless browser session exactly like it would
through any other client. Copy `testdata/library/` to a scratch directory
first (see the Test Library section below and the smoke-testing note in `CLAUDE.md` on
`book.FilePath` being relative to server cwd, not `--library`'s directory)
and point everything at the copy.

### Test Infrastructure

**Mock Device Server** (`internal/protocol/testserver.go`):

- Simulates a ComicRack device for testing
- Implements protocol commands (ReadFile, FileExists, CommandInfo)
- Supports in-memory file system for test data
- Used by integration tests to verify client-server communication

**Test Data**:

- `testdata/ComicDB.xml`: Real ComicRack library XML (226MB) with actual comic metadata
- Used for integration testing library parsing and querying
- `testdata/library/`: Minimal test library for rapid development (see Test Library section below)

### Test Library

**Location**: `testdata/library/`

A minimal test library for rapid development and testing without requiring a production library (65K+ books).

**Contents**:
- **3 comic books** - Small files (league-001.cbz, league-002.cbz, kids-club-002.cbz)
- **4 smart lists** - Pre-configured to match the test comics
- **ComicDb.xml** - Complete ComicRack-compatible library database
- **Documentation** - Comprehensive README with usage examples

**Comic Books** (user-provided, not in git):

1. **The League of Extraordinary Gentlemen: Century #1** (2009) - 80 pages, Top Shelf, Superhero
2. **The League of Extraordinary Gentlemen: Century #2** (2011) - 80 pages, Top Shelf, Superhero
3. **Top Shelf Kids Club #2** (2012) - 48 pages, Top Shelf, Kids

**Smart Lists**:

1. **League Series** - Matches 2 books (Series contains "League")
2. **Top Shelf Publisher** - Matches 3 books (Publisher equals "Top Shelf")
3. **2009-2011** - Matches 2 books (Year in range 2009-2011)
4. **All Comics** - Matches 3 books (PageCount > 0)

**Usage Examples**:

```bash
# Run server with test library
./comic-server server --library testdata/library/ComicDb.xml

# List smart lists
./comic-server config list-smartlists --library testdata/library/ComicDb.xml

# Add list to device
./comic-server config add-list "My Tablet" "League Series" \
  --library testdata/library/ComicDb.xml

# With direct IP ping (bypass multicast)
./comic-server server --library testdata/library/ComicDb.xml \
  --ping-device 192.168.0.24
```

**Advantages**:
- **Fast**: 3 books vs 65K+ books - sync completes in seconds
- **Safe**: No risk of corrupting production library during client-to-server sync tests
- **Portable**: ~6MB total (excluding comic files) - easy to commit to git
- **Predictable**: Known content makes debugging easier
- **Complete**: Real comic files with actual pages, not mocks

**Comic Files**:
- Comic files are **excluded from git** via `.gitignore`
- Users provide their own comics for testing (see `testdata/library/comics/README.md`)
- Recommended sources for public domain comics:
  - [Comic Book Plus](https://comicbookplus.com/) (free registration required)
  - [Digital Comic Museum](https://digitalcomicmuseum.com/)
  - [Internet Archive](https://archive.org/details/comics)

**File Structure**:

```
testdata/library/
├── README.md           # Comprehensive usage guide
├── ComicDb.xml         # Library database with books and smart lists
└── comics/             # Comic book files (user-provided)
    ├── .gitignore      # Excludes *.cbz, *.cbr, *.cb7, *.cbt
    ├── README.md       # Comic sources and setup instructions
    ├── league-001.cbz  # User provides
    ├── league-002.cbz  # User provides
    └── kids-club-002.cbz  # User provides
```

**Testing Scenarios**:

1. **Fresh Device Sync** - Configure "All Comics" smart list → sync 3 books
2. **Partial Sync** - Configure "League Series" smart list → sync 2 books
3. **Year Range Filter** - Configure "2009-2011" smart list → sync 2 books (excludes 2012)
4. **Publisher Filter** - Configure "Top Shelf Publisher" smart list → sync 3 books

**Integration with Tests**:

```go
func TestSyncWithSmartList(t *testing.T) {
    lib, err := library.LoadLibrary("testdata/library/ComicDb.xml")
    // Test sync logic with known data
}
```

## Common Issues

### Multicast Not Working

- Check firewall: Ports 7615/udp, 7614/tcp, 7620/tcp, IGMP protocol
- Verify network interface supports multicast
- Ensure not running in WSL2 (use real Linux or Windows)

### Device Not Discovered

- Check device is broadcasting on network
- Use tcpdump to verify packets: `sudo tcpdump -i any -n udp port 7615`
- Ensure device broadcasts with "ComicRack" prefix

### Tests Failing

- Run `go mod tidy` to ensure dependencies are correct
- Check Go version: `go version` (should be 1.25.1)
- Run `mise install` to ensure tools are up to date
