package zim

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// multiReader presents multiple split ZIM files (.zimaa, .zimab, …)
// as a single contiguous io.ReaderAt, implementing the reader interface.
type multiReader struct {
	paths   []string // sorted file paths for each part
	parts   []reader
	offsets []int64 // cumulative start offset for each part
	total   int64
}

// discoverParts returns the sorted list of split ZIM file paths starting from
// the given .zimaa path. It discovers .zimab, .zimac, … by incrementing the
// two-letter suffix. Returns an error if the initial path doesn't end with a
// valid split suffix or if no files are found.
func discoverParts(firstPath string) ([]string, error) {
	if len(firstPath) < 2 {
		return nil, fmt.Errorf("zim: path too short for split detection: %s", firstPath)
	}

	// Validate suffix pattern: last two chars must be lowercase letters.
	suffix := firstPath[len(firstPath)-2:]
	if suffix[0] < 'a' || suffix[0] > 'z' || suffix[1] < 'a' || suffix[1] > 'z' {
		return nil, fmt.Errorf("zim: not a split ZIM suffix: %q", suffix)
	}

	base := firstPath[:len(firstPath)-2]
	var paths []string

	// Iterate aa, ab, ac, … az, ba, bb, … zz
	for c1 := byte('a'); c1 <= 'z'; c1++ {
		for c2 := byte('a'); c2 <= 'z'; c2++ {
			candidate := base + string([]byte{c1, c2})
			if _, err := os.Stat(candidate); err != nil {
				if os.IsNotExist(err) {
					// If we haven't found any parts yet, keep looking
					// (caller may have started with a non-"aa" suffix).
					// If we already have parts and hit a gap, stop.
					if len(paths) > 0 {
						return paths, nil
					}
					continue
				}
				return nil, err
			}
			paths = append(paths, candidate)
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("zim: no split parts found for %s", firstPath)
	}
	return paths, nil
}

// isSplitZIM returns true if the path ends with a split ZIM suffix (.zimaa, .zimab, etc.).
func isSplitZIM(path string) bool {
	ext := filepath.Ext(path)
	// Split files have extensions like .zimaa, .zimab, etc.
	// The extension is ".zim" + two lowercase letters.
	if len(ext) != 6 {
		return false
	}
	if ext[:4] != ".zim" {
		return false
	}
	return ext[4] >= 'a' && ext[4] <= 'z' && ext[5] >= 'a' && ext[5] <= 'z'
}

// newMultiReader opens all parts and returns a combined reader.
func newMultiReader(paths []string, useMmap bool) (*multiReader, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("zim: no parts to open")
	}

	// Sort paths to ensure correct order.
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)

	parts := make([]reader, 0, len(sorted))
	offsets := make([]int64, 0, len(sorted))
	var total int64

	for _, p := range sorted {
		r, err := openReader(p, useMmap)
		if err != nil {
			// Clean up already-opened parts.
			for _, opened := range parts {
				opened.Close()
			}
			return nil, fmt.Errorf("zim: open part %s: %w", p, err)
		}
		offsets = append(offsets, total)
		total += r.Size()
		parts = append(parts, r)
	}

	return &multiReader{
		paths:   sorted,
		parts:   parts,
		offsets: offsets,
		total:   total,
	}, nil
}

// ReadAt implements io.ReaderAt across all parts.
func (m *multiReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= m.total {
		return 0, io.EOF
	}

	totalRead := 0
	for len(p) > 0 {
		if off >= m.total {
			return totalRead, io.EOF
		}

		// Find which part contains this offset using binary search.
		idx := sort.Search(len(m.offsets), func(i int) bool {
			return m.offsets[i] > off
		}) - 1

		partOff := off - m.offsets[idx]
		partRemain := m.parts[idx].Size() - partOff

		// Read as much as we can from this part.
		toRead := min(int64(len(p)), partRemain)

		n, err := m.parts[idx].ReadAt(p[:toRead], partOff)
		totalRead += n
		p = p[n:]
		off += int64(n)

		if err != nil && !errors.Is(err, io.EOF) {
			return totalRead, err
		}
		if n == 0 {
			return totalRead, io.EOF
		}
	}
	return totalRead, nil
}

// Size returns the total size across all parts.
func (m *multiReader) Size() int64 {
	return m.total
}

// Close closes all underlying part readers.
func (m *multiReader) Close() error {
	var firstErr error
	for _, r := range m.parts {
		if err := r.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
