package catalog

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
const ZIM FileType = "zim"

// supportedExtensions are the file suffixes the scanner will index.
var supportedExtensions = []string{".fb2", ".zip", ".epub", ".zim"}

func isSupportedFile(path string) bool {
	for _, ext := range supportedExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

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

// Scan recursively extracts metadata from all supported files (.fb2, .zip, .epub, .zim) in the directory and stores them in the database.
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

// GetBook returns a file reader for a book with id in outFormat format.
// If the stored source is already in outFormat it is streamed as-is, otherwise
// it is converted (via ebook-convert).
func (s *Service) GetBook(ctx context.Context, id int64, outFormat FileType) (
	file io.Reader,
	cleanup func(),
	err error,
) {
	noop := func() {}

	if outFormat != EPUB && outFormat != FB2 {
		return nil, noop, fmt.Errorf("unsupported output format %q", outFormat)
	}

	bookMetadata, err := s.store.GetBook(ctx, id)
	if err != nil {
		return nil, noop, fmt.Errorf("error getting book metadata: %w", err)
	}

	source, sourceFormat, closeSource, err := s.openSource(bookMetadata)
	if err != nil {
		return nil, closeSource, err
	}

	if outFormat == sourceFormat {
		return source, closeSource, nil
	}

	converted, removeConverted, err := s.convertBook(ctx, source, sourceFormat, outFormat)
	cleanup = func() {
		removeConverted()
		closeSource()
	}
	if err != nil {
		return nil, cleanup, err
	}
	return converted, cleanup, nil
}

// openSource returns a reader for the raw book bytes together with the format
// they are in. For books that live inside a container (a .zip or a .zim) the
// entry is read fully into memory so the container handle can be closed
// immediately; book files are small enough for this to be cheap.
func (s *Service) openSource(md *BookMetadata) (reader io.Reader, format FileType, cleanup func(), err error) {
	cleanup = func() {}

	switch md.FileType {
	case FB2, EPUB:
		f, err := os.Open(md.FilePath)
		if err != nil {
			return nil, "", cleanup, fmt.Errorf("error opening file %q: %w", md.FilePath, err)
		}
		return f, md.FileType, func() { f.Close() }, nil
	case ZIP:
		buf, name, err := readZipEntry(md.FilePath, md.SubFilepath)
		if err != nil {
			return nil, "", cleanup, err
		}
		return bytes.NewReader(buf), formatFromExt(name), cleanup, nil
	case ZIM:
		buf, err := openZIMEntry(md.FilePath, md.SubFilepath)
		if err != nil {
			return nil, "", cleanup, err
		}
		return bytes.NewReader(buf), EPUB, cleanup, nil
	default:
		return nil, "", cleanup, fmt.Errorf("unknown file type %q", md.FileType)
	}
}

// readZipEntry reads a single named entry from a zip archive into memory and
// returns its bytes and name.
func readZipEntry(zipPath, subPath string) ([]byte, string, error) {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", fmt.Errorf("error opening zip path %q: %w", zipPath, err)
	}
	defer z.Close()

	for _, f := range z.File {
		if f.Name != subPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, "", fmt.Errorf("error opening file %q in zip archive %q: %w", f.Name, zipPath, err)
		}
		defer rc.Close()
		buf, err := io.ReadAll(rc)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file %q in zip archive %q: %w", f.Name, zipPath, err)
		}
		return buf, f.Name, nil
	}
	return nil, "", fmt.Errorf("file %q not found in zip archive %q", subPath, zipPath)
}

func formatFromExt(name string) FileType {
	switch {
	case strings.HasSuffix(name, ".epub"):
		return EPUB
	case strings.HasSuffix(name, ".fb2"):
		return FB2
	default:
		return FileType(strings.TrimPrefix(filepath.Ext(name), "."))
	}
}

