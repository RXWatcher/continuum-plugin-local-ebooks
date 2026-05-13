package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGoogleBooksFake(t *testing.T) (*httptest.Server, *GoogleBooks) {
	t.Helper()
	volume := loadFixture(t, "googlebooks_volume.json")
	search := loadFixture(t, "googlebooks_search.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/volumes/zyTCAlFPjgYC"):
			w.Write(volume)
		case strings.HasPrefix(r.URL.Path, "/volumes/notfoundXXXX"):
			w.WriteHeader(404)
		case r.URL.Path == "/volumes":
			w.Write(search)
		default:
			w.WriteHeader(404)
		}
	}))
	g := NewGoogleBooksAt(srv.URL, "test-api-key", "test")
	g.http.Client = srv.Client()
	return srv, g
}

func TestGoogleBooks_GetByID(t *testing.T) {
	srv, g := newGoogleBooksFake(t)
	defer srv.Close()

	c, err := g.Get(context.Background(), "zyTCAlFPjgYC", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Title != "Project Hail Mary" {
		t.Errorf("title %q", c.Title)
	}
	if c.Source != googleBooksID {
		t.Errorf("source %q", c.Source)
	}
	if c.ExternalID != "zyTCAlFPjgYC" {
		t.Errorf("externalID %q", c.ExternalID)
	}
	if c.ISBN != "9780593135204" {
		t.Errorf("isbn %q", c.ISBN)
	}
	if c.PageCount != 476 {
		t.Errorf("pageCount %d", c.PageCount)
	}
	if c.CoverURL == "" {
		t.Error("expected cover URL")
	}
	if !strings.HasPrefix(c.CoverURL, "https://") {
		t.Errorf("cover URL should be https, got %q", c.CoverURL)
	}
}

func TestGoogleBooks_GetMissing(t *testing.T) {
	srv, g := newGoogleBooksFake(t)
	defer srv.Close()

	// "notfoundXXXX" is 12 chars and matches the ID shape, but server returns 404.
	c, err := g.Get(context.Background(), "notfoundXXXX", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate")
	}
}

func TestGoogleBooks_SearchByText(t *testing.T) {
	srv, g := newGoogleBooksFake(t)
	defer srv.Close()

	cs, err := g.Search(context.Background(), "Project Hail Mary", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) < 1 {
		t.Fatalf("expected >=1 candidate, got %d", len(cs))
	}
	if cs[0].Title != "Project Hail Mary" {
		t.Errorf("title %q", cs[0].Title)
	}
}

func TestGoogleBooks_SearchByISBN(t *testing.T) {
	srv, g := newGoogleBooksFake(t)
	defer srv.Close()

	cs, err := g.Search(context.Background(), "9780593135204", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) < 1 {
		t.Fatalf("expected >=1 candidate, got %d", len(cs))
	}
}

func TestGoogleBooks_DisabledWhenNoKey(t *testing.T) {
	g := NewGoogleBooks("", "test")
	if g.Enabled(map[string]bool{googleBooksID: true}) {
		t.Error("expected Enabled=false when apiKey is empty")
	}
}
