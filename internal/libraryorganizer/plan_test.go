package libraryorganizer

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func realPlanOptions() PlanOptions {
	return PlanOptions{
		Profile:        realProfile(),
		Exclude:        ExcludeConfig{ExcludeMode: "Only", ExcludeOperator: "Any"}, // zero rules -> always move
		BaseFolder:     `G:\Comics`,
		FolderTemplate: `{<publisher>}\{<imprint>}\{<series>} ({<volume>}{ <format>})`,
		FileTemplate:   `{<series>}{ Vol.<volume>}{ #<number2>}{ (of <count2>)}{ ({<month>, }<year>)}`,
	}
}

func TestPlan_RealDefaultProfile_ComputesFullPath(t *testing.T) {
	book := &library.ComicBook{
		ID: "b1", Series: "Sandman", Publisher: "DC Comics", Imprint: "Vertigo",
		Volume: 1989, Number: "1", Year: 1989, Month: 1,
		FilePath: `G:\Comics\Sandman\Sandman 01.cbz`,
	}
	moves := Plan([]*library.ComicBook{book}, realPlanOptions())
	if len(moves) != 1 {
		t.Fatalf("len(moves) = %d, want 1", len(moves))
	}
	m := moves[0]
	if m.Skipped || m.Failed || m.Collision {
		t.Fatalf("unexpected flags on move: %+v", m)
	}
	wantRaw := `G:\Comics\DC Comics\Vertigo\Sandman (1989)\Sandman Vol.1989 #01 (January, 1989).cbz`
	if m.NewRawPath != wantRaw {
		t.Errorf("NewRawPath = %q, want %q", m.NewRawPath, wantRaw)
	}
	// No ResolvePath given -> resolved path equals raw path.
	if m.NewResolvedPath != wantRaw {
		t.Errorf("NewResolvedPath = %q, want %q (no ResolvePath given)", m.NewResolvedPath, wantRaw)
	}
	if m.OldResolvedPath != book.FilePath {
		t.Errorf("OldResolvedPath = %q, want %q", m.OldResolvedPath, book.FilePath)
	}
}

func TestPlan_ExcludedBookIsSkippedNotDropped(t *testing.T) {
	opts := realPlanOptions()
	opts.Exclude = ExcludeConfig{
		ExcludeMode:     "Only",
		ExcludeOperator: "Any",
		Rules:           []ExcludeRule{{Field: "Tags", Operator: "contains", Value: "Archive"}},
	}
	book := &library.ComicBook{ID: "b1", Series: "Foo", Publisher: "P", Volume: 1, Tags: "SomethingElse"}
	moves := Plan([]*library.ComicBook{book}, opts)
	if len(moves) != 1 {
		t.Fatalf("len(moves) = %d, want 1 (excluded books still appear, just Skipped)", len(moves))
	}
	if !moves[0].Skipped {
		t.Error("expected Skipped=true for a book that fails the exclude rules")
	}
	if moves[0].NewRawPath != "" {
		t.Errorf("expected no destination computed for a skipped book, got %q", moves[0].NewRawPath)
	}
}

func TestPlan_UnresolvableTemplateReportsFailed(t *testing.T) {
	opts := realPlanOptions()
	opts.FolderTemplate = `{<notarealfield>}`
	book := &library.ComicBook{ID: "b1", Series: "Foo"}
	moves := Plan([]*library.ComicBook{book}, opts)
	if !moves[0].Failed {
		t.Error("expected Failed=true for an unsupported template field")
	}
	if moves[0].FailReason == "" {
		t.Error("expected a non-empty FailReason")
	}
}

func TestPlan_CollisionAgainstExistingFile(t *testing.T) {
	opts := realPlanOptions()
	opts.FileExists = func(path string) bool { return true } // the destination is occupied by some other file
	book := &library.ComicBook{
		ID: "b1", Series: "Sandman", Publisher: "DC Comics", Volume: 1989, Number: "1",
		FilePath: `G:\Elsewhere\old.cbz`,
	}
	moves := Plan([]*library.ComicBook{book}, opts)
	if !moves[0].Collision {
		t.Error("expected Collision=true when FileExists reports the destination occupied")
	}
}

func TestPlan_NoOpWhenAlreadyAtDestination(t *testing.T) {
	opts := realPlanOptions()
	opts.FileExists = func(path string) bool { return true } // the book's OWN file, not a real collision
	rawPath := `G:\Comics\DC Comics\Sandman (1989)\Sandman Vol.1989 #01.cbz`
	book := &library.ComicBook{
		ID: "b1", Series: "Sandman", Publisher: "DC Comics", Volume: 1989, Number: "1",
		FilePath: rawPath,
	}
	// Use a template that reproduces exactly rawPath so old==new.
	opts.FolderTemplate = `{<publisher>}\{<series>} ({<volume>})`
	opts.FileTemplate = `{<series>} Vol.{<volume>} #{<number2>}`
	moves := Plan([]*library.ComicBook{book}, opts)
	if moves[0].NewResolvedPath != moves[0].OldResolvedPath {
		t.Fatalf("test setup didn't produce a no-op: new=%q old=%q", moves[0].NewResolvedPath, moves[0].OldResolvedPath)
	}
	if moves[0].Collision {
		t.Error("a book already at its own planned destination must not be flagged as a collision")
	}
}

func TestPlan_WithinRunCollisionFlagsBothBooks(t *testing.T) {
	opts := realPlanOptions()
	opts.FolderTemplate = `{<publisher>}`
	opts.FileTemplate = `{<series>}`
	book1 := &library.ComicBook{ID: "b1", Series: "Same Name", Publisher: "DC", FilePath: `G:\a.cbz`}
	book2 := &library.ComicBook{ID: "b2", Series: "Same Name", Publisher: "DC", FilePath: `G:\b.cbz`}
	moves := Plan([]*library.ComicBook{book1, book2}, opts)
	if !moves[0].Collision || !moves[1].Collision {
		t.Fatalf("expected both books flagged as colliding with each other, got %+v / %+v", moves[0], moves[1])
	}
}

func TestPlan_ResolvePathAppliedToBothOldAndNew(t *testing.T) {
	opts := realPlanOptions()
	opts.ResolvePath = func(p string) string {
		return "/mnt/comics/" + p // stand-in for a real Windows->Linux mount translation
	}
	book := &library.ComicBook{ID: "b1", Series: "Foo", Publisher: "P", FilePath: `G:\Comics\old.cbz`}
	moves := Plan([]*library.ComicBook{book}, opts)
	if moves[0].OldResolvedPath != "/mnt/comics/"+book.FilePath {
		t.Errorf("OldResolvedPath = %q, want resolved", moves[0].OldResolvedPath)
	}
	if moves[0].NewResolvedPath == moves[0].NewRawPath {
		t.Error("expected NewResolvedPath to differ from the raw path once ResolvePath is applied")
	}
}

func TestPlan_FilelessBookUsesFilelessFormat(t *testing.T) {
	opts := realPlanOptions()
	opts.Profile.FilelessFormat = ".jpg"
	book := &library.ComicBook{ID: "b1", Series: "Foo", Publisher: "P", Volume: 1, FilePath: ""}
	moves := Plan([]*library.ComicBook{book}, opts)
	if moves[0].NewFile == "" {
		t.Fatal("expected a computed file name")
	}
	if got := moves[0].NewFile[len(moves[0].NewFile)-4:]; got != ".jpg" {
		t.Errorf("NewFile = %q, want it to end in .jpg (FilelessFormat)", moves[0].NewFile)
	}
}
