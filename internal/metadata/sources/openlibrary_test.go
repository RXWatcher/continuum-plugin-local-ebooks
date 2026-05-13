package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newOpenLibraryFake(t *testing.T) (*httptest.Server, *OpenLibrary) {
	book := loadFixture(t, "openlibrary_book.json")
	search := loadFixture(t, "openlibrary_search.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/isbn/9780593135204"):
			w.Write(book)
		case strings.HasPrefix(r.URL.Path, "/isbn/9780000000000"):
			w.WriteHeader(404)
		case strings.HasPrefix(r.URL.Path, "/books/"):
			w.Write(book)
		case r.URL.Path == "/search.json":
			w.Write(search)
		default:
			w.WriteHeader(404)
		}
	}))
	o := NewOpenLibraryAt(srv.URL, "https://covers.openlibrary.org", "test")
	o.http.Client = srv.Client()
	return srv, o
}

func TestOpenLibrary_GetByISBN(t *testing.T) {
	srv, o := newOpenLibraryFake(t)
	defer srv.Close()
	c, err := o.Get(context.Background(), "9780593135204", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Title != "Project Hail Mary" {
		t.Errorf("title %q", c.Title)
	}
	if c.Source != "openlibrary" {
		t.Errorf("source %q", c.Source)
	}
	if c.ISBN != "9780593135204" {
		t.Errorf("isbn %q", c.ISBN)
	}
	if c.PageCount != 476 {
		t.Errorf("pages %d", c.PageCount)
	}
	if c.CoverURL == "" {
		t.Error("expected cover URL")
	}
}

func TestOpenLibrary_GetMissing(t *testing.T) {
	srv, o := newOpenLibraryFake(t)
	defer srv.Close()
	c, err := o.Get(context.Background(), "9780000000000", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate")
	}
}

func TestOpenLibrary_SearchByText(t *testing.T) {
	srv, o := newOpenLibraryFake(t)
	defer srv.Close()
	cs, err := o.Search(context.Background(), "Project Hail Mary", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("got %d", len(cs))
	}
	if cs[0].Title != "Project Hail Mary" {
		t.Errorf("title %q", cs[0].Title)
	}
}

func TestOpenLibrary_SearchByISBN(t *testing.T) {
	srv, o := newOpenLibraryFake(t)
	defer srv.Close()
	cs, err := o.Search(context.Background(), "9780593135204", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("got %d", len(cs))
	}
}

func TestOpenLibrary_UnrecognizedID(t *testing.T) {
	srv, o := newOpenLibraryFake(t)
	defer srv.Close()
	c, err := o.Get(context.Background(), "not-an-id", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Errorf("expected nil for unrecognized id")
	}
}
