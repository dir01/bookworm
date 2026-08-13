package catalog

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

// zimFixture is a small Project Gutenberg ZIM archive kept in testdata. See
// testdata/README.md for provenance.
const zimFixture = "testdata/gutenberg_csb_all_2025-10.zim"

func TestReadZIMBooks(t *testing.T) {
	books, err := readZIMBooks(zimFixture)
	if err != nil {
		t.Fatalf("readZIMBooks: %v", err)
	}
	if len(books) == 0 {
		t.Fatalf("no books found in %q", zimFixture)
	}
	t.Logf("indexed %d books", len(books))

	for _, b := range books {
		if b.FileType != ZIM {
			t.Errorf("book %q: FileType = %q, want %q", b.Title, b.FileType, ZIM)
		}
		if b.Title == "" {
			t.Errorf("book with ref %q has empty title", b.SubFilepath)
		}
		if !strings.Contains(b.SubFilepath, "/") {
			t.Errorf("book %q: SubFilepath %q missing namespace prefix", b.Title, b.SubFilepath)
		}
	}

	// Round-trip: reopen the first book's blob and confirm it is a zip (EPUB).
	buf, err := openZIMEntry(zimFixture, books[0].SubFilepath)
	if err != nil {
		t.Fatalf("openZIMEntry(%q): %v", books[0].SubFilepath, err)
	}
	if len(buf) < 4 || string(buf[:2]) != "PK" {
		t.Fatalf("book blob is not a zip/epub (head=%x)", buf[:min(4, len(buf))])
	}
}

// A truncated/incomplete ZIM (e.g. a still-copying or interrupted download) must
// surface as an error rather than yielding a partial book list that indexZIM
// would persist and IsProcessed would then treat as permanently complete.
func TestReadZIMBooksTruncatedFails(t *testing.T) {
	full, err := os.ReadFile(zimFixture)
	if err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(t.TempDir(), "truncated.zim")
	if err := os.WriteFile(truncated, full[:len(full)/2], 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readZIMBooks(truncated); err == nil {
		t.Fatal("expected an error for a truncated ZIM; a partial archive would be committed as complete")
	}
}

func TestServiceIndexesZIM(t *testing.T) {
	books, err := readZIMBooks(zimFixture)
	if err != nil {
		t.Fatalf("readZIMBooks: %v", err)
	}
	token, want := searchableToken(books)
	if token == "" {
		t.Fatalf("fixture has no book with a searchable title word")
	}

	store, _ := newTestStore(t)

	// Copy the fixture into an isolated dir so the scanner sees only the ZIM.
	tempDir := t.TempDir()
	copyFile(t, zimFixture, filepath.Join(tempDir, "gutenberg.zim"))

	svc, err := NewService(tempDir, store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := svc.Scan(ctx); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var found *BookMetadata
	eventually(t, 10*time.Second, 200*time.Millisecond, func() (bool, error) {
		res, err := svc.Search(ctx, token)
		if err != nil {
			return false, err
		}
		for _, b := range res {
			if b.Title == want {
				found = b
				return true, nil
			}
		}
		return false, nil
	})

	reader, cleanup, err := svc.GetBook(ctx, found.ID, EPUB)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	defer cleanup()
	head := make([]byte, 4)
	if _, err := io.ReadFull(reader, head); err != nil {
		t.Fatalf("read book: %v", err)
	}
	if string(head[:2]) != "PK" {
		t.Fatalf("served book is not an epub (head=%x)", head)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %q: %v", src, err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("write %q: %v", dst, err)
	}
}

// searchableToken finds a book whose title contains an ASCII-letter word of at
// least 4 characters and returns that word plus the book's full title.
func searchableToken(books []*BookMetadata) (token, title string) {
	for _, b := range books {
		for _, word := range strings.FieldsFunc(b.Title, func(r rune) bool { return !unicode.IsLetter(r) }) {
			if len(word) >= 4 && isASCIILetters(word) {
				return word, b.Title
			}
		}
	}
	return "", ""
}

func isASCIILetters(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
