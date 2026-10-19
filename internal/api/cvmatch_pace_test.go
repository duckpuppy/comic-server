package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// A pacing pause too long for a web request is a readable 429 with the time to
// wait, not a hung request that the proxy cuts off with a bare 504.
func TestWriteCVError_PaceWait(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.writeCVError(w, fmt.Errorf("fetch issue 1173398: %w", comicvine.ErrPaceWait{After: 250 * time.Second, Reason: "burst"}))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "251" {
		t.Errorf("Retry-After = %q, want 251", got)
	}
	if body := w.Body.String(); !strings.Contains(body, "4m10s") || !strings.Contains(body, "burst") {
		t.Errorf("body = %q", body)
	}
}
