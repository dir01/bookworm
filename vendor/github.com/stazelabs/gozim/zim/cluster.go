package zim

import (
	"encoding/binary"
	"fmt"
)

const (
	clusterCompMask    = 0x0F
	clusterExtendedBit = 0x10
)

// cluster holds the decompressed blobs from a single ZIM cluster.
type cluster struct {
	blobs [][]byte
}

// parseCluster reads and decompresses a cluster from raw data.
// The data should start at the cluster's info byte.
func parseCluster(data []byte) (*cluster, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("zim: cluster data is empty")
	}

	infoByte := data[0]
	compType := infoByte & clusterCompMask
	extended := infoByte&clusterExtendedBit != 0

	clusterData := data[1:]

	// Decompress if needed
	decompressed, err := decompress(clusterData, compType)
	if err != nil {
		return nil, fmt.Errorf("zim: cluster decompression failed: %w", err)
	}

	// Parse blob offsets from decompressed data
	return extractBlobs(decompressed, extended)
}

// extractBlobs parses the offset list and extracts individual blobs from
// decompressed cluster data.
func extractBlobs(data []byte, extended bool) (*cluster, error) {
	offsetSize := 4
	if extended {
		offsetSize = 8
	}

	if len(data) < offsetSize {
		return nil, fmt.Errorf("zim: cluster too small for offset list")
	}

	// First offset tells us the total size of the offset list,
	// which reveals the number of blobs.
	var firstOffset uint64
	if extended {
		firstOffset = binary.LittleEndian.Uint64(data[0:8])
	} else {
		firstOffset = uint64(binary.LittleEndian.Uint32(data[0:4]))
	}

	if firstOffset == 0 || firstOffset%uint64(offsetSize) != 0 {
		return nil, fmt.Errorf("zim: invalid first offset %d", firstOffset)
	}
	if firstOffset > uint64(len(data)) {
		return nil, fmt.Errorf("zim: first offset %d exceeds cluster size %d", firstOffset, len(data))
	}

	numOffsets := int(firstOffset) / offsetSize // N+1 offsets for N blobs
	numBlobs := numOffsets - 1
	if numBlobs < 0 {
		return nil, fmt.Errorf("zim: cluster has no blobs")
	}

	// Read all offsets
	offsets := make([]uint64, numOffsets)
	for i := range numOffsets {
		pos := i * offsetSize
		if pos+offsetSize > len(data) {
			return nil, fmt.Errorf("zim: offset list extends beyond cluster data")
		}
		if extended {
			offsets[i] = binary.LittleEndian.Uint64(data[pos : pos+8])
		} else {
			offsets[i] = uint64(binary.LittleEndian.Uint32(data[pos : pos+4]))
		}
	}

	// Extract blobs using offset pairs
	blobs := make([][]byte, numBlobs)
	for i := range numBlobs {
		start := offsets[i]
		end := offsets[i+1]
		if start > end || end > uint64(len(data)) {
			return nil, fmt.Errorf("zim: invalid blob offsets [%d, %d) in cluster of size %d", start, end, len(data))
		}
		blobs[i] = data[start:end]
	}

	return &cluster{blobs: blobs}, nil
}
