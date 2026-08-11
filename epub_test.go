package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	migrate "github.com/rubenv/sql-migrate"
)

const testContainerXML = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`

const testOPF = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Pride and Prejudice</dc:title>
    <dc:creator opf:file-as="Austen, Jane" opf:role="aut">Jane Austen</dc:creator>
    <dc:language>en</dc:language>
    <dc:subject>Fiction</dc:subject>
    <dc:subject>Romance</dc:subject>
    <dc:date>1813</dc:date>
    <dc:description>A classic novel.</dc:description>
    <meta name="cover" content="cover-img"/>
  </metadata>
  <manifest>
    <item id="cover-img" href="cover.jpg" media-type="image/jpeg"/>
  </manifest>
</package>`

// buildTestEPUB returns the bytes of a minimal but valid .epub built from the
// given OPF package document.
func buildTestEPUB(t *testing.T, opf string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	write := func(name, content string) {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		if _, err := io.WriteString(f, content); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}

	write("mimetype", "application/epub+zip")
	write("META-INF/container.xml", testContainerXML)
	write("content.opf", opf)

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestReadEPUBMetadata(t *testing.T) {
	md, err := readEPUBMetadataFromBytes(buildTestEPUB(t, testOPF))
	if err != nil {
		t.Fatalf("readEPUBMetadataFromBytes: %v", err)
	}

	checks := map[string]struct{ got, want string }{
		"title":      {md.Title, "Pride and Prejudice"},
		"last name":  {md.AuthorLastName, "Austen"},
		"first name": {md.AuthorFirstName, "Jane"},
		"language":   {md.Language, "en"},
		"genre":      {md.Genre, "Fiction, Romance"},
		"date":       {md.Date, "1813"},
		"annotation": {md.Annotation, "A classic novel."},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
	if !md.HasCover {
		t.Errorf("HasCover = false, want true")
	}
}

func TestSplitAuthor(t *testing.T) {
	cases := []struct {
		name      string
		author    opfAuthor
		wantLast  string
		wantFirst string
	}{
		{"file-as comma", opfAuthor{FileAs: "Austen, Jane", Value: "Jane Austen"}, "Austen", "Jane"},
		{"display name only", opfAuthor{Value: "Mark Twain"}, "Twain", "Mark"},
		{"single token", opfAuthor{Value: "Homer"}, "Homer", ""},
		{"comma in value", opfAuthor{Value: "Twain, Mark"}, "Twain", "Mark"},
		{"empty", opfAuthor{}, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			last, first := splitAuthor(c.author)
			if last != c.wantLast || first != c.wantFirst {
				t.Errorf("splitAuthor(%+v) = (%q, %q), want (%q, %q)", c.author, last, first, c.wantLast, c.wantFirst)
			}
		})
	}
}

// TestServiceIndexesEPUB verifies an .epub on disk is scanned, indexed and then
// served back unchanged (EPUB -> EPUB is a passthrough, so no ebook-convert is
// required).
func TestServiceIndexesEPUB(t *testing.T) {
	db := sqlx.MustConnect("sqlite3", ":memory:")
	store := NewSqliteStore(db)
	if _, err := migrate.Exec(db.DB, "sqlite3", &migrate.FileMigrationSource{Dir: "./db/migrations"}, migrate.Up); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	tempDir := t.TempDir()
	epubBytes := buildTestEPUB(t, testOPF)
	if err := os.WriteFile(filepath.Join(tempDir, "pride.epub"), epubBytes, 0o644); err != nil {
		t.Fatalf("write epub: %v", err)
	}

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

	var book *BookMetadata
	eventually(t, 2*time.Second, 100*time.Millisecond, func() (bool, error) {
		res, err := svc.Search(ctx, "Austen")
		if err != nil {
			return false, err
		}
		if len(res) == 1 {
			book = res[0]
			return true, nil
		}
		return false, nil
	})

	if book.FileType != EPUB {
		t.Fatalf("FileType = %q, want %q", book.FileType, EPUB)
	}

	reader, cleanup, err := svc.GetBook(ctx, book.ID, EPUB)
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
