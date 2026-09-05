package zim

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"iter"
	"math/rand/v2"
	"sync"
)

const defaultCacheSize = 16

// maxClusterSize is the upper bound on cluster data we'll allocate (4 GiB).
// Any cluster claiming to be larger than this in a ZIM file is treated as corrupt.
const maxClusterSize = 4 << 30 // 4 GiB

// maxMIMEListSize is the maximum number of bytes read when scanning the
// MIME type list. Real MIME lists are a few KB; 64 KiB is very generous.
const maxMIMEListSize = 64 << 10 // 64 KiB

// Archive represents an opened ZIM file.
type Archive struct {
	r         reader
	hdr       header
	mimeTypes []string

	// titleList holds title-ordered entry indices for ZIM v6.1+ files that
	// store the title ordering in an X/listing/titleOrdered/v1 (or v0) entry
	// instead of the header's title pointer table (TitlePtrPos == 0xFFFF...).
	titleList []uint32

	metadataOnce sync.Once
	metadataMap  map[string]string // cached M-namespace entries

	clusterMu    sync.Mutex
	clusterCache map[uint32]*list.Element // cluster number → list element (value is *lruEntry)
	cacheSize    int
	cacheList    *list.List // front = most recent, back = least recent
	cacheHits    int64
	cacheMisses  int64
}

// lruEntry is stored as the Value in each list.Element.
type lruEntry struct {
	clusterNum uint32
	cluster    *cluster
}

// CacheStats holds runtime statistics for the cluster LRU cache.
type CacheStats struct {
	Capacity int   // configured maximum number of cached clusters
	Size     int   // number of clusters currently cached
	Hits     int64 // total cache hits since archive was opened
	Misses   int64 // total cache misses since archive was opened
	Bytes    int64 // estimated bytes held by currently cached clusters
}

// CacheStats returns a snapshot of the cluster cache statistics.
func (a *Archive) CacheStats() CacheStats {
	a.clusterMu.Lock()
	defer a.clusterMu.Unlock()
	var bytes int64
	for e := a.cacheList.Front(); e != nil; e = e.Next() {
		for _, b := range e.Value.(*lruEntry).cluster.blobs { //nolint:forcetypeassert
			bytes += int64(len(b))
		}
	}
	return CacheStats{
		Capacity: a.cacheSize,
		Size:     len(a.clusterCache),
		Hits:     a.cacheHits,
		Misses:   a.cacheMisses,
		Bytes:    bytes,
	}
}

type options struct {
	cacheSize int
	useMmap   bool
}

// Option configures how a ZIM archive is opened.
type Option func(*options)

// WithCacheSize sets the number of decompressed clusters to cache. Default: 16.
func WithCacheSize(n int) Option {
	return func(o *options) { o.cacheSize = n }
}

// WithMmap controls whether memory mapping is used. Default: true on 64-bit.
func WithMmap(enabled bool) Option {
	return func(o *options) { o.useMmap = enabled }
}

// Open opens a ZIM file for reading.
func Open(path string) (*Archive, error) {
	return OpenWithOptions(path)
}

// OpenWithOptions opens a ZIM file with the given options.
func OpenWithOptions(path string, opts ...Option) (*Archive, error) {
	o := options{
		cacheSize: defaultCacheSize,
		useMmap:   defaultUseMmap(),
	}
	for _, opt := range opts {
		opt(&o)
	}

	var r reader
	if isSplitZIM(path) {
		parts, err := discoverParts(path)
		if err != nil {
			return nil, fmt.Errorf("zim: discover split parts for %s: %w", path, err)
		}
		r, err = newMultiReader(parts, o.useMmap)
		if err != nil {
			return nil, fmt.Errorf("zim: open split %s: %w", path, err)
		}
	} else {
		var err error
		r, err = openReader(path, o.useMmap)
		if err != nil {
			return nil, fmt.Errorf("zim: open %s: %w", path, err)
		}
	}

	a := &Archive{
		r:            r,
		cacheSize:    o.cacheSize,
		clusterCache: make(map[uint32]*list.Element),
		cacheList:    list.New(),
	}

	if err := a.init(); err != nil {
		r.Close()
		return nil, err
	}

	return a, nil
}

