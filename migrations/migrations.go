// Package migrations is the app's own migration set: the directory_owner
// table, which binds a blobfs directory to the unit that owns it, and the
// bookmark table, which joins a unit to one of the files it may reach, with
// one active bookmark per unit. The migrations reference blobfs's tables,
// so the set runs above blobfs's set and under sqlate's default history
// table.
//
// Table, constraint, and index names carry the workspace's prefixes (pk_,
// fk_, uq_, ix_) and never blobfs_, so an app object reads apart from one
// blobfs owns. The SQL is spike-blobfs's consumer set, copied unchanged.
package migrations

import (
	"embed"

	"github.com/standards-lab/sqlate/migrate"
)

//go:embed postgres/*.sql
var files embed.FS

// Migrations returns the app's Postgres set in version order.
func Migrations() ([]migrate.Migration, error) {
	return migrate.Files(files, "postgres")
}
