package zim

import "iter"

// Entries returns an iterator over all entries sorted by path (URL order).
// Iteration stops at the first parse error; use [Archive.AllEntries] for
// error-aware iteration.
func (a *Archive) Entries() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for i := range a.hdr.EntryCount {
			e, err := a.EntryByIndex(i)
			if err != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// AllEntries returns an error-aware iterator over all entries sorted by path
// (URL order). Each step yields (entry, nil) on success or (Entry{}, err) on
// the first parse error, after which iteration stops.
func (a *Archive) AllEntries() iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		for i := range a.hdr.EntryCount {
			e, err := a.EntryByIndex(i)
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

// EntriesByNamespace returns an iterator over entries in a specific namespace.
// Uses binary search to find the namespace bounds — O(log N + k) where k is
// the number of entries in the namespace. Iteration stops at the first parse
// error; use [Archive.AllEntriesByNamespace] for error-aware iteration.
func (a *Archive) EntriesByNamespace(ns byte) iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		lo, hi := a.namespaceBounds(ns)
		for i := lo; i < hi; i++ {
			e, err := a.EntryByIndex(i)
			if err != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// AllEntriesByNamespace returns an error-aware iterator over entries in a
// specific namespace. Uses binary search to find the namespace bounds —
// O(log N + k). Each step yields (entry, nil) on success or (Entry{}, err)
// on the first parse error, after which iteration stops.
func (a *Archive) AllEntriesByNamespace(ns byte) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		lo, hi := a.namespaceBounds(ns)
		for i := lo; i < hi; i++ {
			e, err := a.EntryByIndex(i)
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
