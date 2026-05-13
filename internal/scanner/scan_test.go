package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ContinuumApp/continuum-plugin-ebooksdb/internal/ebookparse"
)

type fakeStore struct {
	ebooks   map[string]string // id -> path
	covers   map[string][]byte
	deleted  map[string]bool
	pathToID map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		ebooks: map[string]string{}, covers: map[string][]byte{},
		deleted: map[string]bool{}, pathToID: map[string]string{},
	}
}

func (f *fakeStore) UpsertEbook(ctx context.Context, lpID int64, id, path, format string,
	size int64, mtime time.Time, p ebookparse.Parsed) (bool, error) {
	_, wasKnown := f.pathToID[path]
	f.ebooks[id] = path
	f.pathToID[path] = id
	return wasKnown, nil
}
func (f *fakeStore) UpsertCover(ctx context.Context, id, ct, source string, b []byte) error {
	f.covers[id] = b
	return nil
}
func (f *fakeStore) ListPaths(ctx context.Context, lpID int64) (map[string]string, error) {
	out := map[string]string{}
	for id, p := range f.ebooks {
		if !f.deleted[id] {
			out[id] = p
		}
	}
	return out, nil
}
func (f *fakeStore) SoftDelete(ctx context.Context, id string) error {
	f.deleted[id] = true
	return nil
}

type fakeEnqueuer struct{ ids []string }

func (f *fakeEnqueuer) Enqueue(ctx context.Context, id string) error {
	f.ids = append(f.ids, id)
	return nil
}

func TestScan_AddsAndEnqueues(t *testing.T) {
	dir := t.TempDir()
	// Create a file with .epub extension. It won't be a valid EPUB; scan
	// continues with empty metadata via the parse-failed branch.
	os.WriteFile(filepath.Join(dir, "book.epub"), []byte("not really an epub"), 0o644)
	store := newFakeStore()
	enq := &fakeEnqueuer{}
	res, err := Walk(context.Background(), dir, 1, Deps{Store: store, EnrichmentQueue: enq})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("added=%d want 1", res.Added)
	}
	if len(enq.ids) != 1 {
		t.Errorf("expected 1 enqueue, got %d", len(enq.ids))
	}
}

func TestScan_SkipsNonEbookFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644)
	store := newFakeStore()
	res, _ := Walk(context.Background(), dir, 1, Deps{Store: store})
	if res.Added != 0 {
		t.Errorf("expected 0 added, got %d", res.Added)
	}
}

func TestScan_SoftDeletesMissingFiles(t *testing.T) {
	dir := t.TempDir()
	store := newFakeStore()
	store.ebooks["preexisting-id"] = filepath.Join(dir, "gone.epub")
	res, _ := Walk(context.Background(), dir, 1, Deps{Store: store})
	if res.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", res.Deleted)
	}
	if !store.deleted["preexisting-id"] {
		t.Error("expected preexisting-id to be soft-deleted")
	}
}

func TestScan_NilEnqueuerIsSafe(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "book.epub"), []byte("x"), 0o644)
	store := newFakeStore()
	res, err := Walk(context.Background(), dir, 1, Deps{Store: store, EnrichmentQueue: nil})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("added=%d", res.Added)
	}
}
