package demo

import (
	"context"
	"errors"
	"fmt"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// DirectoriesArea is the directories tour's working area: a top-level
// directory the demo unit owns, which the tour creates, works under, and
// removes, and clears first when an earlier run left it behind.
const DirectoriesArea = "/demo-directories"

// Unit is the unit the directories tour creates its working area as, a
// fixed UUID, so a rerun lists the same unit's directories.
const Unit = "0199c0de-0000-7000-8000-0000000000de"

// children are the directories the tour creates under its working area,
// in creation order.
var children = []string{"alpha", "bravo", "charlie", "delta", "echo"}

// directories is the directories tour's handle on the files node, the
// one node it declares.
type directories struct {
	store *graph.Node[*files.Store]
}

// storeOf returns the Store the dispatcher built for the run.
func (d directories) storeOf(inv *cli.Invocation) *files.Store { return inv.Get(d.store) }

// Directories is the directories tour over store, the files node: mkdir,
// ls with its paging, sorting, and filtering, stat, mv, and rmdir, with
// the working area created and listed as a unit's. It declares the files
// node alone, so a run builds Postgres and never the object store, and it
// runs with the store unreachable.
func Directories(store *graph.Node[*files.Store]) scenario.Scenario {
	d := directories{store: store}
	return scenario.Scenario{
		Name:    "directories",
		Summary: "Tour the directory commands on Postgres alone: mkdir, ls, stat, mv, rmdir",
		Nodes:   []scenario.Node{store},
		Steps: []scenario.Step{
			{Intent: "Clear " + DirectoriesArea + " if an earlier run left it behind", Action: d.clear},
			{Intent: "Create " + DirectoriesArea + " as the demo unit's: mkdir --unit", Action: d.mkdirArea},
			{Intent: "List the unit's top-level directories: ls / --unit", Action: d.listUnit},
			{Intent: "Create five directories under the working area: mkdir", Action: d.mkdirChildren},
			{Intent: "List the first page of two, by name descending: ls --size 2 --sort name:desc", Action: d.listFirstPage},
			{Intent: "Continue after the page's cursor: ls --after-dirs", Action: d.listNextPage},
			{Intent: "Filter by name: ls --filter name:in:alpha,charlie,echo", Action: d.listFiltered},
			{Intent: "Show one directory's row: stat", Action: d.stat},
			{Intent: "Move echo into alpha, then rename bravo to foxtrot: mv", Action: d.move},
			{Intent: "List the working area and alpha after the moves: ls", Action: d.listMoved},
			{Intent: "Remove the working area, deepest directory first: rmdir", Action: d.removeArea},
		},
	}
}

func (d directories) clear(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	s := d.storeOf(inv)
	_, err := s.Resolve(ctx, DirectoriesArea)
	if errors.Is(err, blobfs.ErrNotFound) {
		r.Note("Nothing to clear: %s does not exist, so this run starts clean.", DirectoriesArea)
		return nil
	}
	if err != nil {
		return err
	}
	r.Note("An earlier run stopped before its last step and left %s behind; it is removed, deepest directory first, so this run starts clean.", DirectoriesArea)
	return removeDirectories(ctx, s, r, DirectoriesArea)
}

func (d directories) mkdirArea(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	r.Note("A directory created with a unit is top-level, and its owner row is written in the same transaction, so the unit's scope starts here.")
	dir, err := d.storeOf(inv).Mkdir(ctx, DirectoriesArea, Unit)
	if err != nil {
		return err
	}
	return showLine(r, "mkdir: %s (id %s, unit %s)", DirectoriesArea, dir.ID, Unit)
}

func (d directories) listUnit(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	r.Note("At the root, a unit's listing is its own top-level directories, read through the owner rows.")
	l := files.Listing{Page: 1, Size: 20, Unit: Unit}
	c, err := d.storeOf(inv).List(ctx, "/", l)
	if err != nil {
		return err
	}
	return showListing(r, l, c)
}

func (d directories) mkdirChildren(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	s := d.storeOf(inv)
	for _, name := range children {
		path := DirectoriesArea + "/" + name
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

// firstPage is the listing of the paging steps: two rows a page, by name
// descending.
func firstPage() (files.Listing, error) {
	sort, err := files.ParseSort("name:desc")
	if err != nil {
		return files.Listing{}, err
	}
	return files.Listing{Page: 1, Size: 2, Sort: []files.Sort{sort}}, nil
}

func (d directories) listFirstPage(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	r.Note("A page by number counts the total; the next-dirs line is the cursor that continues the directory half.")
	l, err := firstPage()
	if err != nil {
		return err
	}
	c, err := d.storeOf(inv).List(ctx, DirectoriesArea, l)
	if err != nil {
		return err
	}
	return showListing(r, l, c)
}

func (d directories) listNextPage(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	r.Note("The cursor continues after the last row the first page showed, under the same sort; a continued half counts nothing.")
	l, err := firstPage()
	if err != nil {
		return err
	}
	s := d.storeOf(inv)
	first, err := s.List(ctx, DirectoriesArea, l)
	if err != nil {
		return err
	}
	if first.Directories.Next == "" {
		return fmt.Errorf("the first page of %s has no cursor to continue after", DirectoriesArea)
	}
	l.After = files.After{Directories: first.Directories.Next}
	c, err := s.List(ctx, DirectoriesArea, l)
	if err != nil {
		return err
	}
	return showListing(r, l, c)
}

func (d directories) listFiltered(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	filter, err := files.ParseFilter("name:in:alpha,charlie,echo")
	if err != nil {
		return err
	}
	l := files.Listing{Page: 1, Size: 20, Filters: []files.Filter{filter}}
	c, err := d.storeOf(inv).List(ctx, DirectoriesArea, l)
	if err != nil {
		return err
	}
	return showListing(r, l, c)
}

func (d directories) stat(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	path := DirectoriesArea + "/alpha"
	dir, err := d.storeOf(inv).Resolve(ctx, path)
	if err != nil {
		return err
	}
	return showDirectory(r, path, dir)
}

func (d directories) move(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	r.Note("A destination that names an existing directory takes the source under its own name; one that names a new path renames. Both stay under the one top-level directory.")
	s := d.storeOf(inv)
	for _, mv := range [][2]string{
		{DirectoriesArea + "/echo", DirectoriesArea + "/alpha"},
		{DirectoriesArea + "/bravo", DirectoriesArea + "/foxtrot"},
	} {
		res, err := s.Move(ctx, mv[0], mv[1])
		if err != nil {
			return err
		}
		if err := showLine(r, "mv: %s -> %s (id %s)", res.From, res.To, res.ID); err != nil {
			return err
		}
	}
	return nil
}

func (d directories) listMoved(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	s := d.storeOf(inv)
	l := files.Listing{Page: 1, Size: 20}
	for _, path := range []string{DirectoriesArea, DirectoriesArea + "/alpha"} {
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

func (d directories) removeArea(ctx context.Context, inv *cli.Invocation, r *scenario.Reporter) error {
	r.Note("rmdir removes an empty directory only, so the tree goes from the leaves up; the working area's owner row goes with it, and a rerun starts clean.")
	return removeDirectories(ctx, d.storeOf(inv), r, DirectoriesArea)
}

// removeDirectories removes the directory at path and every directory
// under it with rmdir, deepest first, showing each removal. A directory
// that holds a file is refused by rmdir, and the refusal is returned: the
// directories tour creates none.
func removeDirectories(ctx context.Context, s *files.Store, r *scenario.Reporter, path string) error {
	l := files.Listing{Page: 1, Size: 100, Total: files.TotalNone}
	for {
		c, err := s.List(ctx, path, l)
		if err != nil {
			return err
		}
		if len(c.Directories.Rows) == 0 {
			break
		}
		for _, sub := range c.Directories.Rows {
			if err := removeDirectories(ctx, s, r, path+"/"+sub.Name); err != nil {
				return err
			}
		}
	}
	dir, err := s.RemoveDirectory(ctx, path)
	if err != nil {
		return err
	}
	return showLine(r, "rmdir: %s (id %s)", path, dir.ID)
}
