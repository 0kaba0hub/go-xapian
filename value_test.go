package xapian

import (
	"path/filepath"
	"testing"
)

// A value comes back with the hit: a database holding more than one mailbox
// cannot say from the docid alone which message a hit is.
func TestSearchCarriesTheDocumentValue(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	w, err := OpenWDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, pair := range []struct {
		term  string
		value string
	}{{"Zhello", "INBOX:11"}, {"Zhello", "Archive:4"}} {
		d := NewDoc()
		if err := d.AddTerm(pair.term); err != nil {
			t.Fatal(err)
		}
		if err := d.SetValue(1, pair.value); err != nil {
			t.Fatal(err)
		}
		if err := w.ReplaceDocument(uint32(i+1), d); err != nil {
			t.Fatal(err)
		}
		d.Free()
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	w.Close()

	db, err := OpenDBMulti([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q, err := QueryTerm("Zhello")
	if err != nil {
		t.Fatal(err)
	}
	defer q.Free()

	hits, err := db.SearchWithValue(q, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("the search found %d documents, two were written", len(hits))
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Value] = true
	}
	if !seen["INBOX:11"] || !seen["Archive:4"] {
		t.Errorf("the hits carry %v, the documents were written with INBOX:11 and Archive:4", seen)
	}

	// Search without the value keeps its old shape: nothing is read that the
	// caller did not ask for.
	plain, err := db.Search(q)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range plain {
		if h.Value != "" {
			t.Errorf("a plain search carried a value: %q", h.Value)
		}
	}
}
