package app

import (
	bfdata "github.com/standards-lab/blobfs/data"
	blobfspg "github.com/standards-lab/blobfs/postgres"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// domain is the graph's domain nodes: the files Store the directory
// commands use, over the infrastructure's database and never its object
// store.
type domain struct {
	in    *infrastructure
	files *graph.Node[*files.Store]
}

// defineDomain defines the domain nodes on g over in. It constructs
// nothing.
func defineDomain(g *graph.Graph, in *infrastructure) *domain {
	d := &domain{in: in}
	d.files = g.Define("files", d.newFiles)
	return d
}

// newFiles constructs the files Store over the database's pool, wrapped in
// sqlate's Postgres dialect, with blobfs's Postgres engine: this is the
// one place the engine is named, as it is fixed for the program. It does
// no I/O. The node's start is the Store's statement check, which runs once
// the database has started, so a schema that is not applied fails the
// command at start, labelled with the node's name, before its body runs.
// The Store holds nothing to shut down.
func (d *domain) newFiles(s *graph.Scope) (*files.Store, error) {
	db := s.Use(d.in.database)
	store, err := files.New(sqlate.Wrap(db.Conn(), postgres.Dialect{}), bfdata.WithEngine(blobfspg.Engine))
	if err != nil {
		return nil, err
	}
	s.OnStart(store.Verify)
	return store, nil
}
