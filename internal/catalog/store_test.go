package catalog

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	migrate "github.com/rubenv/sql-migrate"
)

// newTestStore creates a store backed by a fresh on-disk database (WAL mode needs
// a real file — two pools against ":memory:" would see two separate databases)
// and returns it alongside a plain connection to the same file for raw
// assertions. Both are closed when the test finishes.
func newTestStore(t *testing.T) (*SQLiteStore, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "books.db")

	db := sqlx.MustConnect("sqlite3", sqliteDSN(dbPath, false))
	if _, err := migrate.Exec(db.DB, "sqlite3", &migrate.FileMigrationSource{Dir: "../../db/migrations"}, migrate.Up); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
		_ = db.Close()
	})
	return store, db
}

// A large archive (e.g. a Project Gutenberg ZIM) is stored in a single Store
// call whose batch is split into chunks of batchSize. Regression test: the FTS
// index must contain exactly one row per book, with no duplicates from earlier
// batches, and each book must be found exactly once.
func TestStoreLargeArchiveNoFTSDuplication(t *testing.T) {
	store, db := newTestStore(t)

	const n = 1100 // > batchSize (500): spans three insert batches
	books := make([]*BookMetadata, n)
	for i := range books {
		books[i] = &BookMetadata{
			FileType:       ZIM,
			FilePath:       "gutenberg.zim",
			SubFilepath:    fmt.Sprintf("C/book-%d.epub", i),
			Title:          fmt.Sprintf("Unique Title Number %d", i),
			AuthorLastName: fmt.Sprintf("Author %d", i),
		}
	}

	if err := store.Store(context.Background(), books); err != nil {
		t.Fatalf("Store: %v", err)
	}

	var nbooks, nfts int
	if err := db.Get(&nbooks, "SELECT count(*) FROM books"); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&nfts, "SELECT count(*) FROM books_fts"); err != nil {
		t.Fatal(err)
	}
	if nbooks != n {
		t.Errorf("books count = %d, want %d", nbooks, n)
	}
	if nfts != n {
		t.Errorf("books_fts count = %d, want %d (FTS duplication across batches)", nfts, n)
	}

	// A book from the first batch must be found exactly once (not once per later batch).
	res, err := store.Search(context.Background(), `"Unique Title Number 7"`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("search for a first-batch title returned %d rows, want 1", len(res))
	}
}
