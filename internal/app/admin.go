package app

import (
	"github.com/standards-lab/sqlate"
	sqlatepostgres "github.com/standards-lab/sqlate/postgres"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// admin is the graph's administration nodes: the migrator the schema group
// uses, over the infrastructure's database.
type admin struct {
	in       *infrastructure
	migrator *graph.Node[*schema.Client]
}

// defineAdmin defines the administration nodes on g over in. It constructs
// nothing.
func defineAdmin(g *graph.Graph, in *infrastructure) *admin {
	a := &admin{in: in}
	a.migrator = g.Define("migrator", a.newMigrator)
	return a
}

// newMigrator constructs the schema client over the database's pool,
// wrapped in sqlate's Postgres dialect. It does no I/O: the pool first
// connects when the lifecycle starts the database, after the Build, and the
// client itself opens nothing.
func (a *admin) newMigrator(s *graph.Scope) (*schema.Client, error) {
	db := s.Use(a.in.database)
	return schema.NewClient(sqlate.Wrap(db.Conn(), sqlatepostgres.Dialect{}), nil)
}
