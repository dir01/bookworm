//go:build windows

package zim

import "errors"

// mmapReader is not supported on Windows. defaultUseMmap always returns false
// on Windows, so newMmapReader is never called at runtime, but the type must
// exist for the build to succeed.
type mmapReader struct{}

func newMmapReader(_ string) (*mmapReader, error) {
	return nil, errors.New("zim: mmap not supported on Windows")
}

func (r *mmapReader) ReadAt(_ []byte, _ int64) (int, error) {
	return 0, errors.New("zim: mmap not supported on Windows")
}

func (r *mmapReader) Size() int64 { return 0 }

func (r *mmapReader) Close() error { return nil }