// convertBook converts reader from inFormat to outFormat using ebook-convert.
// The context bounds the ebook-convert subprocess: if it is cancelled (e.g. the
// caller's deadline elapses), the process is killed instead of running on.
func (s *Service) convertBook(
	ctx context.Context,
	reader io.Reader,
	inFormat, outFormat FileType,
) (
	file io.Reader,
	cleanup func(),
	err error,
) {
	cleanup = func() {}

	// ebook-convert infers the source/target format from the file extension, so
	// the temp names must end in the right one. os.CreateTemp fills the "*" with a
	// unique token, giving us a collision-free name without hand-rolling one.
	sourceTempFile, err := os.CreateTemp("", "bookworm-source-*."+string(inFormat))
	if err != nil {
		return nil, cleanup, fmt.Errorf("error creating temporary file: %w", err)
	}
	sourceTemp := sourceTempFile.Name()
	defer os.Remove(sourceTemp)

	if _, err = io.Copy(sourceTempFile, reader); err != nil {
		sourceTempFile.Close()
		return nil, cleanup, fmt.Errorf("error copying file to temporary file: %w", err)
	}
	sourceTempFile.Close()

	outTemp := sourceTemp + "." + string(outFormat)
	cleanup = func() {
		os.Remove(outTemp)
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ebook-convert", sourceTemp, outTemp)
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
	if !isSupportedFile(path) {
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
			if !isSupportedFile(path) {
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

			switch {
			case strings.HasSuffix(path, ".fb2"):
				s.indexFB2(ctx, path)
			case strings.HasSuffix(path, ".zip"):
				s.indexZip(ctx, path)
			case strings.HasSuffix(path, ".epub"):
				s.indexEPUB(ctx, path)
			case strings.HasSuffix(path, ".zim"):
				s.indexZIM(ctx, path)
			}
		}
	}
}

func (s *Service) indexFB2(ctx context.Context, path string) {
	r, err := os.Open(path)
	if err != nil {
		log.Printf("[service] error opening file %q: %v\n", path, err)
		return
	}
	defer r.Close()

	metadata, err := s.readFB2Metadata(r)
	if err != nil {
		log.Printf("[service] error reading metadata from file %q: %v\n", path, err)
		return
	}

	metadata.FileType = FB2
	metadata.FilePath = path

	if err := s.store.Store(ctx, []*BookMetadata{metadata}); err != nil {
		log.Printf("[service] error storing metadata from file %q: %v\n", path, err)
	}
}

func (s *Service) indexZip(ctx context.Context, path string) {
	z, err := zip.OpenReader(path)
	if err != nil {
		log.Printf("[service] error opening zip path %q: %v\n", path, err)
		return
	}
	defer z.Close()

	var mds []*BookMetadata
	var mu sync.Mutex
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
			defer r.Close()

			metadata, err := s.readFB2Metadata(r)
			if err != nil {
				log.Printf("[service] error reading metadata from file %s in zip archive %s: %v\n", f.Name, path, err)
				return
			}

			metadata.FileType = ZIP
			metadata.FilePath = path
			metadata.SubFilepath = f.Name

			mu.Lock()
			mds = append(mds, metadata)
			mu.Unlock()
		}(f)
	}

	wg.Wait()

	log.Printf("[service] storing %d metadatas from %s\n", len(mds), path)

	if len(mds) > 0 {
		if err := s.store.Store(ctx, mds); err != nil {
			log.Printf("[service] ERROR storing metadata from file %q: %v\n", path, err)
		}
	}
}

func (s *Service) indexEPUB(ctx context.Context, path string) {
	metadata, err := readEPUBMetadataFromFile(path)
	if err != nil {
		log.Printf("[service] error reading metadata from epub %q: %v\n", path, err)
		return
	}

	metadata.FileType = EPUB
	metadata.FilePath = path

	if err := s.store.Store(ctx, []*BookMetadata{metadata}); err != nil {
		log.Printf("[service] error storing metadata from epub %q: %v\n", path, err)
	}
}

func (s *Service) indexZIM(ctx context.Context, path string) {
	mds, err := readZIMBooks(path)
	if err != nil {
		log.Printf("[service] error reading zim %q: %v\n", path, err)
		return
	}

	log.Printf("[service] storing %d books from zim %s\n", len(mds), path)

	if len(mds) > 0 {
		if err := s.store.Store(ctx, mds); err != nil {
			log.Printf("[service] ERROR storing metadata from zim %q: %v\n", path, err)
		}
	}
}

func (s *Service) readFB2Metadata(r io.ReadCloser) (*BookMetadata, error) {
	br := bufio.NewReaderSize(r, 1024)
	parser := xmlparser.NewXMLParser(br, "title-info").EnableXpath()

	m := &BookMetadata{}
	// Keep draining the (buffered) channel even after an error so the parser
	// goroutine can finish and does not leak; surface the first error afterwards.
	var parseErr error
	for xml := range parser.Stream() {
		if xml.Err != nil {
			if parseErr == nil {
				parseErr = xml.Err
			}
			continue
		}
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

	if parseErr != nil {
		return nil, fmt.Errorf("parsing fb2: %w", parseErr)
	}
	// A well-formed FB2 always carries a book-title in its title-info; an empty
	// title means we didn't actually parse a book, so refuse it rather than
	// persisting an empty row that pollutes search results.
	if m.Title == "" {
		return nil, fmt.Errorf("fb2 has no book title (unparseable or not an fb2)")
	}

	return m, nil
}

func (s *Service) startFSWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("can't create watcher: %w", err)
	}

	debounceDuration := time.Millisecond * 500
	// debounceTimers is touched both here (in the watcher goroutine) and from the
	// AfterFunc callbacks below, which fire on their own goroutines, so it must be
	// guarded: a concurrent map write would otherwise crash the process.
	var debounceMu sync.Mutex
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
					name := event.Name
					debounceMu.Lock()
					if timer, exists := debounceTimers[name]; exists {
						timer.Reset(debounceDuration)
					} else {
						debounceTimers[name] = time.AfterFunc(debounceDuration, func() {
							s.processFile(name)
							debounceMu.Lock()
							delete(debounceTimers, name)
							debounceMu.Unlock()
						})
					}
					debounceMu.Unlock()
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
