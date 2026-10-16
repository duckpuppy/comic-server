//go:build windows

package backup

func freeBytes(string) (uint64, bool) { return 0, false }

func sameDevice(string, string) bool { return false }
