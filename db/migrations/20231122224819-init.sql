-- +migrate Up
CREATE TABLE books
(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_type TEXT,
    file_path TEXT,
    sub_filepath TEXT,
    title TEXT,
    author_last_name TEXT,
    author_first_name TEXT,
    annotation TEXT,
    genre TEXT,
    date TEXT,
    language TEXT,
    has_cover INTEGER
);
-- For IsProcessed lookups
CREATE INDEX books_file_path_index ON books (file_path);
-- For Search lookups
CREATE VIRTUAL TABLE books_fts USING fts5(id UNINDEXED, title, author_last_name, author_first_name);

-- +migrate Down
DROP TABLE books;
DROP TABLE books_fts;
