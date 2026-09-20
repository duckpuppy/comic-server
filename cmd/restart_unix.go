//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/duckpuppy/comic-server/internal/log"
)

// restartSupported reports whether this platform can re-exec the running
// binary in place. True on Unix (comic-server-9klu).
const restartSupported = true

// execSelf replaces the current process image with a fresh copy of the same
// binary, same arguments and environment. syscall.Exec keeps the PID, which
// is what lets this work as PID 1 in a container (a child-spawn-and-exit
// restart would take the container down with it). Go marks its file
// descriptors close-on-exec, so listener sockets are released at the exec
// and the new process can rebind the same ports. It only returns on failure.
func execSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	// On Linux, os.Executable reads /proc/self/exe, which the kernel suffixes
	// with " (deleted)" if the on-disk binary was unlinked/replaced since
	// this process started (e.g. an in-place upgrade). Trimming it re-execs
	// whatever now sits at that path, which is the upgraded binary.
	if _, statErr := os.Stat(exe); statErr != nil {
		if trimmed := strings.TrimSuffix(exe, " (deleted)"); trimmed != exe {
			exe = trimmed
		}
	}
	log.Info().Str("exe", exe).Msg("Server stopped cleanly, re-executing in place")
	return syscall.Exec(exe, os.Args, os.Environ())
}
