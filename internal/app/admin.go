package app

import (
	"context"

	"github.com/standards-lab/sqlate"
	sqlatepostgres "github.com/standards-lab/sqlate/postgres"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/cli"
)

// schemaGroup builds the schema command group over d, a handle declaring
// Postgres alone: each leaf's body runs in d.Run, and the client it asks
// for wraps the pool d.Postgres brings up in sqlate's Postgres dialect.
func schemaGroup(d *deps) *cli.Command {
	return schema.Commands(schema.Deps{
		Run: d.Run,
		Client: func(ctx context.Context) (*schema.Client, error) {
			db, err := d.Postgres(ctx)
			if err != nil {
				return nil, err
			}
			return schema.NewClient(sqlate.Wrap(db.Conn(), sqlatepostgres.Dialect{}), nil)
		},
	})
}
