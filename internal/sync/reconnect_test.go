package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// fakeSleepSyncer returns a syncer whose sleeps are recorded, not waited.
func fakeSleepSyncer(mc *MockClient, backend library.Backend) (*Syncer, *[]time.Duration) {
	var slept []time.Duration
	s := NewSyncer(mc, backend)
	s.sleep = func(d time.Duration) { slept = append(slept, d) }
	return s, &slept
}

func booksOnDisk(t *testing.T, n int) library.Backend {
	t.Helper()
	dir := t.TempDir()
	books := make([]library.ComicBook, n)
	for i := range books {
		path := filepath.Join(dir, fmt.Sprintf("book%d.cbz", i+1))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("dummy comic %d data", i+1)), 0644); err != nil {
			t.Fatal(err)
		}
		books[i] = library.ComicBook{ID: fmt.Sprintf("book%d", i+1), Title: fmt.Sprintf("Book %d", i+1), PageCount: 10, FilePath: path}
	}
	return library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil)
}

func TestWaitForDevice_BacksOffThenReturnsWhenDeviceIsBack(t *testing.T) {
	mc := NewMockClient()
	fails := 2
	mc.CheckAbortFunc = func() (bool, error) {
		if fails > 0 {
			fails--
			return false, errors.New("i/o timeout")
		}
		return false, nil
	}
	s, slept := fakeSleepSyncer(mc, booksOnDisk(t, 1))

	if err := s.waitForDevice(errors.New("first failure")); err != nil {
		t.Fatalf("waitForDevice = %v, want nil", err)
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	if fmt.Sprint(*slept) != fmt.Sprint(want) {
		t.Errorf("pauses = %v, want %v", *slept, want)
	}
}

func TestWaitForDevice_GivesUpAfterTheWindowAndCapsThePause(t *testing.T) {
	mc := NewMockClient()
	mc.CheckAbortFunc = func() (bool, error) { return false, errors.New("no route to host") }
	s, slept := fakeSleepSyncer(mc, booksOnDisk(t, 1))

	err := s.waitForDevice(errors.New("first failure"))
	if err == nil || err.Error() != "no route to host" {
		t.Fatalf("err = %v, want the last probe error", err)
	}
	var total time.Duration
	for _, d := range *slept {
		total += d
		if d > reconnectMaxDelay {
			t.Errorf("pause %v is over the cap %v", d, reconnectMaxDelay)
		}
	}
	if total < s.reconnectWindow {
		t.Errorf("waited %v, less than the window %v", total, s.reconnectWindow)
	}
	if len(*slept) < 5 || (*slept)[4] != 30*time.Second {
		t.Errorf("pauses should grow 2,4,8,16 then reach 30s: %v", *slept)
	}
}

func TestWaitForDevice_ZeroWindowFailsAtOnce(t *testing.T) {
	mc := NewMockClient()
	s, slept := fakeSleepSyncer(mc, booksOnDisk(t, 1))
	s.SetReconnectWindow(0)

	cause := errors.New("boom")
	if err := s.waitForDevice(cause); !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the cause", err)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v with a zero window", *slept)
	}
}

// A drop between two books is waited out, and the sync then finishes.
func TestPerformSync_SurvivesADropBetweenBooks(t *testing.T) {
	mc := NewMockClient()
	call := 0
	mc.CheckAbortFunc = func() (bool, error) {
		call++
		if call == 2 || call == 3 { // the device is gone for two checks
			return false, errors.New("i/o timeout")
		}
		return false, nil
	}
	s, slept := fakeSleepSyncer(mc, booksOnDisk(t, 3))

	result, err := s.PerformSync()
	if err != nil {
		t.Fatalf("PerformSync: %v", err)
	}
	if result.BooksAdded != 3 {
		t.Errorf("BooksAdded = %d, want 3", result.BooksAdded)
	}
	if len(*slept) == 0 {
		t.Error("expected the sync to wait for the device")
	}
}

// A device that never comes back fails the sync once the window is spent.
func TestPerformSync_FailsWhenTheDeviceNeverComesBack(t *testing.T) {
	mc := NewMockClient()
	call := 0
	mc.CheckAbortFunc = func() (bool, error) {
		call++
		if call == 1 {
			return false, nil
		}
		return false, errors.New("no route to host")
	}
	s, _ := fakeSleepSyncer(mc, booksOnDisk(t, 3))

	result, err := s.PerformSync()
	if err == nil {
		t.Fatal("expected an error")
	}
	if result.BooksAdded > 1 {
		t.Errorf("BooksAdded = %d, want at most 1", result.BooksAdded)
	}
}

// The link drops in the middle of writing a book, leaving a partial file.
// After the device is back the WHOLE book is redone: the partial file is
// deleted first, so it can never be left in place.
func TestPerformSync_RedoesABookBrokenMidWrite(t *testing.T) {
	mc := NewMockClient()
	mc.WriteFileError = errors.New("write tcp: i/o timeout")
	broken := true
	calls := 0
	mc.CheckAbortFunc = func() (bool, error) {
		calls++
		if calls > 1 && broken {
			return false, errors.New("i/o timeout")
		}
		return false, nil
	}
	s, _ := fakeSleepSyncer(mc, booksOnDisk(t, 1))
	s.sleep = func(time.Duration) {
		// The link comes back; the device kept half a book.
		broken = false
		mc.WriteFileError = nil
		mc.files["book1.cbp"] = []byte("partial")
	}

	result, err := s.PerformSync()
	if err != nil {
		t.Fatalf("PerformSync: %v", err)
	}
	if result.BooksAdded != 1 {
		t.Fatalf("BooksAdded = %d, want 1", result.BooksAdded)
	}
	deleted := false
	for _, f := range mc.DeleteFileCalls {
		if f == "book1.cbp" {
			deleted = true
		}
	}
	if !deleted {
		t.Error("the partial book was not deleted before the rewrite")
	}
	if string(mc.files["book1.cbp"]) == "partial" {
		t.Error("the partial book is still on the device")
	}
}
