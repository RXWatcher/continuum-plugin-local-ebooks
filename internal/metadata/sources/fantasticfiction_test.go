package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newFantasticFictionFake serves the search fixture on /search/ and 404s
// everywhere else. No detail-page route exists because Get is a no-op.
func newFantasticFictionFake(t *testing.T) (*httptest.Server, *FantasticFiction) {
	t.Helper()
	search := loadFixture(t, "fantasticfiction_search.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/":
			w.Header().Set("Content-Type", "text/html")
			w.Write(search)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	f := NewFantasticFictionAt(srv.URL, "test-agent")
	f.http.Client = srv.Client()
	return srv, f
}

// TestFantasticFiction_SearchByText is the happy-path assertion: the fixture
// contains three well-formed book blocks plus one malformed one. We assert
// the count and every parser-populated field on the first row so any
// regex regression is immediately visible.
func TestFantasticFiction_SearchByText(t *testing.T) {
	srv, f := newFantasticFictionFake(t)
	defer srv.Close()

	cs, err := f.Search(context.Background(), "project hail mary", "us")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("expected 3 results (3 well-formed blocks, 1 malformed skipped), got %d", len(cs))
	}

	first := cs[0]
	if first.Title != "Project Hail Mary" {
		t.Errorf("first.Title: got %q, want %q", first.Title, "Project Hail Mary")
	}
	if len(first.Authors) != 1 || first.Authors[0] != "Andy Weir" {
		t.Errorf("first.Authors: got %v, want [Andy Weir]", first.Authors)
	}
	if first.PublishedAt != "2021" {
		t.Errorf("first.PublishedAt: got %q, want %q", first.PublishedAt, "2021")
	}
	if first.Source != fantasticFictionID {
		t.Errorf("first.Source: got %q, want %q", first.Source, fantasticFictionID)
	}
	if first.Region != "us" {
		t.Errorf("first.Region: got %q, want %q", first.Region, "us")
	}
	if first.ExternalID != "" {
		t.Errorf("first.ExternalID should be empty (no stable per-book ID), got %q", first.ExternalID)
	}
	if len(first.Genres) != 0 {
		t.Errorf("first.Genres should be empty (hardcoded 'Fiction' dropped), got %v", first.Genres)
	}

	// The Dune block carries a Series: link — assert it round-trips.
	var dune *struct {
		Title, Series string
	}
	for i := range cs {
		if cs[i].Title == "Dune" {
			dune = &struct{ Title, Series string }{cs[i].Title, cs[i].Series}
			break
		}
	}
	if dune == nil {
		t.Fatal("expected the Dune row to be present")
	}
	if dune.Series != "Dune" {
		t.Errorf("dune.Series: got %q, want %q", dune.Series, "Dune")
	}
}

// TestFantasticFiction_SearchEmptyQueryReturnsNil verifies that an empty
// query short-circuits to (nil, nil) without a network call. The fake's
// default 404 means any HTTP request would surface as a non-nil error.
func TestFantasticFiction_SearchEmptyQueryReturnsNil(t *testing.T) {
	srv, f := newFantasticFictionFake(t)
	defer srv.Close()

	cs, err := f.Search(context.Background(), "   ", "us")
	if err != nil {
		t.Errorf("expected nil error for empty query, got %v", err)
	}
	if cs != nil {
		t.Errorf("expected nil results for empty query, got %+v", cs)
	}
}

// TestFantasticFiction_GetReturnsNil documents the design decision that
// Fantastic Fiction has no stable per-book ID we can reliably resolve, so
// Get is a no-op nil-returner for any input. The fake's default 404 means
// any unintended network call would surface as a non-nil error here.
func TestFantasticFiction_GetReturnsNil(t *testing.T) {
	srv, f := newFantasticFictionFake(t)
	defer srv.Close()

	c, err := f.Get(context.Background(), "anything", "us")
	if err != nil {
		t.Errorf("expected nil error from Get, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate from Get, got %+v", c)
	}
}

// TestFantasticFiction_SearchSkipsTitlelessBlocks verifies the malformed
// 4th block in the fixture (a div.book with no <a> title link) is silently
// skipped rather than emitted as an empty-title candidate.
func TestFantasticFiction_SearchSkipsTitlelessBlocks(t *testing.T) {
	srv, f := newFantasticFictionFake(t)
	defer srv.Close()

	cs, err := f.Search(context.Background(), "project hail mary", "us")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range cs {
		if c.Title == "" {
			t.Errorf("found empty-title candidate in results: %+v", c)
		}
	}
}
