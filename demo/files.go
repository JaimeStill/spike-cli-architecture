package demo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// FilesArea is the files tour's working area, a top-level directory the
// tour creates, works under, and removes with rm --recursive, and clears
// first when an earlier run left it behind.
const FilesArea = "/demo-files"

// The files tour's content, held in memory: hello is put with no size, as
// put - streams standard input, and notes with its size, as put of a local
// file states it.
const (
	hello = "hello from the files tour\n"
	notes = "put, cat, cp, and rm run on Postgres and the object store.\n"
)

// objects is the files tour's handle on the nodes it declares: the files
// node for its directories and the objects node for its files.
type objects struct {
	store   *graph.Node[*files.Store]
	objects *graph.Node[*files.Objects]
}

// storeOf and objectsOf return the values the dispatcher built for the
// run.
func (o objects) storeOf(sys *graph.System) *files.Store     { return sys.Get(o.store) }
func (o objects) objectsOf(sys *graph.System) *files.Objects { return sys.Get(o.objects) }

// Files is the files tour over store, the files node, and objs, the
// objects node: put, from memory as put - reads standard input and with a
// stated size, cat, cp, rm, and rm --recursive, in a working area of its
// own. It declares both nodes, since its steps read both, so a run builds
// Postgres and the object store, and a store that cannot be reached fails
// it at start, naming the store's node.
func Files(store *graph.Node[*files.Store], objs *graph.Node[*files.Objects]) scenario.Scenario {
	o := objects{store: store, objects: objs}
	return scenario.Scenario{
		Name:    "files",
		Summary: "Tour the object commands on Postgres and the store: put, cat, cp, rm, rm --recursive",
		Nodes:   []scenario.Node{store, objs},
		Steps: []scenario.Step{
			{Intent: "Clear " + FilesArea + " if an earlier run left it behind", Action: o.clear},
			{Intent: "Create " + FilesArea + " and " + FilesArea + "/docs: mkdir", Action: o.mkdir},
			{Intent: "Put hello.txt from memory with no size, as put - streams stdin", Action: o.putStream},
			{Intent: "Put notes.txt with its size stated, as put of a local file", Action: o.putSized},
			{Intent: "Read hello.txt back: cat", Action: o.cat},
			{Intent: "Copy hello.txt into " + FilesArea + " and read the copy: cp, cat", Action: o.copy},
			{Intent: "List the working area and docs: ls", Action: o.list},
			{Intent: "Remove the copy: rm", Action: o.remove},
			{Intent: "Remove the working area and everything under it: rm --recursive", Action: o.removeTree},
		},
	}
}

func (o objects) clear(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	_, err := o.storeOf(sys).Resolve(ctx, FilesArea)
	if errors.Is(err, blobfs.ErrNotFound) {
		r.Note("Nothing to clear: %s does not exist, so this run starts clean.", FilesArea)
		return nil
	}
	if err != nil {
		return err
	}
	r.Note("An earlier run stopped before its last step and left %s behind; rm --recursive removes it, objects and rows, so this run starts clean.", FilesArea)
	return o.removeArea(ctx, sys, r)
}

func (o objects) mkdir(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	s := o.storeOf(sys)
	for _, path := range []string{FilesArea, FilesArea + "/docs"} {
		dir, err := s.Mkdir(ctx, path, "")
		if err != nil {
			return err
		}
		if err := showLine(r, "mkdir: %s (id %s)", path, dir.ID); err != nil {
			return err
		}
	}
	return nil
}

// put writes content to path and shows the result as put prints it.
func (o objects) put(ctx context.Context, sys *graph.System, r *scenario.Reporter, path string, c files.Content) error {
	res, err := o.objectsOf(sys).Put(ctx, path, c)
	if err != nil {
		return err
	}
	f := res.File
	etag := "-"
	if f.ETag != nil {
		etag = *f.ETag
	}
	return showLine(r, "put: %s (id %s, %d bytes, etag %s)", path, f.ID, sizeOf(f), etag)
}

func (o objects) putStream(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	r.Note("The row is committed pending before any byte is put; the store takes the body to its end, and the row is completed with the size and etag the store reports.")
	return o.put(ctx, sys, r, FilesArea+"/docs/hello.txt", files.Content{
		Body: strings.NewReader(hello), ContentType: "text/plain",
	})
}

func (o objects) putSized(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	return o.put(ctx, sys, r, FilesArea+"/docs/notes.txt", files.Content{
		Body: strings.NewReader(notes), Size: int64(len(notes)), ContentType: "text/plain",
	})
}

// catFile shows the content of the available file at path, and checks it is
// want, the bytes the tour put.
func (o objects) catFile(ctx context.Context, sys *graph.System, r *scenario.Reporter, path, want string) error {
	body, _, err := o.objectsOf(sys).Open(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	b, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("files: cat %s: %w", path, err)
	}
	if string(b) != want {
		return fmt.Errorf("cat %s read %q, want the %q the tour put", path, b, want)
	}
	return r.Show(func(w io.Writer) error {
		_, err := w.Write(b)
		return err
	})
}

func (o objects) cat(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	return o.catFile(ctx, sys, r, FilesArea+"/docs/hello.txt", hello)
}

func (o objects) copy(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	r.Note("A destination that names an existing directory takes the copy under the source's name; the copy is a new row over a new object.")
	res, err := o.objectsOf(sys).Copy(ctx, FilesArea+"/docs/hello.txt", FilesArea)
	if err != nil {
		return err
	}
	if err := showLine(r, "cp: %s -> %s (id %s, %d bytes)", res.From, res.To, res.File.ID, sizeOf(res.File)); err != nil {
		return err
	}
	return o.catFile(ctx, sys, r, res.To, hello)
}

func (o objects) list(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	s := o.storeOf(sys)
	l := files.Listing{Page: 1, Size: 20}
	for _, path := range []string{FilesArea, FilesArea + "/docs"} {
		c, err := s.List(ctx, path, l)
		if err != nil {
			return err
		}
		r.Note("ls %s", path)
		if err := showListing(r, l, c); err != nil {
			return err
		}
	}
	return nil
}

func (o objects) remove(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	path := FilesArea + "/hello.txt"
	f, err := o.objectsOf(sys).Remove(ctx, path)
	if err != nil {
		return err
	}
	return showLine(r, "rm: %s (id %s)", path, f.ID)
}

func (o objects) removeTree(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	r.Note("The branch is marked deleting in one transaction, then swept until no work remains: each file's object and row, then each directory once it is empty. A rerun starts clean.")
	return o.removeArea(ctx, sys, r)
}

// removeArea removes the working area with rm --recursive and shows its
// totals.
func (o objects) removeArea(ctx context.Context, sys *graph.System, r *scenario.Reporter) error {
	res, err := o.objectsOf(sys).RemoveTree(ctx, FilesArea)
	if err != nil {
		return err
	}
	return showLine(r, "rm --recursive: %s (%d files, %d directories)", FilesArea, res.Files, res.Directories)
}
