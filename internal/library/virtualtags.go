package library

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Virtual tags: user-defined, template-computed string properties (ComicRackCE
// VirtualTag01..20). A tag is a slot id plus a caption format; its value for a
// book is the format expanded against that book (see captionformat.go), and
// smart lists match on it with the ordinary string operators.
//
// Definitions are comic-server configuration (server.virtual_tags), not
// library data - ComicRackCE keeps them in desktop app settings the same way.

// MaxVirtualTags is the number of virtual tag slots (VirtualTag01..20).
const MaxVirtualTags = 20

// VirtualTag is one slot's definition.
type VirtualTag struct {
	ID            int
	Name          string
	Description   string
	CaptionFormat string
	Enabled       bool
}

var virtualTags = struct {
	sync.RWMutex
	byID map[int]VirtualTag
}{byID: map[int]VirtualTag{}}

// SetVirtualTags replaces the active tag definitions. Like ComicRackCE
// (VirtualTagsCollection.Init), only tags that are enabled and have both a
// name and a caption format are active; every other slot evaluates to "".
func SetVirtualTags(tags []VirtualTag) {
	active := make(map[int]VirtualTag, len(tags))
	for _, t := range tags {
		if !t.Enabled || t.Name == "" || t.CaptionFormat == "" {
			continue
		}
		if t.ID < 1 || t.ID > MaxVirtualTags {
			continue
		}
		if _, dup := active[t.ID]; !dup {
			active[t.ID] = t
		}
	}
	virtualTags.Lock()
	virtualTags.byID = active
	virtualTags.Unlock()
}

// VirtualTagValue expands virtual tag slot id against a book. Inactive or
// undefined slots return "".
func VirtualTagValue(book *ComicBook, id int) string {
	virtualTags.RLock()
	tag, ok := virtualTags.byID[id]
	virtualTags.RUnlock()
	if !ok {
		return ""
	}
	return ExpandCaptionFormat(tag.CaptionFormat, book)
}

// virtualTagSlot extracts the slot number from a matcher type such as
// "VirtualTag7" (from ComicBookVirtualTag7Matcher).
func virtualTagSlot(t MatcherType) (int, bool) {
	s, ok := strings.CutPrefix(string(t), "VirtualTag")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > MaxVirtualTags {
		return 0, false
	}
	return n, true
}

// ExpandCaptionFormat expands a ComicRack caption format against a book
// (ComicBook.GetFullTitle).
func ExpandCaptionFormat(format string, book *ComicBook) string {
	if format == "" {
		return ""
	}
	return formatCaption(format, func(name string) (string, bool) {
		return captionFieldValue(book, name)
	})
}

// captionFieldValue resolves a {Field} token to text. It follows
// ComicBook.GetFullTitle: a field that has a FooAsText form uses it (so
// {Number} is "5 (of 12)" and {Volume} is "V2", as in ComicRack), names are
// case-insensitive, and an unknown field is ComicRack's null (ok=false).
//
// Differences from ComicRack: there are no "proposed" (filename-derived)
// values here, so Shadow* fields are just the stored values, and the library
// loader leaves unset numbers at 0 where ComicRack uses -1, so 0 is treated
// as unset for Year/Month/Day/Count/AlternateCount/Volume.
func captionFieldValue(book *ComicBook, name string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.TrimPrefix(key, "shadow")

	if v, ok := captionAsText(book, key); ok {
		return v, true
	}

	switch key {
	case "numberonly":
		return book.Number, true
	case "filepath":
		return book.FilePath, true
	case "filename", "filenamewithextension", "filedirectory":
		p := strings.ReplaceAll(book.FilePath, `\`, "/")
		base := p
		dir := ""
		if i := strings.LastIndex(p, "/"); i >= 0 {
			base, dir = p[i+1:], p[:i]
		}
		switch key {
		case "filedirectory":
			return filepath.FromSlash(dir), true
		case "filename":
			return strings.TrimSuffix(base, filepath.Ext(base)), true
		default:
			return base, true
		}
	}

	return captionRawField(book, key)
}

func captionAsText(book *ComicBook, key string) (string, bool) {
	positive := func(n int) string {
		if n > 0 {
			return strconv.Itoa(n)
		}
		return ""
	}
	yesNo := func(s string) string {
		switch strings.ToLower(s) {
		case "yes":
			return "Yes"
		case "no":
			return "No"
		}
		return ""
	}

	switch key {
	case "yearastext", "year":
		return positive(book.Year), true
	case "monthastext", "month":
		return positive(book.Month), true
	case "dayastext", "day":
		return positive(book.Day), true
	case "countastext", "count":
		return positive(book.Count), true
	case "alternatecountastext", "alternatecount":
		return positive(book.AlternateCount), true
	case "volumeastext", "volume":
		if book.Volume > 0 {
			return "V" + strconv.Itoa(book.Volume), true
		}
		return "", true
	case "numberastext", "number":
		return formatCaptionNumber(book.Number, book.Count), true
	case "alternatenumberastext", "alternatenumber":
		return formatCaptionNumber(book.AlternateNumber, book.AlternateCount), true
	case "ratingastext", "rating":
		return formatCaptionRating(book.Rating), true
	case "communityratingastext", "communityrating":
		return formatCaptionRating(book.CommunityRating), true
	case "coverastext", "cover":
		for _, p := range book.Pages {
			if p.Type == PageTypeFrontCover {
				return "Yes", true
			}
		}
		return "No", true
	case "mangaastext", "manga":
		if strings.EqualFold(book.Manga, "YesRightToLeft") {
			return "Yes (Right to Left)", true
		}
		return yesNo(book.Manga), true
	case "blackandwhiteastext", "blackandwhite":
		return yesNo(book.BlackAndWhite), true
	case "seriescompleteastext", "seriescomplete":
		return yesNo(book.SeriesComplete), true
	case "openedcountastext", "openedcount":
		return strconv.Itoa(book.OpenCount), true
	}
	return "", false
}

func formatCaptionNumber(number string, count int) string {
	if number == "" {
		return ""
	}
	text := number
	if text == "-" {
		text = ""
	}
	if count > 0 {
		text += fmt.Sprintf(" (of %d)", count)
	}
	return text
}

func formatCaptionRating(r float64) string {
	if r <= 0 {
		return "None"
	}
	return strconv.FormatFloat(r, 'f', 1, 64)
}

// captionRawField reads an exported ComicBook field by case-insensitive name
// and renders it the way ComicRack's string conversion does.
func captionRawField(book *ComicBook, key string) (string, bool) {
	v := reflect.ValueOf(*book)
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || strings.ToLower(f.Name) != key {
			continue
		}
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.String:
			return fv.String(), true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.FormatInt(fv.Int(), 10), true
		case reflect.Float32, reflect.Float64:
			return strconv.FormatFloat(fv.Float(), 'f', -1, 64), true
		case reflect.Bool:
			return dotNetBool(fv.Bool()), true
		case reflect.Struct:
			if ct, ok := fv.Interface().(ComicTime); ok {
				if ct.IsZero() {
					return "1/1/0001 12:00:00 AM", true
				}
				return formatDotNetDate(ct.Time, "M/d/yyyy h:mm:ss tt"), true
			}
		}
		return "", false
	}
	return "", false
}