// init reads the header and MIME type list.
func (a *Archive) init() error {
	// Read header
	buf := make([]byte, headerSize)
	if _, err := a.r.ReadAt(buf, 0); err != nil {
		return fmt.Errorf("zim: read header: %w", err)
	}

	var err error
	a.hdr, err = parseHeader(buf)
	if err != nil {
		return err
	}

	// Read MIME type list.
	// The MIME list starts at MIMEListPos and is terminated by an empty
	// null-terminated string (double-null). We read a fixed chunk and let
	// parseMIMEList find the terminator. Real MIME lists are a few KB;
	// 64 KiB is more than enough.
	mimeStart := int64(a.hdr.MIMEListPos)
	mimeReadLen := min(int64(maxMIMEListSize), a.r.Size()-mimeStart)
	mimeBuf := make([]byte, mimeReadLen)
	if _, err := a.r.ReadAt(mimeBuf, mimeStart); err != nil {
		return fmt.Errorf("zim: read MIME list: %w", err)
	}
	a.mimeTypes = parseMIMEList(mimeBuf)

	// ZIM v6.1+ may store title ordering in a listing entry instead of the
	// header's title pointer table. Load it if TitlePtrPos is the sentinel.
	if a.hdr.TitlePtrPos == noTitlePtrList {
		if err := a.loadTitleListing(); err != nil {
			return err
		}
	}

	return nil
}

// loadTitleListing reads the title-ordered listing from X/listing/titleOrdered/v1
// (preferred) or v0. These entries contain a flat array of uint32 entry indices
// in title-sorted order, used by ZIM v6.1+ files.
func (a *Archive) loadTitleListing() error {
	// Try v1 first (content entries only, sorted by title), then v0
	for _, path := range []string{"X/listing/titleOrdered/v1", "X/listing/titleOrdered/v0"} {
		e, err := a.EntryByPath(path)
		if err != nil {
			continue
		}
		data, err := e.ReadContent()
		if err != nil {
			return fmt.Errorf("zim: read title listing %s: %w", path, err)
		}
		if len(data)%4 != 0 {
			return fmt.Errorf("zim: title listing %s has invalid size %d", path, len(data))
		}
		count := len(data) / 4
		a.titleList = make([]uint32, count)
		for i := range a.titleList {
			idx := binary.LittleEndian.Uint32(data[i*4 : i*4+4])
			if idx >= a.hdr.EntryCount {
				return fmt.Errorf("zim: title listing %s: index %d at position %d out of range (entry count %d)",
					path, idx, i, a.hdr.EntryCount)
			}
			a.titleList[i] = idx
		}
		return nil
	}
	// No listing found — title iteration won't work, but don't fail the open.
	return nil
}

// Close closes the archive and releases resources.
func (a *Archive) Close() error {
	a.clusterMu.Lock()
	a.clusterCache = nil
	a.cacheList = nil
	a.clusterMu.Unlock()
	return a.r.Close()
}

// SplitPart describes one file in a split ZIM archive.
type SplitPart struct {
	Path string // absolute file path
	Size int64  // size in bytes
}

// IsSplit reports whether this archive was opened from split ZIM files.
func (a *Archive) IsSplit() bool {
	_, ok := a.r.(*multiReader)
	return ok
}

// SplitParts returns information about each part file if the archive was
// opened from split ZIM files. Returns nil for non-split archives.
func (a *Archive) SplitParts() []SplitPart {
	mr, ok := a.r.(*multiReader)
	if !ok {
		return nil
	}
	result := make([]SplitPart, len(mr.parts))
	for i, r := range mr.parts {
		result[i] = SplitPart{
			Path: mr.paths[i],
			Size: r.Size(),
		}
	}
	return result
}

// UUID returns the archive's unique identifier.
func (a *Archive) UUID() [16]byte { return a.hdr.UUID }

// EntryCount returns the total number of entries.
func (a *Archive) EntryCount() uint32 { return a.hdr.EntryCount }

// ClusterCount returns the total number of clusters.
func (a *Archive) ClusterCount() uint32 { return a.hdr.ClusterCount }

