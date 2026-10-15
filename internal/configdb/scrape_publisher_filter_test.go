package configdb

import (
	"reflect"
	"testing"
)

func TestScrapePublisherFilter_NilWhenUnset(t *testing.T) {
	db := newTestDB(t)
	got, err := db.GetScrapePublisherFilter()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestScrapePublisherFilter_RoundTripAndClear(t *testing.T) {
	db := newTestDB(t)
	want := ScrapePublisherFilter{Allow: []string{"DC", "Marvel"}, Block: []string{"Panini"}}
	if err := db.UpsertScrapePublisherFilter(want); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetScrapePublisherFilter()
	if err != nil || got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %+v err %v, want %+v", got, err, want)
	}

	// Saving two empty lists must read back as "saved, empty", not nil:
	// that is how the UI overrides a config.yaml filter with no filter.
	if err := db.UpsertScrapePublisherFilter(ScrapePublisherFilter{}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetScrapePublisherFilter()
	if err != nil || got == nil || len(got.Allow) != 0 || len(got.Block) != 0 {
		t.Fatalf("got %+v err %v, want saved-empty", got, err)
	}
}
