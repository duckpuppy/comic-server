# Performance Analysis - comic-server

**Date**: 2025-11-17
**Test Environment**: Linux, Intel i7-14700KF (28 cores), Go 1.25.4
**Test Library**: testdata/library/ComicDB.xml (151 books, 0.50 MB, 10 smart lists)

## Executive Summary

Performance profiling identified **library save operations** as the primary bottleneck, consuming ~3.1ms and allocating 2.4MB per save on a small library (151 books). This occurs after **every reverse sync** when device metadata changes are written back to the library.

For large libraries (65K+ books), this could scale to **100-200ms+ per save**, making reverse sync significantly slower.

**Key Finding**: The server saves the **entire library XML** even when only one book's metadata changed, resulting in unnecessary overhead.

## Benchmark Results

### Library Load/Save Operations

| Benchmark | Time (ms) | Memory (MB) | Allocations |
|-----------|-----------|-------------|-------------|
| **LoadLibrary** | 8.9 | 4.3 | 73,853 |
| **SaveLibrary** | **3.1** | **2.4** | **6,236** |
| **LoadThenSave** | 12.6 | 6.7 | 80,089 |
| **UpdateSingleBook** | 3.1 | 2.4 | 6,236 |

**Critical Path**: UpdateSingleBook takes same time as SaveLibrary (~3.1ms) because it saves the entire library after updating one book.

### Smart List Evaluation

| Complexity | Time (µs) | Memory (KB) | Allocations |
|------------|-----------|-------------|-------------|
| **Simple (1 matcher)** | 20.5 | 17 | 428 |
| **Medium (3 matchers)** | 22.8 | 19.5 | 579 |
| **Complex (6 matchers OR)** | 128 | 106 | 2,827 |
| **Real Smart List** | 237 | 164 | 3,020 |

**Observation**: Smart list evaluation is reasonably fast (<250µs even for complex matchers). Not a bottleneck.

### Library Iteration

| Operation | Time (ns/iter) | Memory | Allocations |
|-----------|----------------|--------|-------------|
| **IterateAllBooks** | 26.3 | 0 | 0 |
| **AccessTitle** | 25.9 | 0 | 0 |
| **AccessMultipleFields** | 25.0 | 0 | 0 |

**Observation**: Iteration is extremely fast with zero allocations. Not a bottleneck.

### Book Lookup

| Operation | Time (ns) | Memory | Allocations |
|-----------|-----------|--------|-------------|
| **GetBook** | 1.3 | 0 | 0 |

**Observation**: Book lookup is essentially free (array access). Not a bottleneck.

## CPU Profiling Results

### LoadLibrary CPU Profile

**Total Time**: 2.35s across 135 iterations (~17.4ms per load)

**Top Functions**:
- **88%** - `encoding/xml.(*Decoder).unmarshalPath` - XML parsing
- **9%** - `encoding/xml.(*Decoder).getc` - Character reading
- **4%** - `bytes.(*Buffer).WriteByte` - Buffer operations
- **4%** - `bytes.(*Reader).ReadByte` - Reading bytes

**Analysis**: Library load is dominated by XML unmarshaling, which is unavoidable without changing file format.

### SaveLibrary CPU Profile

**Total Time**: 1.84s across 384 iterations (~4.8ms per save)

**Top Functions**:
- **17%** - `encoding/xml.(*printer).EscapeString` - Escaping XML special characters
- **13%** - `unicode/utf8.DecodeRuneInString` - UTF-8 parsing
- **11%** - `bufio.(*Writer)` operations - Buffered writing
- **7%** - `runtime.memclrNoHeapPointers` - Memory clearing

**Analysis**: Save is dominated by XML marshaling and string escaping. Main overhead is formatting and encoding.

## Memory Profiling Results

### LoadLibrary Memory Allocations

**Total Allocations**: ~1GB for 151 books (~6.6MB per book)

**Allocation Breakdown**:
- **163 MB (16%)** - `reflect.growslice` - Growing slices during unmarshaling
- **114 MB (11%)** - `os.readFileContents` - Reading file into memory
- **332 MB (33%)** - `encoding/xml.(*Decoder).Token` - XML tokenization
- **880 MB (88%)** - `encoding/xml.(*Decoder).Decode` - Total XML unmarshaling

**Analysis**: High memory usage due to:
1. Reading entire file into memory (114 MB)
2. XML parser creating temporary buffers (332 MB for tokenization)
3. Growing slices as we unmarshal (163 MB)

