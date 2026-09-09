package libraryorganizer

import (
	"encoding/xml"
	"fmt"
	"io"
)

// xmlItem is the "Name/Value" pair shape every one of a profile's seven
// Item-list collections uses in losettingsx.dat.
type xmlItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

type xmlItemList struct {
	Items []xmlItem `xml:"Item"`
}

type xmlExcludeRule struct {
	Field    string `xml:"Field,attr"`
	Operator string `xml:"Operator,attr"`
	Value    string `xml:"Value,attr"`
}

type xmlExcludeRules struct {
	Operator    string           `xml:"Operator,attr"`
	ExcludeMode string           `xml:"ExcludeMode,attr"`
	Rules       []xmlExcludeRule `xml:"ExcludeRule"`
}

// xmlProfile mirrors losettingsx.dat's <Profile> element - ground-truthed
// against the user's real file (comic-server-3bz.2), not guessed.
type xmlProfile struct {
	Name    string `xml:"Name,attr"`
	Version string `xml:"Version,attr"`

	FolderTemplate string `xml:"FolderTemplate"`
	BaseFolder     string `xml:"BaseFolder"`
	FileTemplate   string `xml:"FileTemplate"`
	EmptyFolder    string `xml:"EmptyFolder"`

	EmptyData           xmlItemList `xml:"EmptyData"`
	Postfix             xmlItemList `xml:"Postfix"`
	Prefix              xmlItemList `xml:"Prefix"`
	Seperator           xmlItemList `xml:"Seperator"`
	IllegalCharacters   xmlItemList `xml:"IllegalCharacters"`
	Months              xmlItemList `xml:"Months"`
	TextBox             xmlItemList `xml:"TextBox"`
	ExcludedEmptyFolder xmlItemList `xml:"ExcludedEmptyFolder"`
	ExcludeFolders      xmlItemList `xml:"ExcludeFolders"`
	FailedFields        xmlItemList `xml:"FailedFields"`

	UseFolder             bool   `xml:"UseFolder"`
	UseFileName           bool   `xml:"UseFileName"`
	DontAskWhenMultiOne   bool   `xml:"DontAskWhenMultiOne"`
	ExcludeOperator       string `xml:"ExcludeOperator"`
	RemoveEmptyFolder     bool   `xml:"RemoveEmptyFolder"`
	MoveFileless          bool   `xml:"MoveFileless"`
	FilelessFormat        string `xml:"FilelessFormat"`
	ExcludeMode           string `xml:"ExcludeMode"`
	FailEmptyValues       bool   `xml:"FailEmptyValues"`
	MoveFailed            bool   `xml:"MoveFailed"`
	FailedFolder          string `xml:"FailedFolder"`
	Mode                  string `xml:"Mode"`
	CopyMode              bool   `xml:"CopyMode"`
	AutoSpaceFields       bool   `xml:"AutoSpaceFields"`
	ReplaceMultipleSpaces bool   `xml:"ReplaceMultipleSpaces"`
	CopyReadPercentage    bool   `xml:"CopyReadPercentage"`

	ExcludeRules xmlExcludeRules `xml:"ExcludeRules"`
}

type xmlProfiles struct {
	LastUsed string       `xml:"LastUsed,attr"`
	Profiles []xmlProfile `xml:"Profile"`
}

// ImportedItem is one Name/Value entry in one of a profile's item
// collections, ready for a caller to persist (e.g. via
// configdb.DB.CreateLOProfileItem).
type ImportedItem struct {
	Category string
	Name     string
	Value    string
}

// ImportedExcludeRule is one condition in a profile's exclude-rule set.
type ImportedExcludeRule struct {
	Field    string
	Operator string
	Value    string
}

