package files

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/output"
)

// Commands builds the directory commands, mkdir, ls, stat, mv, and rmdir,
// over store, the composition root's node for the [Store]. Each declares
// store with Use, so the dispatcher builds and starts the database and the
// store's statement check before the body runs, and shuts them down after;
// the body reads the Store from the Invocation's System. None declares the
// object store. Each validates its arguments and flags in Args, so a
// malformed path-or-id, filter, sort term, or total mode is a usage error
// before anything is built.
func Commands(store *graph.Node[*Store]) []*cli.Command {
	g := group{store: store}
	return []*cli.Command{
		g.mkdir().Use(store),
		g.list().Use(store),
		g.stat().Use(store),
		g.move().Use(store),
		g.removeDirectory().Use(store),
	}
}

// group is the directory commands' handle on their Store node.
type group struct {
	store *graph.Node[*Store]
}

// storeOf returns the Store the dispatcher built for inv.
func (g group) storeOf(inv *cli.Invocation) *Store {
	return inv.System.Get(g.store)
}

// mkdir is mkdir <path>: the last segment created under its existing
// parent. There is no -p; a missing parent is an error.
func (g group) mkdir() *cli.Command {
	return &cli.Command{
		Name:     "mkdir",
		Summary:  "Create a directory under an existing parent",
		Synopsis: "<path>",
		Args:     cli.ExactArgs(1),
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			dir, err := g.storeOf(inv).Mkdir(ctx, inv.Args[0])
			if err != nil {
				return err
			}
			return output.Line(inv.Stdout, fmt.Sprintf("mkdir: %s (id %s)", inv.Args[0], dir.ID))
		},
	}
}

// list is ls <path|id:<uuid>>: the directories under the directory first,
// then its files, each one page, with a line per half stating the page and
// the total, and the cursor lines only under --cursors.
func (g group) list() *cli.Command {
	var f listingFlags
	var ref Ref
	var l Listing
	cmd := &cli.Command{
		Name:     "ls",
		Summary:  "List a directory: its directories, then its files, one page each",
		Synopsis: "<path|id:<uuid>>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(1)(args); err != nil {
				return err
			}
			var err error
			if ref, err = parseRefArg(args[0]); err != nil {
				return err
			}
			l, err = f.listing()
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			s := g.storeOf(inv)
			var c Contents
			var err error
			if ref.ID != "" {
				c, err = s.ListDirectory(ctx, ref.ID, l)
			} else {
				c, err = s.List(ctx, ref.Path, l)
			}
			if err != nil {
				return err
			}
			entries := make([]output.Entry, 0, len(c.Directories.Rows)+len(c.Files.Rows))
			for _, d := range c.Directories.Rows {
				entries = append(entries, output.Entry{Kind: "dir", Name: d.Name, ID: d.ID, Updated: d.UpdatedAt})
			}
			for _, file := range c.Files.Rows {
				entries = append(entries, output.Entry{Kind: "file", Name: file.Name, ID: file.ID, Size: file.Size, Status: string(file.Status), Updated: file.UpdatedAt})
			}
			dirs, files := pageOf(l, l.After.Directories, c.Directories), pageOf(l, l.After.Files, c.Files)
			if !f.cursors {
				dirs.Next, files.Next = "", ""
			}
			return output.Listing(inv.Stdout, entries, dirs, files)
		},
	}
	f.bind(cmd)
	return cmd
}

