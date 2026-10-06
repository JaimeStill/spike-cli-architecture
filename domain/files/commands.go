package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/output"
)

// Commands builds the directory commands, mkdir, ls, stat, mv, and rmdir,
// and the bookmark command with its add, ls, and rm subcommands, over
// store, the composition root's node for the [Store]. Each declares store
// with Use, the bookmark subcommands through their parent, so the
// dispatcher builds and starts the database and the store's statement
// check before the body runs, and shuts them down after; the body reads the
// Store from the Invocation's System. None declares the object store. Each
// validates its arguments and flags in Args, so a malformed path-or-id,
// unit, filter, sort term, or total mode is a usage error before anything
// is built, and each bookmark subcommand requires --unit, so a run without
// it is a usage error too.
func Commands(store *graph.Node[*Store]) []*cli.Command {
	g := group{store: store}
	return []*cli.Command{
		g.mkdir().Use(store),
		g.list().Use(store),
		g.stat().Use(store),
		g.move().Use(store),
		g.removeDirectory().Use(store),
		g.bookmark().Use(store),
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

// mkdir is mkdir <path> [--unit <uuid>]: the last segment created under
// its existing parent, and with --unit, at a top-level path, the owner row
// that binds it to the unit, in the same transaction. There is no -p; a
// missing parent is an error.
func (g group) mkdir() *cli.Command {
	var unit string
	cmd := &cli.Command{
		Name:     "mkdir",
		Summary:  "Create a directory under an existing parent",
		Synopsis: "<path>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(1)(args); err != nil {
				return err
			}
			var err error
			unit, err = parseUnit(unit)
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			dir, err := g.storeOf(inv).Mkdir(ctx, inv.Args[0], unit)
			if err != nil {
				return err
			}
			if unit != "" {
				return output.Line(inv.Stdout, fmt.Sprintf("mkdir: %s (id %s, unit %s)", inv.Args[0], dir.ID, unit))
			}
			return output.Line(inv.Stdout, fmt.Sprintf("mkdir: %s (id %s)", inv.Args[0], dir.ID))
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit that owns the directory, a UUID; top-level paths only")
	return cmd
}

// list is ls <path|id:<uuid>>: the directories under the directory first,
// then its files, each one page, with a line per half stating the page and
// the total, and the cursor lines only under --cursors. With --unit the
// unit must own the path's top-level directory, and ls / lists the unit's
// own top-level directories; a listing by id takes no unit.
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
			if l, err = f.listing(); err != nil {
				return err
			}
			if ref.ID != "" && l.Unit != "" {
				return cli.Usagef("ls %s --unit: a listing by id has no path to derive the unit's scope from; list the path instead", args[0])
			}
			return nil
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
			return WriteContents(inv.Stdout, l, c, f.cursors)
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
					return WriteFileRecord(inv.Stdout, "", e.File)
				}
				return WriteDirectoryRecord(inv.Stdout, "", e.Directory)
			}
			f, err := s.Stat(ctx, ref.Path)
			if err == nil {
				return WriteFileRecord(inv.Stdout, ref.Path, f)
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
			return WriteDirectoryRecord(inv.Stdout, ref.Path, dir)
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

// bookmark is the bookmark command: add, ls, and rm, each under the unit
// its required --unit names.
func (g group) bookmark() *cli.Command {
	return (&cli.Command{
		Name:    "bookmark",
		Summary: "Bookmark files for a unit, at most one of them active: add, ls, rm",
	}).Add(g.bookmarkAdd(), g.bookmarkList(), g.bookmarkRemove())
}

// bookmarkAdd is bookmark add <path> --unit <uuid> [--active]: the unit's
// bookmark of the file at the path, active when asked, and refused while
// another bookmark of the unit is active.
func (g group) bookmarkAdd() *cli.Command {
	var unit string
	var active bool
	cmd := &cli.Command{
		Name:     "add",
		Summary:  "Bookmark the file at a path for a unit; --active makes it the unit's one active bookmark",
		Synopsis: "<path>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(1)(args); err != nil {
				return err
			}
			var err error
			unit, err = parseUnit(unit)
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			f, err := g.storeOf(inv).AddBookmark(ctx, inv.Args[0], unit, active)
			if err != nil {
				return err
			}
			state := "inactive"
			if active {
				state = "active"
			}
			return output.Line(inv.Stdout, fmt.Sprintf("bookmark add: %s (file %s, unit %s, %s)", inv.Args[0], f.ID, unit, state))
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit that bookmarks the file, a UUID")
	cmd.Flags().BoolVar(&active, "active", false, "make this bookmark the unit's one active bookmark; refused while another is active")
	cmd.Require("unit")
	return cmd
}

// bookmarkList is bookmark ls --unit <uuid>: the unit's bookmarks with
// their files' full paths, one page by number, and a line stating the page
// and the total.
func (g group) bookmarkList() *cli.Command {
	var f pageFlags
	var unit string
	var l Listing
	cmd := &cli.Command{
		Name:    "ls",
		Summary: "List a unit's bookmarks with their files' paths, one page",
		Args: func(args []string) error {
			if err := cli.NoArgs(args); err != nil {
				return err
			}
			var err error
			if unit, err = parseUnit(unit); err != nil {
				return err
			}
			l, err = f.listing()
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			p, err := g.storeOf(inv).ListBookmarks(ctx, unit, l)
			if err != nil {
				return err
			}
			entries := make([]output.BookmarkEntry, 0, len(p.Rows))
			for _, b := range p.Rows {
				entries = append(entries, output.BookmarkEntry{Path: b.Path, Size: b.Size, Status: string(b.Status), Active: b.Active, Updated: b.UpdatedAt})
			}
			return output.Bookmarks(inv.Stdout, entries, pageOf(l, "", p))
		},
	}
	f.bind(cmd)
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit whose bookmarks to list, a UUID")
	cmd.Require("unit")
	return cmd
}

// bookmarkRemove is bookmark rm <path> --unit <uuid>: the unit's bookmark
// of the file at the path removed, active or not.
func (g group) bookmarkRemove() *cli.Command {
	var unit string
	cmd := &cli.Command{
		Name:     "rm",
		Summary:  "Remove a unit's bookmark of the file at a path, active or not",
		Synopsis: "<path>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(1)(args); err != nil {
				return err
			}
			var err error
			unit, err = parseUnit(unit)
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			f, err := g.storeOf(inv).RemoveBookmark(ctx, inv.Args[0], unit)
			if err != nil {
				return err
			}
			return output.Line(inv.Stdout, fmt.Sprintf("bookmark rm: %s (file %s, unit %s)", inv.Args[0], f.ID, unit))
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit whose bookmark to remove, a UUID")
	cmd.Require("unit")
	return cmd
}

// ObjectCommands builds the object commands, put, cat, cp, and rm, over
// objects, the composition root's node for [Objects]. Each declares
// objects with Use, so the dispatcher builds and starts the database, the
// Store's statement check, and the object store before the body runs; a
// store that cannot be reached fails the command at start, naming the
// store's node, before anything is read or written. Each validates its
// arguments in Args, so a malformed path-or-id, or an id where a path is
// needed, is a usage error before anything is built.
func ObjectCommands(objects *graph.Node[*Objects]) []*cli.Command {
	g := objectGroup{objects: objects}
	return []*cli.Command{
		g.put().Use(objects),
		g.cat().Use(objects),
		g.copy().Use(objects),
		g.remove().Use(objects),
	}
}

// objectGroup is the object commands' handle on their Objects node.
type objectGroup struct {
	objects *graph.Node[*Objects]
}

// objectsOf returns the Objects the dispatcher built for inv.
func (g objectGroup) objectsOf(inv *cli.Invocation) *Objects {
	return inv.System.Get(g.objects)
}

// put is put <local-file|-> <path|id:<uuid>> [--content-type <type>]: the
// local file, or standard input for -, written as the file at the path,
// or into the directory with the id under the local file's base name.
// Standard input has no name, so - takes a path.
func (g objectGroup) put() *cli.Command {
	var contentType string
	var dst Ref
	cmd := &cli.Command{
		Name:     "put",
		Summary:  "Upload a local file, or stdin for -, as the file at a path or into a directory",
		Synopsis: "<local-file|-> <path|id:<uuid>>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(2)(args); err != nil {
				return err
			}
			var err error
			if dst, err = parseRefArg(args[1]); err != nil {
				return err
			}
			if dst.ID != "" && args[0] == "-" {
				return cli.Usagef("put - %s: stdin has no name to store under; give the destination as a path", args[1])
			}
			return nil
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			src := inv.Args[0]
			body, size, closeBody, err := openLocal(inv.Stdin, src)
			if err != nil {
				return fmt.Errorf("files: put %s: %w", src, err)
			}
			defer closeBody()
			c := Content{Body: body, Size: size, ContentType: declaredType(contentType, src)}
			o := g.objectsOf(inv)
			label := inv.Args[1]
			var res PutResult
			if dst.ID != "" {
				name := filepath.Base(src)
				label = name + " in " + inv.Args[1]
				res, err = o.PutFile(ctx, dst.ID, name, c)
			} else {
				res, err = o.Put(ctx, dst.Path, c)
			}
			if err != nil {
				return err
			}
			resumed := ""
			if res.Resumed {
				resumed = ", resumed the pending row"
			}
			f := res.File
			return output.Line(inv.Stdout, fmt.Sprintf("put: %s (id %s, %d bytes, etag %s%s)", label, f.ID, sizeOf(f), etagOf(f), resumed))
		},
	}
	cmd.Flags().StringVar(&contentType, "content-type", "", "the media type to store with the object; the default comes from the local file's extension, else application/octet-stream")
	return cmd
}

