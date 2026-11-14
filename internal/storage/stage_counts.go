package storage

import (
	"fmt"

	"github.com/duckpuppy/comic-server/internal/library"
)

// StageCounts counts books per stored workflow stage with a few grouped
// queries, instead of loading all of them (tracker #72). The keys are passed in
// by the caller so this package need not import workflow.
func (db *DB) StageCounts(stageKey, noCVMatchKey, cvdbSkipTag string) (library.StageCounts, error) {
	out := library.StageCounts{Stages: map[string]int{}}

	rows, err := db.Query(`
		SELECT v.value, COUNT(*)
		FROM books b
		JOIN book_custom_values v ON v.book_id = b.id AND v.key = ?
		WHERE b.deleted_at IS NULL AND b.file_path <> ''
		GROUP BY v.value`, stageKey)
	if err != nil {
		return out, fmt.Errorf("count stages: %w", err)
	}
	for rows.Next() {
		var value string
		var n int
		if err := rows.Scan(&value, &n); err != nil {
			rows.Close()
			return out, fmt.Errorf("count stages: %w", err)
		}
		out.Stages[value] = n
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, fmt.Errorf("count stages: %w", err)
	}
	rows.Close()

	if err := db.QueryRow(`SELECT COUNT(*) FROM books WHERE deleted_at IS NULL AND file_path <> ''`).Scan(&out.FilesTotal); err != nil {
		return out, fmt.Errorf("count books with a file: %w", err)
	}
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM books b JOIN book_custom_values v ON v.book_id = b.id AND v.key = ?
		WHERE b.deleted_at IS NULL`, noCVMatchKey).Scan(&out.CVNoMatch); err != nil {
		return out, fmt.Errorf("count not-in-ComicVine books: %w", err)
	}
	if err := db.QueryRow(`
		SELECT COUNT(DISTINCT t.book_id) FROM book_tags t JOIN books b ON b.id = t.book_id
		WHERE b.deleted_at IS NULL AND TRIM(t.tag) = ?`, cvdbSkipTag).Scan(&out.CVDBSkip); err != nil {
		return out, fmt.Errorf("count CVDBSKIP books: %w", err)
	}
	return out, nil
}
