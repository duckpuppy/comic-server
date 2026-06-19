package comicvine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/log"
)

// Syncer runs background synchronization of ComicVine data.
type Syncer struct {
	client *Client
	cache  *Cache
}

// NewSyncer creates a new background syncer.
func NewSyncer(client *Client, cache *Cache) *Syncer {
	return &Syncer{client: client, cache: cache}
}

// SeedFromLibrary extracts comicvine_volume IDs from the library's custom values
// and ensures they exist as pending records in the cache.
func (s *Syncer) SeedFromLibrary(customValuesStores []string) (seeded int, err error) {
	seen := make(map[int]bool)
	for _, store := range customValuesStores {
		cvID := extractCVVolumeID(store)
		if cvID > 0 && !seen[cvID] {
			seen[cvID] = true
		}
	}

	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}

	if err := s.cache.EnsureVolumesExist(ids); err != nil {
		return 0, fmt.Errorf("seed volumes: %w", err)
	}
	return len(ids), nil
}

// BuildOwnedCounts builds a map of cvVolumeID → number of books owned,
// for sync prioritization.
func BuildOwnedCounts(customValuesStores []string) map[int]int {
	counts := make(map[int]int)
	for _, store := range customValuesStores {
		cvID := extractCVVolumeID(store)
		if cvID > 0 {
			counts[cvID]++
		}
	}
	return counts
}

// Run executes the background sync loop. It processes pending volumes one at a time,
// respecting the pacer and circuit breaker. Returns when the context is cancelled
// or all volumes are synced.
func (s *Syncer) Run(ctx context.Context, owned map[int]int) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		pending, err := s.cache.PendingVolumeIDs(owned)
		if err != nil {
			return fmt.Errorf("get pending volumes: %w", err)
		}
		if len(pending) == 0 {
			log.Info().Msg("ComicVine sync complete: all volumes synced")
			return nil
		}

		log.Info().Int("pending", len(pending)).Msg("ComicVine sync: processing pending volumes")

		for _, cvID := range pending {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			err := s.syncVolume(ctx, cvID)
			if err != nil {
				var circuitErr ErrCircuitOpen
				if errors.As(err, &circuitErr) {
					wait := time.Until(circuitErr.Until)
					log.Warn().
						Str("backoff_until", circuitErr.Until.Format(time.RFC3339)).
						Dur("wait", wait).
						Msg("ComicVine sync: circuit breaker open, pausing")

					select {
					case <-time.After(wait):
						continue
					case <-ctx.Done():
						return ctx.Err()
					}
				}

				log.Error().Err(err).Int("cv_volume_id", cvID).Msg("ComicVine sync: volume failed")
				s.markFailed(cvID)
				continue
			}
		}
	}
}

func (s *Syncer) syncVolume(ctx context.Context, cvID int) error {
	existing, err := s.cache.GetVolume(cvID)
	if err != nil {
		return err
	}

	// Fetch volume metadata if not yet synced
	if existing == nil || existing.SyncStatus != SyncStatusSynced {
		vol, err := s.client.FetchVolume(ctx, cvID)
		if err != nil {
			return err
		}

		cached := &CachedVolume{
			CVID:       vol.ID,
			Name:       vol.Name,
			Publisher:  vol.Publisher.Name,
			StartYear:  vol.StartYear,
			IssueCount: vol.CountOfIssues,
			SiteURL:    vol.SiteDetailURL,
			SyncStatus: SyncStatusSynced,
			SyncedAt:   time.Now(),
		}
		if err := s.cache.UpsertVolume(cached); err != nil {
			return fmt.Errorf("cache volume %d: %w", cvID, err)
		}

		log.Debug().
			Int("cv_id", vol.ID).
			Str("name", vol.Name).
			Int("issues", vol.CountOfIssues).
			Msg("ComicVine sync: volume fetched")
	}

	// Fetch issue list if not yet done
	if existing == nil || !existing.IssuesSynced {
		issues, err := s.client.FetchVolumeIssues(ctx, cvID)
		if err != nil {
			return err
		}

		cached := make([]CachedIssue, len(issues))
		for i, iss := range issues {
			cached[i] = CachedIssue{
				CVID:       iss.ID,
				VolumeCVID: cvID,
				Number:     iss.IssueNumber,
				Name:       iss.Name,
				CoverDate:  iss.CoverDate,
				StoreDate:  iss.StoreDate,
			}
		}
		if err := s.cache.UpsertIssues(cached); err != nil {
			return fmt.Errorf("cache issues for volume %d: %w", cvID, err)
		}

		// Mark issues as synced
		v, _ := s.cache.GetVolume(cvID)
		if v != nil {
			v.IssuesSynced = true
			v.SyncedAt = time.Now()
			s.cache.UpsertVolume(v)
		}

		log.Debug().
			Int("cv_volume_id", cvID).
			Int("issues", len(issues)).
			Msg("ComicVine sync: issues fetched")
	}

	return nil
}

func (s *Syncer) markFailed(cvID int) {
	v, err := s.cache.GetVolume(cvID)
	if err != nil || v == nil {
		return
	}
	v.SyncStatus = SyncStatusFailed
	s.cache.UpsertVolume(v)
}

// extractCVVolumeID parses comicvine_volume=NNNN from a CustomValuesStore string.
func extractCVVolumeID(store string) int {
	for _, pair := range strings.Split(store, ",") {
		pair = strings.TrimSpace(pair)
		if !strings.HasPrefix(pair, "comicvine_volume=") {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		return id
	}
	return 0
}
