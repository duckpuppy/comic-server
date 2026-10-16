package comicvine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Job log levels: what a person watching a scrape needs to see.
const (
	JobLogInfo = "info"
	JobLogWarn = "warn"
)

// JobLogFunc receives one human-readable step of a scrape job
// (comic-server-5wer: live log for a Re-scrape).
type JobLogFunc func(jobID, level, msg string)

type jobLogKey struct{}

type jobLogger struct {
	jobID string
	fn    JobLogFunc
}

// withJobLog attaches a job's log sink to ctx so the client and pacer, which
// know nothing about jobs, can report what they are doing for it.
func withJobLog(ctx context.Context, jobID string, fn JobLogFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, jobLogKey{}, jobLogger{jobID: jobID, fn: fn})
}

// jobLog writes a step to the job log carried by ctx, if any.
func jobLog(ctx context.Context, level, format string, args ...any) {
	l, ok := ctx.Value(jobLogKey{}).(jobLogger)
	if !ok {
		return
	}
	l.fn(l.jobID, level, fmt.Sprintf(format, args...))
}

// untilText is the wait until t for a log line ("47m0s"). A duration, not
// a clock time: the server's time zone is often not the reader's.
func untilText(t time.Time) string {
	return max(time.Until(t), 0).Round(time.Second).String()
}

// fileBase is a path's last element, splitting on both separators since a
// library written on Windows keeps backslash paths.
func fileBase(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
