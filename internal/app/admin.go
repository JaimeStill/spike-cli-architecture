package app

import (
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/postgres"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// admin is the graph's administration nodes: the migrator the schema group
// uses, over the infrastructure's database.
type admin struct {
	in       *infrastructure
	migrator *graph.Node[*migrate.Migrator]
}

// defineAdmin defines the administration nodes on g over in. It constructs
// nothing.
func defineAdmin(g *graph.Graph, in *infrastructure) *admin {
	a := &admin{in: in}
	a.migrator = g.Define("migrator", a.newMigrator)
	return a
}

// mountAdmin builds the administration commands at the root: the schema
// group over the migrator node.
func mountAdmin(a *admin) []*cli.Command {
	return []*cli.Command{schema.Commands(a.migrator)}
}

// newMigrator constructs the schema migrator over the database's pool,
// wrapped in sqlate's Postgres dialect. It does no I/O: the pool first
// connects when the lifecycle starts the database, after the Build, and the
// migrator itself opens nothing.
func (a *admin) newMigrator(s *graph.Scope) (*migrate.Migrator, error) {
	db := s.Use(a.in.database)
	return schema.NewMigrator(sqlate.Wrap(db.Conn(), postgres.Dialect{}))
}
