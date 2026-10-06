package files

import (
	"io"

	"github.com/standards-lab/blobfs"
)

// TotalMode says whether a listing asks for its total.
type TotalMode int

const (
	// TotalExact, the default, asks for the total in the same statement as
	// the page.
	TotalExact TotalMode = iota

	// TotalNone omits the total. The page's Total is NoTotal.
	TotalNone
)

// NoTotal is the Total of a page that carries none: the listing asked for
// TotalNone, the half was read after a cursor, or the page is empty and is
// not the first, so no row carried the count.
const NoTotal = -1

// Listing is one page request of ls, as the command line states it: the
// 1-based page and its size, the filters and the sort terms in order, the
// total mode, and the cursors to continue each half from. It is the
// domain's own shape of a read request; database.go lowers it to the query
// library's directives, since no other file of the package names them.
type Listing struct {
	Page    int
	Size    int
	Filters []Filter
	Sort    []Sort
	Total   TotalMode
	After   After
}

// Filter is one filter term of a Listing: a declared field of a listing,
// an operator by the query library's name (eq, ne, gt, ge, lt, le, like,
// null, notnull, in), and the value, which is the text the command line
// gave and which the engine casts to the field's type; null and notnull
// take no value, and in takes a []any of texts. blobfs refuses an unknown
// field, an unknown operator, or a value of the wrong shape before the
// statement runs. The file half of ls takes every filter and the directory
// half those naming a field both halves share, as the sort terms are
// taken.
type Filter struct {
	Field string
	Op    string
	Value any
}

// Sort is one sort term of a Listing: a declared field of a listing and
// its direction.
type Sort struct {
	Field      string
	Descending bool
}

// After holds the cursors a listing continues from, one per half, each the
// Next of an earlier page of that half under the same sort and filters. A
// half whose cursor is empty is read by page number. A half read by cursor
// ignores Page and carries no total, whatever Total says.
type After struct {
	Directories string
	Files       string
}

// Page is one page of one half of a listing: its rows, its total, which is
// NoTotal when the page carries none, More, whether rows remain after this
// page, and Next, the cursor that continues the half after this page,
// empty on the last page and when the half's sort cannot be continued by
// cursor.
type Page[T any] struct {
	Rows  []T
	Total int
	More  bool
	Next  string
}

// Contents is what ls returns for one directory: the path it listed, empty
// for a listing by id, the directories under it, and the files in it, each
// one page under the same Listing with its own total. Both halves are read
// in one read-only repeatable-read transaction, so they agree with each
// other.
type Contents struct {
	Path        string
	Directories Page[blobfs.Directory]
	Files       Page[blobfs.File]
}

// EntryKind names what an entry of the tree is.
type EntryKind string

const (
	// EntryDirectory is a directory.
	EntryDirectory EntryKind = "directory"

	// EntryFile is a file.
	EntryFile EntryKind = "file"
)

// Entry is the row an id names, as stat and mv find it by id: a file, or a
// directory when no file has the id. Kind says which of the two rows is
// set.
type Entry struct {
	Kind      EntryKind
	File      blobfs.File
	Directory blobfs.Directory
}

// MoveRequest is one move by id, as MoveEntry takes it: the kind and id of
// the entry to move, the id of the directory it moves into, the name it
// takes there (empty keeps its name), and the version the caller read (0
// when none was read, in which case the row's current version guards the
// move).
type MoveRequest struct {
	Kind        EntryKind
	ID          string
	DirectoryID string
	Name        string
	Version     int64
}

// MoveResult is what mv returns: what kind of entry moved, its id, the
// path it was at, and the path it is at now, which is the destination
// itself when the destination named a new path and the destination with
// the source's name appended when it named an existing directory.
type MoveResult struct {
	Kind EntryKind
	ID   string
	From string
	To   string
}

// Content is what a put writes: the body, its length in bytes when known
// (0 when it is not, as for standard input, so the store takes the body
// to its end), and the content type the file declares.
type Content struct {
	Body        io.Reader
	Size        int64
	ContentType string
}

// PutResult is what put returns: the row, available, and whether the put
// resumed a pending row an earlier put left rather than write a new one.
type PutResult struct {
	File    blobfs.File
	Resumed bool
}

// CopyResult is what cp returns: the source's path, the copy's path, and
// the copy's row, available.
type CopyResult struct {
	From string
	To   string
	File blobfs.File
}

// TreeRemoval is what rm --recursive returns: how many files and
// directories its sweep removed, summed over its passes.
type TreeRemoval struct {
	Files       int
	Directories int
}
