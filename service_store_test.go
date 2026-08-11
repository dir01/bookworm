package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	migrate "github.com/rubenv/sql-migrate"
)

func newTestStore(t *testing.T) (*SqliteStore, *sqlx.DB) {
	t.Helper()
	db := sqlx.MustConnect("sqlite3", ":memory:")
	if _, err := migrate.Exec(db.DB, "sqlite3", &migrate.FileMigrationSource{Dir: "./db/migrations"}, migrate.Up); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return NewSqliteStore(db), db
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
