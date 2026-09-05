package zim

import (
	"bytes"
	"io"
)

// Item provides access to content data for non-redirect entries.
type Item struct {
	entry Entry
}

// blob returns the raw blob slice from the cluster cache.
func (i Item) blob() ([]byte, error) {
	c, err := i.entry.archive.readCluster(i.entry.clusterNum)
	if err != nil {
		return nil, err
	}
	if int(i.entry.blobNum) >= len(c.blobs) {
		return nil, ErrNotFound
	}
	return c.blobs[i.entry.blobNum], nil
}

// Data reads and returns the blob content for this item.
func (i Item) Data() (Blob, error) {
	data, err := i.blob()
	if err != nil {
		return Blob{}, err
	}
	return Blob{data: data}, nil
}

// Size returns the size of this item's content in bytes.
// The cluster is decompressed and cached if not already, but no data is copied.
func (i Item) Size() (int64, error) {
	data, err := i.blob()
	if err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}

// Reader returns an io.Reader over this item's content.
// The cluster is decompressed and cached if not already.
func (i Item) Reader() (io.Reader, error) {
	data, err := i.blob()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// MIMEType returns the MIME type of this item.
func (i Item) MIMEType() string { return i.entry.MIMEType() }

// Path returns the path of this item.
func (i Item) Path() string { return i.entry.Path() }

// FullPath returns the full path including namespace.
func (i Item) FullPath() string { return i.entry.FullPath() }
