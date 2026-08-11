package main

import (
	"fmt"
	"io"
	"log"
	"strings"

	zim "github.com/tim-st/go-zim"
)

// epubMimetype is the ZIM directory-entry mimetype for a Project Gutenberg book.
const epubMimetype = "application/epub+zip"

// readZIMBooks enumerates every EPUB entry in a ZIM archive (e.g. a Project
// Gutenberg library) and returns one BookMetadata per book. FileType, FilePath
// and SubFilepath are populated so the entry can be reopened on download.
func readZIMBooks(path string) ([]*BookMetadata, error) {
	z, err := zim.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening zim %q: %w", path, err)
	}
	defer z.Close()

	mimes := z.MimetypeList()

	// The library seeks a single shared file handle per read, so entries must be
	// read sequentially rather than concurrently.
	var mds []*BookMetadata
	for i := range z.ArticleCount() {
		e, err := z.EntryAtURLPosition(i)
		if err != nil {
			continue
		}
		if e.IsRedirect() {
			continue
		}
		if mt := e.Mimetype(); int(mt) >= len(mimes) || mimes[mt] != epubMimetype {
			continue
		}

		r, _, err := z.BlobReader(&e)
		if err != nil {
			log.Printf("[zim] error reading blob for %q in %q: %v", string(e.URL()), path, err)
			continue
		}
		buf, err := io.ReadAll(r)
		if err != nil {
			log.Printf("[zim] error reading epub bytes for %q in %q: %v", string(e.URL()), path, err)
			continue
		}

		md, err := readEPUBMetadataFromBytes(buf)
		if err != nil {
			log.Printf("[zim] error reading metadata for %q in %q: %v", string(e.URL()), path, err)
			continue
		}
		md.FileType = ZIM
		md.FilePath = path
		md.SubFilepath = zimEntryRef(&e)
		mds = append(mds, md)
	}

	return mds, nil
}

// openZIMEntry reopens the EPUB blob for a stored entry reference and returns
// its bytes.
func openZIMEntry(path, ref string) ([]byte, error) {
	ns, url := parseZimEntryRef(ref)

	z, err := zim.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening zim %q: %w", path, err)
	}
	defer z.Close()

	entry, _, found := z.EntryWithURL(zim.Namespace(ns), []byte(url))
	if !found {
		return nil, fmt.Errorf("zim: entry %q not found in %q", ref, path)
	}
	if entry.IsRedirect() {
		if entry, err = z.FollowRedirect(&entry); err != nil {
			return nil, fmt.Errorf("zim: following redirect for %q: %w", ref, err)
		}
	}

	r, _, err := z.BlobReader(&entry)
	if err != nil {
		return nil, fmt.Errorf("zim: reading blob for %q: %w", ref, err)
	}
	return io.ReadAll(r)
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
