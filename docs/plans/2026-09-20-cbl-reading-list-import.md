# CBL reading list import — design spec

_comic-server-r1j4. Research-only bead; no code written. Written 2026-09-20._

## 0. Summary

Import ComicRack's CBL reading-list format, primarily from
[DieselTech/CBL-ReadingLists](https://github.com/DieselTech/CBL-ReadingLists)
— the community-maintained "semi-official" collection of reading orders
(1704 `.cbl` files across 16 publisher folders as of this writing). Local
file import is a cheap secondary path built on the same parser.

**Recommended first slice:** one-shot import of a single CBL — from a local
file/upload, or picked from a browsable/searchable clone of the DieselTech
repo — via CLI, REST API, and a Settings/Lists UI panel. No watch/reimport
yet (see §6 for why).

## 1. The CBL format (ground-truthed against `../ComicRackCE` source)

Root element `<ReadingList>`. Schema from
`ComicRack.Engine/Database/ComicReadingListContainer.cs` and
`ComicReadingListItem.cs`:

```xml
<ReadingList xmlns:xsd="..." xmlns:xsi="...">
  <Name>List Name</Name>
  <NumIssues>1156</NumIssues>          <!-- NOT in ComicRack's base schema, see below -->
  <Books>
    <Book Series="Detective Comics" Number="27" Volume="1937" Year="1939">
      <Database Name="cv" Series="18058" Issue="105764" /> <!-- NOT in ComicRack's base schema -->
      <Id>00000000-0000-0000-0000-000000000000</Id>  <!-- ComicRack's own internal book GUID -->
      <FileName></FileName>                            <!-- optional, often empty -->
    </Book>
    ...
  </Books>
</ReadingList>
```

A CBL can *also* be a smart list: a `MatcherMode` attribute plus
`ComicBookMatcher` children, reusing the exact same matcher XML our own
smart lists already parse. Most real-world CBLs are plain book lists,
though — recommend **v1 supports the plain book-list path only**; smart-list
CBLs are a cheap fast-follow since the matcher parser already exists.

### The `<Database>` extension (not in ComicRack's own code)

Every real file sampled from DieselTech (6 files, 5 different publisher
folders, different contributors, 25–1156 books each, 100% coverage,
1:1 with `<Book>` count) carries a `<Database Name="cv" Series="X"
Issue="Y"/>` child — a ComicVine series/issue ID pair. This is **not**
part of ComicRack's own `ComicReadingListItem` class; it looks like a
Kavita/Komga-originated extension (both listed as compatible tools in the
DieselTech README). ComicRack's XML deserializer silently ignores it.
comic-server should read it — see §2.

`Number` is free text, not always numeric (one sampled file used `"¼"` for
a Sonic the Hedgehog one-shot) — matches comic-server's own model already.

## 2. Matching strategy

### 2a. ComicVine-ID path (primary, DieselTech-specific)

When `<Database Name="cv" Issue="...">` is present, match directly against
comic-server's own ComicVine issue ID data (97.5% of the real library is
CV-tagged — see project memory). This should give a much higher match rate
than string matching and is **not something ComicRack itself can do** —
it predates the `<Database>` extension.

### 2b. String-matching fallback (ComicRack parity path)

For entries without a usable `<Database>` element (a different/older CBL,
a non-DieselTech source, or a missing tag), replicate ComicRack's own
algorithm exactly — ground-truthed from
`ComicIdListItem.CreateFromReadingList`
(`ComicRack.Engine/Database/ComicIdListItem.cs:154-233`):

1. Try the CBL's internal `Id` (GUID) against the library — only ever
   fires on reimporting your own previously-exported list.
2. Try exact `FileName` match, if set.
3. Else, `Number` exact match **and** `Series` match, three widening
   passes, stop at the first with ≥1 hit:
   - a. exact case-insensitive series string
   - b. same, with a trailing `vol/volume/v <N>` token stripped from both
     sides first (regex `\bv(ol(ume)?)?\.?\s?\d+\b`)
   - c. same as (b), plus strip everything that isn't `a-z0-9` and the
     literal words "the"/"and" (regex `[^a-z0-9]|\bthe\b|\band\b`) —
     effectively a slugified compare
4. If >1 candidate remains: narrow to `|year - target year| <= 1` (keep
   the wider set if this eliminates everyone).
5. If still >1: narrow to exact `Volume` match (same keep-wider-set rule).
6. If still >1 and `Format` is set: narrow to case-insensitive `Format`
   match (same rule).
