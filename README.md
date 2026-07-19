# go-xapian

A thin, dependency-free cgo binding over the [Xapian](https://xapian.org) C++
search library. It exposes writable/read databases, documents, queries and
search results — with no application-domain assumptions — so any Go program can
build full-text search on top of Xapian.

Because cgo cannot call C++ directly (name mangling, templates, exceptions,
RAII), every call crosses through a small `extern "C"` shim (`shim.cc` /
`shim.h`) that converts Xapian C++ exceptions into Go errors. All handles are
opaque pointers owned by the caller until the matching `Close`/`Free`.

## Requirements

- Go 1.26+
- Xapian with development headers at build time:
  - Debian/Ubuntu: `sudo apt-get install libxapian-dev`
  - macOS (Homebrew): `brew install xapian`

## Usage

```go
package main

import (
	"fmt"

	"github.com/0kaba0hub/go-xapian"
)

func main() {
	w, err := xapian.OpenWDB("/tmp/idx")
	if err != nil {
		panic(err)
	}
	doc := xapian.NewDoc()
	_ = doc.AddTerm("hello")
	_ = w.ReplaceDocument(1, doc)
	doc.Free()
	_ = w.Commit()
	w.Close()

	db, _ := xapian.OpenDBMulti([]string{"/tmp/idx"})
	defer db.Close()
	q, _ := xapian.QueryTerm("hello")
	defer q.Free()
	hits, _ := db.Search(q)
	fmt.Println(hits) // [{1 ...}]
}
```

## API surface

- `WDB` — writable database: `OpenWDB`, `Commit`, `Close`, `ReplaceDocument`,
  `DeleteDocument`, `SetMetadata`, `GetMetadata`, `DocCount`, `DocExists`.
- `DB` — read database (combines shards): `OpenDBMulti`, `Close`, `LastDocID`,
  `DocIDs`, `Compact`, `Search`.
- `Doc` — document builder: `NewDoc`, `Free`, `AddTerm`, `AddBooleanTerm`.
- `Query` — query tree: `QueryWildcard`, `QueryTerm`, `QueryMatchAll`,
  `QueryCombine`, `Free`.
- `MSetEntry` — one search hit (`DocID`, `Weight`).

## License

GPL-2.0-or-later — see [LICENSE](LICENSE). (Xapian itself is GPL-2.0-or-later;
linking it makes this binding GPL too.)
