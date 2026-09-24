// Package xapian is a thin cgo binding over the Xapian C++ search library. It
// exposes only writable/read databases, documents, queries and search results,
// with no application-domain assumptions, so it can be reused by any Go program
// that needs full-text search on top of Xapian.
//
// cgo cannot call C++ directly (name mangling, templates, exceptions, RAII), so
// every call crosses through the extern "C" shim in shim.cc / shim.h, which
// converts Xapian C++ exceptions into malloc'd error strings. All handles are
// opaque pointers owned by the caller until the matching Close/Free.
//
// The package requires libxapian with its development headers at build time.
package xapian

/*
#cgo CXXFLAGS: -std=c++17
#cgo darwin CXXFLAGS: -I/opt/homebrew/include
#cgo darwin LDFLAGS: -L/opt/homebrew/lib
#cgo LDFLAGS: -lxapian

#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"
)

func takeErr(cerr *C.char) error {
	if cerr == nil {
		return errors.New("xapian: unknown error")
	}
	defer C.free(unsafe.Pointer(cerr))
	return errors.New("xapian: " + C.GoString(cerr))
}

// WDB is a writable Xapian database handle.
type WDB struct{ h unsafe.Pointer }

// OpenWDB opens (creating if absent) a writable database at path.
func OpenWDB(path string) (*WDB, error) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	var cerr *C.char
	h := C.fcx_wdb_open(cp, &cerr)
	if h == nil {
		return nil, takeErr(cerr)
	}
	return &WDB{h: h}, nil
}

// Commit flushes pending changes to disk.
func (w *WDB) Commit() error {
	var cerr *C.char
	if C.fcx_wdb_commit(w.h, &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// Close releases the handle. Idempotent.
func (w *WDB) Close() {
	if w.h != nil {
		C.fcx_wdb_close(w.h)
		w.h = nil
	}
}

// ReplaceDocument stores d under docid, replacing any existing document.
func (w *WDB) ReplaceDocument(docid uint32, d *Doc) error {
	var cerr *C.char
	if C.fcx_wdb_replace_document(w.h, C.uint(docid), d.h, &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// AddDocument lets the database choose the id, which is what a store keyed by
// a document value rather than by its number wants.
func (w *WDB) AddDocument(d *Doc) (uint32, error) {
	var cerr *C.char
	id := C.fcx_wdb_add_document(w.h, d.h, &cerr)
	if id == 0 {
		return 0, takeErr(cerr)
	}
	return uint32(id), nil
}

// DeleteDocument removes docid. existed reports whether it was present;
// a not-found document is not an error.

func (w *WDB) DeleteDocument(docid uint32) (existed bool, err error) {
	var cerr *C.char
	var cex C.int
	if C.fcx_wdb_delete_document(w.h, C.uint(docid), &cex, &cerr) != 0 {
		return false, takeErr(cerr)
	}
	return cex != 0, nil
}

// DeleteByTerm removes every document carrying term: with a term unique to a
// document that is the delete of that one.
func (w *WDB) DeleteByTerm(term string) error {
	var cerr *C.char
	if C.fcx_wdb_delete_by_term(w.h, termPtr(term), C.size_t(len(term)), &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// DocIDsByTerm reads the term's posting list: the ids of the documents that
// carry it, ascending, without asking the matcher anything.
func (w *WDB) DocIDsByTerm(term string) ([]uint32, error) {
	return docIDsByTerm(func(buf *C.uint, cap C.size_t, cerr **C.char) C.int {
		return C.fcx_wdb_docids_by_term(w.h, termPtr(term), C.size_t(len(term)), buf, cap, cerr)
	})
}

// DocTerms lists one document's terms that start with prefix. An empty prefix
// lists them all.
func (w *WDB) DocTerms(docid uint32, prefix string) ([]string, error) {
	terms, _, err := w.docTerms(docid, prefix)
	return terms, err
}

// docTerms also says how many terms the walk looked at, which is what tells a
// prefix range from a walk of the whole document.
func (w *WDB) docTerms(docid uint32, prefix string) ([]string, int, error) {
	var cerr *C.char
	var n, examined C.size_t
	p := C.fcx_wdb_doc_terms(w.h, C.uint(docid), termPtr(prefix), C.size_t(len(prefix)), &n, &examined, &cerr)
	if p == nil {
		if cerr != nil {
			return nil, 0, takeErr(cerr)
		}
		return nil, int(examined), nil
	}
	defer C.free(unsafe.Pointer(p))
	block := C.GoStringN(p, C.int(n))
	var out []string
	for _, t := range strings.Split(block, "\x00") {
		if t != "" {
			out = append(out, t)
		}
	}
	return out, int(examined), nil
}

// SetMetadata stores an arbitrary key/value pair in the database metadata.
func (w *WDB) SetMetadata(key, value string) error {
	ck := C.CString(key)
	cv := C.CString(value)
	defer C.free(unsafe.Pointer(ck))
	defer C.free(unsafe.Pointer(cv))
	var cerr *C.char
	if C.fcx_wdb_set_metadata(w.h, ck, cv, &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// GetMetadata reads a metadata value; a missing key returns "".
func (w *WDB) GetMetadata(key string) (string, error) {
	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))
	var cerr *C.char
	cv := C.fcx_wdb_get_metadata(w.h, ck, &cerr)
	if cv == nil {
		return "", takeErr(cerr)
	}
	defer C.free(unsafe.Pointer(cv))
	return C.GoString(cv), nil
}

// DocCount returns the number of documents in the writable database.

func (w *WDB) DocCount() (uint32, error) {
	var cerr *C.char
	n := C.fcx_wdb_get_doccount(w.h, &cerr)
	if cerr != nil {
		return 0, takeErr(cerr)
	}
	return uint32(n), nil
}

// LastDocID is the highest id the database has handed out, which is what says
// how close a long-lived store is to the limit of the type.
func (w *WDB) LastDocID() (uint32, error) {
	var cerr *C.char
	id := C.fcx_wdb_last_docid(w.h, &cerr)
	if id == 0 && cerr != nil {
		return 0, takeErr(cerr)
	}
	return uint32(id), nil
}

// DocExists reports whether docid is present.
func (w *WDB) DocExists(docid uint32) (bool, error) {
	var cerr *C.char
	r := C.fcx_wdb_doc_exists(w.h, C.uint(docid), &cerr)
	if r < 0 {
		return false, takeErr(cerr)
	}
	return r == 1, nil
}

// DB is a read-only Xapian database, optionally combining several on-disk
// shards into one searchable view.
type DB struct{ h unsafe.Pointer }

// OpenDBMulti opens paths as one combined read-only database. An empty paths
// slice returns (nil, nil).
func OpenDBMulti(paths []string) (*DB, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	cpaths := make([]*C.char, len(paths))
	for i, p := range paths {
		cpaths[i] = C.CString(p)
	}
	defer func() {
		for _, cp := range cpaths {
			C.free(unsafe.Pointer(cp))
		}
	}()
	var cerr *C.char
	h := C.fcx_db_open_multi(&cpaths[0], C.size_t(len(paths)), &cerr)
	if h == nil {
		return nil, takeErr(cerr)
	}
	return &DB{h: h}, nil
}

// Close releases the handle. Idempotent.
func (d *DB) Close() {
	if d.h != nil {
		C.fcx_db_close(d.h)
		d.h = nil
	}
}

// LastDocID returns the highest document id across the combined shards.
func (d *DB) LastDocID() (uint32, error) {
	var cerr *C.char
	n := C.fcx_db_get_lastdocid(d.h, &cerr)
	if cerr != nil {
		return 0, takeErr(cerr)
	}
	return uint32(n), nil
}

// DocIDs returns every document id in ascending order.
func (d *DB) DocIDs() ([]uint32, error) {
	var out []uint32
	buf := make([]C.uint, 4096)
	prev := C.uint(0)
	for {
		var cerr *C.char
		n := C.fcx_db_docids(d.h, prev, &buf[0], C.size_t(len(buf)), &cerr)
		if n < 0 {
			return nil, takeErr(cerr)
		}
		for i := 0; i < int(n); i++ {
			out = append(out, uint32(buf[i]))
		}
		if int(n) < len(buf) {
			return out, nil
		}
		prev = buf[n-1]
	}
}

// Compact writes a single optimized copy of the combined database to dest.
func (d *DB) Compact(dest string) error {
	cd := C.CString(dest)
	defer C.free(unsafe.Pointer(cd))
	var cerr *C.char
	if C.fcx_db_compact(d.h, cd, &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// Doc is a Xapian document under construction.
type Doc struct{ h unsafe.Pointer }

// NewDoc allocates an empty document. The caller must Free it (or hand it to
// WDB.ReplaceDocument, which does not take ownership — Free it afterwards).
func NewDoc() *Doc { return &Doc{h: C.fcx_doc_new()} }

// Free releases the document. Idempotent.
func (d *Doc) Free() {
	if d.h != nil {
		C.fcx_doc_free(d.h)
		d.h = nil
	}
}

// AddTerm indexes a free-text term.
//
// The term's own bytes are passed rather than a C copy. Adding a term is the
// one call an indexer makes per token, and C.CString made each one cost three
// crossings into C — malloc, the call, free — where one is needed: measured at
// 71ns per term against 16ns for the call alone. Xapian builds a std::string
// from the bytes and keeps that, so nothing here outlives the call.
func (d *Doc) AddTerm(term string) error {
	var cerr *C.char
	if C.fcx_doc_add_term(d.h, termPtr(term), C.size_t(len(term)), &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// AddBooleanTerm indexes a boolean (filter) term.
func (d *Doc) AddBooleanTerm(term string) error {
	var cerr *C.char
	if C.fcx_doc_add_boolean_term(d.h, termPtr(term), C.size_t(len(term)), &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// SetValue stores opaque bytes in a numbered slot. A value is not indexed and
// not searchable, and unlike a term it comes back with a search hit.
func (d *Doc) SetValue(slot uint32, value string) error {
	var cerr *C.char
	if C.fcx_doc_set_value(d.h, C.uint(slot), termPtr(value), C.size_t(len(value)), &cerr) != 0 {
		return takeErr(cerr)
	}
	return nil
}

// empty backs the pointer for a zero-length term: unsafe.StringData("") may be
// nil, and a nil pointer with length zero is not the same argument to C as a
// valid pointer with length zero.
var empty = [1]byte{}

func termPtr(s string) *C.char {
	if len(s) == 0 {
		return (*C.char)(unsafe.Pointer(&empty[0]))
	}
	return (*C.char)(unsafe.Pointer(unsafe.StringData(s)))
}

// Query is a Xapian query tree node.
type Query struct{ h unsafe.Pointer }

// QueryWildcard builds a wildcard (prefix) query from pattern.
func QueryWildcard(pattern string) (*Query, error) {
	cp := C.CString(pattern)
	defer C.free(unsafe.Pointer(cp))
	var cerr *C.char
	h := C.fcx_query_wildcard(cp, &cerr)
	if h == nil {
		return nil, takeErr(cerr)
	}
	return &Query{h: h}, nil
}

// QueryTerm builds an exact-term query.
func QueryTerm(term string) (*Query, error) {
	var cerr *C.char
	h := C.fcx_query_term(termPtr(term), C.size_t(len(term)), &cerr)
	if h == nil {
		return nil, takeErr(cerr)
	}
	return &Query{h: h}, nil
}

// QueryMatchAll builds a query that matches every document.
func QueryMatchAll() (*Query, error) {
	var cerr *C.char
	h := C.fcx_query_match_all(&cerr)
	if h == nil {
		return nil, takeErr(cerr)
	}
	return &Query{h: h}, nil
}

// Op selects how QueryCombine joins two subqueries.
type Op int

const (
	OpAND    = Op(C.FCX_OP_AND)
	OpOR     = Op(C.FCX_OP_OR)
	OpANDNOT = Op(C.FCX_OP_AND_NOT)
)

// QueryCombine joins a and b under op. It consumes a and b (both are freed
// here) and returns the combined query.
func QueryCombine(op Op, a, b *Query) (*Query, error) {
	defer a.Free()
	defer b.Free()
	var cerr *C.char
	h := C.fcx_query_combine(C.int(op), a.h, b.h, &cerr)
	if h == nil {
		return nil, takeErr(cerr)
	}
	return &Query{h: h}, nil
}

// Free releases the query. Safe on a nil receiver or nil handle.
func (q *Query) Free() {
	if q != nil && q.h != nil {
		C.fcx_query_free(q.h)
		q.h = nil
	}
}

// MSetEntry is one search hit: a document id and its relevance weight.
type MSetEntry struct {
	DocID  uint32
	Weight float64
	// Value is the document value asked for by SearchWithValue, empty otherwise.
	Value string
}

// SearchWithValue is Search with the value in slot carried back on every hit,
// which is what a docid alone no longer says.
func (d *DB) SearchWithValue(q *Query, slot uint32) ([]MSetEntry, error) {
	return d.search(q, true, slot)
}

// DocIDsByTerm reads the term's posting list on the read database.
func (d *DB) DocIDsByTerm(term string) ([]uint32, error) {
	return docIDsByTerm(func(buf *C.uint, cap C.size_t, cerr **C.char) C.int {
		return C.fcx_db_docids_by_term(d.h, termPtr(term), C.size_t(len(term)), buf, cap, cerr)
	})
}

// docIDsByTerm grows the buffer until the shim stops filling it: a term may
// name one document or every document in the database.
func docIDsByTerm(fill func(buf *C.uint, cap C.size_t, cerr **C.char) C.int) ([]uint32, error) {
	for capacity := 64; ; capacity *= 4 {
		buf := make([]C.uint, capacity)
		var cerr *C.char
		n := int(fill(&buf[0], C.size_t(capacity), &cerr))
		if n < 0 {
			return nil, takeErr(cerr)
		}
		if n == capacity {
			continue
		}
		out := make([]uint32, n)
		for i := 0; i < n; i++ {
			out[i] = uint32(buf[i])
		}
		return out, nil
	}
}

// Search runs q against the read database and returns the ranked hits.
func (d *DB) Search(q *Query) ([]MSetEntry, error) {
	return d.search(q, false, 0)
}

func (d *DB) search(q *Query, withValue bool, slot uint32) ([]MSetEntry, error) {
	var cerr *C.char
	m := C.fcx_db_search(d.h, q.h, &cerr)
	if m == nil {
		return nil, takeErr(cerr)
	}
	defer C.fcx_mset_free(m)
	n := int(C.fcx_mset_size(m))
	out := make([]MSetEntry, 0, n)
	for i := 0; i < n; i++ {
		var w C.double
		docid := C.fcx_mset_docid(m, C.size_t(i), &w)
		ent := MSetEntry{DocID: uint32(docid), Weight: float64(w)}
		if withValue {
			var vlen C.size_t
			var verr *C.char
			vp := C.fcx_mset_value(m, C.size_t(i), C.uint(slot), &vlen, &verr)
			if vp != nil {
				ent.Value = C.GoStringN(vp, C.int(vlen))
				C.free(unsafe.Pointer(vp))
			} else if verr != nil {
				return nil, takeErr(verr)
			}
		}
		out = append(out, ent)
	}
	return out, nil
}
