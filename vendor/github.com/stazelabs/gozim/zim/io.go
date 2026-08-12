package zim

import (
	"io"
	"os"
	"runtime"
)

// reader is the internal I/O abstraction for reading ZIM files.
type reader interface {
	io.ReaderAt
	Size() int64
	Close() error
}

// preadReader uses os.File.ReadAt (pread) for file access.
type preadReader struct {
	f    *os.File
	size int64
}

func newPreadReader(path string) (*preadReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &preadReader{f: f, size: info.Size()}, nil
}

func (r *preadReader) ReadAt(p []byte, off int64) (int, error) {
	return r.f.ReadAt(p, off)
}

func (r *preadReader) Size() int64 {
	return r.size
}

func (r *preadReader) Close() error {
	return r.f.Close()
}

// openReader opens a file using mmap on supported platforms, pread otherwise.
func openReader(path string, useMmap bool) (reader, error) {
	if useMmap {
		return newMmapReader(path)
	}
	return newPreadReader(path)
}

// defaultUseMmap returns true on 64-bit non-Windows platforms.
func defaultUseMmap() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"
}
