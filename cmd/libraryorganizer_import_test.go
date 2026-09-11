package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
)

const sampleLOSettingsForImportTest = `<?xml version="1.0" encoding="utf-8"?>
<Profiles LastUsed="Default">
  <Profile Name="Default" Version="2.1">
    <FolderTemplate>{&lt;publisher&gt;}\{&lt;series&gt;} ({&lt;volume&gt;})</FolderTemplate>
    <BaseFolder>G:\Comics</BaseFolder>
    <FileTemplate>{&lt;series&gt;}{ #&lt;number2&gt;}</FileTemplate>
    <EmptyFolder />
    <UseFolder>true</UseFolder>
    <UseFileName>true</UseFileName>
    <RemoveEmptyFolder>true</RemoveEmptyFolder>
    <MoveFileless>false</MoveFileless>
    <FilelessFormat>.jpg</FilelessFormat>
    <ExcludeMode>Only</ExcludeMode>
    <FailEmptyValues>false</FailEmptyValues>
    <MoveFailed>false</MoveFailed>
    <FailedFolder>G:\Comics\_Failed</FailedFolder>
    <Mode>Move</Mode>
    <CopyMode>false</CopyMode>
    <AutoSpaceFields>true</AutoSpaceFields>
    <ReplaceMultipleSpaces>true</ReplaceMultipleSpaces>
    <ExcludeRules Operator="Any" ExcludeMode="Only" />
  </Profile>
</Profiles>`

// TestRunLibraryOrganizerImport_TranslatesBaseFolderThroughConfiguredMount
// is the regression test for a real user report: BaseFolder is recorded
// in losettingsx.dat exactly as the Windows host that ran ComicRack saw
// it ("G:\Comics") - a path that does not exist on the machine
// comic-server actually runs on (a Docker container, most commonly).
// When server.library_source_root/library_mount_root are configured,
// import must translate BaseFolder (and FailedFolder) to the real mount
// path, the same translation already applied when reading a book's own
// recorded File path.
func TestRunLibraryOrganizerImport_TranslatesBaseFolderThroughConfiguredMount(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`server:
  library_source_root: 'G:\Comics'
  library_mount_root: /mnt/comics
`), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	datPath := filepath.Join(dir, "losettingsx.dat")
	if err := os.WriteFile(datPath, []byte(sampleLOSettingsForImportTest), 0o644); err != nil {
		t.Fatalf("write losettingsx.dat: %v", err)
	}

	origConfigFile, origLoImportPath, origLoImportForce := configFile, loImportPath, loImportForce
	t.Cleanup(func() { configFile, loImportPath, loImportForce = origConfigFile, origLoImportPath, origLoImportForce })
	configFile = configPath
	loImportPath = datPath
	loImportForce = false

	if err := runLibraryOrganizerImport(nil, nil); err != nil {
		t.Fatalf("runLibraryOrganizerImport: %v", err)
	}

	db, err := configdb.Open(filepath.Join(dir, "config.db"))
	if err != nil {
		t.Fatalf("open config.db: %v", err)
	}
	defer db.Close()

	profiles, err := db.ListLOProfiles()
	if err != nil {
		t.Fatalf("ListLOProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	if profiles[0].BaseFolder != "/mnt/comics" {
		t.Errorf("BaseFolder = %q, want %q (translated via library_source_root/library_mount_root)", profiles[0].BaseFolder, "/mnt/comics")
	}
	if profiles[0].FailedFolder != "/mnt/comics/_Failed" {
		t.Errorf("FailedFolder = %q, want %q", profiles[0].FailedFolder, "/mnt/comics/_Failed")
	}
}

// TestRunLibraryOrganizerImport_NoMountConfiguredLeavesBaseFolderAsIs
// covers the common case (no library_source_root/library_mount_root set,
// e.g. comic-server running on the same Windows/host filesystem that
// wrote the library) - BaseFolder must pass through unchanged, not get
// mangled by a translation that was never configured.
func TestRunLibraryOrganizerImport_NoMountConfiguredLeavesBaseFolderAsIs(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server: {}\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	datPath := filepath.Join(dir, "losettingsx.dat")
	if err := os.WriteFile(datPath, []byte(sampleLOSettingsForImportTest), 0o644); err != nil {
		t.Fatalf("write losettingsx.dat: %v", err)
	}

	origConfigFile, origLoImportPath, origLoImportForce := configFile, loImportPath, loImportForce
	t.Cleanup(func() { configFile, loImportPath, loImportForce = origConfigFile, origLoImportPath, origLoImportForce })
	configFile = configPath
	loImportPath = datPath
	loImportForce = false

	if err := runLibraryOrganizerImport(nil, nil); err != nil {
		t.Fatalf("runLibraryOrganizerImport: %v", err)
	}

	db, err := configdb.Open(filepath.Join(dir, "config.db"))
	if err != nil {
		t.Fatalf("open config.db: %v", err)
	}
	defer db.Close()

	profiles, err := db.ListLOProfiles()
	if err != nil {
		t.Fatalf("ListLOProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].BaseFolder != `G:\Comics` {
		t.Errorf(`BaseFolder = %q, want "G:\Comics" unchanged`, profiles[0].BaseFolder)
	}
}