// stat is stat <path|id:<uuid>>: the file's row, or the directory's when
// no file is at the path or has the id, one field per line. A record by id
// carries no path line.
func (g group) stat() *cli.Command {
	var ref Ref
	return &cli.Command{
		Name:     "stat",
		Summary:  "Show a file's or a directory's row, one field per line",
		Synopsis: "<path|id:<uuid>>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(1)(args); err != nil {
				return err
			}
			var err error
			ref, err = parseRefArg(args[0])
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			s := g.storeOf(inv)
			if ref.ID != "" {
				e, err := s.Find(ctx, ref.ID)
				if err != nil {
					return err
				}
				if e.Kind == EntryFile {
					return output.Record(inv.Stdout, fileRecord("", e.File))
				}
				return output.Record(inv.Stdout, directoryRecord("", e.Directory))
			}
			f, err := s.Stat(ctx, ref.Path)
			if err == nil {
				return output.Record(inv.Stdout, fileRecord(ref.Path, f))
			}
			if !errors.Is(err, blobfs.ErrNotFound) && !errors.Is(err, blobfs.ErrRootDirectory) {
				return err
			}
			dir, dirErr := s.Resolve(ctx, ref.Path)
			if errors.Is(dirErr, blobfs.ErrNotFound) {
				return err
			}
			if dirErr != nil {
				return dirErr
			}
			return output.Record(inv.Stdout, directoryRecord(ref.Path, dir))
		},
	}
}

// move is mv <src> <dst>: the directory or file at src moved into the
// existing directory dst, or to the new path dst, in one transaction. With
// two ids, the entry with the first, a file or else a directory, moves
// into the directory with the second and keeps its name.
func (g group) move() *cli.Command {
	var src, dst Ref
	return &cli.Command{
		Name:     "mv",
		Summary:  "Move or rename a directory or a file within its top-level directory",
		Synopsis: "<src> <dst>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(2)(args); err != nil {
				return err
			}
			var err error
			src, dst, err = parsePair("mv", args[0], args[1])
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			s := g.storeOf(inv)
			var res MoveResult
			var err error
			if src.ID != "" {
				e, findErr := s.Find(ctx, src.ID)
				if findErr != nil {
					return findErr
				}
				res, err = s.MoveEntry(ctx, MoveRequest{Kind: e.Kind, ID: src.ID, DirectoryID: dst.ID})
			} else {
				res, err = s.Move(ctx, src.Path, dst.Path)
			}
			if err != nil {
				return err
			}
			return output.Line(inv.Stdout, fmt.Sprintf("mv: %s -> %s (id %s)", res.From, res.To, res.ID))
		},
	}
}

// removeDirectory is rmdir <path>: an empty directory removed. A directory
// that still has contents is refused, and so is the root.
func (g group) removeDirectory() *cli.Command {
	return &cli.Command{
		Name:     "rmdir",
		Summary:  "Remove an empty directory",
		Synopsis: "<path>",
		Args:     cli.ExactArgs(1),
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			dir, err := g.storeOf(inv).RemoveDirectory(ctx, inv.Args[0])
			if err != nil {
				return err
			}
			return output.Line(inv.Stdout, fmt.Sprintf("rmdir: %s (id %s)", inv.Args[0], dir.ID))
		},
	}
}

// fileRecord lays a file row out as the fields stat prints, in order. The
// path line is left out when path is empty, as it is for a stat by id.
func fileRecord(path string, f blobfs.File) []output.Field {
	size, etag := "-", "-"
	if f.Size != nil {
		size = strconv.FormatInt(*f.Size, 10)
	}
	if f.ETag != nil {
		etag = *f.ETag
	}
	fields := []output.Field{
		{Name: "id", Value: f.ID},
		{Name: "name", Value: f.Name},
		{Name: "status", Value: string(f.Status)},
		{Name: "size", Value: size},
		{Name: "content-type", Value: f.ContentType},
		{Name: "etag", Value: etag},
		{Name: "key", Value: f.Key},
		{Name: "version", Value: strconv.FormatInt(f.Version, 10)},
		{Name: "created", Value: f.CreatedAt.UTC().Format(time.RFC3339)},
		{Name: "updated", Value: f.UpdatedAt.UTC().Format(time.RFC3339)},
	}
	if path != "" {
		fields = append([]output.Field{{Name: "path", Value: path}}, fields...)
	}
	return fields
}

