# Reverse Sync Manual Testing Guide

This guide walks through manually testing the reverse sync functionality with a real Android/iOS device.

## Overview

**Reverse sync** syncs metadata changes from the device back to the library:
- Reading progress (current page, last page read, open count, opened time)
- User ratings
- User annotations (notes, reviews, summaries, tags)
- Checked flag
- Page metadata (bookmarks, page types)

## Prerequisites

- Comic-server built and ready to run
- Android or iOS device with ComicRack app installed
- Device and server on the same network
- Test library at `testdata/library/ComicDb.xml`
- At least one comic book file in `testdata/library/comics/`

## Test Setup

### 1. Start the Server

```bash
# From comic-server directory
./comic-server server \
  --library testdata/library/ComicDb.xml \
  --log-level debug
```

**Note:** Debug logging shows detailed reverse sync operations.

### 2. Device Discovery

On your device:
- Open ComicRack app
- Go to Settings → Wireless Sync
- Tap "Search for Servers"
- Select your server when it appears

Alternatively, if multicast discovery doesn't work (WSL2, VPN, etc.):
```bash
# Use direct IP ping
./comic-server server \
  --library testdata/library/ComicDb.xml \
  --log-level debug \
  --ping-device 192.168.0.24  # Replace with your device IP
```

### 3. Configure Smart List for Device

```bash
# List available smart lists
./comic-server config list-smartlists --library testdata/library/ComicDb.xml

# Example output:
# Smart Lists in library:
# - Ace Magazines [ID: {list-id-1}] (4 books)
# - Ajax-Farrell [ID: {list-id-2}] (3 books)
# - 1950s Comics [ID: {list-id-3}] (7 books)
# - All Comics [ID: {list-id-4}] (all books)

# Add a list to your device
./comic-server config add-list "YOUR_DEVICE_NAME" "All Comics" \
  --library testdata/library/ComicDb.xml
```

## Test Scenarios

### Test 1: Reading Progress Sync

**Goal:** Verify that reading a comic on the device updates the library.

**Steps:**
1. Trigger a sync from device (tap sync button)
2. Wait for sync to complete
3. Open a comic on the device
4. Read a few pages (e.g., go to page 5)
5. Close the comic
6. Trigger another sync from device
7. Check library file on server

**Verification:**
```bash
# Search for CurrentPage in library
grep -A 5 'Id="atomic-war-001"' testdata/library/ComicDb.xml | grep CurrentPage

# Should show something like:
# <CurrentPage>5</CurrentPage>
```

**Expected Debug Logs:**
```
{"level":"info","message":"Updating library with reading state from device"}
{"level":"debug","book_id":"atomic-war-001","title":"Atomic War! #1","old_page":0,"new_page":5,"message":"Updating CurrentPage from device"}
{"level":"info","updated_count":1,"message":"Updated library metadata from device, saving library"}
{"level":"info","message":"Library saved successfully"}
```

### Test 2: Rating Sync

**Goal:** Verify that rating a comic on the device updates the library.

**Steps:**
1. On device, open a comic
2. Rate it (e.g., 4 or 5 stars)
3. Close the comic
4. Trigger sync from device
5. Check library file

**Verification:**
```bash
grep -A 5 'Id="atomic-war-001"' testdata/library/ComicDb.xml | grep Rating

# Should show something like:
# <Rating>4</Rating>  (or 5 for 5 stars)
```

**Expected Debug Logs:**
```
{"level":"debug","book_id":"atomic-war-001","title":"Atomic War! #1","old_rating":0,"new_rating":4,"message":"Updating Rating from device"}
```

### Test 3: Notes/Tags Sync

**Goal:** Verify that adding notes/tags on device updates the library.

**Steps:**
1. On device, long-press a comic → Edit Details
2. Add notes: "Great story!"
3. Add tags: "favorite,superhero"
4. Save changes
5. Trigger sync from device
6. Check library file

**Verification:**
```bash
grep -A 10 'Id="atomic-war-001"' testdata/library/ComicDb.xml | grep -E "(Notes|Tags)"

# Should show:
# <Notes>Great story!</Notes>
# <Tags>favorite,superhero</Tags>
```

**Expected Debug Logs:**
```
{"level":"debug","book_id":"atomic-war-001","title":"Atomic War! #1","message":"Updating Notes from device"}
{"level":"debug","book_id":"atomic-war-001","title":"Atomic War! #1","old_tags":"","new_tags":"favorite,superhero","message":"Updating Tags from device"}
```

### Test 4: Page Bookmarks Sync

**Goal:** Verify that bookmarking pages on device updates the library.

