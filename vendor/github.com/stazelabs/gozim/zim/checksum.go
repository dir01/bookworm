package zim

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
)

// Verify checks the archive's MD5 checksum.
// Returns nil if the checksum matches, ErrChecksumMismatch if not.
func (a *Archive) Verify() error {
	checksumPos := int64(a.hdr.ChecksumPos)
	if checksumPos <= 0 || checksumPos >= a.r.Size() {
		return fmt.Errorf("zim: invalid checksum position %d", checksumPos)
	}

	// Read the stored checksum (16 bytes at ChecksumPos)
	storedChecksum := make([]byte, md5.Size)
	if _, err := a.r.ReadAt(storedChecksum, checksumPos); err != nil {
		return fmt.Errorf("zim: read checksum: %w", err)
	}

	// Compute MD5 of everything before the checksum
	h := md5.New()
	buf := make([]byte, 64*1024) // 64KB read buffer
	var offset int64
	for offset < checksumPos {
		n := int64(len(buf))
		if offset+n > checksumPos {
			n = checksumPos - offset
		}
		nr, err := a.r.ReadAt(buf[:n], offset)
		if nr > 0 {
			h.Write(buf[:nr])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("zim: read for checksum: %w", err)
		}
		offset += int64(nr)
	}

	computed := h.Sum(nil)
	for i := range computed {
		if computed[i] != storedChecksum[i] {
			return fmt.Errorf("%w: computed %x, stored %x", ErrChecksumMismatch, computed, storedChecksum)
		}
	}

	return nil
}
