package library

// CompressedSeriesName exposes the series-name normalisation used by the
// ComicBookDuplicateMatcher (articles and separators stripped, uppercased)
// so the duplicate-review feature (internal/duplicates, comic-server-as8l)
// groups series exactly the way the smart list matcher does.
func CompressedSeriesName(series string) string {
	return compressedSeriesName(series)
}
