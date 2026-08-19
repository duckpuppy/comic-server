package comicvine

import (
	"strconv"
	"testing"
	"time"
)

func TestNameScore(t *testing.T) {
	tests := []struct {
		book, series string
		want         float64
	}{
		{"Batman", "Batman", 5},
		{"Batman", "Batman Beyond", 4},                       // +5 match, -1 leftover "beyond"
		{"X-Men 2099", "X-Men 2099", 15},                     // hyphen splits into 3 words: x, men, 2099 — all match
		{"Amazing Spider Man", "The Amazing Spider Man", 14}, // 3 matches (+15), -1 leftover "the"
		{"", "Batman", -1},
		{"Batman", "", -1},
	}
	for _, tt := range tests {
		t.Run(tt.book+"/"+tt.series, func(t *testing.T) {
			if got := nameScore(tt.book, tt.series); got != tt.want {
				t.Errorf("nameScore(%q, %q) = %v, want %v", tt.book, tt.series, got, tt.want)
			}
		})
	}
}

func TestPublisherScore(t *testing.T) {
	tests := []struct {
		publisher string
		want      float64
	}{
		{"DC Comics", 0},
		{"Marvel", 0},
		{"Panini Comics", -6},
		{"Marvel Italia", -6},
		{"Marvel UK", -6},
		{"Semic_AS", -6},
		{"Abril", -6},
	}
	for _, tt := range tests {
		if got := publisherScore(tt.publisher); got != tt.want {
			t.Errorf("publisherScore(%q) = %v, want %v", tt.publisher, got, tt.want)
		}
	}
}

func TestBookScore(t *testing.T) {
	tests := []struct {
		issueNum      string
		countOfIssues int
		want          float64
	}{
		{"5", 10, 100},   // 5-1=4 <= 10
		{"15", 10, -100}, // 15-1=14 > 10
		{"5", 150, 100},  // long-running series always good
		{"", 10, 100},    // missing issue number treated as very low, always passes the range check
	}
	for _, tt := range tests {
		got := bookScore(tt.issueNum, tt.countOfIssues)
		if got != tt.want {
			t.Errorf("bookScore(%q, %d) = %v, want %v", tt.issueNum, tt.countOfIssues, got, tt.want)
		}
	}
}

func TestYearScore(t *testing.T) {
	currentYear := 2026
	tests := []struct {
		bookYear, seriesYear int
		want                 float64
	}{
		{2020, 2018, 0},    // series predates book, fine
		{2020, 2020, 0},    // same year, fine
		{0, 2018, 0},       // invalid book year -> no penalty
		{2020, 0, -100},    // book year valid, series year missing
		{2018, 2020, -500}, // series started after book was published
	}
	for _, tt := range tests {
		got := yearScore(tt.bookYear, tt.seriesYear, currentYear)
		if got != tt.want {
			t.Errorf("yearScore(%d, %d) = %v, want %v", tt.bookYear, tt.seriesYear, got, tt.want)
		}
	}
}

func TestRecencyScore(t *testing.T) {
	currentYear := 2026
	if got := recencyScore(2016, currentYear); got != -0.1 {
		t.Errorf("recencyScore(2016) = %v, want -0.1", got)
	}
	if got := recencyScore(0, currentYear); got != -1.0 {
		t.Errorf("recencyScore(0) = %v, want -1.0", got)
	}
}

func TestPriorScore(t *testing.T) {
	if got := priorScore(100, 100); got != 7 {
		t.Errorf("priorScore matching = %v, want 7", got)
	}
	if got := priorScore(100, 200); got != 0 {
		t.Errorf("priorScore mismatched = %v, want 0", got)
	}
	if got := priorScore(100, 0); got != 0 {
		t.Errorf("priorScore no prior = %v, want 0", got)
	}
}

func TestScoreVolumes_KnownSeries(t *testing.T) {
	currentYear := time.Now().Year()
	parsed := ParsedFilename{Series: "Batman", IssueNumber: "1", Year: 2016}

	candidates := []Volume{
		{ID: 1, Name: "Batman", StartYear: "2016", Publisher: Publisher{Name: "DC Comics"}, CountOfIssues: 100},
		{ID: 2, Name: "Batman Beyond", StartYear: "1999", Publisher: Publisher{Name: "DC Comics"}, CountOfIssues: 47},
		{ID: 3, Name: "Batman", StartYear: strconv.Itoa(currentYear + 5), Publisher: Publisher{Name: "Panini Comics"}, CountOfIssues: 12},
	}

	results := ScoreVolumes(parsed, candidates, 0)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Volume.ID != 1 {
		t.Errorf("best match = volume %d, want 1", results[0].Volume.ID)
	}
}

