package komga

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TargetType is which kind of Komga entity a smart list syncs into. It
// mirrors config.KomgaTargetType without importing internal/config, so this
// package stays config-agnostic like comic-server's other feature packages
// (internal/comicvine, internal/sync) - the cmd layer translates.
type TargetType string

const (
	TargetCollection TargetType = "collection" // series-level grouping
	TargetReadList   TargetType = "readlist"   // book-level grouping, can span series
)

// Target maps one comic-server smart list to one Komga collection or read
// list.
type Target struct {
	ListID    string
	KomgaName string
	Type      TargetType
}

// SyncOptions configures a Syncer.
type SyncOptions struct {
	BaseURL    string
	APIKey     string
	LocalRoot  string
	RemoteRoot string
	Targets    []Target

	// Interval is how often to rebuild the Komga path index and re-push
	// every target. comic-server has no way to detect library changes
	// while running (see comic-server-bwz), so this is a scheduled push
	// rather than change-triggered. Defaults to 15 minutes if <= 0.
	Interval time.Duration
}

const defaultSyncInterval = 15 * time.Minute

// Syncer periodically rebuilds the Komga path index and pushes each
// configured target's current smart-list result into Komga.
type Syncer struct {
	client  *Client
	backend library.Backend
	opts    SyncOptions

	targetsMu sync.RWMutex // protects opts.Targets specifically (see SetTargets)

	trigger chan struct{}
}

// NewSyncer creates a Syncer. backend is used to evaluate each target's
// smart list against comic-server's current in-memory library.
func NewSyncer(backend library.Backend, opts SyncOptions) *Syncer {
	if opts.Interval <= 0 {
		opts.Interval = defaultSyncInterval
	}
	return &Syncer{
		client:  NewClient(opts.BaseURL, opts.APIKey),
		backend: backend,
		opts:    opts,
		trigger: make(chan struct{}, 1),
	}
}

// SetTargets replaces the set of targets pushed on each sync pass. Safe to
// call concurrently with Run - e.g. from the web UI's Komga target
// management endpoints (comic-server-d3w), so target changes take effect
// without a config reload or restart. Call TriggerNow afterward to push the
// new set immediately rather than waiting for the next interval.
func (s *Syncer) SetTargets(targets []Target) {
	s.targetsMu.Lock()
	defer s.targetsMu.Unlock()
	s.opts.Targets = targets
}

// Targets returns the current target set.
func (s *Syncer) Targets() []Target {
	s.targetsMu.RLock()
	defer s.targetsMu.RUnlock()
	targets := make([]Target, len(s.opts.Targets))
	copy(targets, s.opts.Targets)
	return targets
}

// TriggerNow requests an immediate sync pass in addition to the regular
// interval (e.g. when the library was just reloaded from an external
// change). Safe to call before Run starts or concurrently with it; requests
// that arrive while one is already pending are coalesced into one pass.
func (s *Syncer) TriggerNow() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// TargetResult reports the outcome of pushing one target - for logging now,
// and later (separate, planned work) for surfacing unmatched books in the
// web UI.
type TargetResult struct {
	Target Target

	// MatchedCount is the number of entries pushed to Komga: distinct
	// series for a Collection target, distinct issues for a Read List
	// target. For a Collection this is normally much smaller than
	// SourceBookCount (many issues per series), which is expected and not
	// itself a sign anything went wrong - see SourceBookCount.
	MatchedCount int

	// SourceBookCount is the number of issues the underlying smart list
	// matched, before series deduplication. Recorded so the UI can show
	// e.g. "42 series (1,739 issues)" for a Collection instead of a bare
	// series count that looks like most of the list went missing.
	SourceBookCount int

	Unmatched []UnmatchedBook
	Err       error
}

// Run performs an immediate sync, then repeats on opts.Interval - or
// whenever TriggerNow is called - until ctx is canceled. onResult is called
// once per target per sync pass (and once with a zero Target if building
// the Komga index itself fails); it may be nil.
func (s *Syncer) Run(ctx context.Context, onResult func(TargetResult)) {
	s.syncOnce(ctx, onResult)

	ticker := time.NewTicker(s.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx, onResult)
		case <-s.trigger:
			s.syncOnce(ctx, onResult)
		}
	}
}

func (s *Syncer) syncOnce(ctx context.Context, onResult func(TargetResult)) {
	targets := s.Targets()
	if len(targets) == 0 {
		return
	}

	idx, err := BuildIndex(ctx, s.client)
	if err != nil {
		// BuildIndex already wraps err with context; pass it through as-is.
		if onResult != nil {
			onResult(TargetResult{Err: err})
		}
		return
	}

	for _, target := range targets {
		result := s.syncTarget(ctx, idx, target)
		if onResult != nil {
			onResult(result)
		}
	}
}

func (s *Syncer) syncTarget(ctx context.Context, idx *Index, target Target) TargetResult {
	list, err := s.backend.FindListByID(target.ListID)
	if err != nil {
		return TargetResult{Target: target, Err: fmt.Errorf("find list %s: %w", target.ListID, err)}
	}

	books, err := s.backend.MatchBooks(list)
	if err != nil {
		return TargetResult{Target: target, Err: fmt.Errorf("evaluate list %s: %w", target.ListID, err)}
	}

	var matched []string
	var unmatched []UnmatchedBook

	switch target.Type {
	case TargetCollection:
		matched, unmatched = idx.ResolveCollectionSeries(books, s.opts.LocalRoot, s.opts.RemoteRoot)
		err = s.client.UpsertCollection(ctx, target.KomgaName, matched)
	case TargetReadList:
		matched, unmatched = idx.ResolveReadListBooks(books, s.opts.LocalRoot, s.opts.RemoteRoot)
		err = s.client.UpsertReadList(ctx, target.KomgaName, matched)
	default:
		err = fmt.Errorf("unknown target type %q", target.Type)
	}

	if err != nil {
		err = fmt.Errorf("push target %q: %w", target.KomgaName, err)
	}

	return TargetResult{Target: target, MatchedCount: len(matched), SourceBookCount: len(books), Unmatched: unmatched, Err: err}
}