7. Take the first remaining candidate — **this is an arbitrary tie-break
   in the reference implementation itself**, not "still ambiguous, skip
   it." comic-server should match this behavior for parity rather than
   invent a stricter rule ComicRack doesn't have.
8. If nothing matched at all: ComicRack creates an **empty placeholder
   book** (Series/Number/Volume/Year/Format only, no file) and asks the
   user whether to keep it in the list or drop it
   (`ComicListLibraryBrowser.ImportList`, the "UnsolvedBookItems" dialog).
   It is not silent and not a hard failure.

### 2c. Unmatched-entry handling (comic-server has no desktop dialog)

Recommend: **default to drop + report** unmatched entries in an import
summary (series/number/year of each miss), with an explicit opt-in flag
to add them to the existing **wanted-books** mechanism instead of
inventing a separate placeholder concept — see 2e.

### 2d. Match-correction UI (user design note, 2026-09-20)

The string-matching fallback path (§2b) can produce a wrong match: it
takes the first candidate on an ambiguous tie (step 7, matching
ComicRack's own behavior) and normalized-series comparisons can overlap
two different real series. Recommend a review/correction surface, scoped
to a single import's results, not a general-purpose relink tool:

- After an import, show each entry with which path matched it (CV-ID /
  string-fallback / unmatched), and for string-fallback matches, which
  candidate(s) were considered.
- Let the user re-point a wrong match to a different book, or unmatch it
  (moving it to unmatched/wanted per §2c).
- Not needed for CV-ID matches in the common case — that path is a direct
  ID lookup, not a heuristic guess — though a manual override should
  still be available for the rare bad-CV-ID-in-the-source-file case.
- This can be a fast-follow after the plain import ships, once real
  string-fallback match quality is visible (many CBLs may hit the CV-ID
  path 100% of the time per this session's sampling, shrinking how often
  this UI is actually needed — measure before over-building it).

### 2e. Missing issues → wanted list (user design note, 2026-09-20)

comic-server already has a wanted-books mechanism (**comic-server-38f7**):
a "wanted" book is an ordinary book record with `FilePath == ""` (see
`internal/api/wanted.go`, `POST /api/library/workflow/wanted`). Reuse this
directly for CBL entries the library doesn't own, instead of a bespoke
placeholder-book concept:

- On unmatched entries (§2c), offer "add missing issues to wanted list"
  (per-import, or per-entry) rather than an import-specific stub.
- This also gives a natural home for "issues I own zero copies of, from a
  reading order I imported" as a discoverable, already-supported view
  (whatever the Wanted UI already surfaces), no new UI needed for that
  part.
- Populate wanted-book fields (Series/Number/Volume/Year/Format, and the
  CV ID when the `<Database>` element provided one) from the CBL entry
  directly.

### 2d. Measuring real match rate — not yet done

This bead is research-only; no matcher code was written, so no real match
rate against the 66K-book library has been measured. **Do this as step 1
of implementation**, before committing to any UI/API shape beyond the
bare minimum — a low match rate on the CV-ID path would be a surprise
worth knowing before building around it.

## 3. Source: DieselTech/CBL-ReadingLists as the primary source

Treat the DieselTech repo as first-class, not one option among many:

- **Configured repo URL**, defaulting to
  `https://github.com/DieselTech/CBL-ReadingLists` — never hardcoded.
  "Semi-official" means a single third-party dependency that can be
  renamed, moved, forked, or abandoned; the server should tolerate an
  unreachable repo by falling back to the last good local clone rather
  than breaking.
- **Local clone**, refreshed via `git fetch` (real `git` binary — see §5).
  One fetch covers all 1704 files; avoids GitHub API rate limits.
- **Browse/search UI**: 16 top-level publisher folders, hierarchical
  subfolders by character/event/creator, 1704 files total — this needs
  search and folder navigation, not a flat list.
- **Local file import** stays as a cheap secondary path sharing the same
  parser (`CBL bytes → parsed list → match → write`), independent of
  where the bytes came from.

### License (checked directly, not assumed)

No `LICENSE` file in the repo (only `.gitattributes`, `.gitignore`,
`README.md`); GitHub reports `license: null`. The README explicitly
disclaims ownership: *"These lists are not created by me. Credit goes to
the original authors."* — so even the curator isn't clearly positioned to
grant a redistribution license.

**Build only the personal-use case**: each comic-server instance
clones/fetches for its own operator's private use — nothing is
redistributed onward to other users of the software. This is functionally
equivalent to an RSS reader fetching a public page. Do **not** bundle CBL
files into release artifacts or run any shared cache that serves other
comic-server installs without asking the DieselTech maintainer first.

## 4. Data model

Existing tables (`lists`, `reading_list_items`, ComicDb.xml
`ComicReadingListItem`) look reusable as-is for the *result* of an import
— they weren't designed with import provenance in mind, though. Recommend
adding, scoped to imported lists only (not a schema change to every
list):

- source (`local_file` / `git:<repo-url>:<path>`)
- source commit SHA (for git sources) / content hash (for local files)
- last-imported timestamp
- per-entry match state (matched / placeholder / dropped), useful for a
  "N of M matched" summary and any future reimport diffing

Ordering must be preserved — `reading_list_items` is presumably already
ordered; confirm during implementation.

## 5. Git-based change detection (for watch/reimport, phase 3+)

- **Shell out to the real `git` binary** — comic-server runs in Docker, so
  a known git binary can be guaranteed there (user decision, 2026-09-20;
  supersedes an earlier go-git pure-Go-library investigation — see bead
  notes for that research if revisited). Real `git diff -M` gives working
  rename detection for free. **Open question**: local/non-Docker runs
  (`just run-dev`, contributor machines) — recommend the feature
  gracefully disables (not a hard startup dependency) when no `git`
  binary is found, rather than requiring git everywhere.
- Diff the stored last-imported commit SHA against the new `HEAD` (per
  list or per repo — a full clone makes a repo-wide SHA-vs-SHA diff cheap
  and covers every list in one step).
- **Git does not record renames — it infers them** (`git diff -M`/`git log
  --follow` guess by content similarity). A rename plus a big edit in the
  same commit can show as delete+add instead. **Do not key an imported
  list's identity on file path alone.** Use path + rename detection, and
  treat an ambiguous case as "new list, flag the old one as orphaned" —
  never silently drop the user's list.
- Avoid a shallow clone — it breaks diffing an old SHA against `HEAD`.
- If the stored SHA no longer exists in history (force-push, rebase
  upstream), fall back to a full content-hash comparison instead of
  failing.

## 6. Recommended phasing

- **Phase 1 (this spec's recommended first slice):** one-shot import.
  Local file/upload, or pick-from-browsable-DieselTech-clone. CLI + REST
  API + a Settings/Lists UI panel. Matcher: §2a/2b/2c. No watch/reimport.
- **Phase 2:** import-by-URL for arbitrary CBLs (not just DieselTech),
  unmatched-entries report as a first-class UI view.
- **Phase 3:** watch/reimport, gated on real match-rate data from phase 1
  — a low match rate would make watch mostly noise (re-importing entries
  that were never going to match anyway). Needs: merge-vs-overwrite policy
  for a re-imported list a user has since edited, handling of entries
  removed upstream, and the git-based change detection in §5.

## 7. Other open questions for implementation

- **Export/round-trip**: an imported list must still export as valid
  ComicRackCE-compatible XML (`comic-server export`, comic-server-bcl6).
  Should be automatic if it reuses the existing list model, but verify.
- **Device sync**: reading lists already sync to devices via
  `sync_information.xml`. Confirm an imported list behaves identically —
  likely yes, no special-casing needed, but verify.
- **Security**: untrusted XML from the network — apply size limits and
  safe (non-XXE) XML parsing, same caution as any external file import.
  A fixed, server-configured repo URL (not arbitrary user-supplied URLs
  in phase 1) keeps the SSRF surface small; revisit when phase 2 allows
  arbitrary URLs.

## 8. Follow-up implementation beads to file (not filed yet)

Once this spec is accepted:
1. CBL parser (`<ReadingList>`/`<Book>`/`<Database>` → Go structs) — small.
2. Matcher: CV-ID path + ComicRack-parity string-matching fallback
   (§2a/2b), plus a scratch script to measure real match rate against the
   library before building further. — medium, do this first.
3. Import data-model additions (§4) — small.
4. Local-file import: CLI + API + minimal UI — medium (depends on 1-3).
5. DieselTech clone/browse/search (§3) — medium-large, can follow 4.
6. Watch/reimport (§5, §6 phase 3) — large, blocked on match-rate data
   from bead 2 and a merge-policy decision; do not start until phase 1
   ships and match rate is known.
7. Add unmatched CBL entries to the existing wanted-books mechanism
   (§2e, comic-server-38f7) — small, depends on 2-4. Do this instead of
   a bespoke placeholder-book concept.
8. Match-correction UI for an import's results (§2d) — medium, depends
   on 4. Fast-follow after phase 1 ships; hold until real string-fallback
   match quality is visible from bead 2's scratch measurement - may turn
   out to be needed rarely if CV-ID coverage stays near 100%.
