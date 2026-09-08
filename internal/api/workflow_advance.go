package api

import (
	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/log"
)

// loadWorkflowRulesets loads the currently-configured Data Manager rules
// for use by workflow.AdvanceIfAtOrBefore's just-in-time InferStage
// fallback (comic-server-1iv.2) - needed only for the rare book that
// still has no explicit workflow stage (e.g. one imported after the
// one-time backfill ran). A missing config.db or a load failure degrades
// to no rulesets rather than failing the caller's real work (scan-info/
// cbz-convert/Data Manager/scrape), since stage tracking is a side
// effect of those actions, not their purpose.
func (s *Server) loadWorkflowRulesets() []datamanager.Ruleset {
	if s.configDB == nil {
		return nil
	}
	rulesets, err := datamanager.LoadRulesets(s.configDB)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load Data Manager rules for workflow stage advancement")
		return nil
	}
	return rulesets
}
