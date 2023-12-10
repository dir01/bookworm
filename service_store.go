package main

import (
	"context"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"sync"
)

func NewSqliteStore(db *sqlx.DB) *SqliteStore {
	return &SqliteStore{db: db, mutex: sync.Mutex{}}
}

type SqliteStore struct {
	db    *sqlx.DB
	mutex sync.Mutex
}

func (s *SqliteStore) Store(ctx context.Context, metadatas []*BookMetadata) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.NamedExec(`
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
		_ = tx.Rollback()
		return err
	}

	_, err = tx.NamedExec(`
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
        FROM books WHERE file_path = :file_path`, metadatas)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (s *SqliteStore) IsProcessed(ctx context.Context, path string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var exists bool
	err := s.db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM books WHERE file_path = ?)", path)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (s *SqliteStore) Search(ctx context.Context, term string) ([]*BookMetadata, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var metadatas []*BookMetadata
	err := s.db.SelectContext(ctx, &metadatas, `
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
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var book BookMetadata
	err := s.db.GetContext(ctx, &book, "SELECT * FROM books WHERE id = ?", id)
	if err != nil {
		return nil, err
	}

	return &book, nil
}
