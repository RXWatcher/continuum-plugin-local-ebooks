package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ContinuumApp/continuum-plugin-local-ebooks/internal/ebookparse"
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

// TestScan_SkipsSymlinkedEbook guards the symlink-escape vector: a symlink
// placed inside a library root must NOT be followed and ingested, otherwise
// an attacker who can write into a scanned dir reads arbitrary host files.
func TestScan_SkipsSymlinkedEbook(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "book.epub"), []byte("real book"), 0o644)
	if err := os.Symlink(secret, filepath.Join(dir, "evil.epub")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	store := newFakeStore()
	res, err := Walk(context.Background(), dir, 1, Deps{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("added=%d, want 1 (only the real book, not the symlink)", res.Added)
	}
	for _, p := range store.ebooks {
		if filepath.Base(p) == "evil.epub" {
			t.Fatal("symlink ebook was ingested — symlink escape not prevented")
		}
	}
}

// TestScan_SkipsSymlinkedSidecarCover guards the same escape via cover.jpg.
func TestScan_SkipsSymlinkedSidecarCover(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.bin")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "book.epub"), []byte("real book"), 0o644)
	if err := os.Symlink(secret, filepath.Join(dir, "cover.jpg")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	store := newFakeStore()
	if _, err := Walk(context.Background(), dir, 1, Deps{Store: store}); err != nil {
		t.Fatal(err)
	}
	for id, b := range store.covers {
		if string(b) == "PRIVATE KEY BYTES" {
			t.Fatalf("sidecar cover symlink exfiltrated secret bytes for id %s", id)
		}
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
