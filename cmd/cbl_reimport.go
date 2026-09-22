package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/cblrepo"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/spf13/cobra"
)

// cblReimportTimeout matches internal/api/cbl_reimport.go's
// cblReimportCheckTimeout - the same git sync/diff work, just driven from
// the CLI instead of an HTTP handler.
const cblReimportTimeout = 2 * time.Minute

var cblReimportCmd = &cobra.Command{
	Use:   "cbl-reimport",
	Short: "Check or apply CBL reading-list reimports (comic-server-waeu)",
	Long: `Re-syncs git-sourced CBL-imported reading lists (comic-server-zw0o) with
their upstream file's current content - the CLI equivalent of the web
UI's "Check for Updates" / "Reimport" buttons on a list's detail page,
for scripting and cron use outside the web UI.

Local-file imports have no upstream to check and are always skipped.

With no --list and no --all, this only CHECKS every git-sourced
CBL-imported list against the configured repo's current HEAD and prints
each one's status - no list is modified. --list or --all actually apply
a reimport (full replace of the list's book membership - see
storage.CBLReimportPolicy; nothing to merge, since comic-server has no
feature that lets a user hand-edit a reading list's membership at all).

Examples:
  comic-server cbl-reimport --db library.db
  comic-server cbl-reimport --db library.db --list "{GUID-1234}"
  comic-server cbl-reimport --db library.db --all
  comic-server cbl-reimport --db library.db --all --dry-run`,
	RunE: runCBLReimport,
}

var (
	cblReimportDBPath   string
	cblReimportListID   string
	cblReimportAll      bool
	cblReimportDryRun   bool
	cblReimportRepoURL  string
	cblReimportClonePth string
)

func init() {
	rootCmd.AddCommand(cblReimportCmd)

	cblReimportCmd.Flags().StringVar(&cblReimportDBPath, "db", "", "Path to SQLite database file (required)")
	cblReimportCmd.Flags().StringVar(&cblReimportListID, "list", "", "Reimport this one list ID (mutually exclusive with --all)")
	cblReimportCmd.Flags().BoolVar(&cblReimportAll, "all", false, "Reimport every list with an actionable upstream change (mutually exclusive with --list)")
	cblReimportCmd.Flags().BoolVar(&cblReimportDryRun, "dry-run", false, "With --list/--all, report what would happen without writing")
	cblReimportCmd.Flags().StringVar(&cblReimportRepoURL, "repo-url", "", "Override server.cbl_repo.url from the config file")
	cblReimportCmd.Flags().StringVar(&cblReimportClonePth, "clone-path", "", "Override server.cbl_repo.clone_path from the config file")

	cblReimportCmd.MarkFlagRequired("db")
}

