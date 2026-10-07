package schema

import (
	"context"
	"slices"

	blobfspostgres "github.com/standards-lab/blobfs/postgres"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"

	"github.com/JaimeStill/spike-cli-architecture/migrations"
)

// AppSet is the name of the app's migration set in the migrator and in its
// status rows. blobfs's set carries the name its source exports.
const AppSet = "app"

// Sets returns the two migration sets bottom first: blobfs's set, under its
// own history table, and then the app's set, under sqlate's default table.
func Sets() ([]migrate.Set, error) {
	blobfsSet, err := blobfspostgres.Migrations()
	if err != nil {
		return nil, err
	}
	appSet, err := migrations.Migrations()
	if err != nil {
		return nil, err
	}
	return []migrate.Set{blobfsSet, {Name: AppSet, Migrations: appSet}}, nil
}

// NewMigrator builds sqlate's multi-set migrator over db for the sets
// [Sets] returns. It performs no I/O: the migrator validates the sets and
// opens nothing. Its Up, Reset, and Status are the schema operations as
// they stand; [Down] is the revert the migrator does not offer.
func NewMigrator(db *sqlate.DB) (*migrate.Migrator, error) {
	sets, err := Sets()
	if err != nil {
		return nil, err
	}
	return migrate.New(db, sets, migrate.Options{})
}

// Down reverts every applied migration of m's sets, the app's set first,
// so its foreign keys into blobfs's tables never block the revert. The
// history tables stay, where m's Reset drops them. Each set reverts in its
// own locked run, so a failure in blobfs's set leaves the app's set
// reverted.
func Down(ctx context.Context, m *migrate.Migrator) error {
	for _, l := range slices.Backward(m.Layers()) {
		if err := l.Down(ctx, len(l.Migrations())); err != nil {
			return err
		}
	}
	return nil
}
