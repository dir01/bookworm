package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"

	zim "github.com/stazelabs/gozim/zim"
)

// epubMimetype is the ZIM entry mimetype for a Project Gutenberg book.
const epubMimetype = "application/epub+zip"

// readZIMBooks enumerates every EPUB entry in a ZIM archive (e.g. a Project
// Gutenberg library) and returns one BookMetadata per book. FileType, FilePath
// and SubFilepath (the entry's namespaced full path, e.g. "C/Title.123.epub")
// are populated so the entry can be reopened on download.
//
// A failure to read/decompress an entry is treated as archive corruption (e.g. a
// truncated or still-copying file) and returned as an error, so the caller does
// not persist a partial archive that IsProcessed would then treat as permanently
// complete. An individual EPUB whose metadata cannot be parsed is a per-book data
// problem and is skipped, so one bad book does not block the rest.
func readZIMBooks(path string) ([]*BookMetadata, error) {
	a, err := openZIMArchive(path)
	if err != nil {
		return nil, err
	}
	defer a.Close()

	var mds []*BookMetadata
	for e, err := range a.AllEntries() {
		if err != nil {
			return nil, fmt.Errorf("zim: iterating %q: %w", path, err)
		}
		if e.IsRedirect() || e.MIMEType() != epubMimetype {
			continue
		}

		data, err := e.ReadContentCopy()
		if err != nil {
			return nil, fmt.Errorf("zim: reading %q in %q: %w", e.FullPath(), path, err)
		}

		md, err := readEPUBMetadataFromBytes(data)
		if err != nil {
			log.Printf("[zim] skipping unreadable epub %q in %q: %v", e.FullPath(), path, err)
			continue
		}
		md.FileType = ZIM
		md.FilePath = path
		md.SubFilepath = e.FullPath()
		mds = append(mds, md)
	}

	return mds, nil
}

// openZIMEntry reopens the EPUB blob for a stored entry reference (the entry's
// namespaced full path) and returns its bytes.
func openZIMEntry(path, ref string) ([]byte, error) {
	a, err := openZIMArchive(path)
	if err != nil {
		return nil, err
	}
	defer a.Close()

	e, err := a.EntryByPath(ref)
	if err != nil {
		return nil, fmt.Errorf("zim: entry %q not found in %q: %w", ref, path, err)
	}
	if e, err = e.Resolve(); err != nil {
		return nil, fmt.Errorf("zim: resolving %q in %q: %w", ref, path, err)
	}
	return e.ReadContentCopy()
}

// openZIMArchive opens a ZIM archive after verifying it is complete.
func openZIMArchive(path string) (*zim.Archive, error) {
	if err := checkZIMComplete(path); err != nil {
		return nil, err
	}
	a, err := zim.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening zim %q: %w", path, err)
	}
	return a, nil
}

// checkZIMComplete guards against a still-copying or truncated archive. The ZIM
// header records the position of the trailing checksum, so the file must be at
// least checksumPos+16 bytes; a shorter file on disk is incomplete. This is read
// straight from the header so it does not depend on the reader library, which
// would otherwise happily enumerate a partial (or empty) set of entries.
func checkZIMComplete(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// checksumPos is a little-endian uint64 at offset 72 of the 80-byte header.
	var buf [8]byte
	if _, err := f.ReadAt(buf[:], 72); err != nil {
		return fmt.Errorf("zim %q: reading header: %w", path, err)
	}
	expected := int64(binary.LittleEndian.Uint64(buf[:])) + 16 // + md5 checksum

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() < expected {
		return fmt.Errorf("zim %q is incomplete: %d of %d bytes on disk", path, fi.Size(), expected)
	}
	return nil
}
