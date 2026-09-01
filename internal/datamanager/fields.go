// Package datamanager ports ComicRack's "Data Manager" plugin rule engine
// (CRDataManager - dmClasses.py/dmMod.py/dmGlobals.py) into comic-server.
// See comic-server-764 for the full design record: file format, reuse
// decisions against internal/library's smart-list matcher engine, and the
// user's confirmed scope decisions.
package datamanager

import "github.com/duckpuppy/comic-server/internal/library"

// FieldKind mirrors dataman.ini's field-type groupings (stringKeys,
// numericalKeys, pseudoNumericalKeys, dateTimeKeys, boolKeys, yesNoKeys,
// mangaYesNoKeys, multiValueKeys, languageISOKeys), which determine which
// rule/action modifiers are valid for a field and how MatchValue gets
// interpreted.
type FieldKind int

const (
	KindString FieldKind = iota
	KindNumeric
	// KindPseudoNumeric covers Number/AlternateNumber - ComicRack issue
	// numbers like "1A" or "10.5" that need prefix/digit-run/suffix
	// comparison, not plain float parsing (see comic-server-764's design
	// notes on Greater/Less semantics for these two fields).
	KindPseudoNumeric
	KindDate
	KindBool
	KindYesNo
	KindManga
	KindLanguage
	// KindMultiValue fields (Characters, Genre, Tags, Teams, Writer, etc)
	// store a comma-separated list on the book and support Add/Remove
	// actions that operate on individual list members, not the whole
	// string.
	KindMultiValue
)

// FieldDef describes one Data Manager field: how to read it (via the
// shared smart-list matcher engine's MatcherType, or a direct bool getter
// for the handful of fields with no MatcherType equivalent), whether an
// action may write to it (dataman.ini's ReadOnlyKeys - FileDirectory,
// FileFormat, FileIsMissing, FileName, FilePath, HasBeenOpened,
// HasBeenRead can only ever be rule conditions, never action targets),
// and its FieldKind.
type FieldDef struct {
	// MatcherType is the internal/library MatcherType this field reads
	// through, when one exists. Empty for the handful of fields with a
	// direct BoolGetter instead.
	MatcherType library.MatcherType
	Kind        FieldKind
	Writable    bool
	// BoolGetter reads the field directly off a book, for the handful of
	// fields with no MatcherType equivalent in the shared matcher engine
	// (ComicInfoIsDirty, EnableProposed - real ComicBook fields; the
	// derived HasBeenOpened/HasBeenRead; ComicBookIsDirty - no
	// comic-server equivalent exists at all, since it describes
	// ComicRack's own internal dirty-tracking rather than book data,
	// so its getter always returns false).
	BoolGetter func(*library.ComicBook) bool
}

