package catalog

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	zim "github.com/stazelabs/gozim/zim"
)

// readZIMBooks enumerates every book entry in a ZIM archive (e.g. a Project
// Gutenberg library) and returns one BookMetadata per book. Entries are selected
// by MIME type, so any supported leaf format (EPUB, FB2) is indexed. FileType,
// FilePath and SubFilepath (the entry's namespaced full path, e.g.
// "C/Title.123.epub") are populated so the entry can be reopened on download.
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

	// Reading a blob out of the archive (decompress) is serialized inside gozim
	// by a single mutex, so the producer loop reads them one at a time. Parsing
	// the book (an EPUB's zip/OPF, an FB2's XML), however, is the dominant
	// per-book cost and is lock-free (it works on a private copy of the bytes),
	// so we fan that out to a pool of workers. The jobs channel is bounded by the
	// worker count, which also caps how many blobs are held in memory at once.
	type job struct {
		ref    string
		format FileType
		data   []byte
	}

	workers := runtime.NumCPU()
	jobs := make(chan job, workers)

	var mu sync.Mutex
	var mds []*BookMetadata
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for j := range jobs {
				md, err := readLeafMetadata(j.format, j.data)
				if err != nil {
					// A single unreadable book is a per-book data problem, not
					// archive corruption: skip it so one bad book does not block
					// the rest.
					log.Printf("[zim] skipping unreadable book %q in %q: %v", j.ref, path, err)
					continue
				}
				md.FileType = ZIM
				md.FilePath = path
				md.SubFilepath = j.ref
				mu.Lock()
				mds = append(mds, md)
				mu.Unlock()
			}
		})
	}

	// A failure to iterate or read/decompress an entry is treated as archive
	// corruption (e.g. a truncated or still-copying file): stop, drain the
	// workers, and return the error so the caller does not persist a partial
	// archive that IsProcessed would then treat as permanently complete.
	var readErr error
	for e, err := range a.AllEntries() {
		if err != nil {
			readErr = fmt.Errorf("zim: iterating %q: %w", path, err)
			break
		}
		format, ok := leafFormatFromMIME(e.MIMEType())
		if e.IsRedirect() || !ok {
			continue
		}
		data, err := e.ReadContentCopy()
		if err != nil {
			readErr = fmt.Errorf("zim: reading %q in %q: %w", e.FullPath(), path, err)
			break
		}
		jobs <- job{ref: e.FullPath(), format: format, data: data}
	}
	close(jobs)
	wg.Wait()

	if readErr != nil {
		return nil, readErr
	}
	return mds, nil
}

// openZIMEntry reopens the book blob for a stored entry reference (the entry's
// namespaced full path) and returns its bytes together with the entry's MIME
// type, from which the caller derives the leaf format.
func openZIMEntry(path, ref string) ([]byte, string, error) {
	a, err := openZIMArchive(path)
	if err != nil {
		return nil, "", err
	}
	defer a.Close()

	e, err := a.EntryByPath(ref)
	if err != nil {
		return nil, "", fmt.Errorf("zim: entry %q not found in %q: %w", ref, path, err)
	}
	if e, err = e.Resolve(); err != nil {
		return nil, "", fmt.Errorf("zim: resolving %q in %q: %w", ref, path, err)
	}
	data, err := e.ReadContentCopy()
	if err != nil {
		return nil, "", fmt.Errorf("zim: reading %q in %q: %w", ref, path, err)
	}
	return data, e.MIMEType(), nil
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
