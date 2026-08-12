//go:build !windows

package zim

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
)

// mmapReader maps the entire file into memory for zero-copy access.
type mmapReader struct {
	data []byte
	size int64
	mu   sync.RWMutex
}

func newMmapReader(path string) (*mmapReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, fmt.Errorf("zim: file is empty")
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, fmt.Errorf("zim: mmap failed: %w", err)
	}

	r := &mmapReader{data: data, size: size}
	runtime.SetFinalizer(r, (*mmapReader).Close)
	return r, nil
}

func (r *mmapReader) ReadAt(p []byte, off int64) (int, error) {
	r.mu.RLock()
	data := r.data
	r.mu.RUnlock()
	if data == nil {
		return 0, fmt.Errorf("zim: read from closed mmap reader")
	}
	if off < 0 || off >= r.size {
		return 0, io.EOF
	}
	n := copy(p, data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *mmapReader) Size() int64 {
	return r.size
}

func (r *mmapReader) Close() error {
	r.mu.Lock()
	data := r.data
	r.data = nil
	r.mu.Unlock()
	if data == nil {
		return nil
	}
	runtime.SetFinalizer(r, nil)
	return syscall.Munmap(data)
}