// builtinFields maps every field name dataman.ini's allowedKeys/allowedVals
// grammar recognizes to its comic-server equivalent. A field name NOT in
// this table is a custom value (Concept, "Concept Status", Event,
// Chronology, "Data Manager processed", TempAnnual, etc in the user's
// real dataman.dat) - detected purely by exclusion, matching the real
// plugin's own behavior (ground-truthed against the user's actual file;
// there is no Custom(...) wrapper syntax in the real on-disk format - see
// comic-server-764's design notes).
var builtinFields = map[string]FieldDef{
	"AddedTime":           {MatcherType: library.MatcherTypeAddedTime, Kind: KindDate, Writable: true},
	"AgeRating":           {MatcherType: library.MatcherTypeAgeRating, Kind: KindString, Writable: true},
	"AlternateCount":      {MatcherType: library.MatcherTypeAlternateCount, Kind: KindNumeric, Writable: true},
	"AlternateSeries":     {MatcherType: library.MatcherTypeAlternateSeries, Kind: KindString, Writable: true},
	"AlternateNumber":     {MatcherType: library.MatcherTypeAlternateNumber, Kind: KindPseudoNumeric, Writable: true},
	"BlackAndWhite":       {MatcherType: library.MatcherTypeBlackAndWhite, Kind: KindYesNo, Writable: true},
	"BookNotes":           {MatcherType: library.MatcherTypeBookNotes, Kind: KindString, Writable: true},
	"BookOwner":           {MatcherType: library.MatcherTypeBookOwner, Kind: KindString, Writable: true},
	"BookPrice":           {MatcherType: library.MatcherTypeBookPrice, Kind: KindNumeric, Writable: true},
	"BookStore":           {MatcherType: library.MatcherTypeBookStore, Kind: KindString, Writable: true},
	"BookAge":             {MatcherType: library.MatcherTypeBookAge, Kind: KindString, Writable: true},
	"BookCondition":       {MatcherType: library.MatcherTypeBookCondition, Kind: KindString, Writable: true},
	"BookLocation":        {MatcherType: library.MatcherTypeBookLocation, Kind: KindString, Writable: true},
	"Characters":          {MatcherType: library.MatcherTypeCharacters, Kind: KindMultiValue, Writable: true},
	"Checked":             {MatcherType: library.MatcherTypeChecked, Kind: KindBool, Writable: true},
	"Colorist":            {MatcherType: library.MatcherTypeColorist, Kind: KindMultiValue, Writable: true},
	"CommunityRating":     {MatcherType: library.MatcherTypeCommunityRating, Kind: KindNumeric, Writable: true},
	"Count":               {MatcherType: library.MatcherTypeCount, Kind: KindNumeric, Writable: true},
	"CoverArtist":         {MatcherType: library.MatcherTypeCoverArtist, Kind: KindMultiValue, Writable: true},
	"Day":                 {MatcherType: library.MatcherTypeDay, Kind: KindNumeric, Writable: true},
	"Editor":              {MatcherType: library.MatcherTypeEditor, Kind: KindMultiValue, Writable: true},
	"FileDirectory":       {MatcherType: library.MatcherTypeDirectory, Kind: KindString, Writable: false},
	"FileFormat":          {MatcherType: library.MatcherTypeFileFormat, Kind: KindString, Writable: false},
	"FileIsMissing":       {MatcherType: library.MatcherTypeIsMissing, Kind: KindBool, Writable: false},
	"FileName":            {MatcherType: library.MatcherTypeFile, Kind: KindString, Writable: false},
	"FilePath":            {MatcherType: library.MatcherTypeFullPath, Kind: KindString, Writable: false},
	"Format":              {MatcherType: library.MatcherTypeFormat, Kind: KindString, Writable: true},
	"Genre":               {MatcherType: library.MatcherTypeGenre, Kind: KindMultiValue, Writable: true},
	"Imprint":             {MatcherType: library.MatcherTypeImprint, Kind: KindString, Writable: true},
	"Inker":               {MatcherType: library.MatcherTypeInker, Kind: KindMultiValue, Writable: true},
	"ISBN":                {MatcherType: library.MatcherTypeISBN, Kind: KindString, Writable: true},
	"LanguageISO":         {MatcherType: library.MatcherTypeLanguage, Kind: KindLanguage, Writable: true},
	"Letterer":            {MatcherType: library.MatcherTypeLetterer, Kind: KindMultiValue, Writable: true},
	"Locations":           {MatcherType: library.MatcherTypeLocations, Kind: KindMultiValue, Writable: true},
	"MainCharacterOrTeam": {MatcherType: library.MatcherTypeMainCharacterOrTeam, Kind: KindString, Writable: true},
	"Manga":               {MatcherType: library.MatcherTypeManga, Kind: KindManga, Writable: true},
	"Month":               {MatcherType: library.MatcherTypeMonth, Kind: KindNumeric, Writable: true},
	"Notes":               {MatcherType: library.MatcherTypeNotes, Kind: KindString, Writable: true},
	"Number":              {MatcherType: library.MatcherTypeNumber, Kind: KindPseudoNumeric, Writable: true},
	"PageCount":           {MatcherType: library.MatcherTypePageCount, Kind: KindNumeric, Writable: true},
	"Penciller":           {MatcherType: library.MatcherTypePenciller, Kind: KindMultiValue, Writable: true},
	"Publisher":           {MatcherType: library.MatcherTypePublisher, Kind: KindString, Writable: true},
	"Published":           {MatcherType: library.MatcherTypePublished, Kind: KindDate, Writable: true},
	"Rating":              {MatcherType: library.MatcherTypeRating, Kind: KindNumeric, Writable: true},
	"ReleasedTime":        {MatcherType: library.MatcherTypeReleased, Kind: KindDate, Writable: true},
	"Review":              {MatcherType: library.MatcherTypeReview, Kind: KindString, Writable: true},
	"ScanInformation":     {MatcherType: library.MatcherTypeScanInfo, Kind: KindString, Writable: true},
	"Series":              {MatcherType: library.MatcherTypeSeries, Kind: KindString, Writable: true},
	"SeriesComplete":      {MatcherType: library.MatcherTypeSeriesComplete, Kind: KindYesNo, Writable: true},
	"SeriesGroup":         {MatcherType: library.MatcherTypeSeriesGroup, Kind: KindString, Writable: true},
	"StoryArc":            {MatcherType: library.MatcherTypeStoryArc, Kind: KindString, Writable: true},
	"Summary":             {MatcherType: library.MatcherTypeSummary, Kind: KindString, Writable: true},
	"Tags":                {MatcherType: library.MatcherTypeTags, Kind: KindMultiValue, Writable: true},
	"Teams":               {MatcherType: library.MatcherTypeTeams, Kind: KindMultiValue, Writable: true},
	"Title":               {MatcherType: library.MatcherTypeTitle, Kind: KindString, Writable: true},
	"Translator":          {MatcherType: library.MatcherTypeTranslator, Kind: KindMultiValue, Writable: true},
	"Volume":              {MatcherType: library.MatcherTypeVolume, Kind: KindNumeric, Writable: true},
	"Web":                 {MatcherType: library.MatcherTypeWeb, Kind: KindString, Writable: true},
	"Writer":              {MatcherType: library.MatcherTypeWriter, Kind: KindMultiValue, Writable: true},
	"Year":                {MatcherType: library.MatcherTypeYear, Kind: KindNumeric, Writable: true},

	// Fields with no MatcherType equivalent - read directly off the book.
	"ComicInfoIsDirty": {Kind: KindBool, Writable: true, BoolGetter: func(b *library.ComicBook) bool { return b.ComicInfoIsDirty }},
	"EnableProposed":   {Kind: KindBool, Writable: true, BoolGetter: func(b *library.ComicBook) bool { return b.EnableProposed }},
	"HasBeenOpened":    {Kind: KindBool, Writable: false, BoolGetter: func(b *library.ComicBook) bool { return b.OpenCount > 0 }},
	"HasBeenRead":      {Kind: KindBool, Writable: false, BoolGetter: func(b *library.ComicBook) bool { return b.OpenCount > 0 && !b.IsUnread() }},
	// ComicBookIsDirty describes ComicRack's own internal dirty-tracking
	// state, not book data - comic-server has no equivalent concept, so
	// this always reads as false rather than being left unsupported.
	"ComicBookIsDirty": {Kind: KindBool, Writable: true, BoolGetter: func(*library.ComicBook) bool { return false }},
}

// IsCustomField reports whether name is a custom value (e.g. Concept,
// "Concept Status") rather than a built-in ComicRack field - detected by
// exclusion, matching the real plugin's own behavior.
func IsCustomField(name string) bool {
	_, ok := builtinFields[name]
	return !ok
}
