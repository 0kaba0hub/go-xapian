package xapian

import (
	"fmt"
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

// The database chooses the id, so nothing on our side counts documents: the
// identity a hit is recognised by is the value, not the number.
func TestAddDocumentLetsTheDatabaseChooseTheID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	w, err := OpenWDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint32
	for _, v := range []string{"INBOX:1", "INBOX:2", "Archive:1"} {
		d := NewDoc()
		if err := d.AddTerm("Zx"); err != nil {
			t.Fatal(err)
		}
		if err := d.SetValue(1, v); err != nil {
			t.Fatal(err)
		}
		id, aerr := w.AddDocument(d)
		if aerr != nil {
			t.Fatal(aerr)
		}
		d.Free()
		ids = append(ids, id)
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	last, err := w.LastDocID()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	seen := map[uint32]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			t.Fatalf("the ids are %v, which is not three distinct non-zero ids", ids)
		}
		seen[id] = true
	}
	if last < ids[len(ids)-1] {
		t.Errorf("the last id reads %d, the database handed out %v", last, ids)
	}
}

// A unique term is the address of a document: it finds it, reads what it
// carries, and deletes it, with no match decision in between.
func TestATermFindsReadsAndDeletesTheDocument(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	w, err := OpenWDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, folders := range [][]string{{"XFa", "XFb"}, {"XFb"}} {
		d := NewDoc()
		if err := d.AddBooleanTerm("G" + folders[0]); err != nil {
			t.Fatal(err)
		}
		for _, f := range folders {
			if err := d.AddBooleanTerm(f); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := w.AddDocument(d); err != nil {
			t.Fatal(err)
		}
		d.Free()
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}

	ids, err := w.DocIDsByTerm("XFb")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("the term names %d documents, two carry it", len(ids))
	}
	terms, err := w.DocTerms(ids[0], "XF")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 2 || terms[0] != "XFa" || terms[1] != "XFb" {
		t.Errorf("the document carries %v, want XFa and XFb", terms)
	}

	if err := w.DeleteByTerm("XFa"); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	left, err := w.DocIDsByTerm("XFb")
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 {
		t.Errorf("after deleting by one term %d documents carry the other, want 1", len(left))
	}
}

// The prefix is a range in a sorted term list, not a filter over it: a message
// carries hundreds of text terms and the copy-removal path wants two.
func TestDocTermsWalksThePrefixRangeOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	w, err := OpenWDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	d := NewDoc()
	for i := 0; i < 500; i++ {
		if err := d.AddTerm(fmt.Sprintf("Zword%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"XFaaa", "XFbbb"} {
		if err := d.AddBooleanTerm(f); err != nil {
			t.Fatal(err)
		}
	}
	id, err := w.AddDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Free()
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}

	terms, examined, err := w.docTerms(id, "XF")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 2 || terms[0] != "XFaaa" || terms[1] != "XFbbb" {
		t.Fatalf("the document answers %v, want the two XF terms", terms)
	}
	// Two in the range and the one after it that ends the walk: anything near
	// 502 means the whole term list was read.
	if examined > 3 {
		t.Errorf("the walk looked at %d terms for two, so it did not skip to the prefix", examined)
	}
}

// A copy is removed from a message's document by stripping its terms, not by
// rewriting the document: the text terms and their positions cannot be rebuilt.
func TestATermIsStrippedFromAStoredDocument(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	w, err := OpenWDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	d := NewDoc()
	for _, term := range []string{"Zbody", "XFaaa", "XFbbb"} {
		if err := d.AddTerm(term); err != nil {
			t.Fatal(err)
		}
	}
	id, err := w.AddDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Free()
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}

	stored, err := w.GetDocument(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := stored.RemoveTerm("XFaaa"); err != nil {
		t.Fatal(err)
	}
	// A term it does not carry is not an error: the copy is already gone.
	if err := stored.RemoveTerm("XFzzz"); err != nil {
		t.Fatalf("removing a term the document does not carry failed: %v", err)
	}
	if err := w.ReplaceDocument(id, stored); err != nil {
		t.Fatal(err)
	}
	stored.Free()
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}

	terms, err := w.DocTerms(id, "XF")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0] != "XFbbb" {
		t.Errorf("the document carries %v after the strip, want XFbbb alone", terms)
	}
	body, err := w.DocIDsByTerm("Zbody")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 {
		t.Errorf("the text term names %d documents after the strip, want 1", len(body))
	}
}
