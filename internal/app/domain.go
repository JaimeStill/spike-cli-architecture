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

// domain is the graph's domain nodes: the files Service the directory and
// bookmark commands use, over the infrastructure's database and never its
// object store, and the files domain's Storage the object commands use,
// over the Service and the object store.
type domain struct {
	in      *infrastructure
	files   *graph.Node[*files.Service]
	storage *graph.Node[*files.Storage]
}

// defineDomain defines the domain nodes on g over in. It constructs
// nothing.
func defineDomain(g *graph.Graph, in *infrastructure) *domain {
	d := &domain{in: in}
	d.files = g.Define("files", d.newFiles)
	d.storage = g.Define("storage", d.newStorage)
	return d
}

// mountDomain builds the files domain's commands at the root, each
// declaring the node it reads.
func mountDomain(d *domain) []*cli.Command {
	return files.Commands(d.files, d.storage)
}

// newFiles constructs the files Service over the database's pool, wrapped
// in sqlate's Postgres dialect, with blobfs's Postgres engine: this is the
// one place the engine is named, as it is fixed for the program. It does
// no I/O. The Service's own Start, its statement check, is the node's
// start, which runs once the database has started, so a schema that is not
// applied fails the command at start, labelled with the node's name, before
// its body runs. The Service holds nothing to shut down.
func (d *domain) newFiles(s *graph.Scope) (*files.Service, error) {
	db := s.Use(d.in.database)
	return files.New(sqlate.Wrap(db.Conn(), postgres.Dialect{}), bfdata.WithEngine(blobfspg.Engine))
}

// newStorage constructs the object operations over the files Service and
// the object store. It does no I/O and records no start of its own: the
// store's start creates its container and probes it, and the Service's
// start checks the statements, so by the time the command's body runs
// both are known to work, and a store that cannot be reached fails the
// command at start, labelled with the store's node name.
func (d *domain) newStorage(s *graph.Scope) (*files.Storage, error) {
	return files.NewStorage(s.Use(d.files), s.Use(d.in.store)), nil
}
