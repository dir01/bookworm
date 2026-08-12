package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	zim "github.com/tim-st/go-zim"
)

// epubMimetype is the ZIM directory-entry mimetype for a Project Gutenberg book.
const epubMimetype = "application/epub+zip"

// zimReader hands out cached *zim.File handles keyed by path.
//
// tim-st/go-zim's File.Close only closes the underlying os.File; it never closes
// the zstd.Decoder that Open creates (and whose worker goroutines stay alive), so
// opening a ZIM per request would leak goroutines and memory in a long-running
// process. We instead open each ZIM once and reuse it, which bounds the leak to a
// single decoder per file for the process lifetime. A *zim.File is not safe for
// concurrent use (reads seek a shared file handle and reset a shared decoder), so
// access to each handle is serialized.
type zimReader struct {
	mu    sync.Mutex
	files map[string]*zimHandle
}

type zimHandle struct {
	mu sync.Mutex
	f  *zim.File
}

func newZimReader() *zimReader {
	return &zimReader{files: make(map[string]*zimHandle)}
}

// with runs fn against the cached handle for path (opening it on first use),
// holding that handle's lock for the duration.
func (zr *zimReader) with(path string, fn func(*zim.File) error) error {
	zr.mu.Lock()
	h, ok := zr.files[path]
	if !ok {
		f, err := zim.Open(path)
		if err != nil {
			zr.mu.Unlock()
			return fmt.Errorf("error opening zim %q: %w", path, err)
		}
		if err := checkZIMComplete(path, f); err != nil {
			f.Close()
			zr.mu.Unlock()
			return err
		}
		h = &zimHandle{f: f}
		zr.files[path] = h
	}
	zr.mu.Unlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	return fn(h.f)
}

// checkZIMComplete guards against a still-copying or truncated archive. The ZIM
// header records the file's full size (the checksum sits at the very end), so a
// file shorter than that on disk is incomplete. go-zim itself does not validate
// this and would otherwise enumerate a partial (or empty) set of entries without
// error, which the caller would then persist as if the archive were complete.
func checkZIMComplete(path string, f *zim.File) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if expected := int64(f.Filesize()); fi.Size() < expected {
		return fmt.Errorf("zim %q is incomplete: %d of %d bytes on disk", path, fi.Size(), expected)
	}
	return nil
}

// books enumerates every EPUB entry in a ZIM archive (e.g. a Project Gutenberg
// library) and returns one BookMetadata per book. FileType, FilePath and
// SubFilepath are populated so the entry can be reopened on download.
//
// A failure to read/decompress an entry's blob is treated as archive corruption
// (e.g. a truncated or still-copying file) and returned as an error, so the
// caller does not persist a partial archive that IsProcessed would then treat as
// permanently complete. An individual EPUB whose metadata cannot be parsed is a
// per-book data problem and is skipped, so one bad book does not block the rest.
func (zr *zimReader) books(path string) ([]*BookMetadata, error) {
	var mds []*BookMetadata

	err := zr.with(path, func(z *zim.File) error {
		mimes := z.MimetypeList()
		for i := range z.ArticleCount() {
			e, err := z.EntryAtURLPosition(i)
			if err != nil {
				return fmt.Errorf("zim: reading entry %d in %q: %w", i, path, err)
			}
			if e.IsRedirect() {
				continue
			}
			if mt := e.Mimetype(); int(mt) >= len(mimes) || mimes[mt] != epubMimetype {
				continue
			}

			r, _, err := z.BlobReader(&e)
			if err != nil {
				return fmt.Errorf("zim: reading blob for %q in %q: %w", string(e.URL()), path, err)
			}
			buf, err := io.ReadAll(r)
			if err != nil {
				return fmt.Errorf("zim: reading epub bytes for %q in %q: %w", string(e.URL()), path, err)
			}

			md, err := readEPUBMetadataFromBytes(buf)
			if err != nil {
				log.Printf("[zim] skipping unreadable epub %q in %q: %v", string(e.URL()), path, err)
				continue
			}
			md.FileType = ZIM
			md.FilePath = path
			md.SubFilepath = zimEntryRef(&e)
			mds = append(mds, md)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return mds, nil
}

// entry reopens the EPUB blob for a stored entry reference and returns its bytes.
func (zr *zimReader) entry(path, ref string) ([]byte, error) {
	ns, url := parseZimEntryRef(ref)

	var out []byte
	err := zr.with(path, func(z *zim.File) error {
		entry, _, found := z.EntryWithURL(zim.Namespace(ns), []byte(url))
		if !found {
			return fmt.Errorf("zim: entry %q not found in %q", ref, path)
		}
		if entry.IsRedirect() {
			var err error
			if entry, err = z.FollowRedirect(&entry); err != nil {
				return fmt.Errorf("zim: following redirect for %q: %w", ref, err)
			}
		}
		r, _, err := z.BlobReader(&entry)
		if err != nil {
			return fmt.Errorf("zim: reading blob for %q: %w", ref, err)
		}
		out, err = io.ReadAll(r)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// zimEntryRef encodes an entry's namespace and URL as "<namespace>/<url>" so it
// can be looked up again later, independent of its position in the archive.
func zimEntryRef(e *zim.DirectoryEntry) string {
	return fmt.Sprintf("%s/%s", e.Namespace().String(), string(e.URL()))
}

func parseZimEntryRef(ref string) (namespace byte, url string) {
	// The namespace is a single character followed by '/', then the URL (which
	// may itself contain '/').
	if i := strings.IndexByte(ref, '/'); i > 0 {
		return ref[0], ref[i+1:]
	}
	return 'C', ref
}
