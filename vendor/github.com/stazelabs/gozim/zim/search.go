package zim

import (
	"iter"
	"strings"
	"unicode"
)

// EntriesByTitlePrefix returns an iterator over entries in namespace ns whose
// title starts with prefix (case-sensitive). Binary search locates the lower
// bound in the title pointer list — O(log N) to find the start, O(k) to
// iterate k results.
//
// An empty prefix iterates all entries in the namespace in title order.
// Iteration stops at the first parse error; use [Archive.AllEntriesByTitlePrefix]
// for error-aware iteration.
func (a *Archive) EntriesByTitlePrefix(ns byte, prefix string) iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		count := a.titleCount()
		// Binary search for lower bound: first index where
		// compareTitleKey(entry) >= (ns, prefix).
		lo, hi := uint32(0), count
		for lo < hi {
			mid := lo + (hi-lo)/2
			e, err := a.entryByTitleIndex(mid)
			if err != nil {
				return
			}
			if compareTitleKey(e.Namespace(), e.Title(), ns, prefix) < 0 {
				lo = mid + 1
			} else {
				hi = mid
			}
		}

		// Iterate forward while namespace matches and title has the prefix.
		for i := lo; i < count; i++ {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				return
			}
			if e.Namespace() != ns {
				return
			}
			if !strings.HasPrefix(e.Title(), prefix) {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// AllEntriesByTitlePrefix is like [Archive.EntriesByTitlePrefix] but
// error-aware. Each step yields (entry, nil) on success or (Entry{}, err) on
// the first parse error, after which iteration stops.
func (a *Archive) AllEntriesByTitlePrefix(ns byte, prefix string) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		count := a.titleCount()
		lo, hi := uint32(0), count
		for lo < hi {
			mid := lo + (hi-lo)/2
			e, err := a.entryByTitleIndex(mid)
			if err != nil {
				yield(Entry{}, err)
				return
			}
			if compareTitleKey(e.Namespace(), e.Title(), ns, prefix) < 0 {
				lo = mid + 1
			} else {
				hi = mid
			}
		}

		for i := lo; i < count; i++ {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				yield(Entry{}, err)
				return
			}
			if e.Namespace() != ns {
				return
			}
			if !strings.HasPrefix(e.Title(), prefix) {
				return
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

// EntriesByTitlePrefixFold is like EntriesByTitlePrefix but case-insensitive
// using Unicode simple case folding. Because the title list is sorted
// case-sensitively, this requires a full linear scan — O(N). Prefer
// EntriesByTitlePrefix when case sensitivity is acceptable.
// Iteration stops at the first parse error; use [Archive.AllEntriesByTitlePrefixFold]
// for error-aware iteration.
func (a *Archive) EntriesByTitlePrefixFold(ns byte, prefix string) iter.Seq[Entry] {
	folded := strings.ToLower(prefix)
	return func(yield func(Entry) bool) {
		count := a.titleCount()
		for i := range count {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				return
			}
			if e.Namespace() != ns {
				continue
			}
			if hasPrefixFold(e.Title(), folded) {
				if !yield(e) {
					return
				}
			}
		}
	}
}

// AllEntriesByTitlePrefixFold is like [Archive.EntriesByTitlePrefixFold] but
// error-aware. Each step yields (entry, nil) on success or (Entry{}, err) on
// the first parse error, after which iteration stops.
func (a *Archive) AllEntriesByTitlePrefixFold(ns byte, prefix string) iter.Seq2[Entry, error] {
	folded := strings.ToLower(prefix)
	return func(yield func(Entry, error) bool) {
		count := a.titleCount()
		for i := range count {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				yield(Entry{}, err)
				return
			}
			if e.Namespace() != ns {
				continue
			}
			if hasPrefixFold(e.Title(), folded) {
				if !yield(e, nil) {
					return
				}
			}
		}
	}
}

// hasPrefixFold reports whether s starts with prefix under Unicode case folding.
// prefix must already be lowered.
func hasPrefixFold(s, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	return strings.HasPrefix(strings.Map(unicode.ToLower, s), prefix)
}

// HasFulltextIndex reports whether the archive contains an embedded full-text
// Xapian index (X/fulltext/xapian). Such indexes are created by libzim 7+ and
// enable full-text search via external Xapian tools.
func (a *Archive) HasFulltextIndex() bool {
	_, err := a.EntryByPath("X/fulltext/xapian")
	return err == nil
}

// HasTitleIndex reports whether the archive contains an embedded title
// suggestion Xapian index (X/title/xapian).
func (a *Archive) HasTitleIndex() bool {
	_, err := a.EntryByPath("X/title/xapian")
	return err == nil
}

// FulltextIndexFormat returns the Xapian backend format name (e.g. "Glass",
// "Honey") of the embedded full-text index, or "" if no index is present or
// the format cannot be determined.
func (a *Archive) FulltextIndexFormat() string {
	return a.xapianFormat("X/fulltext/xapian")
}

// TitleIndexFormat returns the Xapian backend format name of the embedded
// title suggestion index, or "" if no index is present.
func (a *Archive) TitleIndexFormat() string {
	return a.xapianFormat("X/title/xapian")
}

// xapianFormat reads the Xapian backend magic from an index entry.
// The data starts with: version (1 byte), magic length (1 byte), magic string.
// Known formats: "Xapian Glass" (1.4+), "Xapian Honey" (1.5+),
// "Xapian Chert" (legacy), "Xapian Flint" (obsolete).
func (a *Archive) xapianFormat(path string) string {
	e, err := a.EntryByPath(path)
	if err != nil {
		return ""
	}
	data, err := e.ReadContent()
	if err != nil || len(data) < 4 {
		return ""
	}
	magicLen := int(data[1])
	if magicLen <= 0 || 2+magicLen > len(data) {
		return ""
	}
	magic := strings.TrimRight(string(data[2:2+magicLen]), "\x00\x04")
	// Strip "Xapian " prefix to return just the backend name.
	if after, ok := strings.CutPrefix(magic, "Xapian "); ok {
		return after
	}
	return magic
}
