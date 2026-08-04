package xapian

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hasDoc reports whether a search for term finds anything.
func hasDoc(t *testing.T, dir, term string) bool {
	t.Helper()
	db, err := OpenDBMulti([]string{dir})
	if err != nil {
		t.Fatalf("OpenDBMulti: %v", err)
	}
	defer db.Close()
	q, err := QueryTerm(term)
	if err != nil {
		t.Fatalf("QueryTerm %q: %v", term, err)
	}
	defer q.Free()
	hits, err := db.Search(q)
	if err != nil {
		t.Fatalf("Search %q: %v", term, err)
	}
	return len(hits) > 0
}

// AddTerm hands C the bytes of a Go string, so the one thing that would make it
// unsafe is Xapian keeping the pointer. It does not — add_term builds a
// std::string and stores that — and this checks the consequence rather than the
// claim: every source string is made unreachable and the heap is churned before
// the terms are read back.
//
// A retained pointer would not fail loudly. It would return whatever landed in
// that memory afterwards, which is why this is asserted rather than reasoned
// about.
func TestTermsSurviveTheStringsTheyCameFrom(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")

	terms := make([]string, 0, 64)
	for i := 0; i < 64; i++ {
		// Built at run time, so each is a fresh allocation rather than a
		// constant living in the binary for the whole process.
		terms = append(terms, fmt.Sprintf("term%s%d", strings.Repeat("x", i%7), i))
	}
	writeDoc(t, dir, 1, terms, nil)

	// Churn the heap hard enough that anything freed would be overwritten.
	runtime.GC()
	churn := make([][]byte, 0, 4096)
	for i := 0; i < 4096; i++ {
		b := make([]byte, 64)
		for j := range b {
			b[j] = 'Z'
		}
		churn = append(churn, b)
	}
	runtime.GC()
	runtime.KeepAlive(churn)

	for _, term := range terms {
		if !hasDoc(t, dir, term) {
			t.Errorf("term %q is not in the index — what was stored did not outlive the Go string it came from", term)
		}
	}
}

// A zero-length term must reach C as a valid pointer with length zero rather
// than as nil: unsafe.StringData("") may be nil, and the two are not the same
// argument to C.
func TestEmptyTermDoesNotPassNil(t *testing.T) {
	doc := NewDoc()
	defer doc.Free()
	// What Xapian makes of an empty term is Xapian's business; not handing it a
	// null pointer is the binding's.
	_ = doc.AddTerm("")
	_ = doc.AddBooleanTerm("")
}

// Terms are bytes, not text. A term carrying a NUL would have been truncated
// there by any NUL-terminated interface — silently indexing something shorter
// than the caller asked for, which no error would report.
func TestTermWithAnInteriorNULIsKeptWhole(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	const term = "before\x00after"
	writeDoc(t, dir, 1, []string{term}, nil)

	if !hasDoc(t, dir, term) {
		t.Errorf("%q is not in the index — it was truncated", term)
	}
	if hasDoc(t, dir, "before") {
		t.Error(`the term was stored as "before" — truncated at the NUL`)
	}
}

// The measurement the change was made for. Run with -bench AddTerm; the two
// arms differ only in whether the term is copied into the C heap first.
func BenchmarkAddTerm(b *testing.B) {
	terms := make([]string, 512)
	for i := range terms {
		terms[i] = fmt.Sprintf("term%s%d", strings.Repeat("x", i%9), i)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		doc := NewDoc()
		for _, t := range terms {
			if err := doc.AddTerm(t); err != nil {
				b.Fatalf("AddTerm: %v", err)
			}
		}
		doc.Free()
	}
}