This is mostly unavoidable with `encoding/xml`.

### SaveLibrary Memory Allocations

**Total Allocations**: ~1.1GB for 151 books (~7.3MB per book)

**Allocation Breakdown**:
- **721 MB (63%)** - `bytes.growSlice` - Growing buffer as we marshal XML
- **243 MB (21%)** - `SaveLibrary` function overhead - File operations
- **362 MB (32%)** - `ComicTime.MarshalXML` - Custom time format marshaling
- **126 MB (11%)** - `encoding/xml.(*printer).marshalAttr` - Marshaling attributes

**Key Finding**: `ComicTime.MarshalXML` allocates 362 MB (32% of total). This is called for every time field in every book.

**Analysis**:
- Buffer growth (721 MB) is expected as we build the XML output
- ComicTime marshaling (362 MB) is a potential optimization target
- File operation overhead (243 MB) includes creating temp files

## Bottleneck Analysis

### 🔴 Critical Bottleneck: Full Library Save on Single Book Update

**Problem**:
- Reverse sync updates a single book's metadata (reading progress, rating, etc.)
- Server saves **entire library XML** (151 books → 0.5 MB XML)
- Takes 3.1ms + 2.4MB allocations for small library
- For large library (65K books → 200+ MB XML), could take **100-200ms per save**

**Impact**:
- Occurs after every reverse sync (device → server)
- Typical sync updates 5-10 books → 5-10 library saves
- For 10 books × 100ms = **1 second overhead** just for saving

**Evidence**:
```
BenchmarkUpdateSingleBook-28    1156    3071293 ns/op    2401007 B/op    6236 allocs/op
```

This benchmark:
1. Updates CurrentPage on one book
2. Saves entire library
3. Result: Same time as SaveLibrary benchmark

### 🟡 Medium Bottleneck: ComicTime Marshaling

**Problem**:
- `ComicTime.MarshalXML` allocates 362 MB (32% of save memory)
- Called for every time field: OpenedTime, Added, Released, LastModified, etc.
- Each call: format time string → encode XML element

**Impact**:
- Not a time bottleneck (fast enough)
- Significant memory overhead
- Contributes to GC pressure

**Evidence**:
```
ComicTime.MarshalXML allocations: 362 MB (32% of total save allocations)
```

### 🟢 Non-Bottlenecks

These operations are **fast enough** and do NOT require optimization:

1. **Smart List Evaluation**: <250µs for complex matchers
2. **Library Iteration**: 26ns per book (zero allocations)
3. **Book Lookup**: 1.3ns (essentially free)
4. **Book Count**: 151 books is tiny compared to production (65K+ books)

## Proposed Optimizations

### 1. Dirty Tracking and Skip-Save Optimization (Already Implemented ✅)

**Status**: Already implemented in reverse sync code.

**What It Does**:
```go
// Check if any fields actually changed
hasChanges := (book.CurrentPage != deviceBook.CurrentPage) ||
              (book.OpenCount != deviceBook.OpenCount) ||
              // ... other fields

if !hasChanges {
    return false, nil  // Skip library save
}
```

**Benefit**: Avoids unnecessary library saves when device sends unchanged metadata.

**Limitation**: Still saves entire library when ANY book changes.

### 2. Batch Save Optimization (Already Implemented ✅)

**Status**: Already implemented - reverse sync accumulates all book updates, then saves once.

**What It Does**:
```go
// Update all books from device
for _, deviceBook := range deviceBooks {
    updateLibraryReadingState(deviceBook)
}
// Save library once after all updates
library.SaveLibrary()
```

**Benefit**: One save per sync instead of one save per book.

**Limitation**: Still saves entire library even if only one book changed.

### 3. 🎯 Incremental Save (HIGH PRIORITY)

**Proposed Solution**: Only write changed books to disk, not entire library.

**Approaches**:

#### Option A: Partial XML Update (Complex)
- Parse existing XML
- Update only changed `<Book>` elements
- Requires XML manipulation library (e.g., `github.com/beevik/etree`)
- Risky: Could corrupt library file

#### Option B: Separate Metadata Files (Breaking Change)
- Store reading state in separate files (e.g., `ComicDb.ReadingState.xml`)
- Only write changed reading state file
- ComicRackCE won't recognize this format
- **Rejected**: Breaks ComicRack compatibility

#### Option C: In-Memory Cache + Periodic Flush (RECOMMENDED)
- Keep modified books in memory
- Write to disk on timer (e.g., every 30 seconds) or on shutdown
- Reduces frequency of full library saves
- Risk: Data loss if server crashes before flush

