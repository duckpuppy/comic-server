package comicvine

import (
	"errors"
	"fmt"
	"io/fs"
	"sync/atomic"

	rardecode "github.com/nwaples/rardecode/v2"
)

// Archive health (comic-server-4inw). Every read path that opens a comic
// archive (covers, page lists, page reads, convert, ComicInfo reads) funnels
// open failures through openFailure, which tells an optional hook that the
// file looks corrupt. Nothing here opens an archive on its own: the corrupted
// archive flag is recorded lazily from reads that already happen, so it costs
// no extra I/O on the shared disks.

// ArchiveOpenError is a failure to open a comic archive container. Its
// message keeps the older "open cbz: ..." form.
type ArchiveOpenError struct {
	Format string // cbz, cbr or cb7
	Path   string
	Err    error
}

func (e *ArchiveOpenError) Error() string { return fmt.Sprintf("open %s: %v", e.Format, e.Err) }
func (e *ArchiveOpenError) Unwrap() error { return e.Err }

// archiveFailureHook is told about open failures that look like corruption.
var archiveFailureHook atomic.Pointer[func(path string, err error)]

// SetArchiveFailureHook installs fn (nil removes it). It runs on whatever
// goroutine hit the failure, so it must be quick and safe to call
// concurrently.
func SetArchiveFailureHook(fn func(path string, err error)) {
	if fn == nil {
		archiveFailureHook.Store(nil)
		return
	}
	archiveFailureHook.Store(&fn)
}

func openFailure(format, path string, err error) error {
	e := &ArchiveOpenError{Format: format, Path: path, Err: err}
	if IsCorruptOpenError(err) {
		if h := archiveFailureHook.Load(); h != nil {
			(*h)(path, e)
		}
	}
	return e
}

// IsCorruptOpenError reports whether err from opening an archive means the
// file's own content is bad: the container could not be parsed. File-system
// errors (missing, permission, I/O or network-mount trouble) are not, since
// those say nothing about the archive and may pass, and a password-protected
// RAR is not corrupt either.
func IsCorruptOpenError(err error) bool {
	if err == nil {
		return false
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return false
	}
	if errors.Is(err, rardecode.ErrBadPassword) {
		return false
	}
	return true
}
