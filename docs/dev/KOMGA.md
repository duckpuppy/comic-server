# Komga sync

`internal/komga`. `Syncer.Run` runs a pass every 15 minutes over the configured targets (collection or read list per smart list).

## Read status push (tracker #2, phase 1 = #32)

Per target, `SyncReadStatus` pushes read state to Komga (`pushReadStatus` in `sync.go`).

- **Read only.** It sends `PATCH completed:true` for a book comic-server has as read. It never sends "unread" (`DELETE`). Reason: comic-server cannot yet tell "never read" from "read in Komga", so the old unconditional push reset every Komga read on the next pass.
- **Skips books Komga already has as READ.** One paged `GET /api/v1/books?read_status=READ` per pass (`Client.ListReadBookIDs`), not one call per book.
- **If that listing fails, nothing is pushed.** The books are reported in `ReadStatusFailed`.
- Result fields: `ReadStatusPushed` (PATCHes sent), `ReadStatusSkipped` (unread here, or already read in Komga), `ReadStatusFailed`.

## Policy while ComicRack is still in use

Read wins: a book read in either place counts as read. Unread does not propagate in either direction yet.

## Not built (see tracker)

- Phase 2 (#33): pull Komga reads into comic-server and a three-way merge so unread can propagate. Waits for ComicRack retirement.
- Phase 3 (#34): reading position (page) sync.
- Full ComicRack retirement follow-up: see the "after ComicRack is retired" issue linked from #2.