**Recommended**: Option C (In-Memory Cache + Periodic Flush)

**Implementation**:
```go
type LibraryCache struct {
    library         *Library
    dirtyBooks      map[string]bool  // Track which books changed
    lastSave        time.Time
    flushInterval   time.Duration    // e.g., 30 seconds
}

func (lc *LibraryCache) UpdateBook(bookID string, updates ...) {
    // Apply updates
    lc.dirtyBooks[bookID] = true

    // Auto-flush if enough time passed
    if time.Since(lc.lastSave) > lc.flushInterval {
        lc.Flush()
    }
}

func (lc *LibraryCache) Flush() {
    if len(lc.dirtyBooks) == 0 {
        return  // Nothing changed
    }

    // Save entire library (still, but less frequently)
    library.SaveLibrary()
    lc.dirtyBooks = make(map[string]bool)
    lc.lastSave = time.Now()
}
```

**Benefits**:
- Reduces save frequency from "every sync" to "every 30 seconds"
- For 10 syncs in 30 seconds: 1 save instead of 10 saves = **10x reduction**
- Still maintains ComicRack compatibility
- Graceful shutdown ensures data is saved

**Risks**:
- Data loss if server crashes between syncs (mitigated by flush-on-shutdown)
- Slightly increased memory usage (dirty book tracking)

**Expected Impact**:
- Current: 10 books × 3.1ms = 31ms overhead per sync session
- With cache: 3.1ms amortized over 10 syncs = **0.3ms overhead per sync**
- **~10x performance improvement** for reverse sync

### 4. 🎯 ComicTime Marshaling Optimization (MEDIUM PRIORITY)

**Proposed Solution**: Pre-format time strings and cache them.

**Current Implementation**:
```go
func (ct ComicTime) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
    if ct.Time.IsZero() {
        return nil
    }
    s := ct.Time.Format("2006-01-02T15:04:05")  // Allocates string every time
    return e.EncodeElement(s, start)
}
```

**Optimized Implementation**:
```go
// Option A: Cache formatted string
type ComicTime struct {
    time.Time
    cachedString string  // Pre-formatted string
}

func (ct *ComicTime) SetTime(t time.Time) {
    ct.Time = t
    ct.cachedString = t.Format("2006-01-02T15:04:05")
}

func (ct ComicTime) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
    if ct.Time.IsZero() {
        return nil
    }
    return e.EncodeElement(ct.cachedString, start)
}
```

**Benefits**:
- Reduces allocations during marshaling
- Trades memory (cached strings) for CPU (formatting)

**Risks**:
- Increased memory per book (~20 bytes per time field × 5 fields = 100 bytes per book)
- For 65K books: 6.5 MB additional memory
- Must update cached string whenever time changes

**Expected Impact**:
- Reduces save memory allocations from 2.4 MB to ~2.0 MB (~17% reduction)
- Minimal time improvement (formatting is fast)

**Verdict**: Lower priority - memory trade-off not worth it for small gain.

### 5. 🟢 Library Format Change (LONG-TERM)

**Proposed Solution**: Replace XML with more efficient format (SQLite, Protocol Buffers, JSON).

**Options**:

#### Option A: SQLite Database
**Pros**:
- Fast partial updates (UPDATE single row)
- Indexed queries for fast book lookup
- Transaction support (atomic updates)
- Standard format

**Cons**:
- **Breaks ComicRack compatibility** (deal-breaker)
- Requires migration tool
- More complex to implement

#### Option B: Protocol Buffers
**Pros**:
- Binary format (faster than XML)
- Smaller file size
- Strongly typed

**Cons**:
- **Breaks ComicRack compatibility** (deal-breaker)
- Requires schema definition
- Not human-readable

**Verdict**: **Rejected** - Must maintain ComicRack XML compatibility.

## Recommendations

### Phase 1: Immediate Optimizations (v0.9)

1. ✅ **Already Implemented**: Dirty tracking and batch save
2. ✅ **Already Implemented**: Skip-save when no changes

### Phase 2: High-Impact Optimizations (v1.0)

3. ⬜ **Implement In-Memory Cache + Periodic Flush**
   - Priority: **HIGH**
   - Effort: **Medium** (2-3 days)
   - Impact: **~10x reduction in save overhead**
   - Risk: **Low** (data loss mitigated by flush-on-shutdown)

