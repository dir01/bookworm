// Package zim provides a pure Go implementation for reading ZIM files.
//
// ZIM is the open archive format used by OpenZIM/Kiwix for storing offline
// web content such as Wikipedia. This package supports ZIM format versions
// 5 and 6, including XZ and Zstandard compressed clusters.
//
// Open a ZIM file with [Open] or [OpenWithOptions], then use [Archive] methods
// to look up entries by path, title, or index. Content is accessed through
// [Entry.ReadContent] (which resolves redirects) or via [Entry.Item] and
// [Item.Data] for lower-level access.
//
// The package uses memory-mapped I/O by default on 64-bit systems and caches
// decompressed clusters in an LRU cache for efficient repeated access.
package zim
