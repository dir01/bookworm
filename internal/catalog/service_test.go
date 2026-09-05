package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"
)

func TestService(t *testing.T) {
	mkSvc := func() (tempDir string, svc *Service) {
		store, _ := newTestStore(t)

		tempDir = t.TempDir()

		var err error
		svc, err = NewService(tempDir, store)
		if err != nil {
			t.Fatalf("can't create service: %v", err)
		}

		return tempDir, svc
	}

	tempDir, svc := mkSvc()

	// region Test that files already present in the directory appeared in the index after .Scan()
	createTestZIPArchive(t, path.Join(tempDir, "books.zip"), "The Iliad - Homer - FB2.fb2")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Run(ctx); err != nil {
		t.Fatalf("can't run service: %v", err)
	}

	if err := svc.Scan(ctx); err != nil {
		t.Fatalf("can't scan dir: %v", err)
	}

	eventually(t, 1*time.Second, 100*time.Millisecond, func() (bool, error) {
		if results, err := svc.Search(ctx, "iliad"); err != nil {
			return false, err
		} else {
			return len(results) == 1, nil
		}
	})
	// endregion

	// region Test that files added after .Scan() appeared in the index as well (using fsnotify watcher)
	createTestZIPArchive(t, path.Join(tempDir, "books2.zip"), "On Dreams - Aristotle - FB2.fb2")

	eventually(t, 5*time.Second, 100*time.Millisecond, func() (bool, error) {
		if results, err := svc.Search(ctx, "Aristotle"); err != nil {
			return false, err
		} else {
			return len(results) == 1, nil
		}
	})
	// endregion

	// region  Test that plain .fb2 files are also indexed
	copyTestFB2(t, tempDir, "The Republic of Plato - FB2.fb2")

	eventually(t, 5*time.Second, 100*time.Millisecond, func() (bool, error) {
		if results, err := svc.Search(ctx, "The Republic"); err != nil {
			return false, err
		} else {
			return len(results) == 1, nil
		}
	})
	// endregion

	// region Test ability to get converted book
	if results, err := svc.Search(ctx, "The Republic"); err != nil {
		t.Fatalf("error searching The Republic: %v", err)
	} else {
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		file, cleanup, err := svc.GetBook(ctx, results[0].ID, EPUB)
		if err != nil {
			t.Fatalf("error getting converted book: %v", err)
		}
		defer cleanup()
		if file == nil {
			t.Fatalf("expected file, got nil")
		}
	}
	// endregion

	// region Test ability to get converted books from zip archive
	if results, err := svc.Search(ctx, "Aristotle"); err != nil {
		t.Fatalf("error searching Aristotle: %v", err)
	} else {
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		file, cleanup, err := svc.GetBook(ctx, results[0].ID, EPUB)
		if err != nil {
			t.Fatalf("error getting converted book: %v", err)
		}
		defer cleanup()
		if file == nil {
			t.Fatalf("expected file, got nil")
		}
	} // endregion
}

// TestServiceIndexesMixedZIP verifies a .zip archive is indexed by content, not
// by a single hard-coded entry type: an .epub and an .fb2 sharing one archive
// are both extracted. This is the "any leaf format in any container" contract.
func TestServiceIndexesMixedZIP(t *testing.T) {
	store, _ := newTestStore(t)

	tempDir := t.TempDir()

	epubBytes := buildTestEPUB(t, testOPF) // "Pride and Prejudice", Jane Austen
	fb2Bytes, err := os.ReadFile(filepath.Join("testdata", "The Iliad - Homer - FB2.fb2"))
	if err != nil {
		t.Fatalf("read fb2 fixture: %v", err)
	}

	writeZIP(t, filepath.Join(tempDir, "mixed.zip"), map[string][]byte{
		"pride.epub": epubBytes,
		"iliad.fb2":  fb2Bytes,
	})

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

	// The EPUB entry is the new capability: previously indexZIP only looked at .fb2.
	var epubBook *BookMetadata
	eventually(t, 2*time.Second, 100*time.Millisecond, func() (bool, error) {
		res, err := svc.Search(ctx, "Austen")
		if err != nil {
			return false, err
		}
		if len(res) == 1 {
			epubBook = res[0]
			return true, nil
		}
		return false, nil
	})

	if epubBook.FileType != ZIP {
		t.Fatalf("FileType = %q, want %q", epubBook.FileType, ZIP)
	}
	if epubBook.SubFilepath != "pride.epub" {
		t.Fatalf("SubFilepath = %q, want %q", epubBook.SubFilepath, "pride.epub")
	}

	// The .fb2 sharing the archive is still indexed too.
	eventually(t, 2*time.Second, 100*time.Millisecond, func() (bool, error) {
		res, err := svc.Search(ctx, "Homer")
		if err != nil {
			return false, err
		}
		return len(res) == 1, nil
	})

	// The epub is served back byte-for-byte (EPUB -> EPUB is a passthrough).
	reader, cleanup, err := svc.GetBook(ctx, epubBook.ID, EPUB)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	defer cleanup()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read book: %v", err)
	}
	if !bytes.Equal(got, epubBytes) {
		t.Fatalf("served epub differs from source (got %d bytes, want %d)", len(got), len(epubBytes))
	}
}

func writeZIP(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()

	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip %q: %v", path, err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	for name, content := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func eventually(t *testing.T, ttl time.Duration, tick time.Duration, f func() (bool, error)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for condition: %v", ctx.Err())
		case <-time.After(tick):
			ok, err := f()
			if err != nil {
				t.Fatalf("error checking condition: %v", err)
			}
			if ok {
				return
			}
		}
	}
}

func copyTestFB2(t *testing.T, destDir string, files ...string) {
	t.Helper()

	for _, file := range files {
		src := filepath.Join("testdata", file)
		dest := filepath.Join(destDir, file)
		err := os.Link(src, dest)
		if err != nil {
			t.Fatalf("Failed to link file: %v", err)
		}
	}
}

func createTestZIPArchive(t *testing.T, path string, files ...string) {
	t.Helper()

	for i, file := range files {
		files[i] = filepath.Join("testdata", file)
	}

	outFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer outFile.Close()

	// Create a new zip archive
	w := zip.NewWriter(outFile)

	// Add files to zip
	for _, file := range files {
		f, err := w.Create(file)
		if err != nil {
			t.Fatalf("Failed to add file to zip: %v", err)
		}
		bytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		_, err = f.Write(bytes)
		if err != nil {
			t.Fatalf("Failed to write file to zip: %v", err)
		}
	}

	// Close the archive
	err = w.Close()
	if err != nil {
		t.Fatalf("Failed to close zip writer: %v", err)
	}
}
