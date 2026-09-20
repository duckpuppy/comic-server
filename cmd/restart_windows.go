//go:build windows

package cmd

import "errors"

// restartSupported is false on Windows: there is no syscall.Exec, and
// emulating an in-place restart by spawning a child and exiting would
// change the PID and detach from any service manager. The API endpoint
// answers 501 and the UI keeps its manual-restart message (comic-server-9klu).
const restartSupported = false

func execSelf() error {
	return errors.New("restart is not supported on Windows")
}
