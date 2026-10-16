package api

import (
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// migrateBatch is how many books one UpdateBooks call carries.
const migrateBatch = 200

// MigrateCVDBSkip copies the old ComicRack CVDBSKIP tag into the
// comic_server_cv_nomatch marker (comic-server-5wer). A book that already had
// a stored Scrape stage kept it, so the stage fix in InferStage alone never
// moved it. The tag is left on the book (ComicRack still reads it); only the
// marker is added and a stored Scrape stage is advanced. Idempotent: a book
// with the marker, with a ComicVine tag, or without a file is skipped, so it
// is safe to run at every start and after every import (ComicRack's XML still
// carries the tag). Returns how many books changed.
func (s *Server) MigrateCVDBSkip() int {
	if s.backend == nil {
		return 0
	}
	all, err := s.backend.GetAllBooks()
	if err != nil {
		log.Warn().Err(err).Msg("CVDBSKIP migration: could not list books")
		return 0
	}
	var ids []string
	for i := range all {
		b := &all[i]
		if b.FilePath == "" || !workflow.IsCVDBSkip(b) || workflow.IsNoCVMatch(b) {
			continue
		}
		if _, tagged := library.GetCustomValue(b.CustomValuesStore, "comicvine_volume"); tagged {
			continue
		}
		ids = append(ids, b.ID)
	}
	if len(ids) == 0 {
		return 0
	}

	backend := s.audited(audit.ActionCVNoMatch)
	today := time.Now().Format("2006-01-02")
	changed := 0
	var batch []*library.ComicBook
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := backend.UpdateBooks(batch); err != nil {
			log.Error().Err(err).Int("books", len(batch)).Msg("CVDBSKIP migration: save failed")
		} else {
			changed += len(batch)
		}
		batch = batch[:0]
	}
	for _, id := range ids {
		book, err := backend.GetBook(id) // a copy, so the audit diff sees the change
		if err != nil || book == nil {
			continue
		}
		if workflow.SetNoCVMatch(book, workflow.NoCVMatchLegacy, today) {
			batch = append(batch, book)
		}
		if len(batch) >= migrateBatch {
			flush()
		}
	}
	flush()
	if changed > 0 {
		s.InvalidateWorkflowCache()
		log.Info().Int("books", changed).Msg("CVDBSKIP migration: copied the old tag into the not-in-ComicVine marker")
	}
	return changed
}
