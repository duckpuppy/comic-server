package library

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/duckpuppy/comic-server/internal/log"
)

const (
	// defaultDebounce coalesces rapid successive filesystem events (many
	// apps, including ComicRack, write a library XML in several syscalls -
	// temp file writes, a final write, a rename) into a single reload.
	defaultDebounce = 2 * time.Second

	// selfWriteGrace suppresses reload for file-change events that follow
	// shortly after comic-server's own write (Flush, or LibraryCache's
	// periodic auto-flush), so comic-server doesn't reload in response to
	// its own writes.
	selfWriteGrace = 3 * time.Second
)

// ReloadableBackend is implemented by any Backend that can reload its data
// from path (an XML file, or a source re-imported into a database - see
// storage.SQLiteBackend.Reload) in place, without a process restart.
type ReloadableBackend interface {
	Reload() error
	BookCount() int
}

// lastWriteTimer is implemented by backends where comic-server's own writes
// can touch the watched file (XMLBackend, via its periodic cache flush),
// so the watcher needs to suppress reloading in response to its own write.
// Backends that never write the watched path (e.g. SQLiteBackend, which
// only ever writes to its database file, never the XML source it reimports
// from) simply don't implement this, and self-write suppression is skipped.
type lastWriteTimer interface {
	LastWriteTime() time.Time
}

// Watcher watches a library source file for external changes (e.g.
// ComicRack saving the library, possibly from another machine writing to a
// shared/synced path) and reloads it into backend automatically.
type Watcher struct {
	backend  ReloadableBackend
	path     string
	debounce time.Duration
	grace    time.Duration
	onReload []func()
	fsw      *fsnotify.Watcher
}

// NewWatcher creates a Watcher for backend's source file at path. The
// parent directory is watched (not the file directly) so that atomic
// write-then-rename saves - which replace the file's inode - are still
// detected.
func NewWatcher(backend ReloadableBackend, path string) (*Watcher, error) {
	if path == "" {
		return nil, fmt.Errorf("library watcher: no path configured")
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("library watcher: create fsnotify watcher: %w", err)
	}

	dir := filepath.Dir(path)
	if err := fsw.Add(dir); err != nil {
		fsw.Close()
		return nil, fmt.Errorf("library watcher: watch directory %q: %w", dir, err)
	}

	return &Watcher{
		backend:  backend,
		path:     path,
		debounce: defaultDebounce,
		grace:    selfWriteGrace,
		fsw:      fsw,
	}, nil
}

// OnReload registers a callback invoked after a successful reload. Not
// called when a change is detected but suppressed as comic-server's own
// write, nor when a reload attempt fails. Must be called before Run.
func (w *Watcher) OnReload(fn func()) {
	w.onReload = append(w.onReload, fn)
}

// Run blocks, watching for file changes and reloading the backend until
// ctx is canceled.
func (w *Watcher) Run(ctx context.Context) {
	defer w.fsw.Close()

	target := filepath.Base(w.path)
	var debounceTimer *time.Timer
	var debounceC <-chan time.Time

	log.Info().Str("path", w.path).Msg("Library watcher: watching for external changes")

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != target {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(w.debounce)
				debounceC = debounceTimer.C
			} else {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(w.debounce)
			}

		case <-debounceC:
			debounceTimer = nil
			debounceC = nil
			w.handleChange()

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("Library watcher: fsnotify error")
		}
	}
}

func (w *Watcher) handleChange() {
	if lwt, ok := w.backend.(lastWriteTimer); ok && time.Since(lwt.LastWriteTime()) < w.grace {
		log.Debug().Msg("Library watcher: change follows comic-server's own recent write, skipping reload")
		return
	}

	log.Info().Str("path", w.path).Msg("Library watcher: external change detected, reloading")
	if err := w.backend.Reload(); err != nil {
		log.Error().Err(err).Msg("Library watcher: reload failed")
		return
	}
	log.Info().Int("books", w.backend.BookCount()).Msg("Library watcher: reload complete")

	for _, fn := range w.onReload {
		fn()
	}
}