// cat is cat <path|id:<uuid>>: the available file's content streamed to
// stdout as it is.
func (g objectGroup) cat() *cli.Command {
	var ref Ref
	return &cli.Command{
		Name:     "cat",
		Summary:  "Write an available file's content to stdout",
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
			o := g.objectsOf(inv)
			var body io.ReadCloser
			var err error
			if ref.ID != "" {
				body, _, err = o.OpenFile(ctx, ref.ID)
			} else {
				body, _, err = o.Open(ctx, ref.Path)
			}
			if err != nil {
				return err
			}
			defer func() { _ = body.Close() }()
			if _, err := io.Copy(inv.Stdout, body); err != nil {
				return fmt.Errorf("files: cat %s: %w", inv.Args[0], err)
			}
			return nil
		},
	}
}

// copy is cp <src> <dst>: the available file at src copied into the
// existing directory dst under its own name, or to the new path dst. With
// two ids, the file with the first is copied into the directory with the
// second under its own name. A name already taken is refused.
func (g objectGroup) copy() *cli.Command {
	var src, dst Ref
	return &cli.Command{
		Name:     "cp",
		Summary:  "Copy an available file into a directory or to a new path",
		Synopsis: "<src> <dst>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(2)(args); err != nil {
				return err
			}
			var err error
			src, dst, err = parsePair("cp", args[0], args[1])
			return err
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			o := g.objectsOf(inv)
			var res CopyResult
			var err error
			if src.ID != "" {
				res, err = o.CopyFile(ctx, src.ID, dst.ID)
			} else {
				res, err = o.Copy(ctx, src.Path, dst.Path)
			}
			if err != nil {
				return err
			}
			f := res.File
			return output.Line(inv.Stdout, fmt.Sprintf("cp: %s -> %s (id %s, %d bytes, etag %s)", res.From, res.To, f.ID, sizeOf(f), etagOf(f)))
		},
	}
}

