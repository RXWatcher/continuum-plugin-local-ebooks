package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGutenbergFake(t *testing.T) (*httptest.Server, *Gutenberg) {
	book := loadFixture(t, "gutenberg_book.json")
	search := loadFixture(t, "gutenberg_search.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/books" && r.URL.Query().Get("search") != "":
			w.Write(search)
		case r.URL.Path == "/books/84":
			w.Write(book)
		case strings.HasPrefix(r.URL.Path, "/books/99999999"):
			w.WriteHeader(404)
		default:
			w.WriteHeader(404)
		}
	}))
	g := NewGutenbergAt(srv.URL, "test")
	g.http.Client = srv.Client()
	return srv, g
}

func TestGutenberg_GetByID(t *testing.T) {
	srv, g := newGutenbergFake(t)
	defer srv.Close()
	c, err := g.Get(context.Background(), "84", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Title != "Frankenstein; Or, The Modern Prometheus" {
		t.Errorf("title %q", c.Title)
	}
	if c.Source != "gutenberg" {
		t.Errorf("source %q", c.Source)
	}
	if c.ExternalID != "84" {
		t.Errorf("external id %q", c.ExternalID)
	}
	if len(c.Authors) != 1 || c.Authors[0] != "Shelley, Mary Wollstonecraft" {
		t.Errorf("authors %v", c.Authors)
	}
	if c.Language != "en" {
		t.Errorf("language %q", c.Language)
	}
	// Subjects in fixture has 7 entries; we must cap at 5.
	if len(c.Genres) != 5 {
		t.Errorf("expected 5 genres (capped), got %d", len(c.Genres))
	}
	if c.CoverURL == "" {
		t.Error("expected cover URL")
	}
	if !strings.Contains(c.CoverURL, "pg84.cover") {
		t.Errorf("cover URL %q", c.CoverURL)
	}
}

func TestGutenberg_GetMissing(t *testing.T) {
	srv, g := newGutenbergFake(t)
	defer srv.Close()
	c, err := g.Get(context.Background(), "99999999", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate")
	}
}

func TestGutenberg_GetNonNumericReturnsNil(t *testing.T) {
	srv, g := newGutenbergFake(t)
	defer srv.Close()
	c, err := g.Get(context.Background(), "OL12345W", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for non-numeric id, got %+v", c)
	}
}

func TestGutenberg_SearchByText(t *testing.T) {
	srv, g := newGutenbergFake(t)
	defer srv.Close()
	cs, err := g.Search(context.Background(), "frankenstein", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d candidates, want 2", len(cs))
	}
	first := cs[0]
	if first.Title != "Frankenstein; Or, The Modern Prometheus" {
		t.Errorf("title %q", first.Title)
	}
	if len(first.Authors) != 1 || first.Authors[0] != "Shelley, Mary Wollstonecraft" {
		t.Errorf("authors %v", first.Authors)
	}
	if first.CoverURL == "" {
		t.Error("expected cover URL")
	}
	if first.Source != "gutenberg" {
		t.Errorf("source %q", first.Source)
	}
	if first.ExternalID != "84" {
		t.Errorf("external id %q", first.ExternalID)
	}
}
