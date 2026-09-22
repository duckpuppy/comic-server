# Multi-library / library profiles — research note

_comic-server-7lj. Research-only pass, 2026-09-22. No code written._

## 0. TL;DR

The original bead framing ("one server process serves multiple
libraries, devices pick one over the wireless protocol") **isn't
feasible for real ComicRack clients** — the wireless sync protocol has
no concept of a library at all (checked directly against
`docs/WIRELESS_SYNC_PROTOCOL.md`: no `LibraryId`, no selection command,
nothing). A device just talks to whatever server it discovers on the
network; the server IS the library, as far as the protocol is
concerned.

**What the bead's actual use cases need** (separate adult/child
libraries, per-household-member libraries, test vs. production) is
already achievable today, with **zero code changes**, by running
multiple independent `comic-server` processes — each its own port,
library, and device set. That already works because `config.db`'s path
is derived from `--config`'s directory, not a shared fixed location.

**Two real gaps** stop that from being clean/documented today:
1. `comicvine_cache.db`'s path is hardcoded to the shared XDG data
   dir — no override flag exists, so two profiles on one host would
   collide writing to it.
2. The CBL repo git clone defaults to that same shared dir, so two
   profiles would race `git pull`-ing into it unless each profile's
   config explicitly overrides `cbl_repo.clone_path`.

**Recommendation:** close out the original "design multi-library
architecture + protocol extensions" framing as not worth pursuing, and
file a small follow-up (see §5) that fixes the two collision gaps and
documents the "run separate instances" pattern as the supported way to
do this. That's an afternoon of work, not an epic.

## 1. Why single-process, protocol-level library selection doesn't work

Checked `docs/WIRELESS_SYNC_PROTOCOL.md` directly (not from memory):

- **Discovery** (UDP multicast, port 7615): a device broadcasts
  `ComicRack[Variant]:{device_id}[:Sync]`. The server replies. Nothing
  in that exchange names a library — the device is discovering a
  *server*, not selecting among libraries a server might offer.
- **Session start** (`CommandStart`, `CommandInfo`): validates the
  device (SHA1 hash of Model/Manufacturer/Serial/Edition/Version) and
  starts syncing. Again, no library identifier anywhere in the frame
  format.

This is because ComicRack's own desktop app only ever managed one
library at a time — the protocol was never designed with multi-library
in mind, and comic-server can't retrofit a selection step into it
without real ComicRack Android/iOS clients (which comic-server doesn't
control) already knowing to send one. Extending the protocol would only
work for comic-server's own future clients, not the actual devices this
project exists to serve.

A same-process-different-libraries design that instead routes
*silently* by device ID (e.g. "device X always gets library A" via a
config.db mapping, no protocol change) is technically possible - device
validation already happens before anything is served - but it buys
nothing over just running separate processes, while adding real
complexity: every singleton in the current codebase (`s.backend`, the
list cache, the Komga syncer, the ComicVine sync loop, the REST API's
single mux) would need to become per-library, and the web UI would need
a library switcher. That's the "epic" scope the original bead was
gesturing at, and it's not justified by the use cases actually listed.

## 2. What already works today (verified in code, not assumed)

`cmd/server.go`:
```go
configDBPath := filepath.Join(filepath.Dir(configPath), "config.db")
```
`config.db` (devices, per-device list assignments, Komga targets, scan-info
settings, CBZ-convert settings, watch folders, UI settings — everything
`internal/configdb` owns) lives **next to** whatever `--config` points
at, not a fixed location. So:

```bash
comic-server server --config ~/.config/comic-server/kids/config.yaml \
  --port 7620 --discovery-port 7615

comic-server server --config ~/.config/comic-server/adult/config.yaml \
  --port 7621 --discovery-port 7616
```

...are two fully independent servers today: separate library
(`library_path`/`database_path` inside each profile's own
`config.yaml`), separate `config.db` (separate device registrations,
separate Komga targets), separate ports. No cross-talk. This is the
"multi-library" the bead's use cases actually describe (separate
libraries for separate people/purposes), just achieved as separate
instances rather than one process juggling several.

**Discovery port must differ per profile** — two servers sharing
`224.34.123.90:7615` would both receive every device's broadcast and
race to reply; that's a real, not hypothetical, requirement to call out
in docs.

## 3. The two real collision gaps

Checked every XDG-data-dir consumer in `cmd/server.go`:

| Path | Derivation | Override exists? |
|---|---|---|
| `config.db` | `dir(--config)/config.db` | Yes (via `--config`) |
| `comicvine_cache.db` | `config.GetDataDir()/comicvine_cache.db` (fixed XDG path) | **No** |
| CBL repo clone | `server.cbl_repo.clone_path`, defaults to `config.GetDataDir()/cbl-repo` | Yes, but the *default* collides |
| Cover cache | `server.cover_cache_dir`, defaults to XDG cache dir | Yes, but the *default* collides |

`comicvine_cache.db` is the one genuine hard blocker: `wireScraperAPI`
and `startComicVineSync` in `cmd/server.go` both hardcode
`config.GetDataDir()/comicvine_cache.db` with no flag, env var, or
config field to redirect it. Two profiles on the same host sharing a
ComicVine cache is probably *harmless* in practice (the cache is keyed
by ComicVine volume/issue ID, not library-specific data), but it's
still an unreviewed shared-write path between two otherwise-isolated
processes and should be made explicit, not accidental.

Cover cache and CBL repo clone already have override flags/config
fields — the gap there is just that the *default* silently collides if
a profile's config doesn't set them, which a "profiles" doc/helper
should make impossible to miss rather than something the user
discovers via a corrupted cache.

## 4. Non-findings (checked, not an issue)

- **Book ID collisions across libraries**: SQLite backend book IDs are
  UUIDs (import-generated), astronomically unlikely to collide even if
  someone did try to merge caches later. Not a real risk.
- **Device ID collisions**: a physical device's ID is derived from its
  own hardware (Model/Manufacturer/Serial), not server-assigned - the
  same tablet registering against two different profile instances would
  get two independent registrations, one per config.db, which is
  correct behavior (a device syncing against two libraries is exactly
  the "per-household-member" use case, just requiring the user to add
  it twice, once per profile's device list).

## 5. Recommended follow-up scope (small, separate bead)

If this is worth doing at all beyond documentation:

1. Add `COMIC_SERVER_COMICVINE_CACHE_PATH` (or a `--comicvine-cache-path`
   flag + config field) so `comicvine_cache.db`'s location is no longer
   hardcoded — same pattern as `cover_cache_dir`/`trash_path`.
2. Document the "run separate instances" pattern in
   `docs/` (a `MULTI_LIBRARY.md` or a section in the main README):
   config layout, required distinct ports (control + discovery), and
   the three paths (`cover_cache_dir`, `cbl_repo.clone_path`, ComicVine
   cache once overridable) a profile's config should set explicitly
   rather than rely on the shared default.
3. Optional, genuinely optional: a `--profile NAME` convenience flag
   that resolves to `$XDG_CONFIG_HOME/comic-server/profiles/NAME/` for
   `--config` (and, transitively, `config.db`) instead of requiring the
   user to spell out full paths - purely a UX nicety over what already
   works, not a capability gap.

None of this touches the wireless protocol, the library backend, or any
existing singleton in the server - it's a docs pass plus one small
config-path fix.

## 6. What NOT to do

Don't build single-process concurrent multi-library serving with
protocol-level device selection. It can't work for real ComicRack
clients (§1), and the "silent server-side routing by device ID"
alternative (also §1) only buys complexity, not capability, over
running separate processes.
