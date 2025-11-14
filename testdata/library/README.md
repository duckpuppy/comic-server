# Test Library for comic-server

This directory contains a minimal test library for rapid development and testing of sync functionality without requiring a large production library.

## Contents

- **Comic files directory** (`comics/`) - Place 3-5 small comic files here (see comics/README.md)
- **4 pre-configured smart lists** - Cover common filtering scenarios
- **ComicDb.xml** - Complete ComicRack-compatible library database

**Note:** Comic files are not included in the repository. You need to provide your own comics for testing. See `comics/README.md` for sources of public domain comics.

## Comic Books

1. **The League of Extraordinary Gentlemen: Century #1** (2009)
   - Series: The League of Extraordinary Gentlemen: Century
   - Publisher: Top Shelf
   - Pages: 80
   - Genre: Superhero

2. **The League of Extraordinary Gentlemen: Century #2** (2011)
   - Series: The League of Extraordinary Gentlemen: Century
   - Publisher: Top Shelf
   - Pages: 80
   - Genre: Superhero

3. **Top Shelf Kids Club #2** (2012)
   - Series: Top Shelf Kids Club
   - Publisher: Top Shelf
   - Pages: 48
   - Genre: Kids

## Smart Lists

### 1. League Series
- **Matches:** 2 books
- **Filter:** Series contains "League"
- **Books:** League #1, League #2

### 2. Top Shelf Publisher
- **Matches:** 3 books (all)
- **Filter:** Publisher equals "Top Shelf"
- **Books:** All 3 books

### 3. 2009-2011
- **Matches:** 2 books
- **Filter:** Year in range 2009-2011
- **Books:** League #1 (2009), League #2 (2011)

### 4. All Comics
- **Matches:** 3 books (all)
- **Filter:** PageCount > 0
- **Books:** All 3 books
- **Use case:** Testing full library sync

## Usage

### Run Server with Test Library

```bash
# From project root
./comic-server server --library testdata/library/ComicDb.xml

# With debug logging
./comic-server server --library testdata/library/ComicDb.xml --log-level debug

# With specific smart list
./comic-server config add-list <device-name> "League Series" \
  --library testdata/library/ComicDb.xml
```

### List Smart Lists

```bash
./comic-server config list-smartlists --library testdata/library/ComicDb.xml
```

Output:
```
Found 4 smart lists:
  1. League Series (ID: {test-list-league})
  2. Top Shelf Publisher (ID: {test-list-topshelf})
  3. 2009-2011 (ID: {test-list-years})
  4. All Comics (ID: {test-list-all})
```

### Test Sync Without Production Data

This test library is ideal for:

1. **Client-to-Server Sync Testing** - Safe to test reading state updates without risking production data
2. **Smart List Testing** - Multiple lists with different filter types
3. **Fast Iteration** - Only 3 books sync in seconds vs minutes
4. **Network Isolation** - Test multicast discovery issues without production devices
5. **Integration Tests** - Can be used in automated testing

### With Direct IP Ping (Bypass Multicast Discovery)

```bash
# Ping device directly at specific IP (useful for WSL2, VPNs, complex networks)
./comic-server server --library testdata/library/ComicDb.xml \
  --ping-device 192.168.0.24

# Custom port
./comic-server server --library testdata/library/ComicDb.xml \
  --ping-device 192.168.0.24:7614
```

## Testing Scenarios

### Scenario 1: Fresh Device Sync
1. Start server with test library
2. Configure device with "All Comics" smart list
3. Sync - should transfer all 3 books

### Scenario 2: Partial Sync
1. Configure device with "League Series" smart list
2. Sync - should transfer only 2 books (League #1, #2)

### Scenario 3: Year Range Filter
1. Configure device with "2009-2011" smart list
2. Sync - should transfer 2 books (excludes 2012 Kids Club)

### Scenario 4: Publisher Filter
1. Configure device with "Top Shelf Publisher" smart list
2. Sync - should transfer all 3 books

## File Structure

```
testdata/library/
├── README.md           # This file
├── ComicDb.xml         # Library database with books and smart lists
└── comics/             # Comic book files
    ├── league-001.cbz
    ├── league-002.cbz
    └── kids-club-002.cbz
```

## Advantages Over Production Library

- **Fast**: 3 books vs 65K+ books - sync completes in seconds
- **Safe**: No risk of corrupting production library during client-to-server sync tests
- **Portable**: 5.7MB total - easy to commit to git, transfer to test machines
- **Predictable**: Known content makes debugging easier
- **Complete**: Real comic files with actual pages, not mocks

## Integration with Tests

This library can be used in integration tests:

```go
func TestSyncWithSmartList(t *testing.T) {
    lib, err := library.LoadLibrary("testdata/library/ComicDb.xml")
    // Test sync logic with known data
}
```

## Notes

- File paths in ComicDb.xml are relative to project root
- Smart list IDs use `{test-list-*}` format for clarity
- Comics are real files from Top Shelf Comics (open-source friendly publisher)
- Metadata is accurate and complete for realistic testing
