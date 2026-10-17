package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/duckpuppy/comic-server/internal/backup"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/spf13/cobra"
)

var (
	restoreDir           string
	restoreApply         bool
	restoreIncludeConfig bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore [backup-name]",
	Short: "Restore comic-server's own state from a backup (server must be stopped)",
	Long: `Restores config.db, library.db and the ComicVine cache from a backup made by
the Backups feature.

With no name it lists the backups. With a name it checks the backup and shows
what would change. Nothing is written until you add --apply.

Before it replaces anything it takes a safety backup of the current files in
the same folder. It refuses a damaged backup (checksum mismatch) and a backup
whose database is newer than this build understands.

Backup settings (folder, schedule) are never restored: the current ones stay.
config.yaml is only restored with --include-config.

Stop the server first. Start it again after the restore.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRestore,
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().StringVar(&restoreDir, "backup-dir", "", "Folder that holds the backups (default: the folder saved in Settings > Backups)")
	restoreCmd.Flags().BoolVar(&restoreApply, "apply", false, "Do the restore (without it, only show the plan)")
	restoreCmd.Flags().BoolVar(&restoreIncludeConfig, "include-config", false, "Also restore config.yaml from the backup")
}

func runRestore(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	configDBPath := filepath.Join(filepath.Dir(configPath), "config.db")

	dir := restoreDir
	if dir == "" {
		dir = savedBackupDir(configDBPath)
	}
	if dir == "" {
		return fmt.Errorf("no backup folder: pass --backup-dir, or save one in Settings > Backups")
	}

	if len(args) == 0 {
		return listBackups(cmd, dir)
	}
	name := args[0]

	cachePath, err := resolveComicVineCachePath(cfg.Server.ComicVineCachePath)
	if err != nil {
		return fmt.Errorf("resolve ComicVine cache path: %w", err)
	}
	targets := []backup.RestoreTarget{
		{File: "config.db", Path: configDBPath, MaxSchema: configdb.SchemaVersion,
			KeepTables: []string{"backup_settings", "backup_runs"}},
		{File: "library.db", Path: cfg.Server.DatabasePath, MaxSchema: storage.SchemaVersion},
		{File: "comicvine_cache.db", Path: cachePath, MaxSchema: comicvine.CacheSchemaVersion},
	}
	if restoreIncludeConfig {
		targets = append(targets, backup.RestoreTarget{File: "config.yaml", Path: configPath})
	}

	out := cmd.OutOrStdout()
	plan, err := backup.PlanRestore(dir, name, targets)
	if err != nil {
		return err
	}
	printPlan(cmd, plan)
	if !restoreApply {
		fmt.Fprintln(out, "\nNothing was changed. Stop the server, then add --apply to restore.")
		return nil
	}

	port := cfg.Server.ServerPort
	if port == 0 {
		port = 7620
	}
	if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second); err == nil {
		c.Close()
		return fmt.Errorf("a server is answering on port %d. Stop it first, then run restore again", port)
	}

	done, err := backup.ApplyRestore(context.Background(), dir, name, targets, Version)
	if err != nil {
		return err
	}
	if done.PreRestore != "" {
		fmt.Fprintf(out, "\nSafety backup of the old files: %s\n", filepath.Join(dir, done.PreRestore))
	}
	fmt.Fprintln(out, "Restore done. Start the server again.")
	return nil
}

func listBackups(cmd *cobra.Command, dir string) error {
	out := cmd.OutOrStdout()
	list, err := backup.List(dir)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintf(out, "No backups in %s\n", dir)
		return nil
	}
	fmt.Fprintf(out, "Backups in %s (newest first):\n", dir)
	for _, e := range list {
		fmt.Fprintf(out, "  %s  %s  app %s  %d books  %.1f MB\n", e.Name,
			e.Manifest.CreatedAt.Local().Format("2006-01-02 15:04"), e.Manifest.AppVersion,
			e.Manifest.BookCount, float64(e.Manifest.TotalSize())/(1<<20))
	}
	fmt.Fprintln(out, "\nPreview one with: comic-server restore <name>")
	return nil
}

func printPlan(cmd *cobra.Command, p *backup.RestorePlan) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Backup %s (made %s by app %s) passed its checksum check.\n", p.Name,
		p.Manifest.CreatedAt.Local().Format("2006-01-02 15:04"), p.Manifest.AppVersion)
	if p.Manifest.BookCount > 0 {
		fmt.Fprintf(out, "It holds %d books.\n", p.Manifest.BookCount)
	}
	for _, it := range p.Items {
		if it.Restore {
			fmt.Fprintf(out, "  RESTORE %-20s -> %s\n", it.File, it.Path)
		} else {
			fmt.Fprintf(out, "  skip    %-20s %s\n", it.File, it.Reason)
		}
	}
	fmt.Fprintln(out, "Backup settings (folder, schedule) stay as they are now.")
}

// savedBackupDir reads the backup folder from config.db without running any
// migration, so a restore preview never changes the database.
func savedBackupDir(configDBPath string) string {
	db, err := sql.Open("sqlite", "file:"+configDBPath+"?mode=ro")
	if err != nil {
		return ""
	}
	defer db.Close()
	var dir string
	if err := db.QueryRow(`SELECT dir FROM backup_settings WHERE id = 1`).Scan(&dir); err != nil {
		return ""
	}
	return dir
}
