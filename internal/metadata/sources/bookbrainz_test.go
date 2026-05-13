package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const bbValidUUID = "b1a2c3d4-5e6f-7a8b-9c0d-1e2f3a4b5c6d"

func newBookBrainzFake(t *testing.T) (*httptest.Server, *BookBrainz) {
	edition := loadFixture(t, "bookbrainz_edition.json")
	search := loadFixture(t, "bookbrainz_search.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search" && r.URL.Query().Get("type") == "edition" && r.URL.Query().Get("q") != "":
			_, _ = w.Write(search)
		case r.URL.Path == "/edition/"+bbValidUUID:
			_, _ = w.Write(edition)
		case strings.HasPrefix(r.URL.Path, "/edition/"):
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	b := NewBookBrainzAt(srv.URL, "test")
	b.http.Client = srv.Client()
	return srv, b
}

func TestBookBrainz_GetByBBID(t *testing.T) {
	srv, b := newBookBrainzFake(t)
	defer srv.Close()

	c, err := b.Get(context.Background(), bbValidUUID, "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Source != "bookbrainz" {
		t.Errorf("source %q", c.Source)
	}
	if c.ExternalID != bbValidUUID {
		t.Errorf("external id %q", c.ExternalID)
	}
	if c.Title != "Project Hail Mary" {
		t.Errorf("title %q", c.Title)
	}
	if len(c.Authors) < 1 || c.Authors[0] != "Andy Weir" {
		t.Errorf("authors %v", c.Authors)
	}
	if c.Publisher != "Ballantine Books" {
		t.Errorf("publisher %q", c.Publisher)
	}
	if c.PublishedAt != "2021" {
		// PublishedAt must be the 4-digit year extracted from "2021-05-04".
		t.Errorf("published_at %q (want 4-digit year)", c.PublishedAt)
	}
	// ISBN-13 must be preferred over the ISBN-10 also present in the fixture.
	if c.ISBN != "9780593135204" {
		t.Errorf("isbn %q (expected ISBN-13 preferred)", c.ISBN)
	}
	if c.Language != "English" {
		t.Errorf("language %q", c.Language)
	}
	if !strings.Contains(c.Description, "astronaut") {
		t.Errorf("description %q (expected annotation content)", c.Description)
	}
	if len(c.Raw) == 0 {
		t.Error("expected Raw to be populated")
	}
	if c.Region != "us" {
		t.Errorf("region %q", c.Region)
	}
}

func TestBookBrainz_GetMissing(t *testing.T) {
	srv, b := newBookBrainzFake(t)
	defer srv.Close()

	// A valid-shaped UUID the fake server has no fixture for.
	c, err := b.Get(context.Background(), "00000000-0000-0000-0000-000000000000", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate, got %+v", c)
	}
}

func TestBookBrainz_GetNonUUIDReturnsNil(t *testing.T) {
	// No server needed: non-UUID input must short-circuit before any HTTP call.
	b := NewBookBrainzAt("http://invalid.example.invalid", "test")
	c, err := b.Get(context.Background(), "not-a-uuid", "us")
	if err != nil {
		t.Fatalf("expected no error for non-UUID id, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for non-UUID id, got %+v", c)
	}
}

func TestBookBrainz_SearchByText(t *testing.T) {
	srv, b := newBookBrainzFake(t)
	defer srv.Close()

	cs, err := b.Search(context.Background(), "project hail mary", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d candidates, want 2", len(cs))
	}
	first := cs[0]
	if first.Title != "Project Hail Mary" {
		t.Errorf("title %q", first.Title)
	}
	if len(first.Authors) != 1 || first.Authors[0] != "Andy Weir" {
		t.Errorf("authors %v", first.Authors)
	}
	if first.Source != "bookbrainz" {
		t.Errorf("source %q", first.Source)
	}
	if first.ExternalID != bbValidUUID {
		t.Errorf("external id %q", first.ExternalID)
	}
	if first.ISBN != "9780593135204" {
		t.Errorf("isbn %q", first.ISBN)
	}
	// Second result has only year-level date; year extraction must still work.
	if cs[1].PublishedAt != "2021" {
		t.Errorf("second published_at %q", cs[1].PublishedAt)
	}
}