func runCBLReimport(cmd *cobra.Command, args []string) error {
	if cblReimportListID != "" && cblReimportAll {
		return fmt.Errorf("--list and --all are mutually exclusive")
	}

	if _, err := os.Stat(cblReimportDBPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", cblReimportDBPath)
	}

	repo, err := openCBLRepoForCLI()
	if err != nil {
		return err
	}

	db, err := storage.Open(cblReimportDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cblReimportTimeout)
	defer cancel()

	if err := repo.Sync(ctx); err != nil {
		if errors.Is(err, cblrepo.ErrGitNotFound) {
			return fmt.Errorf("git binary not found - CBL reimport requires git to be installed on this host")
		}
		return fmt.Errorf("sync repo: %w", err)
	}

	lists, err := db.ListCBLImportedLists()
	if err != nil {
		return fmt.Errorf("list cbl-imported lists: %w", err)
	}

	type candidate struct {
		listID, name, path string
		change             cblrepo.PathChange
		checkErr           error
	}
	var candidates []candidate
	skippedLocal := 0
	for _, l := range lists {
		repoURL, path, ok := parseGitCBLSourceCLI(l.Source)
		if !ok {
			skippedLocal++
			continue
		}
		if repoURL != repo.URL {
			candidates = append(candidates, candidate{listID: l.ListID, name: l.Name, path: path,
				checkErr: fmt.Errorf("list's source repo is not the configured CBL repo")})
			continue
		}
		change, err := repo.CheckPathChange(ctx, l.SourceRef, path)
		candidates = append(candidates, candidate{listID: l.ListID, name: l.Name, path: path, change: change, checkErr: err})
	}

	if cblReimportListID == "" && !cblReimportAll {
		// Check-only mode - print every candidate's status, write nothing.
		fmt.Printf("Checked %d git-sourced CBL-imported list(s) against %s (%d local-file import(s) skipped, no upstream to check):\n\n",
			len(candidates), repo.URL, skippedLocal)
		for _, c := range candidates {
			if c.checkErr != nil {
				fmt.Printf("  %s  %-40s  error: %v\n", c.listID, c.name, c.checkErr)
				continue
			}
			status := c.change.Status.String()
			if c.change.Status == cblrepo.PathRenamed {
				status += " -> " + c.change.NewPath
			}
			fmt.Printf("  %s  %-40s  %s\n", c.listID, c.name, status)
		}
		if len(candidates) > 0 {
			fmt.Println("\nRe-run with --list <id> or --all to apply a reimport.")
		}
		return nil
	}

	targets := candidates
	if cblReimportListID != "" {
		targets = nil
		for _, c := range candidates {
			if c.listID == cblReimportListID {
				targets = append(targets, c)
				break
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("list %q not found among git-sourced CBL-imported lists (wrong ID, a local-file import, or a different configured repo)", cblReimportListID)
		}
	}

	applied, skipped, failed := 0, 0, 0
	for _, c := range targets {
		if c.checkErr != nil {
			fmt.Printf("SKIP    %s  %-40s  %v\n", c.listID, c.name, c.checkErr)
			skipped++
			continue
		}

		fetchPath := c.path
		switch c.change.Status {
		case cblrepo.PathUnchanged:
			fmt.Printf("SKIP    %s  %-40s  unchanged\n", c.listID, c.name)
			skipped++
			continue
		case cblrepo.PathRenamed:
			fetchPath = c.change.NewPath
		case cblrepo.PathOrphaned:
			fmt.Printf("SKIP    %s  %-40s  orphaned upstream - refused, list left as-is\n", c.listID, c.name)
			skipped++
			continue
		case cblrepo.PathBaseUnknown:
			fmt.Printf("SKIP    %s  %-40s  upstream history rewritten - delete and freshly re-import instead\n", c.listID, c.name)
			skipped++
			continue
		}

		if cblReimportDryRun {
			fmt.Printf("DRY-RUN %s  %-40s  would reimport from %s\n", c.listID, c.name, fetchPath)
			applied++
			continue
		}

		result, err := applyCBLReimport(repo, db, c.listID, fetchPath)
		if err != nil {
			fmt.Printf("FAIL    %s  %-40s  %v\n", c.listID, c.name, err)
			failed++
			continue
		}
		fmt.Printf("OK      %s  %-40s  matched %d (cv_id) + %d (series/number), %d unmatched\n",
			c.listID, c.name, result.MatchedCVID, result.MatchedOther, result.Unmatched)
		applied++
	}

	fmt.Printf("\n%d applied, %d skipped, %d failed\n", applied, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("%d list(s) failed to reimport", failed)
	}
	return nil
}

// applyCBLReimport parses fetchPath from repo at its current HEAD and
// replaces listID's membership with the result - the CLI's equivalent of
// internal/api/cbl_reimport.go's handleCBLReimport body.
func applyCBLReimport(repo *cblrepo.Repo, db *storage.DB, listID, fetchPath string) (*storage.CBLImportResult, error) {
	f, err := repo.Open(fetchPath)
	if err != nil {
		return nil, fmt.Errorf("open upstream file: %w", err)
	}
	defer f.Close()

	rl, err := cbl.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse upstream CBL file: %w", err)
	}
	if rl.IsSmartList() {
		return nil, storage.ErrCBLIsSmartList
	}

	newHead, err := repo.HeadCommit()
	if err != nil {
		return nil, fmt.Errorf("get clone head commit: %w", err)
	}
	newSource := storage.CBLImportSource{
		Source:    fmt.Sprintf("git:%s:%s", repo.URL, fetchPath),
		SourceRef: newHead,
	}

	return db.ReimportCBL(listID, rl, newSource)
}

// openCBLRepoForCLI builds a *cblrepo.Repo from config.yaml's
// server.cbl_repo settings (same source the running server uses),
// overridden by --repo-url/--clone-path when given - see
// cmd/server.go's identical resolveCBLRepoClonePath for the "empty
// clone_path" default.
func openCBLRepoForCLI() (*cblrepo.Repo, error) {
	url := cblReimportRepoURL
	clonePath := cblReimportClonePth

	if url == "" || clonePath == "" {
		configPath, err := GetConfigPath()
		if err != nil {
			return nil, fmt.Errorf("get config path: %w", err)
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		if url == "" {
			url = cfg.Server.CBLRepo.URL
		}
		if clonePath == "" {
			clonePath = cfg.Server.CBLRepo.ClonePath
		}
	}

	if url == "" {
		return nil, fmt.Errorf("no CBL repo URL configured - set server.cbl_repo.url in the config file or pass --repo-url")
	}
	resolvedClonePath, err := resolveCBLRepoClonePath(clonePath)
	if err != nil {
		return nil, fmt.Errorf("resolve clone path: %w", err)
	}
	return cblrepo.New(url, resolvedClonePath), nil
}

// parseGitCBLSourceCLI mirrors internal/api/cbl_reimport.go's
// parseGitCBLSource (unexported there, package api) - splits a
// "git:<repo-url>:<path>" CBLImportSource.Source string. Returns
// ok=false for anything else (local_file, empty). repo-url itself
// contains colons (https://...) but never a path component with one, so
// split on the LAST colon.
func parseGitCBLSourceCLI(source string) (repoURL, path string, ok bool) {
	const prefix = "git:"
	if !strings.HasPrefix(source, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(source, prefix)
	i := strings.LastIndex(rest, ":")
	if i < 0 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}