// directoryRecord lays a directory row out as the fields stat prints, in
// the file record's order for the fields the two share; the parent is -
// for the root. The path line is left out when path is empty.
func directoryRecord(path string, d blobfs.Directory) []output.Field {
	parent := "-"
	if d.ParentID != nil {
		parent = *d.ParentID
	}
	fields := []output.Field{
		{Name: "id", Value: d.ID},
		{Name: "parent", Value: parent},
		{Name: "name", Value: d.Name},
		{Name: "version", Value: strconv.FormatInt(d.Version, 10)},
		{Name: "created", Value: d.CreatedAt.UTC().Format(time.RFC3339)},
		{Name: "updated", Value: d.UpdatedAt.UTC().Format(time.RFC3339)},
	}
	if path != "" {
		fields = append([]output.Field{{Name: "path", Value: path}}, fields...)
	}
	return fields
}

// Ref is one argument that names an entry: an absolute path, or a row's id
// written as id:<uuid>. Exactly one of Path and ID is set. A path starts
// with /, so the two forms never collide.
type Ref struct {
	Path string
	ID   string
}

// ParseRef reads one path-or-id argument. Text after an id: prefix must be
// a UUID other than the root's, checked by blobfs.ParseID before any I/O,
// and is returned in canonical form; anything else is taken as a path,
// which the Store validates.
func ParseRef(arg string) (Ref, error) {
	rest, ok := strings.CutPrefix(arg, "id:")
	if !ok {
		return Ref{Path: arg}, nil
	}
	id, err := blobfs.ParseID(rest)
	if err != nil {
		return Ref{}, err
	}
	return Ref{ID: id}, nil
}

// parseRefArg is ParseRef for a command's argument: a refused id is a
// usage error.
func parseRefArg(arg string) (Ref, error) {
	ref, err := ParseRef(arg)
	if err != nil {
		return Ref{}, cli.Usagef("%w", err)
	}
	return ref, nil
}

// parsePair reads the two arguments of mv, which takes two paths or two
// ids and not one of each, since the id form takes a source id and a
// destination directory id together. Every refusal is a usage error.
func parsePair(command, first, second string) (Ref, Ref, error) {
	src, err := parseRefArg(first)
	if err != nil {
		return Ref{}, Ref{}, err
	}
	dst, err := parseRefArg(second)
	if err != nil {
		return Ref{}, Ref{}, err
	}
	if (src.ID != "") != (dst.ID != "") {
		return Ref{}, Ref{}, cli.Usagef("%s %s %s: give two paths, or two ids as id:<uuid> for the source and the destination directory", command, first, second)
	}
	return src, dst, nil
}

// pageOf describes one half's page for the output: the request's page and
// size, whether the half was read after a cursor (after not empty), the
// rows listed, the total as the half reported it, marked counted when the
// listing asked for one and the half was read by number, whether rows
// remain, and the cursor of the next page.
func pageOf[T any](l Listing, after string, p Page[T]) output.Page {
	out := output.Page{
		Number: l.Page, Size: l.Size, Listed: len(p.Rows), Total: p.Total,
		Counted: l.Total == TotalExact && after == "", Cursor: after != "", More: p.More, Next: p.Next,
	}
	if p.Total == NoTotal {
		out.Total = output.NoTotal
	}
	return out
}

// listingFlags is the flag set ls takes: the page and its size, the
// repeatable sort and filter terms, the total mode, the cursor of each
// half, and whether to print the next ones.
type listingFlags struct {
	page, size            int
	sort, filter          []string
	total                 string
	afterDirs, afterFiles string
	cursors               bool
}

