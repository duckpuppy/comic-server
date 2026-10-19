// Package backup writes consistent snapshots of comic-server's own state
// (comic-server-yzdt): config.db, library.db, the ComicVine cache and
// config.yaml. Comic files are out of scope (ZFS snapshots / the nightly sync).
//
// One backup is one directory holding the snapshot files and a manifest.json
// (app version, per-file size and sha256, schema versions, book count). It is
// built under a ".partial-" name and renamed into place, so a backup that
// exists is a complete one. SQLite files are copied with VACUUM INTO, which is
// consistent while the server keeps running; a plain file copy of a WAL
// database is not.
package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	// NamePrefix starts every backup directory name. Pruning only ever touches
	// directories with this prefix and a readable manifest.
	NamePrefix = "backup-"
	// ManifestName is the manifest file inside a backup directory.
	ManifestName = "manifest.json"
	// ManifestVersion is the manifest format version.
	ManifestVersion = 1

	partialPrefix = ".partial-"
)

// Source is one SQLite database to snapshot.
type Source struct {
	// File is the name inside the backup, for example "config.db".
	File string
	// Path is the live database file. Used only to estimate the space needed.
	Path string
	// Snapshot writes a consistent copy to dest (which does not exist yet).
	Snapshot func(dest string) error
	// Count, if set, returns a row count from the finished snapshot file
	// (the library's book count). The result goes into the manifest.
	Count func(snapshotPath string) (int, error)
}

// ExtraFile is a plain file copied as it is (config.yaml).
type ExtraFile struct {
	File string
	Path string
}

// FileEntry describes one file of a backup.
type FileEntry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is the manifest.json of a backup.
type Manifest struct {
	ManifestVersion int            `json:"manifest_version"`
	AppVersion      string         `json:"app_version"`
	CreatedAt       time.Time      `json:"created_at"`
	Files           []FileEntry    `json:"files"`
	SchemaVersions  map[string]int `json:"schema_versions,omitempty"`
	BookCount       int            `json:"book_count,omitempty"`
	Notes           []string       `json:"notes,omitempty"`
}

// TotalSize is the sum of the file sizes.
func (m *Manifest) TotalSize() int64 {
	var n int64
	for _, f := range m.Files {
		n += f.Size
	}
	return n
}

// Options configures one Run.
type Options struct {
	Dir        string // where backups live
	Keep       int    // newest backups to keep after a successful run; < 1 keeps all
	AppVersion string
	Sources    []Source
	Extras     []ExtraFile
	Notes      []string // copied into the manifest (for example "XML backend: library not included")
	Now        func() time.Time
}

// VacuumInto writes a consistent copy of db to dest with VACUUM INTO.
func VacuumInto(db *sql.DB, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	if _, err := db.Exec("VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("vacuum into %s: %w", dest, err)
	}
	return nil
}

// Run takes one backup and returns its manifest and directory name.
func Run(ctx context.Context, opt Options) (*Manifest, string, error) {
	if opt.Dir == "" {
		return nil, "", errors.New("backup folder is not set")
	}
	if !filepath.IsAbs(opt.Dir) {
		return nil, "", errors.New("backup folder must be an absolute path")
	}
	if len(opt.Sources)+len(opt.Extras) == 0 {
		return nil, "", errors.New("nothing to back up")
	}
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	if err := os.MkdirAll(opt.Dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create backup folder: %w", err)
	}

	// A full disk must fail loudly before any copying starts.
	var need int64
	for _, s := range opt.Sources {
		if fi, err := os.Stat(s.Path); err == nil {
			need += fi.Size()
		}
	}
	for _, e := range opt.Extras {
		if fi, err := os.Stat(e.Path); err == nil {
			need += fi.Size()
		}
	}
	need += need / 10 // VACUUM INTO can differ a little from the live size
	if free, ok := freeBytes(opt.Dir); ok && free < uint64(need) {
		return nil, "", fmt.Errorf("not enough space in %s: need about %d MB, %d MB free", opt.Dir, need>>20, free>>20)
	}

	created := now()
	name := uniqueName(opt.Dir, NamePrefix+created.Format("20060102-150405"))
	tmp := filepath.Join(opt.Dir, partialPrefix+name)
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return nil, "", fmt.Errorf("create backup directory: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(tmp)
		}
	}()

	m := &Manifest{ManifestVersion: ManifestVersion, AppVersion: opt.AppVersion, CreatedAt: created.UTC(),
		SchemaVersions: map[string]int{}, Notes: opt.Notes}

	for _, s := range opt.Sources {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		dest := filepath.Join(tmp, s.File)
		if err := s.Snapshot(dest); err != nil {
			return nil, "", fmt.Errorf("back up %s: %w", s.File, err)
		}
		if err := os.Chmod(dest, 0o600); err != nil {
			return nil, "", err
		}
		entry, err := describe(dest, s.File)
		if err != nil {
			return nil, "", err
		}
		m.Files = append(m.Files, entry)
		if v, err := userVersion(dest); err == nil {
			m.SchemaVersions[s.File] = v
		} else {
			return nil, "", fmt.Errorf("the %s snapshot does not open: %w", s.File, err)
		}
		if s.Count != nil {
			if n, err := s.Count(dest); err == nil {
				m.BookCount = n
			}
		}
	}
	for _, e := range opt.Extras {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		dest := filepath.Join(tmp, e.File)
		if err := copyFile(e.Path, dest); err != nil {
			return nil, "", fmt.Errorf("back up %s: %w", e.File, err)
		}
		entry, err := describe(dest, e.File)
		if err != nil {
			return nil, "", err
		}
		m.Files = append(m.Files, entry)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, ManifestName), data, 0o600); err != nil {
		return nil, "", fmt.Errorf("write manifest: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(opt.Dir, name)); err != nil {
		return nil, "", fmt.Errorf("finish backup: %w", err)
	}
	ok = true
	if opt.Keep > 0 {
		Prune(opt.Dir, opt.Keep)
	}
	return m, name, nil
}

