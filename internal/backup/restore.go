package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RestoreTarget says where one file of a backup goes on restore.
type RestoreTarget struct {
	// File is the name inside the backup ("config.db").
	File string
	// Path is the live file that gets replaced.
	Path string
	// MaxSchema is the newest PRAGMA user_version this build understands for a
	// database file. 0 means the file is not a database (config.yaml).
	MaxSchema int
	// KeepTables are tables copied from the live database into the restored
	// one, so a restore does not change them (config.db: backup settings).
	KeepTables []string
}

// RestoreItem is one line of a restore plan.
type RestoreItem struct {
	File    string `json:"file"`
	Path    string `json:"path,omitempty"`
	Size    int64  `json:"size"`
	Schema  int    `json:"schema,omitempty"`
	Restore bool   `json:"restore"`
	Reason  string `json:"reason,omitempty"` // why a file is left alone
}

// RestorePlan is what a restore would do.
type RestorePlan struct {
	Name       string        `json:"name"`
	Manifest   *Manifest     `json:"manifest"`
	Items      []RestoreItem `json:"items"`
	PreRestore string        `json:"pre_restore,omitempty"` // name of the safety backup, set by Apply
}

const stagedSuffix = ".restore-new"

// PlanRestore checks a backup and says what restoring it would do. It writes
// nothing. It fails when the backup is damaged (checksum mismatch), was made by
// a newer manifest format, or holds a database newer than this build knows.
func PlanRestore(dir, name string, targets []RestoreTarget) (*RestorePlan, error) {
	if name == "" || name != filepath.Base(name) || !strings.HasPrefix(name, NamePrefix) {
		return nil, fmt.Errorf("%q is not a backup name", name)
	}
	if err := Verify(dir, name); err != nil {
		return nil, fmt.Errorf("backup is damaged: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, name, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.ManifestVersion > ManifestVersion {
		return nil, fmt.Errorf("backup uses manifest format %d; this build reads up to %d", m.ManifestVersion, ManifestVersion)
	}
	byFile := map[string]RestoreTarget{}
	for _, t := range targets {
		byFile[t.File] = t
	}
	plan := &RestorePlan{Name: name, Manifest: &m}
	for _, f := range m.Files {
		it := RestoreItem{File: f.Name, Size: f.Size}
		t, ok := byFile[f.Name]
		switch {
		case !ok:
			it.Reason = "not restored (no target, or not asked for)"
		case t.Path == "":
			it.Reason = "not restored (no path configured)"
		default:
			it.Path = t.Path
			it.Restore = true
			if t.MaxSchema > 0 {
				it.Schema = m.SchemaVersions[f.Name]
				if it.Schema > t.MaxSchema {
					return nil, fmt.Errorf("%s has schema %d; this build only understands up to %d. Use a newer comic-server to restore it",
						f.Name, it.Schema, t.MaxSchema)
				}
			}
		}
		plan.Items = append(plan.Items, it)
	}
	restorable := false
	for _, it := range plan.Items {
		restorable = restorable || it.Restore
	}
	if !restorable {
		return nil, errors.New("nothing in this backup can be restored with the current settings")
	}
	return plan, nil
}

// ApplyRestore restores a backup. The server must be stopped. Order:
//  1. plan (checksums, schema check)
//  2. a safety backup of the current files, in the same folder
//  3. every file is copied next to its target and checked
//  4. only then are the targets swapped in
//
// A failure before step 4 leaves the live files untouched.
func ApplyRestore(ctx context.Context, dir, name string, targets []RestoreTarget, appVersion string) (*RestorePlan, error) {
	plan, err := PlanRestore(dir, name, targets)
	if err != nil {
		return nil, err
	}
	byFile := map[string]RestoreTarget{}
	for _, t := range targets {
		byFile[t.File] = t
	}

	// Step 2: safety backup of what is there now.
	var srcs []Source
	var extras []ExtraFile
	for _, it := range plan.Items {
		if !it.Restore {
			continue
		}
		if _, err := os.Stat(it.Path); err != nil {
			continue // nothing there yet (fresh machine)
		}
		t := byFile[it.File]
		if t.MaxSchema > 0 {
			path := it.Path
			srcs = append(srcs, Source{File: it.File, Path: path, Snapshot: func(dest string) error {
				db, err := sql.Open("sqlite", "file:"+path+"?_busy_timeout=5000")
				if err != nil {
					return err
				}
				defer db.Close()
				return VacuumInto(db, dest)
			}})
		} else {
			extras = append(extras, ExtraFile{File: it.File, Path: it.Path})
		}
	}
	if len(srcs)+len(extras) > 0 {
		_, pre, err := Run(ctx, Options{
			Dir: dir, AppVersion: appVersion, Sources: srcs, Extras: extras,
			Notes: []string{"Safety backup taken before restoring " + name},
		})
		if err != nil {
			return nil, fmt.Errorf("safety backup failed, nothing was changed: %w", err)
		}
		plan.PreRestore = pre
	}

	// Step 3: stage.
	var staged []string
	cleanup := func() {
		for _, p := range staged {
			os.Remove(p)
		}
	}
	type move struct{ from, to string }
	var moves []move
	for _, it := range plan.Items {
		if !it.Restore {
			continue
		}
		t := byFile[it.File]
		if err := ctx.Err(); err != nil {
			cleanup()
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(it.Path), 0o755); err != nil {
			cleanup()
			return nil, err
		}
		dest := it.Path + stagedSuffix
		os.Remove(dest)
		staged = append(staged, dest)
		if err := copyFile(filepath.Join(dir, name, it.File), dest); err != nil {
			cleanup()
			return nil, fmt.Errorf("stage %s: %w", it.File, err)
		}
		if t.MaxSchema > 0 {
			if err := checkIntegrity(dest); err != nil {
				cleanup()
				return nil, fmt.Errorf("%s failed its integrity check: %w", it.File, err)
			}
			if len(t.KeepTables) > 0 {
				if _, err := os.Stat(it.Path); err == nil {
					if err := carryTables(dest, it.Path, t.KeepTables); err != nil {
						cleanup()
						return nil, fmt.Errorf("keep current %s settings: %w", it.File, err)
					}
				}
			}
		}
		moves = append(moves, move{dest, it.Path})
	}

	// Step 4: swap. Stale WAL/SHM files of the old database must go first: a
	// leftover -wal would be replayed onto the restored file.
	for _, mv := range moves {
		os.Remove(mv.to + "-wal")
		os.Remove(mv.to + "-shm")
		if err := os.Rename(mv.from, mv.to); err != nil {
			return plan, fmt.Errorf("swap %s: %w (safety backup: %s)", mv.to, err, plan.PreRestore)
		}
	}
	return plan, nil
}

func checkIntegrity(path string) error {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return errors.New(res)
	}
	return nil
}