func TestAssignConfidence(t *testing.T) {
	tests := []struct {
		name    string
		scores  []float64
		wantTop string
	}{
		{"single candidate", []float64{50}, ConfidenceHigh},
		{"clear winner", []float64{100, 80}, ConfidenceHigh},
		{"close scores", []float64{100, 98}, ConfidenceLow},
		{"medium separation", []float64{100, 95}, ConfidenceMedium},
		{"no good matches", []float64{-50, -80}, ConfidenceLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]MatchResult, len(tt.scores))
			for i, s := range tt.scores {
				results[i] = MatchResult{Score: s}
			}
			assignConfidence(results)
			if results[0].Confidence != tt.wantTop {
				t.Errorf("top confidence = %v, want %v (scores %v)", results[0].Confidence, tt.wantTop, tt.scores)
			}
		})
	}
}

func TestScoreVolumes_XMen2099(t *testing.T) {
	parsed := ParsedFilename{Series: "X-Men 2099", IssueNumber: "3", Year: 1993}
	candidates := []Volume{
		{ID: 1, Name: "X-Men 2099", StartYear: "1993", Publisher: Publisher{Name: "Marvel"}, CountOfIssues: 35},
		{ID: 2, Name: "X-Men", StartYear: "1991", Publisher: Publisher{Name: "Marvel"}, CountOfIssues: 200},
	}
	results := ScoreVolumes(parsed, candidates, 0)
	if results[0].Volume.ID != 1 {
		t.Errorf("best match = volume %d, want 1 (X-Men 2099)", results[0].Volume.ID)
	}
}

func TestScoreVolumes_PriorSelectionBonus(t *testing.T) {
	parsed := ParsedFilename{Series: "Batman", IssueNumber: "1", Year: 2016}
	candidates := []Volume{
		{ID: 1, Name: "Batman", StartYear: "2016", CountOfIssues: 100},
		{ID: 2, Name: "Batman", StartYear: "2016", CountOfIssues: 100},
	}
	// Without a prior, tied volumes should be ambiguous (low confidence).
	results := ScoreVolumes(parsed, candidates, 0)
	if results[0].Confidence != ConfidenceLow {
		t.Errorf("tied confidence = %v, want low", results[0].Confidence)
	}

	// With volume 2 as the prior selection, it should win and gain confidence.
	results = ScoreVolumes(parsed, candidates, 2)
	if results[0].Volume.ID != 2 {
		t.Errorf("best match = volume %d, want 2 (prior)", results[0].Volume.ID)
	}
}

