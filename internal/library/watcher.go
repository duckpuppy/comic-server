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

// Watcher watches a library XML file for external changes (e.g. ComicRack
// saving the library, possibly from another machine writing to a shared/
// synced path) and reloads it into backend automatically.
type Watcher struct {
	backend  *XMLBackend
	path     string
	debounce time.Duration
	grace    time.Duration
	onReload []func()
	fsw      *fsnotify.Watcher
}

// NewWatcher creates a Watcher for backend's library file. The parent
// directory is watched (not the file directly) so that atomic
// write-then-rename saves - which replace the file's inode - are still
// detected.
func NewWatcher(backend *XMLBackend, path string) (*Watcher, error) {
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
	if time.Since(w.backend.LastWriteTime()) < w.grace {
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