// MajorVersion returns the ZIM format major version.
func (a *Archive) MajorVersion() uint16 { return a.hdr.MajorVersion }

// MinorVersion returns the ZIM format minor version.
func (a *Archive) MinorVersion() uint16 { return a.hdr.MinorVersion }

// MIMETypes returns the list of MIME types in the archive.
func (a *Archive) MIMETypes() []string {
	result := make([]string, len(a.mimeTypes))
	copy(result, a.mimeTypes)
	return result
}

// HasMainEntry returns true if the archive has a designated main page.
func (a *Archive) HasMainEntry() bool { return a.hdr.MainPage != noMainPage }

// MainEntry returns the main page entry.
func (a *Archive) MainEntry() (Entry, error) {
	if !a.HasMainEntry() {
		return Entry{}, ErrNotFound
	}
	return a.EntryByIndex(a.hdr.MainPage)
}

// EntryByIndex returns the entry at the given index in the URL-ordered list.
func (a *Archive) EntryByIndex(idx uint32) (Entry, error) {
	if idx >= a.hdr.EntryCount {
		return Entry{}, ErrNotFound
	}

	// Read the URL pointer for this index (stack-allocated buffer)
	ptrOffset := int64(a.hdr.URLPtrPos) + int64(idx)*8
	var ptrBuf [8]byte
	if _, err := a.r.ReadAt(ptrBuf[:], ptrOffset); err != nil {
		return Entry{}, fmt.Errorf("zim: read URL pointer at index %d: %w", idx, err)
	}
	entryOffset := int64(binary.LittleEndian.Uint64(ptrBuf[:]))

	return a.readEntryAt(entryOffset, idx)
}

