# Storage, Backups and Audit Log

The SQLite library backend, import/export, backups and restore, and the per-book audit log.

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

## SQLite Storage (internal/storage/)

**Experimental feature** for importing ComicRack libraries into SQLite for improved query performance.

**Import Command**:

```bash
# Import library into SQLite database
comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db

# Preview changes without modifying database
comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db --dry-run

# Verbose output
comic-server import --xml /path/to/ComicDb.xml --db /path/to/library.db --verbose
```

**Database Info Command**:

```bash
# Show database statistics
comic-server db info --db /path/to/library.db
```

**Export Command** (comic-server-bcl6): the reverse of import - reconstructs
a full `ComicDb.xml` (every book, every smart/reading list with matchers)
from the SQLite database, for real ComicRackCE to open. Three
comic-server-only smart list matcher types (`ComicServerCVSeriesCompleteMatcher`,
`ComicServerCVMissingCountMatcher`, `ComicServerCVPercentOwnedMatcher` - see
ComicVine Enrichment in [COMICVINE.md](COMICVINE.md) and the matchers in [SMART_LISTS.md](SMART_LISTS.md)) aren't recognized by ComicRackCE, so exporting a
smart list containing one drops just that matcher (keeping the list's other
matchers/structure intact) and prints a warning per drop.

```bash
# Export SQLite database back to ComicDb.xml
comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml

# Preview without writing the XML file
comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml --dry-run

# Verbose output
comic-server export --db /path/to/library.db --xml /path/to/ComicDb.xml --verbose
```

**Key Features**:

- **Idempotent import**: Importing the same XML twice results in no changes
- **Change detection**: Uses SHA256 hashing to detect modified records
- **Automatic updates**: Adds new books, updates changed books, deletes removed books
- **Normalized storage**: Custom values and tags stored in separate tables for fast queries
- **Pure Go**: Uses `modernc.org/sqlite` (no CGO required)

**Schema Highlights**:

- `books` table with 40+ columns matching ComicBook struct
- `book_custom_values` table (book_id, key, value) with index on (key, value)
- `book_tags` table (book_id, tag) with index on tag
- `lists` table with JSON matchers for smart list definitions
- `reading_list_items` junction table for reading list references
- `library_metadata` table for library ID, name, import timestamp

**Note**: This feature is experimental. The server currently still uses the in-memory XML library for sync operations. Future versions will add the option to read directly from SQLite.

## Backups

Tracker issue #6 (was comic-server-yzdt), backup half; **restore is not built**. `internal/backup` writes one directory per backup under the chosen folder (`backup-YYYYMMDD-HHMMSS`) holding `config.db`, `library.db` (SQLite backend only), `comicvine_cache.db` (option) and `config.yaml` (option, holds the ComicVine key), plus `manifest.json` (app version, per-file size and sha256, `PRAGMA user_version` per database, book count). SQLite files are copied with `VACUUM INTO` (`backup.VacuumInto`; `configdb.DB`, `storage.SQLiteBackend` and `comicvine.Cache` each have `SnapshotTo`), which is consistent while the server runs; the backend lock is held only to read the handle so a long copy never blocks writers. A backup is built under `.partial-<name>` and renamed into place, so a listed backup is complete; a failed run removes its partial directory. Files are 0600, directories 0700. Before copying, free space is checked (unix only). After a good run the newest `keep` backups stay; prune only touches `backup-*` directories with a readable manifest. `backup.Verify` re-hashes a backup against its manifest.

