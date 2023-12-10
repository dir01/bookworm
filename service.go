package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/tamerh/xml-stream-parser"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileType string

const FB2 FileType = "fb2"
const ZIP FileType = "zip"
const EPUB FileType = "epub"

type BookMetadata struct {
	// Database ID
	ID int64 `db:"id"`

	// Zip or FB2
	FileType FileType `db:"file_type"`

	// Relative to the directory
	FilePath string `db:"file_path"`

	// Name of file inside zip archive
	SubFilepath string `db:"sub_filepath"`

	Title           string `db:"title"`
	AuthorLastName  string `db:"author_last_name"`
	AuthorFirstName string `db:"author_first_name"`
	Annotation      string `db:"annotation"`
	Genre           string `db:"genre"`
	Date            string `db:"date"`
	Language        string `db:"language"`
	HasCover        bool   `db:"has_cover"`
}

type ServiceStore interface {
	// Store saves a batch of books metadata to database.
	// If books records already exist in database, they will be updated.
	// If books reside in an archive, all books of the archive MUST be updated at once.
	// This is before we rely on existence of any book from archive in the database
	// to determine whether the archive was processed or not (see `IsProcessed`).
	Store(ctx context.Context, metadatas []*BookMetadata) error

	// IsProcessed returns true if a file at path was processed or not
	IsProcessed(ctx context.Context, path string) (bool, error)

	// Search returns a list of books with term in title or author name
	Search(ctx context.Context, term string) ([]*BookMetadata, error)

	// GetBook returns a single book metadata by id
	GetBook(ctx context.Context, id int64) (*BookMetadata, error)
}

func NewService(path string, store ServiceStore) (*Service, error) {
	return &Service{
		dirPath:     path,
		filesChan:   make(chan string),
		fileWorkers: 5,
		store:       store,
	}, nil
}

// Service is in charge of:
// - scanning the dirPath for books and extracting metadata from them (Scan)
// - watching the dirPath for new books during the runtime (Run)
// - searching for books by title or author name (Search)
// - getting a book file reader by book id (GetBook)
type Service struct {
	dirPath     string
	filesChan   chan string
	fileWorkers int
	store       ServiceStore
}

// Run starts the service: starts the file system watcher and required workers
func (s *Service) Run(ctx context.Context) error {
	if err := s.startFSWatcher(ctx); err != nil {
		return fmt.Errorf("can't setup fsnotify: %w", err)
	}

	for i := 0; i < s.fileWorkers; i++ {
		go s.startFileWorker(ctx)
	}

	return nil
}

// Scan recursively extracts metadata from all .zip and .fb2 files in the directory and stores them in the database.
func (s *Service) Scan(ctx context.Context) error {
	return filepath.Walk(s.dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking the path %q: %w", s.dirPath, err)
		}

		if info.IsDir() {
			return nil
		}

		s.processFile(path)

		return nil
	})
}

// Search returns a list of books with term in title or author name
func (s *Service) Search(ctx context.Context, searchTerm string) ([]*BookMetadata, error) {
	if books, err := s.store.Search(ctx, searchTerm); err != nil {
		return nil, fmt.Errorf("error searching for books: %w", err)
	} else {
		return books, nil
	}
}

// GetBook returns a file reader for a book with id in outFormat format
func (s *Service) GetBook(ctx context.Context, id int64, outFormat FileType) (
	file io.Reader,
	cleanup func(),
	err error,
) {
	if outFormat != EPUB && outFormat != FB2 {
		return nil, nil, fmt.Errorf("unsupported output format %q", outFormat)
	}

	bookMetadata, err := s.store.GetBook(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting book metadata: %w", err)
	}

	cleanup = func() {}

	var sourceFile io.Reader
	switch bookMetadata.FileType {
	case FB2:
		sourceFile, err = os.Open(bookMetadata.FilePath)
		if err != nil {
			return nil, cleanup, fmt.Errorf("error opening file %q: %w", bookMetadata.FilePath, err)
		}
	case ZIP:
		z, err := zip.OpenReader(bookMetadata.FilePath)
		if err != nil {
			return nil, cleanup, fmt.Errorf("error opening zip path %q: %w", bookMetadata.FilePath, err)
		}
		for _, f := range z.File {
			if f.Name == bookMetadata.SubFilepath {
				sourceFile, err = f.Open()
				if err != nil {
					return nil, cleanup, fmt.Errorf("error opening file %q in zip archive %q: %w", f.Name, bookMetadata.FilePath, err)
				}
				goto convert // we might need to format the file, so we can't return here
			}
		}
		return nil, cleanup, fmt.Errorf("file %q not found in zip archive %q", bookMetadata.SubFilepath, bookMetadata.FilePath)
	default:
		return nil, cleanup, fmt.Errorf("unknown file type %q", bookMetadata.FileType)
	}

convert:
	switch outFormat {
	case FB2:
		return sourceFile, cleanup, nil
	case EPUB:
		return s.convertBook(sourceFile, EPUB)
	default:
		return nil, cleanup, fmt.Errorf("unsupported output format %q", outFormat)
	}
}

func (s *Service) convertBook(
	reader io.Reader,
	outFormat FileType,
) (
	file io.Reader,
	cleanup func(),
	err error,
) {
	if outFormat != EPUB {
		return nil, nil, fmt.Errorf("only epub format is supported")
	}

	sourceTemp := filepath.Join(os.TempDir(), fmt.Sprintf("source-%d.fb2", time.Now().UnixNano()))
	sourceTempFile, err := os.Create(sourceTemp)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temporary file: %w", err)
	}

	if _, err = io.Copy(sourceTempFile, reader); err != nil {
		return nil, nil, fmt.Errorf("error copying file to temporary file: %w", err)
	}

	outTemp := sourceTemp + ".epub"

	defer os.Remove(sourceTemp)
	cleanup = func() {
		os.Remove(outTemp)
	}

	var stderr bytes.Buffer
	cmd := exec.Command("ebook-convert", sourceTemp, outTemp)
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return nil, cleanup, fmt.Errorf("failed to convert file: %w, %s", err, stderr.String())
	}

	file, err = os.Open(outTemp)
	if err != nil {
		return nil, cleanup, fmt.Errorf("error opening converted file: %w", err)
	}

	return file, cleanup, nil
}

