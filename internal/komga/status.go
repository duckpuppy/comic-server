package komga

import (
	"sync"
	"time"
)

// UnmatchedBookInfo is the JSON-facing shape of an unmatched book, kept
// separate from UnmatchedBook so the REST/web UI contract doesn't leak
// *library.ComicBook internals.
type UnmatchedBookInfo struct {
	BookID   string `json:"book_id"`
	Series   string `json:"series"`
	Number   string `json:"number"`
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
}

// TargetStatus is a snapshot of one target's most recent sync pass, for the
// web UI (comic-server-1c0).
type TargetStatus struct {
	ListID          string              `json:"list_id"`
	KomgaName       string              `json:"komga_name"`
	Type            TargetType          `json:"type"`
	LastSyncTime    time.Time           `json:"last_sync_time"`
	MatchedCount    int                 `json:"matched_count"`
	SourceBookCount int                 `json:"source_book_count"`
	Unmatched       []UnmatchedBookInfo `json:"unmatched,omitempty"`
	Error           string              `json:"error,omitempty"`

	// SyncReadStatus mirrors Target.SyncReadStatus so the UI can tell
	// "this target never attempted a read-status push" (show nothing)
	// apart from "it attempted one and pushed/failed zero books" (both of
	// which report zero in the fields below).
	SyncReadStatus   bool                `json:"sync_read_status,omitempty"`
	ReadStatusPushed int                 `json:"read_status_pushed,omitempty"`
	ReadStatusFailed []UnmatchedBookInfo `json:"read_status_failed,omitempty"`
}

// Snapshot is the full status payload returned by the REST API: every
// target's current status, plus the last error building the Komga path
// index itself (which blocks every target's push that pass).
type Snapshot struct {
	Targets            []TargetStatus `json:"targets"`
	LastIndexError     string         `json:"last_index_error,omitempty"`
	LastIndexErrorTime *time.Time     `json:"last_index_error_time,omitempty"`
}

// StatusStore holds the most recent TargetResult per target, for display in
// the REST API / web UI. Safe for concurrent use; call Record after every
// sync pass (see Syncer.Run's onResult callback).
type StatusStore struct {
	mu    sync.RWMutex
	byID  map[string]TargetStatus
	order []string // first-seen order, for stable listing

	lastIndexError     string
	lastIndexErrorTime *time.Time
}

// NewStatusStore creates an empty StatusStore.
func NewStatusStore() *StatusStore {
	return &StatusStore{byID: make(map[string]TargetStatus)}
}

// toUnmatchedBookInfos converts UnmatchedBook (internal, carries a
// *library.ComicBook) to the JSON-facing UnmatchedBookInfo shape. Shared by
// Unmatched (membership failures) and ReadStatusFailed (read-status push
// failures) - both are the same shape.
func toUnmatchedBookInfos(unmatched []UnmatchedBook) []UnmatchedBookInfo {
	if len(unmatched) == 0 {
		return nil
	}
	infos := make([]UnmatchedBookInfo, 0, len(unmatched))
	for _, u := range unmatched {
		info := UnmatchedBookInfo{Reason: u.Reason}
		if u.Book != nil {
			info.BookID = u.Book.ID
			info.Series = u.Book.Series
			info.Number = u.Book.Number
			info.FilePath = u.Book.FilePath
		}
		infos = append(infos, info)
	}
	return infos
}

// Record stores the outcome of one target's sync pass. A TargetResult with
// an empty Target.ListID and a non-nil Err represents a failure to build
// the Komga index itself (see Syncer.syncOnce) and is recorded separately,
// not as a per-target entry.
func (s *StatusStore) Record(result TargetResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if result.Target.ListID == "" {
		if result.Err != nil {
			now := time.Now()
			s.lastIndexError = result.Err.Error()
			s.lastIndexErrorTime = &now
		} else {
			s.lastIndexError = ""
			s.lastIndexErrorTime = nil
		}
		return
	}

	status := TargetStatus{
		ListID:           result.Target.ListID,
		KomgaName:        result.Target.KomgaName,
		Type:             result.Target.Type,
		LastSyncTime:     time.Now(),
		MatchedCount:     result.MatchedCount,
		SourceBookCount:  result.SourceBookCount,
		Unmatched:        toUnmatchedBookInfos(result.Unmatched),
		SyncReadStatus:   result.Target.SyncReadStatus,
		ReadStatusPushed: result.ReadStatusPushed,
		ReadStatusFailed: toUnmatchedBookInfos(result.ReadStatusFailed),
	}
	if result.Err != nil {
		status.Error = result.Err.Error()
	}

	if _, exists := s.byID[status.ListID]; !exists {
		s.order = append(s.order, status.ListID)
	}
	s.byID[status.ListID] = status
}

// Snapshot returns the current status of every target, in first-seen
// order, along with the last index-build error (if any).
func (s *StatusStore) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targets := make([]TargetStatus, 0, len(s.order))
	for _, id := range s.order {
		targets = append(targets, s.byID[id])
	}

	return Snapshot{
		Targets:            targets,
		LastIndexError:     s.lastIndexError,
		LastIndexErrorTime: s.lastIndexErrorTime,
	}
}