// remove is rm <path|id:<uuid>>, a file deleted, and rm --recursive
// <path>, a directory and everything beneath it deleted, reported as the
// totals its sweep removed. There is no -r shorthand.
func (g objectGroup) remove() *cli.Command {
	var recursive bool
	var ref Ref
	cmd := &cli.Command{
		Name:     "rm",
		Summary:  "Delete a file, or with --recursive a directory and everything beneath it",
		Synopsis: "<path|id:<uuid>>",
		Args: func(args []string) error {
			if err := cli.ExactArgs(1)(args); err != nil {
				return err
			}
			var err error
			if ref, err = parseRefArg(args[0]); err != nil {
				return err
			}
			if recursive && ref.ID != "" {
				return cli.Usagef("rm --recursive %s: a branch is removed by path, not by id", args[0])
			}
			return nil
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			o := g.objectsOf(inv)
			if recursive {
				res, err := o.RemoveTree(ctx, ref.Path)
				if err != nil {
					return err
				}
				return output.Line(inv.Stdout, fmt.Sprintf("rm --recursive: %s (%d files, %d directories)", ref.Path, res.Files, res.Directories))
			}
			var f blobfs.File
			var err error
			if ref.ID != "" {
				f, err = o.RemoveFile(ctx, ref.ID)
			} else {
				f, err = o.Remove(ctx, ref.Path)
			}
			if err != nil {
				return err
			}
			return output.Line(inv.Stdout, fmt.Sprintf("rm: %s (id %s)", inv.Args[0], f.ID))
		},
	}
	cmd.Flags().BoolVar(&recursive, "recursive", false, "delete the directory at the path and everything beneath it")
	return cmd
}

