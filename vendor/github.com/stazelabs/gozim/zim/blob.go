package zim

import (
	"bytes"
	"io"
)

// Blob is a reference to raw content bytes from a ZIM entry.
//
// IMPORTANT: Blob holds a slice that aliases the internal cluster cache.
// The underlying data may be invalidated when the cluster is evicted from
// the LRU cache (e.g., when reading entries from many different clusters).
// If you need to retain the data beyond the next cluster read, use [Blob.Copy].
type Blob struct {
	data []byte
}

// Bytes returns the raw byte content. The returned slice aliases the
// decompressed cluster cache — callers must not modify it, and the data
// may become invalid after the owning cluster is evicted from the cache.
// Use [Blob.Copy] if you need an independent copy that outlives the cache.
func (b Blob) Bytes() []byte { return b.data }

// Copy returns a newly allocated copy of the blob content.
// The returned slice is safe to retain, modify, and use concurrently,
// as it does not alias the cluster cache.
func (b Blob) Copy() []byte {
	if b.data == nil {
		return nil
	}
	cp := make([]byte, len(b.data))
	copy(cp, b.data)
	return cp
}

// String returns the content as a string.
func (b Blob) String() string { return string(b.data) }

// Size returns the number of bytes in the blob.
func (b Blob) Size() int { return len(b.data) }

// Reader returns an io.Reader over the blob content.
func (b Blob) Reader() io.Reader { return bytes.NewReader(b.data) }
