package library

import "testing"

func TestSetCustomValue(t *testing.T) {
	tests := []struct {
		store, key, value, want string
	}{
		{"", "comicvine_volume", "100", ",comicvine_volume=100"},
		{",comicvine_volume=100", "comicvine_volume", "200", ",comicvine_volume=200"},
		{",a=1,comicvine_volume=100,b=2", "comicvine_volume", "200", ",a=1,comicvine_volume=200,b=2"},
	}
	for _, tt := range tests {
		if got := SetCustomValue(tt.store, tt.key, tt.value); got != tt.want {
			t.Errorf("SetCustomValue(%q, %q, %q) = %q, want %q", tt.store, tt.key, tt.value, got, tt.want)
		}
	}
}

func TestRemoveCustomValue(t *testing.T) {
	store := ",a=1,b=2,c=3"
	if got := RemoveCustomValue(store, "b"); got != ",a=1,c=3" {
		t.Errorf("got %q", got)
	}
	if got := RemoveCustomValue(store, "zz"); got != store {
		t.Errorf("absent key changed store: %q", got)
	}
	if got := RemoveCustomValue(",a=1", "a"); got != "" {
		t.Errorf("last key: %q", got)
	}
}