// openLocal opens the body put uploads: stdin for -, its length unknown,
// or the local file named, its length from the file system so the store
// holds the body to it. The close function releases what was opened.
func openLocal(stdin io.Reader, name string) (body io.Reader, size int64, closeBody func(), err error) {
	if name == "-" {
		return stdin, 0, func() {}, nil
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, 0, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, nil, err
	}
	return file, info.Size(), func() { _ = file.Close() }, nil
}

// declaredType is the content type put declares: the flag when given,
// else the type the standard library registers for the local file's
// extension, else application/octet-stream, which is also stdin's.
func declaredType(flag, local string) string {
	if flag != "" {
		return flag
	}
	if local != "-" {
		if t := mime.TypeByExtension(filepath.Ext(local)); t != "" {
			return t
		}
	}
	return "application/octet-stream"
}

// sizeOf returns a file's size, or 0 when the row records none.
func sizeOf(f blobfs.File) int64 {
	if f.Size == nil {
		return 0
	}
	return *f.Size
}

// etagOf returns a file's etag, or - when the row records none.
func etagOf(f blobfs.File) string {
	if f.ETag == nil {
		return "-"
	}
	return *f.ETag
}

// WriteContents writes c, the contents l listed, as ls prints it: an entry
// line per directory and then per file, and each half's page; with
// cursors, the cursor that continues each half too.
func WriteContents(w io.Writer, l Listing, c Contents, cursors bool) error {
	entries := make([]output.Entry, 0, len(c.Directories.Rows)+len(c.Files.Rows))
	for _, d := range c.Directories.Rows {
		entries = append(entries, output.Entry{Kind: "dir", Name: d.Name, ID: d.ID, Updated: d.UpdatedAt})
	}
	for _, f := range c.Files.Rows {
		entries = append(entries, output.Entry{Kind: "file", Name: f.Name, ID: f.ID, Size: f.Size, Status: string(f.Status), Updated: f.UpdatedAt})
	}
	dirs, files := pageOf(l, l.After.Directories, c.Directories), pageOf(l, l.After.Files, c.Files)
	if !cursors {
		dirs.Next, files.Next = "", ""
	}
	return output.Listing(w, entries, dirs, files)
}

// WriteFileRecord writes a file's row as stat prints it, one field per
// line. The path line is left out when path is empty, as it is for a stat
// by id.
func WriteFileRecord(w io.Writer, path string, f blobfs.File) error {
	return output.Record(w, fileRecord(path, f))
}

