package main

import (
	"archive/zip"
	"context"
	"github.com/jmoiron/sqlx"
	migrate "github.com/rubenv/sql-migrate"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestService(t *testing.T) {
	mkSvc := func() (tempDir string, svc *Service) {
		db := sqlx.MustConnect("sqlite3", ":memory:")
		store := NewSqliteStore(db)

		migrations := &migrate.FileMigrationSource{
			Dir: "./db/migrations",
		}
		_, err := migrate.Exec(db.DB, "sqlite3", migrations, migrate.Up)
		if err != nil {
			t.Fatalf("failed to apply migrations: %v", err)
		}

		tempDir = t.TempDir()

		svc, err = NewService(tempDir, store)
		if err != nil {
			t.Fatalf("can't create service: %v", err)
		}

		return tempDir, svc
	}

	tempDir, svc := mkSvc()

	// region Test that files already present in the directory appeared in the index after .Scan()
	createTestZipArchive(t, path.Join(tempDir, "books.zip"), "The Iliad - Homer - FB2.fb2")

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
	createTestZipArchive(t, path.Join(tempDir, "books2.zip"), "On Dreams - Aristotle - FB2.fb2")

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

func createTestZipArchive(t *testing.T, path string, files ...string) {
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