// ImportedProfile is one parsed profile, ready for a caller to persist.
type ImportedProfile struct {
	ID                    string // caller-assigned before Items/ExcludeRules reference it
	Name                  string
	BaseFolder            string
	FolderTemplate        string
	FileTemplate          string
	EmptyFolder           string
	Mode                  string
	CopyMode              bool
	UseFolder             bool
	UseFileName           bool
	ReplaceMultipleSpaces bool
	AutoSpaceFields       bool
	RemoveEmptyFolder     bool
	MoveFileless          bool
	FilelessFormat        string
	FailEmptyValues       bool
	MoveFailed            bool
	FailedFolder          string
	ExcludeMode           string
	ExcludeOperator       string
	SortOrder             int

	Items        []ImportedItem
	ExcludeRules []ImportedExcludeRule
}

// ImportResult is the full parsed result of one losettingsx.dat file.
type ImportResult struct {
	LastUsed string
	Profiles []ImportedProfile
}

// ParseLOSettings parses a losettingsx.dat file's contents into a flat
// ImportResult, one entry per <Profile> element in document order.
func ParseLOSettings(r io.Reader, genID func() string) (*ImportResult, error) {
	var doc xmlProfiles
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse losettingsx.dat: %w", err)
	}

	res := &ImportResult{LastUsed: doc.LastUsed}
	for i, p := range doc.Profiles {
		id := genID()
		imported := ImportedProfile{
			ID:                    id,
			Name:                  p.Name,
			BaseFolder:            p.BaseFolder,
			FolderTemplate:        p.FolderTemplate,
			FileTemplate:          p.FileTemplate,
			EmptyFolder:           p.EmptyFolder,
			Mode:                  p.Mode,
			CopyMode:              p.CopyMode,
			UseFolder:             p.UseFolder,
			UseFileName:           p.UseFileName,
			ReplaceMultipleSpaces: p.ReplaceMultipleSpaces,
			AutoSpaceFields:       p.AutoSpaceFields,
			RemoveEmptyFolder:     p.RemoveEmptyFolder,
			MoveFileless:          p.MoveFileless,
			FilelessFormat:        p.FilelessFormat,
			FailEmptyValues:       p.FailEmptyValues,
			MoveFailed:            p.MoveFailed,
			FailedFolder:          p.FailedFolder,
			ExcludeMode:           firstNonEmpty(p.ExcludeRules.ExcludeMode, p.ExcludeMode),
			ExcludeOperator:       firstNonEmpty(p.ExcludeRules.Operator, p.ExcludeOperator),
			SortOrder:             i,
		}

		imported.Items = append(imported.Items, collectItems("empty_data", p.EmptyData)...)
		imported.Items = append(imported.Items, collectItems("postfix", p.Postfix)...)
		imported.Items = append(imported.Items, collectItems("prefix", p.Prefix)...)
		imported.Items = append(imported.Items, collectItems("seperator", p.Seperator)...)
		imported.Items = append(imported.Items, collectItems("illegal_characters", p.IllegalCharacters)...)
		imported.Items = append(imported.Items, collectItems("months", p.Months)...)
		imported.Items = append(imported.Items, collectItems("textbox", p.TextBox)...)
		imported.Items = append(imported.Items, collectItems("excluded_empty_folder", p.ExcludedEmptyFolder)...)
		imported.Items = append(imported.Items, collectItems("exclude_folders", p.ExcludeFolders)...)
		imported.Items = append(imported.Items, collectItems("failed_fields", p.FailedFields)...)

		for _, rule := range p.ExcludeRules.Rules {
			imported.ExcludeRules = append(imported.ExcludeRules, ImportedExcludeRule{
				Field:    rule.Field,
				Operator: rule.Operator,
				Value:    rule.Value,
			})
		}

		res.Profiles = append(res.Profiles, imported)
	}

	return res, nil
}

func collectItems(category string, list xmlItemList) []ImportedItem {
	out := make([]ImportedItem, len(list.Items))
	for i, item := range list.Items {
		out[i] = ImportedItem{Category: category, Name: item.Name, Value: item.Value}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
