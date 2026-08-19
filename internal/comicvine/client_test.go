package comicvine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient("test-api-key", WithBaseURL(srv.URL))
}

func TestSearchVolumes(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "Batman" {
			t.Errorf("query = %q, want Batman", got)
		}
		if got := r.URL.Query().Get("resources"); got != "volume" {
			t.Errorf("resources = %q, want volume", got)
		}
		resp := apiResponse[[]Volume]{
			StatusCode: 1,
			Results: []Volume{
				{ID: 1, Name: "Batman", StartYear: "1940", Publisher: Publisher{ID: 10, Name: "DC Comics"}},
				{ID: 2, Name: "Batman Beyond", StartYear: "1999"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	volumes, err := client.SearchVolumes(context.Background(), "Batman")
	if err != nil {
		t.Fatalf("SearchVolumes: %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("got %d volumes, want 2", len(volumes))
	}
	if volumes[0].Name != "Batman" || volumes[0].Publisher.Name != "DC Comics" {
		t.Errorf("volumes[0] = %+v", volumes[0])
	}
}

func TestSearchVolumes_Empty(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse[[]Volume]{StatusCode: 1, Results: []Volume{}}
		json.NewEncoder(w).Encode(resp)
	})

	volumes, err := client.SearchVolumes(context.Background(), "Nonexistent")
	if err != nil {
		t.Fatalf("SearchVolumes: %v", err)
	}
	if len(volumes) != 0 {
		t.Errorf("got %d volumes, want 0", len(volumes))
	}
}

func TestSearchVolumes_APIError(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse[[]Volume]{StatusCode: 100, Error: "Invalid API Key"}
		json.NewEncoder(w).Encode(resp)
	})

	_, err := client.SearchVolumes(context.Background(), "Batman")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid API Key") {
		t.Errorf("error = %v, want it to mention Invalid API Key", err)
	}
}

func TestFetchIssueDetail(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/issue/4000-12345") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		detail := IssueDetail{
			ID:          12345,
			IssueNumber: "1",
			Name:        "Origins",
			CoverDate:   "1940-01-01",
			StoreDate:   "1939-12-15",
			Description: "The first appearance.",
			Image:       ImageURLs{SmallURL: "http://example.com/small.jpg", SuperURL: "http://example.com/super.jpg"},
			PersonCredits: []PersonCredit{
				{ID: 1, Name: "Bill Finger", Role: "writer"},
				{ID: 2, Name: "Bob Kane", Role: "penciller, cover"},
			},
			CharacterCredits: []NamedCredit{{ID: 100, Name: "Batman"}},
			TeamCredits:      []NamedCredit{{ID: 200, Name: "Justice League"}},
			LocationCredits:  []NamedCredit{{ID: 300, Name: "Gotham City"}},
			StoryArcCredits:  []NamedCredit{{ID: 400, Name: "Year One"}},
		}
		detail.Volume.ID = 999
		detail.Volume.Name = "Batman"
		resp := apiResponse[IssueDetail]{StatusCode: 1, Results: detail}
		json.NewEncoder(w).Encode(resp)
	})

	detail, err := client.FetchIssueDetail(context.Background(), 12345)
	if err != nil {
		t.Fatalf("FetchIssueDetail: %v", err)
	}
	if detail.Name != "Origins" || detail.Volume.Name != "Batman" {
		t.Errorf("detail = %+v", detail)
	}
	if len(detail.PersonCredits) != 2 || detail.PersonCredits[0].Role != "writer" {
		t.Errorf("PersonCredits = %+v", detail.PersonCredits)
	}
	if len(detail.CharacterCredits) != 1 || detail.CharacterCredits[0].Name != "Batman" {
		t.Errorf("CharacterCredits = %+v", detail.CharacterCredits)
	}
	if detail.Image.SuperURL != "http://example.com/super.jpg" {
		t.Errorf("Image = %+v", detail.Image)
	}
}

func TestFetchIssueDetail_MissingFields(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse[IssueDetail]{
			StatusCode: 1,
			Results:    IssueDetail{ID: 1, IssueNumber: "1"},
		}
		json.NewEncoder(w).Encode(resp)
	})

	detail, err := client.FetchIssueDetail(context.Background(), 1)
	if err != nil {
		t.Fatalf("FetchIssueDetail: %v", err)
	}
	if detail.PersonCredits != nil {
		t.Errorf("PersonCredits = %+v, want nil", detail.PersonCredits)
	}
	if detail.Name != "" {
		t.Errorf("Name = %q, want empty", detail.Name)
	}
}

func TestFetchIssueDetail_APIError(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse[IssueDetail]{StatusCode: 101, Error: "Object Not Found"}
		json.NewEncoder(w).Encode(resp)
	})

	_, err := client.FetchIssueDetail(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Object Not Found") {
		t.Errorf("error = %v, want it to mention Object Not Found", err)
	}
}

func TestFetchIssueDetail_HTTPError(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := client.FetchIssueDetail(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