4. ⬜ **Add Configurable Flush Interval**
   - Priority: **Medium**
   - Effort: **Low** (1 day)
   - Impact: **Tunable performance** (trade latency for throughput)
   - Risk: **None**

### Phase 3: Future Optimizations (v1.1+)

5. ⬜ **Profile Large Library Performance**
   - Priority: **High**
   - Effort: **Low** (1 day)
   - Impact: **Data-driven decisions** for real-world workloads
   - Risk: **None**

6. ⬜ **ComicTime Caching** (Optional)
   - Priority: **Low**
   - Effort: **Low** (1 day)
   - Impact: **~17% memory reduction during save**
   - Risk: **Low** (increased memory usage)

## Testing Strategy

### Performance Regression Tests

Add performance regression tests to CI:

```go
func TestSaveLibraryPerformance(t *testing.T) {
    lib := loadTestLibrary(t)

    start := time.Now()
    err := SaveLibrary("test.xml", lib)
    duration := time.Since(start)

    // For 151 books, save should be < 10ms
    if duration > 10*time.Millisecond {
        t.Errorf("SaveLibrary too slow: %v (expected < 10ms)", duration)
    }
}
```

### Load Testing

Simulate heavy reverse sync load:

```bash
# Benchmark reverse sync with 100 books updating
go test -bench=BenchmarkReverseSyncMany -benchtime=10s
```

### Production Profiling

Enable profiling in production:

```bash
# Start server with profiling enabled
./comic-server server --library /path/to/ComicDb.xml \
  --pprof-enabled \
  --pprof-port 6060

# Capture CPU profile during sync
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze profile
go tool pprof cpu.prof
```

## Scaling Analysis

### Current Performance (151 Books)

| Operation | Time | Memory | Frequency |
|-----------|------|--------|-----------|
| Load Library | 8.9ms | 4.3 MB | Once at startup |
| Save Library | 3.1ms | 2.4 MB | After each reverse sync |
| Smart List Eval | 237µs | 164 KB | Once per sync |

**Per-Sync Overhead**: ~3.5ms (save + smart list eval)

### Projected Performance (65,000 Books)

**Scaling Factor**: 65,000 / 151 ≈ **430x**

| Operation | Time (Projected) | Memory (Projected) |
|-----------|------------------|-------------------|
| Load Library | 3.8s (8.9ms × 430) | 1.8 GB |
| Save Library | **1.3s** (3.1ms × 430) | 1.0 GB |
| Smart List Eval | 102ms (237µs × 430) | 70 MB |

**Per-Sync Overhead**: **~1.4 seconds** (save + smart list eval)

**With In-Memory Cache** (flush every 30s):
- Save overhead amortized over multiple syncs
- Per-sync overhead: **~100ms** (just smart list eval)
- **~14x improvement**

### Production Library Size Reference

**User's Production Library**:
- Books: 65,000+
- File Size: 200+ MB XML
- Expected save time: **1-2 seconds** without optimization

**With Optimizations**:
- In-memory cache: **~140ms per sync** (14x improvement)
- Acceptable for user experience

## Monitoring Recommendations

### Metrics to Track

Add Prometheus metrics:

```go
// Library save duration histogram
comic_server_library_save_duration_seconds

// Library save frequency counter
comic_server_library_saves_total

// Dirty books gauge (cache size)
comic_server_dirty_books_count

// Flush interval histogram
comic_server_library_flush_interval_seconds
```

### Alerts

Set up alerts for performance degradation:

```yaml
# Alert if library save takes > 2 seconds
- alert: LibrarySaveSlow
  expr: comic_server_library_save_duration_seconds > 2
  for: 5m
  annotations:
    summary: "Library save operation is slow"
```

## Conclusion

**Current Status**:
- Small library (151 books): Performance is acceptable (~3.5ms per sync)
- Large library (65K books): Performance would be poor (~1.4s per sync)

**Recommended Actions**:
1. ✅ Keep existing dirty tracking and batch save optimizations
2. ⬜ Implement in-memory cache with periodic flush (**HIGH PRIORITY**)
3. ⬜ Profile with real production library (65K+ books)
4. ⬜ Add performance regression tests to CI
5. ⬜ Add Prometheus metrics for monitoring

**Expected Outcome**:
- **14x performance improvement** for reverse sync on large libraries
- Acceptable user experience even with 65K+ books
- Maintains full ComicRack XML compatibility

---

**Next Steps**: See [Issue #9 (Performance Optimization)](https://github.com/duckpuppy/comic-server/issues/9) for implementation tracking.
