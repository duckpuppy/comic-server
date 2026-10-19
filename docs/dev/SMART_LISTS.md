# Smart Lists and Matchers

Smart list evaluation, sync filtering, and the matcher types (ComicVine, Expression, Virtual Tag, Duplicate).

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

## Smart List Filtering

**Status**: Phase 1 complete (basic filtering), Phases 2-5 in roadmap

The server implements ComicRack-compatible smart list filtering for syncing subsets of large libraries to devices with limited storage.

### Implemented (Phase 1)

**Smart List Matcher Evaluation** (`internal/library/smartlist.go`):

- 20+ matcher types (Series, Publisher, Year, Genre, PageCount, Rating, etc.)
- String operators: Equals, Contains, ContainsAny, ContainsAll, StartsWith, EndsWith, Regex
- Numeric operators: Equals, Greater, Lesser, InRange
- Date operators: Equals, IsAfter, IsBefore, IsInLastDays, IsInDateRange
- Boolean operators: EqualsYes, EqualsNo, EqualsUnknown
- AND/OR logic with negation support
- Case-insensitive string matching

**Sync Integration** (`internal/sync/`):

```go
syncer := sync.NewSyncer(client, library)
syncer.SetFilterList(smartList)  // Filter by smart list
syncer.PerformSync()              // Only matching books sync
```

Books matching the filter are synced to device. Books on device but not in filter are automatically removed.

### Roadmap (Phases 2-5)

See issues #15, #16, #17 for:

- Per-list sync options (OnlyUnread, Limit, Sort, KeepLastRead, etc.)
- Per-device configuration storage (multiple lists per device)
- CLI/API for configuration management
- Advanced matchers (nested groups, file path matching, etc.)

## Smart List Matchers (comic-server only)

These matchers are comic-server extensions — they won't be recognized by ComicRack.

| XML Type | Matcher | Operators | Description |
|----------|---------|-----------|-------------|
| `ComicServerCVSeriesCompleteMatcher` | CVSeriesComplete | Yes/No/Unknown | Whether you own all known issues |
| `ComicServerCVMissingCountMatcher` | CVMissingCount | Numeric (=, >, <, range) | Number of missing issues |
| `ComicServerCVPercentOwnedMatcher` | CVPercentOwned | Numeric (=, >, <, range) | Percentage of series owned (0-100) |

Books without ComicVine tags or with volumes not yet synced evaluate as "Unknown" for CVSeriesComplete and don't match numeric CV matchers.

## Expression Matcher (`ComicBookExpressionMatcher`)

ComicRack-compatible (same XML type), but a best-effort approximation, not full parity. ComicRackCE evaluates arbitrary Python via IronPython; comic-server evaluates the same `MatchValue` expression via [Starlark](https://github.com/google/starlark-go) (`go.starlark.net`), a Python-like language with a pure-Go implementation — no CGO, no external Python dependency.

- `MatchValue` is a raw expression referencing `ComicBook` field names directly (e.g. `Rating > 3 and "X-Men" in Series`), exposed as Starlark globals via reflection (`internal/library/expression.go`).
- Operator `0` = "is true", operator `1` = "is false" (negates the result) — matches `ComicBookExpressionMatcher.MatchBook`.
- Works: field access, comparisons, boolean logic (`and`/`or`/`not`), string methods (`.lower()`, `in`), simple arithmetic.
- Does not work: Python stdlib (`datetime`, `re`, etc.), Python-3-only syntax, series-stats access (`__bookStats`) — series aggregates are ComicRackCE's own separate `SmartListSeries*Matcher` types, already implemented natively in comic-server (`SmartListSeries*Matcher` in `internal/library`), not something Expression needs to reach into.
- A parse or eval failure never matches (mirrors ComicRack's own error handling) rather than aborting the whole smart list.

## Virtual Tag Matchers (`ComicBookVirtualTag1Matcher`..`20Matcher`)

ComicRack-compatible (same XML types, comic-server-65u). Each is a plain string matcher over a template-computed value: `server.virtual_tags` in config.yaml defines slot id + `caption_format`, and `internal/library/virtualtags.go` expands it per book (`VirtualTagValue`) using a port of ComicRackCE's `ExtendedStringFormater` (`captionformat.go`: `{Field}`, `{Field:fmt}`, `[optional]` groups, `\X` escapes; `captionfuncs.go`: the 13 `$func<>` functions, a small C#-style expression evaluator for `$if`/`$expr`, .NET date/number formatting). Matchers are one generic path - the slot number comes from the matcher `Type` ("VirtualTag7"), no per-slot types. Definitions are config-only (ComicRack keeps them in desktop settings, not the library XML), registered via `library.SetVirtualTags` at startup and SIGHUP. Never SQL-translated (computed value), so any list containing one evaluates fully in memory. Deliberately faithful to ComicRack's quirks (unescaped `>`/`[`/`$` inside a function call break it, `$month`/`$day` are D4-padded, `success |= nested` bracket bookkeeping) - check `../ComicRackCE/cYo.Common/Text/ExtendedStringFormater.cs` before "fixing" any. Not ported: `{{CustomValue}}` lookups, per-book result cache (recomputed on demand instead), proposed/filename values. See docs/CONFIGURATION.md "Virtual Tags" for user-facing docs.

## Duplicate Matcher

ComicRack-compatible duplicate detection across the entire candidate set. Identifies comics that are duplicates by metadata (series/format/volume/number/language/year/month/day/BlackAndWhite) or by file path.

- XML type: `ComicBookDuplicateMatcher`
- Operator 0 (On): returns only duplicate books
- Operator 1 (Off): returns all books
- `Not` flag inverts the result (e.g., NOT + On = non-duplicates only)
- Series name comparison strips articles ("the", "der", "die", etc.) and common separators, matching ComicRackCE's `GroupInfo.CompressedName`
