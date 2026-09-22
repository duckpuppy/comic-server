# Running Multiple Libraries (Profiles)

comic-server doesn't support one server process serving multiple
libraries at once — the ComicRack wireless sync protocol has no concept
of a library at all (see [WIRELESS_SYNC_PROTOCOL.md](./WIRELESS_SYNC_PROTOCOL.md)),
so a device has no way to pick one even if the server offered several.

If you want separate libraries — a kids' library and an adult library,
one per household member, a test library alongside your real one — run
**separate comic-server processes**, each with its own config file.
This already works today; there's nothing to install or enable.

## How it works

Each `comic-server server` process's config.db (devices, per-device
list assignments, Komga targets, and every other runtime-editable
setting) is stored next to whatever `--config` points at. Point two
processes at two different config files and they're fully independent:
different library, different registered devices, different Komga
targets - nothing is shared unless you explicitly share it.

## Setting up a profile

Pick a distinct config directory, port, and discovery port for each
profile:

```bash
# Kids' library
comic-server server \
  --config ~/.config/comic-server/kids/config.yaml \
  --port 7620 --discovery-port 7615

# Adult library
comic-server server \
  --config ~/.config/comic-server/adult/config.yaml \
  --port 7621 --discovery-port 7616
```

**The control port (`--port`) and discovery port (`--discovery-port`)
must both be distinct per profile.** Two processes sharing a discovery
port would both see every device's broadcast and race to answer it.

Each profile's `config.yaml` sets its own library:

```yaml
# ~/.config/comic-server/kids/config.yaml
server:
  library_path: /comics/kids/ComicDb.xml   # or database_path for the SQLite backend
  server_port: 7620
  discovery_port: 7615
```

## Paths to set explicitly per profile

Three paths default to a location shared across the whole host, not
scoped to a profile's config directory. Set all three explicitly in
each profile's `config.yaml` (or via the matching env var/flag) so
profiles don't silently share files:

| Setting | Config key | Flag | Env var |
|---|---|---|---|
| Cover thumbnail cache | `server.cover_cache_dir` | `--cover-cache-dir` | `COMIC_SERVER_COVER_CACHE_DIR` |
| ComicVine enrichment cache | `server.comicvine_cache_path` | `--comicvine-cache-path` | `COMIC_SERVER_COMICVINE_CACHE_PATH` |
| CBL reading-list repo clone | `server.cbl_repo.clone_path` | _(config only)_ | _(config only)_ |

Sharing the cover cache or ComicVine cache between profiles is
harmless in practice (both are content-addressed/ID-keyed, not
library-specific), but the CBL repo clone genuinely isn't safe to
share - two processes running `git pull` into the same directory at
the same time can race. Give each profile its own `clone_path` if you
use CBL import in more than one profile.

Everything else that's per-profile already gets its own path for
free, because it's derived from `--config`'s own directory:
`config.db`, and (unless you point `library_path`/`database_path`
elsewhere) the library file itself.

## Devices

A device that should sync against two different libraries needs to be
registered separately in each profile - each profile has its own
independent device list (`config.db`), so this is exactly the same as
registering it once, no extra step. A device only ever talks to one
server (one profile) per sync session, matching how the protocol
itself has no notion of switching libraries mid-session.

## Running as services

If you run comic-server via systemd (see `scripts/README.md`), give
each profile its own unit file (`comic-server-kids.service`,
`comic-server-adult.service`, ...) with `--config`/`--port`/
`--discovery-port` baked into each unit's `ExecStart`, rather than
trying to parameterize one unit for both.