// readEntryAt reads a directory entry from the given file offset.
func (a *Archive) readEntryAt(offset int64, index uint32) (Entry, error) {
	// Read enough data for the entry. Directory entries are variable-length
	// but typically under 2 KiB. Use a stack-allocated buffer for common
	// cases and fall back to a larger heap buffer if parsing fails.
	var stackBuf [2048]byte
	n := int64(len(stackBuf))
	if offset+n > a.r.Size() {
		n = a.r.Size() - offset
	}
	buf := stackBuf[:n]
	if _, err := a.r.ReadAt(buf, offset); err != nil {
		return Entry{}, fmt.Errorf("zim: read directory entry at offset %d: %w", offset, err)
	}

	entry, _, err := parseDirectoryEntry(buf, a, index)
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// EntryByPath looks up an entry by its full path (e.g., "C/Main_Page").
// Uses binary search over the URL pointer list.
func (a *Archive) EntryByPath(path string) (Entry, error) {
	lo, hi := uint32(0), a.hdr.EntryCount
	for lo < hi {
		mid := lo + (hi-lo)/2
		entry, err := a.EntryByIndex(mid)
		if err != nil {
			return Entry{}, err
		}
		entryPath := entry.FullPath()
		if entryPath == path {
			return entry, nil
		}
		if entryPath < path {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return Entry{}, ErrNotFound
}

// EntryByTitle looks up an entry by namespace and title using binary search
// over the title pointer list. The list is sorted by (namespace, title).
func (a *Archive) EntryByTitle(ns byte, title string) (Entry, error) {
	lo, hi := uint32(0), a.titleCount()
	for lo < hi {
		mid := lo + (hi-lo)/2
		entry, err := a.entryByTitleIndex(mid)
		if err != nil {
			return Entry{}, err
		}
		cmp := compareTitleKey(entry.Namespace(), entry.Title(), ns, title)
		if cmp == 0 {
			return entry, nil
		}
		if cmp < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return Entry{}, ErrNotFound
}

// compareTitleKey compares two (namespace, title) pairs.
// Returns -1, 0, or 1.
func compareTitleKey(ns1 byte, title1 string, ns2 byte, title2 string) int {
	if ns1 < ns2 {
		return -1
	}
	if ns1 > ns2 {
		return 1
	}
	if title1 < title2 {
		return -1
	}
	if title1 > title2 {
		return 1
	}
	return 0
}

// titleCount returns the number of entries in the title-ordered list.
func (a *Archive) titleCount() uint32 {
	if a.titleList != nil {
		return uint32(len(a.titleList))
	}
	return a.hdr.EntryCount
}

// entryByTitleIndex reads the entry at position idx in the title pointer list.
func (a *Archive) entryByTitleIndex(idx uint32) (Entry, error) {
	if a.titleList != nil {
		if int(idx) >= len(a.titleList) {
			return Entry{}, fmt.Errorf("zim: title index %d out of range", idx)
		}
		return a.EntryByIndex(a.titleList[idx])
	}
	ptrOffset := int64(a.hdr.TitlePtrPos) + int64(idx)*4
	var ptrBuf [4]byte
	if _, err := a.r.ReadAt(ptrBuf[:], ptrOffset); err != nil {
		return Entry{}, fmt.Errorf("zim: read title pointer at index %d: %w", idx, err)
	}
	entryIdx := binary.LittleEndian.Uint32(ptrBuf[:])
	return a.EntryByIndex(entryIdx)
}

// EntriesByTitle returns an iterator over all entries sorted by title.
// Iteration stops at the first parse error; use [Archive.AllEntriesByTitle]
// for error-aware iteration.
func (a *Archive) EntriesByTitle() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for i := range a.titleCount() {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// AllEntriesByTitle returns an error-aware iterator over all entries sorted by
// title. Each step yields (entry, nil) on success or (Entry{}, err) on the
// first parse error, after which iteration stops.
func (a *Archive) AllEntriesByTitle() iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		for i := range a.titleCount() {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				yield(Entry{}, err)
				return
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

// Metadata returns the value of a metadata entry (M namespace) by key.
// Results are cached after the first call; subsequent calls for any key
// are served from the cache. Returns ErrNotFound if the key doesn't exist.
func (a *Archive) Metadata(key string) (string, error) {
	a.metadataOnce.Do(a.loadMetadata)
	v, ok := a.metadataMap[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// loadMetadata populates the metadata cache by reading all M-namespace entries.
func (a *Archive) loadMetadata() {
	a.metadataMap = make(map[string]string)
	for e := range a.EntriesByNamespace('M') {
		data, err := e.ReadContent()
		if err != nil {
			continue
		}
		a.metadataMap[e.Path()] = string(data)
	}
}

// namespaceBounds returns the [lo, hi) index range in the URL pointer list
// for entries in the given namespace. Uses binary search — O(log N).
func (a *Archive) namespaceBounds(ns byte) (lo, hi uint32) {
	prefix := string(ns) + "/"
	// Find lower bound: first entry where FullPath() >= prefix
	left, right := uint32(0), a.hdr.EntryCount
	for left < right {
		mid := left + (right-left)/2
		e, err := a.EntryByIndex(mid)
		if err != nil {
			return 0, 0
		}
		if e.FullPath() < prefix {
			left = mid + 1
		} else {
			right = mid
		}
	}
	lo = left

	// Find upper bound: first entry where namespace > ns.
	// Namespace is a single byte, so ns+1 prefix works (unless ns == 0xFF).
	nextPrefix := string(ns+1) + "/"
	left, right = lo, a.hdr.EntryCount
	for left < right {
		mid := left + (right-left)/2
		e, err := a.EntryByIndex(mid)
		if err != nil {
			return lo, lo
		}
		if e.FullPath() < nextPrefix {
			left = mid + 1
		} else {
			right = mid
		}
	}
	hi = left
	return lo, hi
}

// EntryCountByNamespace returns the number of entries in the given namespace.
// Uses binary search to find the namespace bounds — O(log N).
func (a *Archive) EntryCountByNamespace(ns byte) int {
	lo, hi := a.namespaceBounds(ns)
	return int(hi - lo)
}

// TitlePrefixCount returns the number of entries in namespace ns whose title
// starts with prefix. Uses dual binary search — O(log N).
// An empty prefix counts all entries for the namespace in the title-sorted list.
func (a *Archive) TitlePrefixCount(ns byte, prefix string) int {
	count := a.titleCount()

	// Lower bound: first index where (entry.ns, entry.title) >= (ns, prefix).
	lo, hi := uint32(0), count
	for lo < hi {
		mid := lo + (hi-lo)/2
		e, err := a.entryByTitleIndex(mid)
		if err != nil {
			return 0
		}
		if compareTitleKey(e.Namespace(), e.Title(), ns, prefix) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	lower := lo

	// Upper bound: first index past the prefix range.
	lo, hi = lower, count
	if prefix == "" {
		// Find first entry in namespace > ns.
		for lo < hi {
			mid := lo + (hi-lo)/2
			e, err := a.entryByTitleIndex(mid)
			if err != nil {
				return int(lo - lower)
			}
			if e.Namespace() <= ns {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
	} else {
		// nextPrefix is the shortest string that sorts strictly after all strings
		// starting with prefix: increment the last byte that is < 0xFF.
		b := []byte(prefix)
		found := false
		for i := len(b) - 1; i >= 0; i-- {
			if b[i] < 0xff {
				b[i]++
				b = b[:i+1]
				found = true
				break
			}
		}
		if !found {
			// All bytes are 0xFF; find end of namespace.
			for lo < hi {
				mid := lo + (hi-lo)/2
				e, err := a.entryByTitleIndex(mid)
				if err != nil {
					return int(lo - lower)
				}
				if e.Namespace() <= ns {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
		} else {
			nextPrefix := string(b)
			for lo < hi {
				mid := lo + (hi-lo)/2
				e, err := a.entryByTitleIndex(mid)
				if err != nil {
					return int(lo - lower)
				}
				if compareTitleKey(e.Namespace(), e.Title(), ns, nextPrefix) < 0 {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
		}
	}
	return int(lo - lower)
}

// RandomEntry returns a random entry from the given namespace.
// Uses crypto/rand for unbiased selection.
func (a *Archive) RandomEntry(ns byte) (Entry, error) {
	lo, hi := a.namespaceBounds(ns)
	count := hi - lo
	if count == 0 {
		return Entry{}, ErrNotFound
	}
	n := rand.IntN(int(count))
	return a.EntryByIndex(lo + uint32(n))
}

// Illustration returns the illustration (icon/favicon) of the given pixel size.
// It reads the metadata entry at M/Illustration_{size}x{size}@1.
func (a *Archive) Illustration(size int) ([]byte, error) {
	path := fmt.Sprintf("M/Illustration_%dx%d@1", size, size)
	entry, err := a.EntryByPath(path)
	if err != nil {
		return nil, err
	}
	return entry.ReadContent()
}

// ClusterMeta holds per-cluster metadata readable without decompression.
type ClusterMeta struct {
	Offset         uint64 // byte offset in ZIM file
	CompressedSize uint64 // bytes on disk
	Compression    string // "none", "xz", "zstd", "unknown"
	Extended       bool   // uses 8-byte blob offsets
}

// ClusterMetaAt returns metadata for cluster n without decompressing it.
// Only reads the cluster pointer and the first info byte.
func (a *Archive) ClusterMetaAt(n uint32) (ClusterMeta, error) {
	if n >= a.hdr.ClusterCount {
		return ClusterMeta{}, fmt.Errorf("zim: cluster %d out of range (max %d)", n, a.hdr.ClusterCount)
	}
	ptrOffset := int64(a.hdr.ClusterPtrPos) + int64(n)*8
	ptrBuf := make([]byte, 16)
	if _, err := a.r.ReadAt(ptrBuf, ptrOffset); err != nil {
		return ClusterMeta{}, fmt.Errorf("zim: read cluster pointer %d: %w", n, err)
	}
	clusterOffset := binary.LittleEndian.Uint64(ptrBuf[0:8])
	var clusterEnd uint64
	if n+1 < a.hdr.ClusterCount {
		clusterEnd = binary.LittleEndian.Uint64(ptrBuf[8:16])
	} else {
		clusterEnd = a.hdr.ChecksumPos
	}
	var infoBuf [1]byte
	if _, err := a.r.ReadAt(infoBuf[:], int64(clusterOffset)); err != nil {
		return ClusterMeta{}, fmt.Errorf("zim: read cluster %d info byte: %w", n, err)
	}
	compName := "unknown"
	switch infoBuf[0] & clusterCompMask {
	case compNone:
		compName = "none"
	case compLZMA:
		compName = "xz"
	case compZstd:
		compName = "zstd"
	}
	return ClusterMeta{
		Offset:         clusterOffset,
		CompressedSize: clusterEnd - clusterOffset,
		Compression:    compName,
		Extended:       infoBuf[0]&clusterExtendedBit != 0,
	}, nil
}

// ClusterBlobSizes returns the decompressed size of each blob in cluster n.
// Decompresses the cluster (result is cached by the internal LRU cache).
func (a *Archive) ClusterBlobSizes(n uint32) ([]int, error) {
	c, err := a.readCluster(n)
	if err != nil {
		return nil, err
	}
	sizes := make([]int, len(c.blobs))
	for i, b := range c.blobs {
		sizes[i] = len(b)
	}
	return sizes, nil
}

// EntriesInCluster returns all content entries belonging to cluster n.
// Iterates all entries; O(EntryCount).
func (a *Archive) EntriesInCluster(n uint32) ([]Entry, error) {
	if n >= a.hdr.ClusterCount {
		return nil, fmt.Errorf("zim: cluster %d out of range", n)
	}
	var result []Entry
	for i := range a.hdr.EntryCount {
		e, err := a.EntryByIndex(i)
		if err != nil {
			return nil, err
		}
		if !e.IsRedirect() && e.clusterNum == n {
			result = append(result, e)
		}
	}
	return result, nil
}

// readCluster reads, decompresses, and caches a cluster by number.
func (a *Archive) readCluster(clusterNum uint32) (*cluster, error) {
	a.clusterMu.Lock()
	defer a.clusterMu.Unlock()

	// Check cache and promote to front (most-recently-used)
	if elem, ok := a.clusterCache[clusterNum]; ok {
		a.cacheHits++
		a.cacheList.MoveToFront(elem)
		return elem.Value.(*lruEntry).cluster, nil //nolint:forcetypeassert
	}
	a.cacheMisses++

	if clusterNum >= a.hdr.ClusterCount {
		return nil, fmt.Errorf("zim: cluster %d out of range (max %d)", clusterNum, a.hdr.ClusterCount)
	}

	// Read cluster pointer
	ptrOffset := int64(a.hdr.ClusterPtrPos) + int64(clusterNum)*8
	ptrBuf := make([]byte, 16) // read current and next pointer
	if _, err := a.r.ReadAt(ptrBuf, ptrOffset); err != nil {
		return nil, fmt.Errorf("zim: read cluster pointer %d: %w", clusterNum, err)
	}

	clusterOffset := int64(binary.LittleEndian.Uint64(ptrBuf[0:8]))

	// Determine cluster end
	var clusterEnd int64
	if clusterNum+1 < a.hdr.ClusterCount {
		clusterEnd = int64(binary.LittleEndian.Uint64(ptrBuf[8:16]))
	} else {
		clusterEnd = int64(a.hdr.ChecksumPos)
	}

	clusterSize := clusterEnd - clusterOffset
	if clusterSize <= 0 {
		return nil, fmt.Errorf("zim: invalid cluster size %d", clusterSize)
	}
	if clusterSize > maxClusterSize {
		return nil, fmt.Errorf("zim: cluster %d size %d exceeds maximum (%d)", clusterNum, clusterSize, maxClusterSize)
	}

	// Read cluster data
	clusterData := make([]byte, clusterSize)
	if _, err := a.r.ReadAt(clusterData, clusterOffset); err != nil {
		return nil, fmt.Errorf("zim: read cluster %d: %w", clusterNum, err)
	}

	// Parse cluster
	c, err := parseCluster(clusterData)
	if err != nil {
		return nil, fmt.Errorf("zim: parse cluster %d: %w", clusterNum, err)
	}

	// Cache with LRU eviction
	if a.clusterCache != nil {
		if a.cacheList.Len() >= a.cacheSize {
			back := a.cacheList.Back()
			delete(a.clusterCache, back.Value.(*lruEntry).clusterNum) //nolint:forcetypeassert
			a.cacheList.Remove(back)
		}
		elem := a.cacheList.PushFront(&lruEntry{clusterNum: clusterNum, cluster: c})
		a.clusterCache[clusterNum] = elem
	}

	return c, nil
}