// Entry is one existing backup.
type Entry struct {
	Name     string    `json:"name"`
	Manifest *Manifest `json:"manifest"`
}

// List returns the complete backups in dir, newest first. Directories without
// a readable manifest (a partial copy, someone else's folder) are skipped.
func List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, de := range des {
		if !de.IsDir() || !strings.HasPrefix(de.Name(), NamePrefix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, de.Name(), ManifestName))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		out = append(out, Entry{Name: de.Name(), Manifest: &m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.CreatedAt.After(out[j].Manifest.CreatedAt) })
	return out, nil
}

// Prune deletes all but the newest keep complete backups and returns the names
// removed. It only touches directories List recognises, and also sweeps
// leftover ".partial-" directories older than a day (a crash mid-backup).
func Prune(dir string, keep int) []string {
	var removed []string
	entries, err := List(dir)
	if err != nil {
		return nil
	}
	for i, e := range entries {
		if i >= keep {
			if os.RemoveAll(filepath.Join(dir, e.Name)) == nil {
				removed = append(removed, e.Name)
			}
		}
	}
	if des, err := os.ReadDir(dir); err == nil {
		for _, de := range des {
			if de.IsDir() && strings.HasPrefix(de.Name(), partialPrefix) {
				if fi, err := de.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
					os.RemoveAll(filepath.Join(dir, de.Name()))
				}
			}
		}
	}
	return removed
}

// ErrNotABackup is returned by Delete for a name that is not a complete
// backup in dir.
var ErrNotABackup = errors.New("not a backup in this folder")

// Delete removes one complete backup. It only touches a directory List
// recognises (a "backup-" name with a readable manifest), so a stray name, a
// path, a partial copy or someone else's folder is refused.
func Delete(dir, name string) error {
	if name == "" || name != filepath.Base(name) || !strings.HasPrefix(name, NamePrefix) {
		return ErrNotABackup
	}
	entries, err := List(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == name {
			return os.RemoveAll(filepath.Join(dir, name))
		}
	}
	return ErrNotABackup
}

// Verify re-reads every file of a backup and checks size and sha256 against
// its manifest.
func Verify(dir, name string) error {
	base := filepath.Join(dir, name)
	data, err := os.ReadFile(filepath.Join(base, ManifestName))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	for _, f := range m.Files {
		got, err := describe(filepath.Join(base, f.Name), f.Name)
		if err != nil {
			return err
		}
		if got.Size != f.Size || got.SHA256 != f.SHA256 {
			return fmt.Errorf("%s does not match its manifest", f.Name)
		}
	}
	return nil
}

func uniqueName(dir, base string) string {
	name := base
	for i := 2; ; i++ {
		_, e1 := os.Stat(filepath.Join(dir, name))
		_, e2 := os.Stat(filepath.Join(dir, partialPrefix+name))
		if os.IsNotExist(e1) && os.IsNotExist(e2) {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

func describe(path, name string) (FileEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileEntry{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{Name: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// userVersion opens a snapshot read-only and returns its PRAGMA user_version,
// which proves the file is a readable SQLite database.
func userVersion(path string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

// CountRows returns a Count function that runs query on a snapshot and
// returns the single integer it yields.
func CountRows(query string) func(string) (int, error) {
	return func(path string) (int, error) {
		db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
		if err != nil {
			return 0, err
		}
		defer db.Close()
		var n int
		err = db.QueryRow(query).Scan(&n)
		return n, err
	}
}

// SameDevice reports whether a and b are on the same filesystem. false when
// unknown (including on Windows).
func SameDevice(a, b string) bool { return sameDevice(a, b) }
