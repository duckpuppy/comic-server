package library

import (
	"context"
	"sync"
	"testing"
	"time"
)

// waitFor polls cond until it returns true or timeout elapses, failing the
// test on timeout. Used because Watcher reacts to real filesystem events on
// its own debounce timer, not something we can step deterministically.
func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

func TestWatcher_ReloadsOnExternalChange(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(backend, path)
	if err != nil {
		t.Fatal(err)
	}
	w.debounce = 20 * time.Millisecond
	w.grace = 20 * time.Millisecond

	var mu sync.Mutex
	reloadCount := 0
	w.OnReload(func() {
		mu.Lock()
		reloadCount++
		mu.Unlock()
	})

	ctx := t.Context()
	go w.Run(ctx)

	time.Sleep(50 * time.Millisecond) // let the watcher's fsw.Add settle

	externalLib := &ComicLibrary{
		ID:    "test-library",
		Books: []ComicBook{{ID: "book-1", Series: "Batman", Title: "Watcher Detected This"}},
	}
	if err := SaveLibrary(path, externalLib); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 3*time.Second, "reload callback to fire", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reloadCount == 1
	})

	book, err := backend.GetBook("book-1")
	if err != nil || book == nil || book.Title != "Watcher Detected This" {
		t.Errorf("expected backend to reflect externally written change, got %+v (err=%v)", book, err)
	}
}

func TestWatcher_SuppressesSelfTriggeredWrites(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(backend, path)
	if err != nil {
		t.Fatal(err)
	}
	w.debounce = 20 * time.Millisecond
	w.grace = 5 * time.Second // long grace window relative to the test's own timing

	var mu sync.Mutex
	reloadCount := 0
	w.OnReload(func() {
		mu.Lock()
		reloadCount++
		mu.Unlock()
	})

	ctx := t.Context()
	go w.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	// comic-server writing to its own library file (e.g. via Flush) should
	// not trigger a reload.
	if err := backend.UpdateBook(&ComicBook{ID: "book-1", Series: "Batman", Title: "Our Own Write"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Flush(); err != nil {
		t.Fatal(err)
	}

	// Give the watcher time to see the fsnotify event and decide whether to
	// act on it - if it were going to (incorrectly) reload, it would have
	// done so within a couple debounce windows.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	got := reloadCount
	mu.Unlock()
	if got != 0 {
		t.Errorf("expected self-triggered write to be suppressed, got %d reload(s)", got)
	}
}

func TestWatcher_DebouncesRapidWrites(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(backend, path)
	if err != nil {
		t.Fatal(err)
	}
	w.debounce = 100 * time.Millisecond
	w.grace = 20 * time.Millisecond

	var mu sync.Mutex
	reloadCount := 0
	w.OnReload(func() {
		mu.Lock()
		reloadCount++
		mu.Unlock()
	})

	ctx := t.Context()
	go w.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	// Several rapid writes within the debounce window should coalesce into
	// a single reload, not one per write.
	for i := range 5 {
		lib := &ComicLibrary{
			ID:    "test-library",
			Books: []ComicBook{{ID: "book-1", Series: "Batman", Title: "Rapid Write"}},
		}
		_ = i
		if err := SaveLibrary(path, lib); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond) // well within the 100ms debounce window
	}

	waitFor(t, 3*time.Second, "at least one reload to fire", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reloadCount >= 1
	})

	// Give any extra (incorrect) reloads a chance to fire before asserting
	// there was exactly one.
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	got := reloadCount
	mu.Unlock()
	if got != 1 {
		t.Errorf("expected exactly 1 debounced reload for 5 rapid writes, got %d", got)
	}
}

func TestWatcher_StopsOnContextCancel(t *testing.T) {
	path := newTestXMLLibraryFile(t)
	backend, err := NewXMLBackend(path, 0)
	if err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(backend, path)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestNewWatcher_EmptyPathErrors(t *testing.T) {
	backend := NewXMLBackendFromLibrary(&ComicLibrary{}, "", nil)
	if _, err := NewWatcher(backend, ""); err == nil {
		t.Error("expected NewWatcher to error on an empty path")
	}
}
