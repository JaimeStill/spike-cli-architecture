package files

import (
	"context"
	"fmt"

	bfdata "github.com/standards-lab/blobfs/data"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
)

// Store is the domain's client over the database alone: blobfs's
// persistence compiled against the domain's pattern catalog, and the
// session it runs on. It holds no object store, so the directory commands
// run with the store's configuration unread and the store unreachable. Its
// methods are the domain's directory operations. Only this file imports the
// query library: it lowers a Listing to the query library's directives, so
// the operations compose blobfs's methods without naming it.
type Store struct {
	db     *sqlate.DB
	blobfs *bfdata.Store
}

// New builds the catalog from the query library's patterns and blobfs's
// published namespace and compiles blobfs's statements against it for db's
// dialect. opts reach blobfs's store as they are: the composition root
// fixes blobfs's engine with bfdata.WithEngine, and without one the store
// runs blobfs's baseline. No I/O happens here; [Store.Verify] checks the
// statements against the database.
func New(db *sqlate.DB, opts ...bfdata.Option) (*Store, error) {
	catalog, err := query.NewCatalog(query.Patterns(), bfdata.Patterns())
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	fs, err := bfdata.New(catalog, db.Dialect(), opts...)
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	return &Store{db: db, blobfs: fs}, nil
}

// Verify prepares every statement of blobfs's, its engine's included, and
// probes both listings' field contracts against the database, so a schema
// that is not applied, or no longer matches the statements, fails before a
// command does any work. A failure wraps ErrVerify and the causes.
func (s *Store) Verify(ctx context.Context) error {
	if err := s.blobfs.Verify(ctx, s.db); err != nil {
		return fmt.Errorf("%w: %w", ErrVerify, err)
	}
	return nil
}

// directoryFields are the fields a sort term or a filter may name to apply
// to the directory half of ls as well as to the file half. The file half
// takes every term and refuses one it does not declare; a term naming a
// field only files have sorts or filters the files and leaves the
// directory half as it is. Both listings declare status, but a directory's
// status (active, deleting) is not a file's (pending, available,
// deleting), so a status term applies to the files alone. parent_id is
// left out: every row of one directory listing shares it.
var directoryFields = map[string]bool{
	"id": true, "name": true, "version": true, "created_at": true, "updated_at": true,
}

// directives lowers a Listing to the query library's directives for one
// half of ls: the filters and sort terms, all of them for the file half
// (a nil allowed set) and those naming a directory field for the directory
// half, and the total mode. A half continued from a cursor counts nothing,
// so its total is NoTotal whatever the Listing asks.
func directives(l Listing, allowed map[string]bool, continued bool) query.Directives {
	d := query.Directives{}
	for _, t := range l.Sort {
		if allowed == nil || allowed[t.Field] {
			d.Sort = append(d.Sort, query.Sort{Field: t.Field, Descending: t.Descending})
		}
	}
	for _, f := range l.Filters {
		if allowed == nil || allowed[f.Field] {
			d.Filters = append(d.Filters, query.Filter{Field: f.Field, Op: query.Op(f.Op), Value: f.Value})
		}
	}
	if l.Total == TotalNone || continued {
		d.Total = query.TotalNone
	}
	return d
}

// half reads one half of a listing from list, anchored on the directory
// with id: by page number when after is empty, and past the cursor after
// otherwise, each under the half's directives.
func half[T any](ctx context.Context, sess sqlate.Session, list bfdata.Listing[T], id string, l Listing, allowed map[string]bool, after string) (Page[T], error) {
	req := directives(l, allowed, after != "")
	var c query.Collection[T]
	var err error
	if after == "" {
		c, err = list.List(ctx, sess, id, req, query.Page{Number: l.Page, Size: l.Size})
	} else {
		c, err = list.Continue(ctx, sess, id, req, query.Cursor(after), l.Size)
	}
	if err != nil {
		return Page[T]{}, err
	}
	total := c.Total
	if total == query.NoTotal {
		total = NoTotal
	}
	return Page[T]{Rows: c.Items, Total: total, More: c.More, Next: string(c.Next)}, nil
}
