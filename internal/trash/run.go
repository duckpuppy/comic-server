package trash

import (
	"context"
	"time"
)

// DefaultSweepInterval is how often Run sweeps when the caller doesn't
// specify one. Quarantine entries only ever need sweeping once they've
// aged past RetentionDays (days, not minutes), so this doesn't need to be
// frequent - matching the coarser end of the codebase's other background
// intervals (see internal/komga.defaultSyncInterval).
const DefaultSweepInterval = 1 * time.Hour

// Run sweeps immediately, then repeats every interval (DefaultSweepInterval
// if interval <= 0) until ctx is canceled. onResult, if non-nil, is called
// after every pass - including the initial one - so the caller can log
// outcomes the same way internal/komga.Syncer.Run's onResult does.
func (t *Trash) Run(ctx context.Context, interval time.Duration, onResult func(SweepResult)) {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}

	sweep := func() {
		result := t.Sweep(time.Now())
		if onResult != nil {
			onResult(result)
		}
	}

	sweep()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