func TestScoreVolumes_NoCandidates(t *testing.T) {
	results := ScoreVolumes(ParsedFilename{Series: "Batman"}, nil, 0)
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestScoreVolumes_SingleCandidate(t *testing.T) {
	parsed := ParsedFilename{Series: "Obscure Comic", IssueNumber: "1", Year: 2020}
	candidates := []Volume{{ID: 1, Name: "Obscure Comic", StartYear: "2020", CountOfIssues: 5}}
	results := ScoreVolumes(parsed, candidates, 0)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Confidence != ConfidenceHigh {
		t.Errorf("confidence = %v, want high", results[0].Confidence)
	}
}

func TestSelectIssue_ExactMatch(t *testing.T) {
	issues := []Issue{{IssueNumber: "1"}, {IssueNumber: "2"}, {IssueNumber: "3"}}
	got := SelectIssue("2", issues)
	if got == nil || got.IssueNumber != "2" {
		t.Errorf("got %+v, want issue 2", got)
	}
}

func TestSelectIssue_LeadingZeros(t *testing.T) {
	issues := []Issue{{IssueNumber: "001"}, {IssueNumber: "002"}}
	got := SelectIssue("2", issues)
	if got == nil || got.IssueNumber != "002" {
		t.Errorf("got %+v, want issue 002", got)
	}
}

func TestSelectIssue_FractionalIssues(t *testing.T) {
	issues := []Issue{{IssueNumber: "1"}, {IssueNumber: "½"}, {IssueNumber: "2"}}
	for _, query := range []string{"½", "0.5", "1/2"} {
		got := SelectIssue(query, issues)
		if got == nil || got.IssueNumber != "½" {
			t.Errorf("SelectIssue(%q) = %+v, want ½ issue", query, got)
		}
	}
}

func TestSelectIssue_NoMatch(t *testing.T) {
	issues := []Issue{{IssueNumber: "1"}, {IssueNumber: "2"}}
	if got := SelectIssue("99", issues); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestSelectIssue_EmptyInputs(t *testing.T) {
	if got := SelectIssue("", []Issue{{IssueNumber: "1"}}); got != nil {
		t.Errorf("got %+v, want nil for empty issue number", got)
	}
	if got := SelectIssue("1", nil); got != nil {
		t.Errorf("got %+v, want nil for no issues", got)
	}
}

func TestNormalizeIssueNumber(t *testing.T) {
	tests := map[string]string{
		"1":    "1",
		"01":   "1",
		"001":  "1",
		"1.0":  "1",
		"1.5":  "1.5",
		"½":    "0.5",
		"1/2":  "0.5",
		"annu": "annu",
	}
	for in, want := range tests {
		if got := normalizeIssueNumber(in); got != want {
			t.Errorf("normalizeIssueNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyCoverVerification_BoostsMatchingCover(t *testing.T) {
	results := []MatchResult{
		{Volume: Volume{ID: 1, Name: "Batman"}, Score: 100},
		{Volume: Volume{ID: 2, Name: "Batman Beyond"}, Score: 98},
	}
	localHash := CoverHash(0x0F0F0F0F0F0F0F0F)
	coverHashes := map[int]CoverHash{
		1: 0x0F0F0F0F0F0F0F0F, // identical to local -> boosted
		2: 0xFFFFFFFFFFFFFFFF, // maximally different -> penalized
	}

	out := ApplyCoverVerification(results, localHash, coverHashes)
	if out[0].Volume.ID != 1 || out[0].Score <= 100 {
		t.Errorf("volume 1 should be boosted and remain first: %+v", out[0])
	}
	if out[1].Volume.ID != 2 || out[1].Score >= 98 {
		t.Errorf("volume 2 should be penalized: %+v", out[1])
	}
}

func TestApplyCoverVerification_UnknownHashLeftUnchanged(t *testing.T) {
	results := []MatchResult{{Volume: Volume{ID: 1}, Score: 50}}
	out := ApplyCoverVerification(results, 0x0, map[int]CoverHash{})
	if out[0].Score != 50 {
		t.Errorf("score = %v, want unchanged at 50", out[0].Score)
	}
}

func TestApplyCoverVerification_CanFlipRanking(t *testing.T) {
	results := []MatchResult{
		{Volume: Volume{ID: 1, Name: "Wrong Match"}, Score: 100},
		{Volume: Volume{ID: 2, Name: "Right Match"}, Score: 90},
	}
	localHash := CoverHash(0xAAAAAAAAAAAAAAAA)
	coverHashes := map[int]CoverHash{
		1: 0x5555555555555555, // maximally different from local
		2: 0xAAAAAAAAAAAAAAAA, // identical to local
	}

	out := ApplyCoverVerification(results, localHash, coverHashes)
	if out[0].Volume.ID != 2 {
		t.Errorf("expected cover match to overtake a weaker text score, got top=%+v", out[0])
	}
}

func TestAmbiguousByCover(t *testing.T) {
	tests := []struct {
		name    string
		results []MatchResult
		hashes  map[int]CoverHash
		want    bool
	}{
		{
			name: "similar covers are ambiguous",
			results: []MatchResult{
				{Volume: Volume{ID: 1}, Score: 100},
				{Volume: Volume{ID: 2}, Score: 90},
			},
			hashes: map[int]CoverHash{1: 0x0, 2: 0x0},
			want:   true,
		},
		{
			name: "different covers are not ambiguous",
			results: []MatchResult{
				{Volume: Volume{ID: 1}, Score: 100},
				{Volume: Volume{ID: 2}, Score: 90},
			},
			hashes: map[int]CoverHash{1: 0x0, 2: 0xFFFFFFFFFFFFFFFF},
			want:   false,
		},
		{
			name:    "fewer than two candidates",
			results: []MatchResult{{Volume: Volume{ID: 1}, Score: 100}},
			hashes:  map[int]CoverHash{1: 0x0},
			want:    false,
		},
		{
			name: "missing hash for a candidate",
			results: []MatchResult{
				{Volume: Volume{ID: 1}, Score: 100},
				{Volume: Volume{ID: 2}, Score: 90},
			},
			hashes: map[int]CoverHash{1: 0x0},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AmbiguousByCover(tt.results, tt.hashes); got != tt.want {
				t.Errorf("AmbiguousByCover() = %v, want %v", got, tt.want)
			}
		})
	}
}