func (s *Service) processFile(path string) {
	if !strings.HasSuffix(path, ".zip") && !strings.HasSuffix(path, ".fb2") {
		return
	}
	s.filesChan <- path
}

func (s *Service) startFileWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("[service] file worker done")
			return
		case path := <-s.filesChan:
			if !strings.HasSuffix(path, ".zip") && !strings.HasSuffix(path, ".fb2") {
				continue
			}

			if seen, err := s.store.IsProcessed(ctx, path); seen || err != nil {
				if err != nil {
					log.Printf("[service] error checking if file %q is processed: %v\n", path, err)
				}
				log.Printf("[service] skipping file %q, already processed\n", path)
				continue
			}

			log.Printf("[service] processing file %q\n", path)

			if strings.HasSuffix(path, ".fb2") {
				r, err := os.Open(path)
				if err != nil {
					log.Printf("[service] error opening file %q: %v\n", path, err)
					continue
				}

				metadata, err := s.readFB2Metadata(r)
				if err != nil {
					log.Printf("[service] error reading metadata from file %q: %v\n", path, err)
					continue
				}

				metadata.FileType = FB2
				metadata.FilePath = path

				if err := s.store.Store(ctx, []*BookMetadata{metadata}); err != nil {
					log.Printf("[service] error storing metadata from file %q: %v\n", path, err)
					continue
				}
			}

			if strings.HasSuffix(path, ".zip") {
				z, err := zip.OpenReader(path)
				if err != nil {
					log.Printf("[service] error opening zip path %q: %v\n", path, err)
					continue
				}

				var mds []*BookMetadata
				var wg sync.WaitGroup

				for _, f := range z.File {
					if !strings.HasSuffix(f.Name, ".fb2") {
						continue
					}

					wg.Add(1)

					go func(f *zip.File) {
						defer wg.Done()

						r, err := f.Open()
						if err != nil {
							log.Printf("[service] error opening file %s in zip archive %s: %v\n", f.Name, path, err)
							return
						}

						metadata, err := s.readFB2Metadata(r)
						if err != nil {
							log.Printf("[service] error reading metadata from file %s in zip archive %s: %v\n", f.Name, path, err)
							return
						}

						metadata.FileType = ZIP
						metadata.FilePath = path
						metadata.SubFilepath = f.Name
						mds = append(mds, metadata)
					}(f)
				}

				log.Printf("[service] waiting for %d workers to finish processing %s\n", len(z.File), path)

				wg.Wait()

				log.Printf("[service] storing %d metadatas from %s\n", len(mds), path)

				if len(mds) > 0 {
					s.store.Store(ctx, mds)
				}

				z.Close()
			}
		}
	}
}

func (s *Service) readFB2Metadata(r io.ReadCloser) (*BookMetadata, error) {
	br := bufio.NewReaderSize(r, 1024)
	parser := xmlparser.NewXMLParser(br, "title-info").EnableXpath()

	m := &BookMetadata{}
	for xml := range parser.Stream() {
		if title, err := xml.SelectElements("/book-title"); err == nil && len(title) > 0 {
			m.Title = title[0].InnerText
		}
		if authorF, err := xml.SelectElements("/author/first-name"); err == nil && len(authorF) > 0 {
			m.AuthorFirstName = authorF[0].InnerText
		}
		if authorL, err := xml.SelectElements("/author/last-name"); err == nil && len(authorL) > 0 {
			m.AuthorLastName = authorL[0].InnerText
		}
		if annotation, err := xml.SelectElements("/annotation"); err == nil && len(annotation) > 0 {
			m.Annotation = annotation[0].InnerText
		}
		if genre, err := xml.SelectElements("/genre"); err == nil && len(genre) > 0 {
			m.Genre = genre[0].InnerText
		}
		if date, err := xml.SelectElements("/date"); err == nil && len(date) > 0 {
			m.Date = date[0].InnerText
		}
		if lang, err := xml.SelectElements("/lang"); err == nil && len(lang) > 0 {
			m.Language = lang[0].InnerText
		}
		if cover, err := xml.SelectElements("/coverpage/image"); err == nil && len(cover) > 0 {
			m.HasCover = cover[0].InnerText != ""
		}
	}

	return m, nil
}

func (s *Service) startFSWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("can't create watcher: %w", err)
	}

	debounceDuration := time.Millisecond * 500
	debounceTimers := make(map[string]*time.Timer)

	go func() {
		defer watcher.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					log.Println("watcher events channel closed")
					return
				}
				log.Printf("[fsnotify] event: %v\n", event)
				mask := fsnotify.Create | fsnotify.Rename | fsnotify.Write
				if event.Op&mask != 0 {
					if timer, exists := debounceTimers[event.Name]; exists {
						timer.Reset(debounceDuration)
					} else {
						debounceTimers[event.Name] = time.AfterFunc(debounceDuration, func() {
							s.processFile(event.Name)
							delete(debounceTimers, event.Name)
						})
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Println("watcher errors channel closed")
					return
				}
				if err != nil {
					log.Printf("error: %v", err)
				}
			}
		}
	}()

	err = watcher.Add(s.dirPath)
	if err != nil {
		return fmt.Errorf("can't add directory %s to watcher: %w", s.dirPath, err)
	}

	return nil
}
