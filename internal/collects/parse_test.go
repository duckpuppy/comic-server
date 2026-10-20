package collects

import (
	"reflect"
	"testing"
)

func TestParseSummary(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		want  []Candidate
		notes int
	}{
		{"one series range", "Collects LAZARUS #10-15 in one volume.",
			[]Candidate{{Title: "LAZARUS", From: "10", To: "15"}}, 0},
		{"several series",
			"Collecting GHOST RIDER: DANNY KETCH #1-5, GHOST RIDER (2006) #33-35 and GHOST RIDERS: HEAVEN'S ON FIRE #1-6",
			[]Candidate{
				{Title: "GHOST RIDER: DANNY KETCH", From: "1", To: "5"},
				{Title: "GHOST RIDER", Year: "2006", From: "33", To: "35"},
				{Title: "GHOST RIDERS: HEAVEN'S ON FIRE", From: "1", To: "6"}}, 0},
		{"single issue continues the title and an annual is its own series",
			"Collecting STAR WARS (2015) #14-15 and #20 and STAR WARS: DOCTOR APHRA ANNUAL #1",
			[]Candidate{
				{Title: "STAR WARS", Year: "2015", From: "14", To: "15"},
				{Title: "STAR WARS", Year: "2015", From: "20"},
				{Title: "STAR WARS: DOCTOR APHRA ANNUAL", From: "1"}}, 0},
		{"extras are ignored", "Collecting VISION (2015) #11-12, plus extras",
			[]Candidate{{Title: "VISION", Year: "2015", From: "11", To: "12"}}, 0},
		{"en dash and second hash", "Collects Star Wars: Rebel Heist #1–#4.",
			[]Candidate{{Title: "Star Wars: Rebel Heist", From: "1", To: "4"}}, 0},
		{"bare second range and starting with",
			"Collects starting with stories from BATMAN #582-586, 591-594 and BATMAN: OUR WORLDS AT WAR #1",
			[]Candidate{
				{Title: "BATMAN", From: "582", To: "586"},
				{Title: "BATMAN", From: "591", To: "594"},
				{Title: "BATMAN: OUR WORLDS AT WAR", From: "1"}}, 1},
		{"no title", "Collects issues #35-40.",
			[]Candidate{{From: "35", To: "40"}}, 1},
		{"best-selling event", "collecting the best-selling BLACKEST NIGHT #0-8",
			[]Candidate{{Title: "BLACKEST NIGHT", From: "0", To: "8"}}, 0},
		{"marketing after the sentence is not read",
			"Collects X #1-4. Don't miss the #1 New York Times bestseller!",
			[]Candidate{{Title: "X", From: "1", To: "4"}}, 0},
		{"same number twice is one issue", "Collects Y #3-3", []Candidate{{Title: "Y", From: "3"}}, 0},
		{"no collects statement", "A great story. Issue #5 changes everything.", nil, 0},
		{"collects but no issue numbers", "Collects the complete saga.", nil, 0},
	}
	for _, c := range cases {
		got := ParseSummary(c.text)
		if !reflect.DeepEqual(got.Candidates, c.want) {
			t.Errorf("%s:\n got  %+v\n want %+v", c.name, got.Candidates, c.want)
		}
		if len(got.Notes) != c.notes {
			t.Errorf("%s: notes = %v, want %d", c.name, got.Notes, c.notes)
		}
	}
}