**Steps:**
1. On device, open a comic
2. Go to a specific page
3. Add a bookmark (usually via menu → Add Bookmark)
4. Name the bookmark (e.g., "Epic Battle Scene")
5. Trigger sync from device
6. Check library file

**Verification:**
```bash
grep -A 20 'Id="atomic-war-001"' testdata/library/ComicDb.xml | grep -E "(Pages|Bookmark)"

# Should show Pages section with bookmarks:
# <Pages>
#   <Page Image="0" Type="FrontCover"/>
#   <Page Image="15" Bookmark="Epic Battle Scene"/>
# </Pages>
```

**Expected Debug Logs:**
```
{"level":"debug","book_id":"atomic-war-001","title":"Atomic War! #1","old_page_count":0,"new_page_count":2,"message":"Updating page metadata from device"}
```

### Test 5: No Changes Optimization

**Goal:** Verify that library is NOT saved when no changes are detected.

**Steps:**
1. Trigger sync from device (without making any changes)
2. Note the library file modification time before sync
3. Wait for sync to complete
4. Check library file modification time after sync

**Verification:**
```bash
# Before sync
ls -l testdata/library/ComicDb.xml

# Trigger sync from device

# After sync (time should be unchanged)
ls -l testdata/library/ComicDb.xml
```

**Expected Debug Logs:**
```
{"level":"info","message":"Updating library with reading state from device"}
{"level":"debug","message":"No metadata changes detected"}
# Note: NO "Updated library metadata" or "Library saved" messages
```

### Test 6: Multiple Books

**Goal:** Verify that changes to multiple books are all synced.

**Steps:**
1. On device, read multiple comics (e.g., 3 different books)
2. Make changes to each (read pages, add ratings, etc.)
3. Trigger sync from device
4. Check library file

**Expected Debug Logs:**
```
{"level":"info","message":"Updating library with reading state from device"}
{"level":"info","book_id":"atomic-war-001","title":"Atomic War! #1","changed_fields":["CurrentPage","Rating"],"message":"Updated book metadata from device"}
{"level":"info","book_id":"atomic-war-002","title":"Atomic War! #2","changed_fields":["CurrentPage","OpenCount"],"message":"Updated book metadata from device"}
{"level":"info","book_id":"dark-shadows-001","title":"Dark Shadows #1","changed_fields":["CurrentPage","Notes"],"message":"Updated book metadata from device"}
{"level":"info","updated_count":3,"message":"Updated library metadata from device, saving library"}
{"level":"info","message":"Library saved successfully"}
```

## Troubleshooting

### Library Not Updating

**Symptoms:** Changes on device don't appear in library file.

**Check:**
1. Verify library path is set correctly
2. Check file permissions (library file must be writable)
3. Look for errors in debug logs
4. Ensure sync completed successfully (check device status)

**Debug Commands:**
```bash
# Check if library file is writable
ls -l testdata/library/ComicDb.xml

# Run with verbose debug logging
./comic-server server --library testdata/library/ComicDb.xml --log-level debug

# Grep for reverse sync messages
./comic-server server --library testdata/library/ComicDb.xml --log-level debug 2>&1 | grep "Updating library"
```

### Sync Completes But No Changes

**Symptoms:** Sync finishes, but library shows no changes.

**Possible Causes:**
1. No actual changes on device (metadata unchanged)
2. Device books not in library (books were deleted from library)
3. Device sidecar metadata not readable

**Check Debug Logs For:**
```
{"level":"debug","message":"No metadata changes detected"}
# OR
{"level":"debug","book_id":"xxx","device_title":"...","message":"Device book not found in library, skipping metadata update"}
```

### Performance Issues

**Symptoms:** Sync is very slow with large library.

**Notes:**
- Reverse sync only processes books that are on the device
- Library save time depends on library size (65K+ books may take a few seconds)
- Consider performance optimization milestone (future work)

## Success Criteria

✅ Reading progress (CurrentPage, LastPageRead, OpenCount) syncs correctly
✅ User ratings sync correctly
✅ User annotations (Notes, Review, Summary, Tags) sync correctly
✅ Page bookmarks sync correctly
✅ Library file is saved to disk after changes
✅ Library file is NOT saved when no changes (optimization)
✅ Debug logs show detailed sync operations
✅ Multiple books can be updated in one sync

## Next Steps After Manual Testing

Once manual testing is complete:
1. Document any issues found
2. Update CHANGELOG.md with reverse sync feature
3. Consider adding reverse sync to Web UI monitoring
4. Plan performance improvements if needed

## Related Files

- Implementation: `internal/sync/session.go:645-865`
- Tests: `internal/sync/reverse_sync_test.go`
- Protocol docs: `WIRELESS_SYNC_PROTOCOL.md`
