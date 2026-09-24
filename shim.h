/* C ABI over the Xapian C++ API — the minimal surface the flatcurve engine
 * needs. Every call reports failure through err_out (malloc'd message the
 * caller frees); handles are opaque pointers owned by the caller until the
 * matching *_free / *_close. */
#ifndef YARILO_FTS_FLATCURVE_SHIM_H
#define YARILO_FTS_FLATCURVE_SHIM_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void fcx_wdb;   /* Xapian::WritableDatabase */
typedef void fcx_db;    /* Xapian::Database (combined reader) */
typedef void fcx_doc;   /* Xapian::Document */
typedef void fcx_query; /* Xapian::Query */
typedef void fcx_mset;  /* Xapian::MSet */

/* --- writable database ------------------------------------------------- */
fcx_wdb *fcx_wdb_open(const char *path, char **err_out);
int fcx_wdb_commit(fcx_wdb *w, char **err_out);
void fcx_wdb_close(fcx_wdb *w);
/* Lets the database choose the id: returns it, or 0 with err_out set. */
unsigned int fcx_wdb_add_document(fcx_wdb *w, fcx_doc *d, char **err_out);
int fcx_wdb_replace_document(fcx_wdb *w, unsigned int docid, fcx_doc *d,
                             char **err_out);
/* existed_out: 1 when the document was present. DocNotFound is not an error. */
/* Deletes every document carrying term (Xapian's unique-term delete). */
int fcx_wdb_delete_by_term(fcx_wdb *w, const char *term, size_t len,
                           char **err_out);
/* Docids carrying term, ascending, up to cap; returns the count, -1 on error.
 * Reads the postlist, so it costs no match decision. */
int fcx_wdb_docids_by_term(fcx_wdb *w, const char *term, size_t len,
                           unsigned int *buf, size_t cap, char **err_out);
/* The terms of one document that start with prefix, as a NUL-separated block.
 * The caller frees the block with free(); len_out is its length. The walk
 * skips to the prefix and stops at its end; examined_out, when not NULL, is
 * how many terms it looked at. */
char *fcx_wdb_doc_terms(fcx_wdb *w, unsigned int docid, const char *prefix,
                        size_t plen, size_t *len_out, size_t *examined_out,
                        char **err_out);
int fcx_wdb_delete_document(fcx_wdb *w, unsigned int docid, int *existed_out,
                            char **err_out);
int fcx_wdb_set_metadata(fcx_wdb *w, const char *key, const char *value,
                         char **err_out);
char *fcx_wdb_get_metadata(fcx_wdb *w, const char *key, char **err_out);
unsigned int fcx_wdb_get_doccount(fcx_wdb *w, char **err_out);
/* The highest id handed out; 0 with err_out set on error, 0 and no error on an
 * empty database. Says how close a long-lived store is to the type's limit. */
unsigned int fcx_wdb_last_docid(fcx_wdb *w, char **err_out);
int fcx_wdb_doc_exists(fcx_wdb *w, unsigned int docid, char **err_out);

/* --- combined read-only database ---------------------------------------- */
fcx_db *fcx_db_open_multi(const char *const *paths, size_t n, char **err_out);
void fcx_db_close(fcx_db *db);
unsigned int fcx_db_get_lastdocid(fcx_db *db, char **err_out);
unsigned int fcx_db_get_doccount(fcx_db *db, char **err_out);
/* Fills up to cap docids starting after prev (0 = from the beginning);
 * returns the count written, -1 on error. Iteration order is ascending. */
int fcx_db_docids(fcx_db *db, unsigned int prev, unsigned int *buf,
                  size_t cap, char **err_out);
int fcx_db_compact(fcx_db *db, const char *dest, char **err_out);

/* --- document ------------------------------------------------------------ */
fcx_doc *fcx_doc_new(void);
void fcx_doc_free(fcx_doc *d);
/* term is not NUL-terminated: the caller passes the bytes and their length, so
 * a Go string can be handed over without a C copy. Both build a std::string
 * that Xapian keeps; neither retains the pointer past the call. */
int fcx_doc_add_term(fcx_doc *d, const char *term, size_t len, char **err_out);
int fcx_doc_add_boolean_term(fcx_doc *d, const char *term, size_t len, char **err_out);
/* A document value: opaque bytes in a numbered slot, returned with a search
 * hit. Unlike a term it is not indexed, and unlike a term it can be read back. */
/* The document as the database holds it, for editing and replacing. Freed
 * with fcx_doc_free. */
fcx_doc *fcx_wdb_get_document(fcx_wdb *w, unsigned int docid, char **err_out);
/* Removes one term; a term the document does not carry is not an error. */
int fcx_doc_remove_term(fcx_doc *d, const char *term, size_t len,
                        char **err_out);
int fcx_doc_set_value(fcx_doc *d, unsigned int slot, const char *val, size_t len,
                      char **err_out);

/* --- query --------------------------------------------------------------- */
/* op values mirror Xapian::Query::op */
enum {
	FCX_OP_AND = 0,
	FCX_OP_OR = 1,
	FCX_OP_AND_NOT = 2
};
fcx_query *fcx_query_wildcard(const char *pattern, char **err_out);
/* term is not NUL-terminated; see fcx_doc_add_term. Kept symmetric with it
 * so a term that can be indexed can also be queried. */
fcx_query *fcx_query_term(const char *term, size_t len, char **err_out);
fcx_query *fcx_query_match_all(char **err_out);
fcx_query *fcx_query_combine(int op, fcx_query *a, fcx_query *b,
                             char **err_out);
void fcx_query_free(fcx_query *q);

/* --- search --------------------------------------------------------------- */
fcx_mset *fcx_db_search(fcx_db *db, fcx_query *q, char **err_out);
size_t fcx_mset_size(fcx_mset *m);
/* idx < fcx_mset_size(); weight_out may be NULL. */
unsigned int fcx_mset_docid(fcx_mset *m, size_t idx, double *weight_out);
/* The value in slot for the hit at idx. The bytes belong to the caller and are
 * freed with free(); len_out is set to their length. NULL means empty. */
/* Docids carrying term, ascending, up to cap; returns the count, -1 on error. */
int fcx_db_docids_by_term(fcx_db *db, const char *term, size_t len,
                          unsigned int *buf, size_t cap, char **err_out);
char *fcx_mset_value(fcx_mset *m, size_t idx, unsigned int slot, size_t *len_out,
                     char **err_out);
void fcx_mset_free(fcx_mset *m);

#ifdef __cplusplus
}
#endif

#endif
