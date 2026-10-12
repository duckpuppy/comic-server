package comicvine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchIssueImages(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/issue/4000-157819") {
			t.Errorf("path = %s", r.URL.Path)
		}
		fl := r.URL.Query().Get("field_list")
		if !strings.Contains(fl, "associated_images") {
			t.Errorf("field_list = %q, want associated_images", fl)
		}
		w.Write([]byte(`{"status_code":1,"error":"OK","results":{"id":157819,"name":"One: Babylon",
			"image":{"original_url":"https://comicvine.gamespot.com/a/uploads/original/0/9116/808650-1.jpg"},
			"associated_images":[
				{"original_url":"https://comicvine.gamespot.com/a/uploads/original/6/67663/2185429-01c.jpg","id":2185429,"caption":null,"image_tags":"All Images"},
				{"original_url":"https://comicvine.gamespot.com/a/uploads/original/6/67663/2185428-01b.jpg","id":2185428,"caption":null,"image_tags":"All Images"}]}}`))
	})
	got, err := client.FetchIssueImages(context.Background(), 157819)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 157819 || len(got.AssociatedImages) != 2 || got.AssociatedImages[0].ID != 2185429 ||
		got.AssociatedImages[0].Caption != nil || got.AssociatedImages[1].ImageTags != "All Images" ||
		got.Image.OriginalURL == "" {
		t.Errorf("got %+v", got)
	}
}

func TestAllowedImageHost(t *testing.T) {
	ok := []string{"https://comicvine.gamespot.com/a/uploads/original/1.jpg", "https://gamespot.com/x.png"}
	bad := []string{"http://comicvine.gamespot.com/x.jpg", "https://evil.example/x.jpg",
		"https://gamespot.com.evil.example/x.jpg", "file:///etc/passwd", "", "https://127.0.0.1/x.jpg"}
	for _, u := range ok {
		if !AllowedImageHost(u) {
			t.Errorf("%q should be allowed", u)
		}
	}
	for _, u := range bad {
		if AllowedImageHost(u) {
			t.Errorf("%q should be refused", u)
		}
	}
}

func TestDownloadImage(t *testing.T) {
	png := gradientImage(t, 8, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.png":
			w.Write(png)
		case "/html":
			w.Write([]byte("<html>not an image</html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewClient("k")

	data, ext, err := c.DownloadImage(context.Background(), srv.URL+"/ok.png", true)
	if err != nil || ext != ".png" || len(data) != len(png) {
		t.Fatalf("ok: ext=%q err=%v", ext, err)
	}
	if _, _, err := c.DownloadImage(context.Background(), srv.URL+"/html", true); err == nil {
		t.Error("non-image bytes should be rejected")
	}
	if _, _, err := c.DownloadImage(context.Background(), srv.URL+"/missing", true); err == nil {
		t.Error("404 should error")
	}
	if _, _, err := c.DownloadImage(context.Background(), srv.URL+"/ok.png", false); err == nil {
		t.Error("non-allowlisted host must be refused when allowAnyHost is false")
	}
}
