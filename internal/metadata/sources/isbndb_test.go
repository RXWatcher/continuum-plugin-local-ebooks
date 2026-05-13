package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newISBNdbFake(t *testing.T) (*httptest.Server, *ISBNdb) {
	t.Helper()
	book := loadFixture(t, "isbndb_book.json")
	search := loadFixture(t, "isbndb_search.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/book/9780201616224"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(book)
		case strings.HasPrefix(r.URL.Path, "/book/notfound"):
			w.WriteHeader(404)
		case strings.HasPrefix(r.URL.Path, "/books/"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(search)
		default:
			w.WriteHeader(404)
		}
	}))
	i := NewISBNdbAt(srv.URL, "test-api-key", "test")
	i.http.Client = srv.Client()
	return srv, i
}

func TestISBNdb_GetByISBN(t *testing.T) {
	srv, i := newISBNdbFake(t)
	defer srv.Close()

	c, err := i.Get(context.Background(), "9780201616224", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Title != "The Pragmatic Programmer" {
		t.Errorf("title %q", c.Title)
	}
	if c.Source != isbndbID {
		t.Errorf("source %q", c.Source)
	}
	if c.ISBN != "9780201616224" {
		t.Errorf("isbn %q", c.ISBN)
	}
	if len(c.Authors) == 0 {
		t.Error("expected at least one author")
	}
	if c.PageCount != 352 {
		t.Errorf("pageCount %d", c.PageCount)
	}
	if c.CoverURL == "" {
		t.Error("expected cover URL")
	}
	if c.Description == "" {
		t.Error("expected description")
	}
}

func TestISBNdb_GetMissing(t *testing.T) {
	// Use a server that always 404s to verify the ErrNotFound path.
	notFoundSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer notFoundSrv.Close()

	i := NewISBNdbAt(notFoundSrv.URL, "test-api-key", "test")
	i.http.Client = notFoundSrv.Client()

	c, err := i.Get(context.Background(), "9780201616224", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got err=%v c=%v", err, c)
	}
	if c != nil {
		t.Errorf("expected nil candidate")
	}
}

func TestISBNdb_SearchByText(t *testing.T) {
	srv, i := newISBNdbFake(t)
	defer srv.Close()

	cs, err := i.Search(context.Background(), "Pragmatic Programmer", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) < 1 {
		t.Fatalf("expected >=1 candidate, got %d", len(cs))
	}
	if cs[0].Title != "The Pragmatic Programmer" {
		t.Errorf("title %q", cs[0].Title)
	}
	if cs[0].Source != isbndbID {
		t.Errorf("source %q", cs[0].Source)
	}
}

func TestISBNdb_NonISBNGetReturnsNil(t *testing.T) {
	// No server needed — non-ISBN IDs are rejected before any HTTP call.
	i := NewISBNdb("test-api-key", "test")

	c, err := i.Get(context.Background(), "not-an-isbn", "us")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for non-ISBN id, got %+v", c)
	}
}

func TestISBNdb_DisabledWhenNoKey(t *testing.T) {
	i := NewISBNdb("", "test")
	if i.Enabled(map[string]bool{isbndbID: true}) {
		t.Error("expected Enabled=false when apiKey is empty")
	}
}