Settings and history live in config.db (`backup_settings`, `backup_runs`; lazy tables, last 50 runs, no schema bump). `Server.StartBackups` (called from `cmd/server.go`) ticks every minute, reads the settings each time (a UI change needs no restart) and asks `backup.Schedule.NextRun` whether a backup is due: `daily` at HH:MM local time, or every N hours; never-run means due at once. A scheduled run waits while a device sync is active, and after a failure is not retried for an hour. One backup at a time (`errBackupRunning`, 409 on the manual endpoint). `GET|PUT /api/settings/backups` (settings + status + warnings, including "folder is on the same disk as a database"), `POST /api/backups/run` (202, runs in the background), `GET /api/backups` (the folder's backups). UI: Settings > Backups (`js/backupSettings.js`). Restore design decision (2026-10-16): restoring config.db keeps the live `backup_settings` and `backup_runs` tables, so a restore never changes where or when backups run. **Restore** is an offline command, `comic-server restore [name] [--backup-dir D] [--apply] [--include-config]` (`cmd/restore.go`, `backup.PlanRestore` / `ApplyRestore`): no name lists backups, a name previews (checksum via `Verify`, refusal of a database whose `user_version` exceeds `configdb.SchemaVersion` / `storage.SchemaVersion` / `comicvine.CacheSchemaVersion`), `--apply` refuses while a server answers on its port, takes a safety backup of the current files into the same folder (a normal `backup-*` entry, so retention can later prune it), stages every file as `<target>.restore-new`, integrity-checks it, carries `backup_settings`/`backup_runs` over from the live config.db, then renames into place after deleting the old `-wal`/`-shm`. A failure before the swap leaves live files untouched. `config.yaml` is restored only with `--include-config`. No web UI restore on purpose (a live SQLite swap is unsafe). Not done: off-box copy, a per-backup verify button, deleting one backup from the UI.

## Audit Log and Rollback

`internal/audit` (comic-server-vqsj) records what each pipeline action changed. One wrapper, `audit.Backend` (`s.audited(action)` in `internal/api/audit.go`), sits around `library.Backend`: every `UpdateBook(s)` through it diffs the incoming book against the stored one (scalar fields by value, custom values per key, slice fields as a count; the workflow stage is reported as `stage_before`/`stage_after`) and appends rows to the `audit_log` table in config.db (lazy `CREATE TABLE IF NOT EXISTS`, no schema bump). Entries are written only after the update succeeds; a log failure never fails the action.

- **Wrapped paths**: convert_cbz, write_metadata, scan_info, data_manager, organize, scrape (via `SetScraper`), link_wanted, rollback; duplicate trashing adds an entry with `audit.Record`. **Not** wrapped: device reverse sync (reading progress changes constantly and is not a pipeline action) and the CLI `scrape`/`workflow backfill` commands.
- **`audit.Backend.GetBook` returns a copy.** The XML backend's `GetBook` returns a pointer into the live library, so a caller editing it in place would hide its own diff. Anything that read-modify-writes a book should read through the wrapper.
- **Retention**: `audit.Retention` = 180 days, pruned at most hourly on reads of the list endpoint.
- **API**: `GET /api/library/audit?book_id=&action=&limit=&offset=`; `POST /api/library/audit/rollback {book_id, stage}`. History page at `/history`.
- **Rollback is stage-only and backwards-only.** It resets the stage via `workflow.RegressToStageIfPast` so the pipeline re-runs the book; it does not restore field values (the old values are in the entry's diff) and does not move files back. It is itself logged.
- **Undo values** (tracker #14): `POST /api/library/audit/undo {entry_id, fields?, apply, force}` (`internal/api/auditundo.go`), "Undo values…" on the History page. Preview (`apply` false) lists each field of one entry with its current value and a status: `ok` (current still equals the entry's new value), `conflict` (changed since; restored only with `force`), `unchanged`, `unsupported`. Undoable = scalar fields in `editableFields` and `custom:` values that do not start with `comic_server_`; slice/map fields (recorded only as "N items"), file fields, dirty flags and bookkeeping values are refused with a reason. Each restore is read back and refused if the writer cannot reproduce the recorded old value exactly. Applying sets the same dirty flags / organize regression as the edit form, writes nothing if any field fails, 409 when nothing is restorable, and is itself logged as action `undo`. Reading progress and stage are not restored (stage has its own rollback).
