package collects

import "testing"

func TestWarnings(t *testing.T) {
	nums := []string{"1", "2", "3", "Annual 1"}
	cases := []struct {
		name string
		e    Entry
		nums []string
		want int
	}{
		{"in range", Entry{Volume: 1, From: "1", To: "3"}, nums, 0},
		{"leading zero matches", Entry{Volume: 1, From: "01"}, nums, 0},
		{"to missing", Entry{Volume: 1, From: "1", To: "9"}, nums, 1},
		{"both missing", Entry{Volume: 1, From: "8", To: "9"}, nums, 2},
		{"backwards", Entry{Volume: 1, From: "3", To: "1"}, nums, 1},
		{"unknown volume is not a warning", Entry{Volume: 1, From: "8", To: "9"}, nil, 0},
		{"backwards even when unknown", Entry{Volume: 1, From: "9", To: "8"}, nil, 1},
		{"text number", Entry{Volume: 1, From: "annual 1"}, nums, 0},
	}
	for _, c := range cases {
		if got := Warnings(c.e, c.nums); len(got) != c.want {
			t.Errorf("%s: %v, want %d", c.name, got, c.want)
		}
	}
}