// carryTables replaces the named tables of the staged database with the rows
// of the same tables in the live database. A table missing in the live file is
// left as the backup has it. Only columns present in both are copied.
func carryTables(staged, live string, tables []string) error {
	db, err := sql.Open("sqlite", "file:"+staged)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // ATTACH belongs to one connection
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "ATTACH DATABASE ? AS live", live); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, t := range tables {
		var ddl string
		err := tx.QueryRow("SELECT sql FROM live.sqlite_master WHERE type='table' AND name=?", t).Scan(&ddl)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		var have int
		if err := tx.QueryRow("SELECT COUNT(*) FROM main.sqlite_master WHERE type='table' AND name=?", t).Scan(&have); err != nil {
			return err
		}
		if have == 0 {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		} else if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM main."%s"`, t)); err != nil {
			return err
		}
		cols, err := commonColumns(tx, t)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			continue
		}
		list := `"` + strings.Join(cols, `","`) + `"`
		if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO main."%s" (%s) SELECT %s FROM live."%s"`, t, list, list, t)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DETACH DATABASE live"); err != nil {
		return err
	}
	// Leave a single self-contained file behind.
	_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return nil
}

func commonColumns(tx *sql.Tx, table string) ([]string, error) {
	read := func(schema string) (map[string]bool, []string, error) {
		rows, err := tx.Query(fmt.Sprintf(`PRAGMA %s.table_info("%s")`, schema, table))
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		set := map[string]bool{}
		var order []string
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				return nil, nil, err
			}
			set[name] = true
			order = append(order, name)
		}
		return set, order, rows.Err()
	}
	liveSet, _, err := read("live")
	if err != nil {
		return nil, err
	}
	_, mainOrder, err := read("main")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range mainOrder {
		if liveSet[c] {
			out = append(out, c)
		}
	}
	return out, nil
}