// bind defines the listing flags on cmd.
func (f *listingFlags) bind(cmd *cli.Command) {
	fs := cmd.Flags()
	fs.IntVar(&f.page, "page", 1, "the 1-based page to list")
	fs.IntVar(&f.size, "size", 20, "the number of rows per page")
	cli.StringsVar(fs, &f.sort, "sort", nil, "a sort term, <field> or <field>:desc, applied in order")
	cli.StringsVar(fs, &f.filter, "filter", nil, "a filter term, <field>:<op>:<value>, or <field>:null and <field>:notnull")
	fs.StringVar(&f.total, "total", "exact", "exact to count the total, none to omit it")
	fs.StringVar(&f.afterDirs, "after-dirs", "", "continue the directory half after this cursor, from an earlier next-dirs: line")
	fs.StringVar(&f.afterFiles, "after-files", "", "continue the file half after this cursor, from an earlier next-files: line")
	fs.BoolVar(&f.cursors, "cursors", false, "print the next-dirs: and next-files: lines with the cursors that continue each half")
}

// listing builds the Listing the flags state, validating the total mode
// and parsing each filter and sort term. Every refusal is a usage error.
func (f *listingFlags) listing() (Listing, error) {
	l := Listing{Page: f.page, Size: f.size, After: After{Directories: f.afterDirs, Files: f.afterFiles}}
	switch f.total {
	case "exact":
		l.Total = TotalExact
	case "none":
		l.Total = TotalNone
	default:
		return Listing{}, cli.Usagef("--total %q: the mode is exact or none", f.total)
	}
	for _, term := range f.sort {
		s, err := ParseSort(term)
		if err != nil {
			return Listing{}, cli.Usagef("%w", err)
		}
		l.Sort = append(l.Sort, s)
	}
	for _, term := range f.filter {
		filter, err := ParseFilter(term)
		if err != nil {
			return Listing{}, cli.Usagef("%w", err)
		}
		l.Filters = append(l.Filters, filter)
	}
	return l, nil
}

// ParseFilter reads one --filter term: <field>:<op>:<value>, where the
// value is the rest of the term and may hold colons, as a timestamp does;
// <field>:<op> alone for null and notnull; and for in, a value that is a
// comma-separated list. The field and the operator are checked by blobfs
// against the listing's declared fields and the query library's
// operators, so an unknown one is refused there, before the statement
// runs.
func ParseFilter(term string) (Filter, error) {
	field, rest, ok := strings.Cut(term, ":")
	if field == "" || !ok {
		return Filter{}, fmt.Errorf("--filter %q: write <field>:<op>:<value>, or <field>:null or <field>:notnull", term)
	}
	op, value, hasValue := strings.Cut(rest, ":")
	if op == "" {
		return Filter{}, fmt.Errorf("--filter %q: names no operator; write <field>:<op>:<value>", term)
	}
	switch op {
	case "null", "notnull":
		if hasValue {
			return Filter{}, fmt.Errorf("--filter %q: %s takes no value; write %s:%s", term, op, field, op)
		}
		return Filter{Field: field, Op: op}, nil
	case "in":
		if !hasValue {
			return Filter{}, fmt.Errorf("--filter %q: in takes a comma-separated list; write %s:in:<value>,<value>", term, field)
		}
		parts := strings.Split(value, ",")
		values := make([]any, len(parts))
		for i, p := range parts {
			values[i] = p
		}
		return Filter{Field: field, Op: op, Value: values}, nil
	}
	if !hasValue {
		return Filter{}, fmt.Errorf("--filter %q: names no value; write <field>:<op>:<value>, or <field>:null or <field>:notnull", term)
	}
	return Filter{Field: field, Op: op, Value: value}, nil
}

// ParseSort reads one --sort term: a field name, or a field name and :desc
// for descending order; :asc is accepted and means the default.
func ParseSort(term string) (Sort, error) {
	field, direction, hasDirection := strings.Cut(term, ":")
	if field == "" {
		return Sort{}, fmt.Errorf("--sort %q: names no field; write <field> or <field>:desc", term)
	}
	switch {
	case !hasDirection, direction == "asc":
		return Sort{Field: field}, nil
	case direction == "desc":
		return Sort{Field: field, Descending: true}, nil
	}
	return Sort{}, fmt.Errorf("--sort %q: the direction is asc or desc", term)
}