// WriteDirectoryRecord writes a directory's row as stat prints it, one
// field per line. The path line is left out when path is empty.
func WriteDirectoryRecord(w io.Writer, path string, d blobfs.Directory) error {
	return output.Record(w, directoryRecord(path, d))
}

// fileRecord lays a file row out as the fields stat prints, in order. The
// path line is left out when path is empty.
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

// parsePair reads the two arguments of mv or cp, the command named, which
// takes two paths or two ids and not one of each, since the id form takes
// a source id and a destination directory id together. Every refusal is a
// usage error.
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

// pageFlags is the flag set every paged listing takes: the page and its
// size, the repeatable sort term, and the total mode. bookmark ls takes it
// alone.
type pageFlags struct {
	page, size int
	sort       []string
	total      string
}

// bind defines the paging flags on cmd.
func (f *pageFlags) bind(cmd *cli.Command) {
	fs := cmd.Flags()
	fs.IntVar(&f.page, "page", 1, "the 1-based page to list")
	fs.IntVar(&f.size, "size", 20, "the number of rows per page")
	cli.StringsVar(fs, &f.sort, "sort", nil, "a sort term, <field> or <field>:desc, applied in order")
	fs.StringVar(&f.total, "total", "exact", "exact to count the total, none to omit it")
}

// listing builds the Listing the paging flags state, validating the total
// mode and parsing each sort term. Every refusal is a usage error.
func (f *pageFlags) listing() (Listing, error) {
	l := Listing{Page: f.page, Size: f.size}
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
	return l, nil
}

// listingFlags is the flag set ls takes: the paging flags, the repeatable
// filter term, the cursor of each half and whether to print the next ones,
// and the unit to list as.
type listingFlags struct {
	pageFlags
	filter                []string
	afterDirs, afterFiles string
	cursors               bool
	unit                  string
}

// bind defines the listing flags on cmd.
func (f *listingFlags) bind(cmd *cli.Command) {
	f.pageFlags.bind(cmd)
	fs := cmd.Flags()
	cli.StringsVar(fs, &f.filter, "filter", nil, "a filter term, <field>:<op>:<value>, or <field>:null and <field>:notnull")
	fs.StringVar(&f.afterDirs, "after-dirs", "", "continue the directory half after this cursor, from an earlier next-dirs: line")
	fs.StringVar(&f.afterFiles, "after-files", "", "continue the file half after this cursor, from an earlier next-files: line")
	fs.BoolVar(&f.cursors, "cursors", false, "print the next-dirs: and next-files: lines with the cursors that continue each half")
	fs.StringVar(&f.unit, "unit", "", "list as the unit with this id, a UUID; it must own the path's top-level directory, and at / the listing is its own")
}

// listing builds the Listing the flags state, validating the unit and the
// total mode and parsing each filter and sort term. Every refusal is a
// usage error.
func (f *listingFlags) listing() (Listing, error) {
	unit, err := parseUnit(f.unit)
	if err != nil {
		return Listing{}, err
	}
	l, err := f.pageFlags.listing()
	if err != nil {
		return Listing{}, err
	}
	for _, term := range f.filter {
		filter, err := ParseFilter(term)
		if err != nil {
			return Listing{}, cli.Usagef("%w", err)
		}
		l.Filters = append(l.Filters, filter)
	}
	l.After = After{Directories: f.afterDirs, Files: f.afterFiles}
	l.Unit = unit
	return l, nil
}

// parseUnit reads a --unit value: empty when the flag was not given, and
// otherwise a UUID, returned in canonical form. A value that is not one is
// a usage error. A required --unit that is missing is reported by the
// dispatcher's Require, after Args, so the empty value passes here.
func parseUnit(unit string) (string, error) {
	if unit == "" {
		return "", nil
	}
	id, err := uuid.Parse(unit)
	if err != nil {
		return "", cli.Usagef("--unit %q is not a UUID", unit)
	}
	return id.String(), nil
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
