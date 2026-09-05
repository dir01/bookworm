package zim

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Entry represents a directory entry in a ZIM file.
// It may be either a content entry or a redirect entry.
type Entry struct {
	archive   *Archive
	index     uint32
	mimeIndex uint16
	paramLen  uint8
	namespace byte
	revision  uint32
	path      string
	title     string

	// Content entry fields (valid when !IsRedirect())
	clusterNum uint32
	blobNum    uint32

	// Redirect entry fields (valid when IsRedirect())
	redirectIdx uint32
}

// Index returns the entry's position in the URL-ordered pointer list.
func (e Entry) Index() uint32 { return e.index }

// Path returns the entry's path (without namespace prefix).
func (e Entry) Path() string { return e.path }

// Title returns the entry's title. If the title is empty, returns the path.
func (e Entry) Title() string {
	if e.title == "" {
		return e.path
	}
	return e.title
}

// FullPath returns the full path including namespace prefix (e.g., "C/Main_Page").
func (e Entry) FullPath() string {
	return string(e.namespace) + "/" + e.path
}

// Namespace returns the single-byte namespace identifier.
func (e Entry) Namespace() byte { return e.namespace }

// IsRedirect returns true if this entry is a redirect.
func (e Entry) IsRedirect() bool { return e.mimeIndex == mimeRedirect }

// ClusterNum returns the cluster number for this content entry.
// Only valid for non-redirect entries.
func (e Entry) ClusterNum() uint32 { return e.clusterNum }

// BlobNum returns the blob number within the cluster for this content entry.
// Only valid for non-redirect entries.
func (e Entry) BlobNum() uint32 { return e.blobNum }

// MIMEType returns the MIME type string for this entry.
// Returns an empty string for redirect entries.
func (e Entry) MIMEType() string {
	if e.IsRedirect() || e.archive == nil {
		return ""
	}
	if int(e.mimeIndex) >= len(e.archive.mimeTypes) {
		return ""
	}
	return e.archive.mimeTypes[e.mimeIndex]
}

// Item returns the Item for this entry, allowing access to content data.
// Returns ErrIsRedirect if the entry is a redirect.
func (e Entry) Item() (Item, error) {
	if e.IsRedirect() {
		return Item{}, ErrIsRedirect
	}
	return Item{entry: e}, nil
}

// RedirectTarget returns the entry that this redirect points to.
// Returns ErrNotRedirect if the entry is not a redirect.
func (e Entry) RedirectTarget() (Entry, error) {
	if !e.IsRedirect() {
		return Entry{}, ErrNotRedirect
	}
	return e.archive.EntryByIndex(e.redirectIdx)
}

// Resolve follows the redirect chain to the final content entry.
// Returns the entry itself if it's not a redirect.
// Returns ErrRedirectLoop if a cycle is detected.
func (e Entry) Resolve() (Entry, error) {
	if !e.IsRedirect() {
		return e, nil
	}
	seen := map[uint32]bool{e.index: true}
	current := e
	for current.IsRedirect() {
		target, err := current.RedirectTarget()
		if err != nil {
			return Entry{}, err
		}
		if seen[target.index] {
			return Entry{}, ErrRedirectLoop
		}
		seen[target.index] = true
		current = target
	}
	return current, nil
}

// ReadContent resolves redirects and returns the content bytes.
//
// WARNING: The returned slice aliases the internal cluster cache and may be
// invalidated when the cluster is evicted. If you need to buffer or retain
// the data (e.g., when iterating many entries), use [Entry.ReadContentCopy]
// instead.
func (e Entry) ReadContent() ([]byte, error) {
	resolved, err := e.Resolve()
	if err != nil {
		return nil, err
	}
	item, err := resolved.Item()
	if err != nil {
		return nil, err
	}
	blob, err := item.Data()
	if err != nil {
		return nil, err
	}
	return blob.Bytes(), nil
}

// ReadContentCopy resolves redirects and returns a newly allocated copy of
// the content bytes. The returned slice is safe to retain indefinitely and
// does not alias the cluster cache.
func (e Entry) ReadContentCopy() ([]byte, error) {
	resolved, err := e.Resolve()
	if err != nil {
		return nil, err
	}
	item, err := resolved.Item()
	if err != nil {
		return nil, err
	}
	blob, err := item.Data()
	if err != nil {
		return nil, err
	}
	return blob.Copy(), nil
}

// ContentSize resolves redirects and returns the content size in bytes.
// The cluster is decompressed and cached if not already, but no data is copied.
func (e Entry) ContentSize() (int64, error) {
	resolved, err := e.Resolve()
	if err != nil {
		return 0, err
	}
	item, err := resolved.Item()
	if err != nil {
		return 0, err
	}
	return item.Size()
}

// BlobSize returns the uncompressed blob size for this content entry without
// resolving redirects. Returns ErrIsRedirect if called on a redirect entry.
// The cluster is decompressed and cached if not already, so iterating entries
// in cluster order makes repeated calls essentially free.
func (e Entry) BlobSize() (int64, error) {
	item, err := e.Item()
	if err != nil {
		return 0, err
	}
	return item.Size()
}

// ContentReader resolves redirects and returns an io.Reader over the content.
// The cluster is decompressed and cached if not already.
func (e Entry) ContentReader() (io.Reader, error) {
	resolved, err := e.Resolve()
	if err != nil {
		return nil, err
	}
	item, err := resolved.Item()
	if err != nil {
		return nil, err
	}
	return item.Reader()
}

// parseDirectoryEntry parses a directory entry from raw bytes at the given position.
// Returns the parsed entry and the number of bytes consumed.
func parseDirectoryEntry(data []byte, archive *Archive, index uint32) (Entry, int, error) {
	if len(data) < 12 {
		return Entry{}, 0, fmt.Errorf("%w: data too short for directory entry", ErrInvalidEntry)
	}

	le := binary.LittleEndian
	e := Entry{
		archive:   archive,
		index:     index,
		mimeIndex: le.Uint16(data[0:2]),
		paramLen:  data[2],
		namespace: data[3],
		revision:  le.Uint32(data[4:8]),
	}

	var pos int
	if e.IsRedirect() {
		if len(data) < 12 {
			return Entry{}, 0, fmt.Errorf("%w: redirect entry too short", ErrInvalidEntry)
		}
		e.redirectIdx = le.Uint32(data[8:12])
		pos = 12
	} else {
		if len(data) < 16 {
			return Entry{}, 0, fmt.Errorf("%w: content entry too short", ErrInvalidEntry)
		}
		e.clusterNum = le.Uint32(data[8:12])
		e.blobNum = le.Uint32(data[12:16])
		pos = 16
	}

	// Skip parameter data
	pos += int(e.paramLen)
	if pos > len(data) {
		return Entry{}, 0, fmt.Errorf("%w: parameter data extends beyond entry", ErrInvalidEntry)
	}

	// Read null-terminated path
	remaining := data[pos:]
	nullIdx := bytes.IndexByte(remaining, 0)
	if nullIdx < 0 {
		return Entry{}, 0, fmt.Errorf("%w: missing null terminator for path", ErrInvalidEntry)
	}
	e.path = string(remaining[:nullIdx])
	pos += nullIdx + 1

	// Read null-terminated title
	remaining = data[pos:]
	nullIdx = bytes.IndexByte(remaining, 0)
	if nullIdx < 0 {
		return Entry{}, 0, fmt.Errorf("%w: missing null terminator for title", ErrInvalidEntry)
	}
	e.title = string(remaining[:nullIdx])
	pos += nullIdx + 1

	return e, pos, nil
}
