package scaninfo

import "testing"

func TestDefaultListsAreUsable(t *testing.T) {
	scanners, blacklist := DefaultScanners(), DefaultBlacklist()
	if len(scanners) < 50 || len(blacklist) < 20 {
		t.Fatalf("default lists look truncated: %d scanners, %d blacklist", len(scanners), len(blacklist))
	}
	for _, l := range append(append([]string{}, scanners...), blacklist...) {
		if l != "" && l[len(l)-1] == '\r' {
			t.Fatalf("entry %q has a carriage return", l)
		}
	}
	// The real lists must compile into a working detector.
	if _, err := NewDetector(scanners, blacklist, "Scanner:", "Unknown"); err != nil {
		t.Fatalf("NewDetector with defaults: %v", err)
	}
}

func TestDefaultListsAreCopies(t *testing.T) {
	a := DefaultScanners()
	a[0] = "changed"
	if DefaultScanners()[0] == "changed" {
		t.Fatal("DefaultScanners returned shared state")
	}
}
