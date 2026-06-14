package library

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// seriesKey identifies a series by name and volume number, matching ComicRackCE's Key struct.
type seriesKey struct {
	Series string
	Volume int
}

func seriesKeyFor(book *ComicBook) seriesKey {
	return seriesKey{Series: strings.ToLower(book.Series), Volume: book.Volume}
}

// SeriesStats holds aggregate statistics for all books in a series+volume group.
type SeriesStats struct {
	Count                int
	ReadCount            int
	PageCount            int
	PageReadCount        int
	ReadPercentage       int
	AverageRating        float64
	AverageCommunityRating float64
	FirstNumber          float64 // MIN issue number (-1 if no numeric issues)
	LastNumber           float64 // MAX issue number (-1 if no numeric issues)
	MinYear              int
	MaxYear              int
	RunningTimeYears     int
	GapCount             int
	MaxGapSize           float64
	GapStarts            map[float64]bool // issue numbers that are just before a gap
	GapEnds              map[float64]bool // issue numbers that are just after a gap
	AllComplete          string           // "Yes", "No", "Unknown"
	LastOpenedTime       time.Time
	LastAddedTime        time.Time
	LastPublishedTime    time.Time
	LastReleasedTime     time.Time
	MaxCount             int // MAX of books' Count field (series announced count)
	MinCount             int // MIN of books' Count field
}

// buildSeriesStats groups books by (Series, Volume) and computes aggregate stats for each group.
func buildSeriesStats(books []*ComicBook) map[seriesKey]*SeriesStats {
	groups := make(map[seriesKey][]*ComicBook)
	for _, b := range books {
		k := seriesKeyFor(b)
		groups[k] = append(groups[k], b)
	}

	result := make(map[seriesKey]*SeriesStats, len(groups))
	for k, grp := range groups {
		result[k] = computeSeriesStats(grp)
	}
	return result
}

func computeSeriesStats(books []*ComicBook) *SeriesStats {
	s := &SeriesStats{
		GapStarts: make(map[float64]bool),
		GapEnds:   make(map[float64]bool),
		FirstNumber: -1,
		LastNumber:  -1,
	}

	var (
		ratingSum          float64
		ratingCount        int
		communityRatingSum float64
		communityCount     int
		minYear, maxYear   = int(^uint(0)>>1), -int(^uint(0)>>1) - 1 // MaxInt, MinInt
		minCount, maxCount = int(^uint(0)>>1), -int(^uint(0)>>1) - 1
		numbers            []float64
	)

	for _, b := range books {
		s.Count++
		if !b.IsUnread() {
			s.ReadCount++
		}
		s.PageCount += b.PageCount
		s.PageReadCount += b.LastPageRead

		if b.Rating > 0 {
			ratingSum += b.Rating
			ratingCount++
		}
		if b.CommunityRating > 0 {
			communityRatingSum += b.CommunityRating
			communityCount++
		}

		if b.Year > 0 {
			if b.Year < minYear {
				minYear = b.Year
			}
			if b.Year > maxYear {
				maxYear = b.Year
			}
		}

		if b.Count > 0 {
			if b.Count < minCount {
				minCount = b.Count
			}
			if b.Count > maxCount {
				maxCount = b.Count
			}
		}

		if n, ok := parseIssueNumber(b.Number); ok {
			numbers = append(numbers, n)
		}

		if !b.AddedTime.IsZero() && b.AddedTime.After(s.LastAddedTime) {
			s.LastAddedTime = b.AddedTime.Time
		}
		if !b.OpenedTime.IsZero() && b.OpenedTime.After(s.LastOpenedTime) {
			s.LastOpenedTime = b.OpenedTime.Time
		}
		if !b.ReleasedTime.IsZero() && b.ReleasedTime.After(s.LastReleasedTime) {
			s.LastReleasedTime = b.ReleasedTime.Time
		}
		// LastPublishedTime: derived from Year/Month/Day when available
		if b.Year > 0 {
			pub := publishedTime(b)
			if pub.After(s.LastPublishedTime) {
				s.LastPublishedTime = pub
			}
		}

		// AllComplete: if all books agree on SeriesComplete value, use that; otherwise Unknown
		sc := b.SeriesComplete
		if sc == "" {
			sc = "Unknown"
		}
		if s.AllComplete == "" {
			s.AllComplete = sc
		} else if s.AllComplete != sc {
			s.AllComplete = "Unknown"
		}
	}

	if s.Count > 0 {
		s.ReadPercentage = s.ReadCount * 100 / s.Count
	}
	if ratingCount > 0 {
		s.AverageRating = ratingSum / float64(ratingCount)
	}
	if communityCount > 0 {
		s.AverageCommunityRating = communityRatingSum / float64(communityCount)
	}
	if minYear <= maxYear {
		s.MinYear = minYear
		s.MaxYear = maxYear
		s.RunningTimeYears = maxYear - minYear
	}
	if minCount <= maxCount {
		s.MinCount = minCount
		s.MaxCount = maxCount
	}
	if s.AllComplete == "" {
		s.AllComplete = "Unknown"
	}

	if len(numbers) > 0 {
		s.GapCount, s.MaxGapSize, s.GapStarts, s.GapEnds = computeGaps(numbers)
		s.FirstNumber = numbers[0] // numbers is sorted inside computeGaps
		s.LastNumber = numbers[len(numbers)-1]
	}

	return s
}

// computeGaps computes gap statistics from a list of issue numbers.
// numbers slice is sorted in place. Returns gap count, max gap size,
// and sets of numbers that are gap-starts and gap-ends.
func computeGaps(numbers []float64) (gapCount int, maxGapSize float64, gapStarts, gapEnds map[float64]bool) {
	gapStarts = make(map[float64]bool)
	gapEnds = make(map[float64]bool)

	sort.Float64s(numbers)

	// Deduplicate
	deduped := numbers[:0]
	for i, n := range numbers {
		if i == 0 || n != numbers[i-1] {
			deduped = append(deduped, n)
		}
	}

	if len(deduped) < 2 {
		return 0, 0, gapStarts, gapEnds
	}

	prev := deduped[0]
	for _, n := range deduped[1:] {
		gap := n - prev
		if gap > 1.0 {
			gapCount++
			if gap > maxGapSize {
				maxGapSize = gap
			}
			gapStarts[prev] = true
			gapEnds[n] = true
		}
		prev = n
	}
	return gapCount, maxGapSize, gapStarts, gapEnds
}

// parseIssueNumber parses an issue Number string to a float64.
// Returns (number, true) for numeric strings, (0, false) for non-numeric.
func parseIssueNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

// publishedTime returns a time.Time from a book's Year/Month/Day fields.
func publishedTime(b *ComicBook) time.Time {
	month := b.Month
	if month < 1 || month > 12 {
		month = 1
	}
	day := b.Day
	if day < 1 || day > 31 {
		day = 1
	}
	return time.Date(b.Year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
