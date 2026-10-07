package app

import (
	bfdata "github.com/standards-lab/blobfs/data"
	blobfspg "github.com/standards-lab/blobfs/postgres"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// domain is the graph's domain nodes: the files Store the directory and
// bookmark commands use, over the infrastructure's database and never its
// object store, and the files domain's Objects the object commands use,
// over the Store and the object store.
type domain struct {
	in      *infrastructure
	files   *graph.Node[*files.Store]
	objects *graph.Node[*files.Objects]
}

// defineDomain defines the domain nodes on g over in. It constructs
// nothing.
func defineDomain(g *graph.Graph, in *infrastructure) *domain {
	d := &domain{in: in}
	d.files = g.Define("files", d.newFiles)
	d.objects = g.Define("objects", d.newObjects)
	return d
}

// mountDomain builds the domain's commands at the root: the directory and
// bookmark commands over the files node, and the object commands over the
// objects node.
func mountDomain(d *domain) []*cli.Command {
	return append(files.Commands(d.files), files.ObjectCommands(d.objects)...)
}

// newFiles constructs the files Store over the database's pool, wrapped in
// sqlate's Postgres dialect, with blobfs's Postgres engine: this is the
// one place the engine is named, as it is fixed for the program. It does
// no I/O. The Store's own Start, its statement check, is the node's start,
// which runs once the database has started, so a schema that is not
// applied fails the command at start, labelled with the node's name, before
// its body runs. The Store holds nothing to shut down.
func (d *domain) newFiles(s *graph.Scope) (*files.Store, error) {
	db := s.Use(d.in.database)
	return files.New(sqlate.Wrap(db.Conn(), postgres.Dialect{}), bfdata.WithEngine(blobfspg.Engine))
}

// newObjects constructs the object operations over the files Store and the
// object store. It does no I/O and records no start of its own: the
// store's start creates its container and probes it, and the Store's
// start checks the statements, so by the time the command's body runs
// both are known to work, and a store that cannot be reached fails the
// command at start, labelled with the store's node name.
func (d *domain) newObjects(s *graph.Scope) (*files.Objects, error) {
	return files.NewObjects(s.Use(d.files), s.Use(d.in.store)), nil
}
