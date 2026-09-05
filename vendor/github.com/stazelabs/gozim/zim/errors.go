package zim

import "errors"

var (
	ErrInvalidMagic           = errors.New("zim: invalid magic number")
	ErrNotFound               = errors.New("zim: entry not found")
	ErrIsRedirect             = errors.New("zim: entry is a redirect")
	ErrNotRedirect            = errors.New("zim: entry is not a redirect")
	ErrChecksumMismatch       = errors.New("zim: checksum verification failed")
	ErrUnsupportedVersion     = errors.New("zim: unsupported format version")
	ErrUnsupportedCompression = errors.New("zim: unsupported compression type")
	ErrInvalidEntry           = errors.New("zim: invalid directory entry")
	ErrRedirectLoop           = errors.New("zim: redirect loop detected")
)
