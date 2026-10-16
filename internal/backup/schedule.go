package backup

import (
	"fmt"
	"time"
)

// Schedule says when backups are due.
type Schedule struct {
	Mode       string // "daily" (at At, local time) or "interval" (every EveryHours)
	At         string // "HH:MM", used by "daily"
	EveryHours int    // used by "interval"
}

// Validate checks a schedule.
func (s Schedule) Validate() error {
	switch s.Mode {
	case "daily":
		if _, _, err := parseClock(s.At); err != nil {
			return err
		}
	case "interval":
		if s.EveryHours < 1 || s.EveryHours > 24*30 {
			return fmt.Errorf("every_hours must be between 1 and %d", 24*30)
		}
	default:
		return fmt.Errorf(`schedule mode must be "daily" or "interval"`)
	}
	return nil
}

func parseClock(at string) (int, int, error) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return 0, 0, fmt.Errorf(`time of day must look like "03:30"`)
	}
	return t.Hour(), t.Minute(), nil
}

// NextRun reports whether a backup is due at now, and when the next one is
// scheduled after now. lastOK is the start of the last successful backup (zero
// if none). A schedule that has never produced a backup is due at once, so
// turning backups on takes the first backup right away.
func (s Schedule) NextRun(lastOK, now time.Time) (due bool, next time.Time) {
	switch s.Mode {
	case "interval":
		step := time.Duration(s.EveryHours) * time.Hour
		if lastOK.IsZero() {
			return true, now.Add(step)
		}
		at := lastOK.Add(step)
		if !at.After(now) {
			return true, now.Add(step)
		}
		return false, at
	default: // daily
		h, m, err := parseClock(s.At)
		if err != nil {
			return false, time.Time{}
		}
		loc := now.Location()
		slot := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
		if slot.After(now) {
			next = slot
			slot = slot.AddDate(0, 0, -1)
		} else {
			next = slot.AddDate(0, 0, 1)
		}
		// slot is the latest scheduled time at or before now.
		return lastOK.IsZero() || lastOK.Before(slot), next
	}
}
