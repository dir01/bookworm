package catalog

import (
	"context"
	"fmt"
	"net/url"
	"runtime"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// NewSqliteStore opens the books database at dbPath with two connection pools:
//
//   - a write pool limited to a single connection, so writers are serialized by
//     the driver (no application-level mutex) and never raise SQLITE_BUSY against
//     each other;
//   - a read pool with several connections for concurrent Search/GetBook lookups.
//
// The database runs in WAL mode (see sqliteDSN), under which readers do not block
// the writer and the writer does not block readers. This is what decouples a long
// Store transaction (e.g. indexing a whole ZIM archive) from concurrent searches:
// with the previous single shared connection guarded by one mutex, every read
// waited behind the in-flight write.
func NewSqliteStore(dbPath string) (*SqliteStore, error) {
	// The write pool takes an immediate lock on BEGIN so a transaction that
	// starts reading and later writes cannot deadlock against itself on upgrade.
	writeDB, err := sqlx.Connect("sqlite3", sqliteDSN(dbPath, true))
	if err != nil {
		return nil, fmt.Errorf("opening write db %q: %w", dbPath, err)
	}
	writeDB.SetMaxOpenConns(1)

	readDB, err := sqlx.Connect("sqlite3", sqliteDSN(dbPath, false))
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("opening read db %q: %w", dbPath, err)
	}
	readers := max(4, runtime.NumCPU())
	readDB.SetMaxOpenConns(readers)
	readDB.SetMaxIdleConns(readers)

	return &SqliteStore{write: writeDB, read: readDB}, nil
}

// sqliteDSN builds a mattn/go-sqlite3 DSN with the pragmas we rely on. WAL plus a
// busy timeout gives us reader/writer concurrency; immediate transaction locking
// is used only for the write pool.
func sqliteDSN(path string, immediate bool) string {
	q := url.Values{}
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "NORMAL")
	q.Set("_busy_timeout", "5000")
	q.Set("_foreign_keys", "on")
	if immediate {
		q.Set("_txlock", "immediate")
	}
	return "file:" + path + "?" + q.Encode()
}

type SqliteStore struct {
	write *sqlx.DB
	read  *sqlx.DB
}

// Close releases both connection pools.
func (s *SqliteStore) Close() error {
	werr := s.write.Close()
	rerr := s.read.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

func (s *SqliteStore) Store(ctx context.Context, metadatas []*BookMetadata) error {
	tx, err := s.write.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	// avoid "too many SQL variables" error, but still keep transaction
	// partial inserts are no-go because we check if file is already processed
	// by checking if there is at least one row in the table for a given file
	batchSize := 500

	for i := 0; i < len(metadatas); i += batchSize {
		end := min(i+batchSize, len(metadatas))
		if err := s.insertMetadatasBatch(tx, metadatas[i:end]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("can't store batch (%d/%d): %w", i, len(metadatas), err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (s *SqliteStore) insertMetadatasBatch(tx *sqlx.Tx, metadatas []*BookMetadata) error {
	if len(metadatas) == 0 {
		return nil
	}

	res, err := tx.NamedExec(`
        INSERT INTO books (
            file_type,
            file_path,
            sub_filepath,
            title,
            author_last_name,
            author_first_name,
            annotation,
            genre,
            date,
            language,
            has_cover
        ) VALUES (
            :file_type,
            :file_path,
            :sub_filepath,
            :title,
            :author_last_name,
            :author_first_name,
            :annotation,
            :genre,
            :date,
            :language,
            :has_cover
        )`, metadatas)
	if err != nil {
		return err
	}

	// Mirror exactly the rows this batch just inserted into the FTS index. A
	// single multi-row INSERT assigns contiguous rowids ending at LastInsertId,
	// so the batch spans [lastID-len+1, lastID]. Selecting by that rowid range
	// (rather than by file_path) avoids re-indexing earlier batches of the same
	// archive, which previously duplicated FTS rows once an archive exceeded
	// batchSize.
	lastID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	firstID := lastID - int64(len(metadatas)) + 1

	_, err = tx.Exec(`
        INSERT INTO books_fts (
            id,
            title,
            author_last_name,
            author_first_name
        ) SELECT
            id,
            title,
            author_last_name,
            author_first_name
        FROM books WHERE id BETWEEN ? AND ?`, firstID, lastID)
	if err != nil {
		return err
	}

	return nil
}

func (s *SqliteStore) IsProcessed(ctx context.Context, path string) (bool, error) {
	var exists bool
	err := s.read.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM books WHERE file_path = ?)", path)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (s *SqliteStore) Search(ctx context.Context, term string) ([]*BookMetadata, error) {
	var metadatas []*BookMetadata
	err := s.read.SelectContext(ctx, &metadatas, `
		SELECT books.* FROM books_fts
		JOIN books ON books.id = books_fts.id
		WHERE books_fts MATCH $1;
	`, term)
	if err != nil {
		return nil, err
	}

	return metadatas, nil
}

func (s *SqliteStore) GetBook(ctx context.Context, id int64) (*BookMetadata, error) {
	var book BookMetadata
	err := s.read.GetContext(ctx, &book, "SELECT * FROM books WHERE id = ?", id)
	if err != nil {
		return nil, err
	}

	return &book, nil
}
