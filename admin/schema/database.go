package schema

import (
	"context"
	"log/slog"
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

// Client runs the schema operations over the multi-set migrator.
type Client struct {
	migrator *migrate.Migrator
}

// NewClient builds the migrator over db for the sets [Sets] returns,
// logging each applied and reverted migration to logger; a nil logger is
// silent. It performs no I/O: the migrator validates the sets and opens
// nothing.
func NewClient(db *sqlate.DB, logger *slog.Logger) (*Client, error) {
	sets, err := Sets()
	if err != nil {
		return nil, err
	}
	m, err := migrate.New(db, sets, migrate.Options{Logger: logger})
	if err != nil {
		return nil, err
	}
	return &Client{migrator: m}, nil
}

// Up applies every pending migration of both sets, blobfs's set first.
func (c *Client) Up(ctx context.Context) error {
	return c.migrator.Up(ctx)
}

// Down reverts every applied migration of both sets, the app's set first,
// so its foreign keys into blobfs's tables never block the revert. The
// history tables stay. Each set reverts in its own locked run, so a failure
// in blobfs's set leaves the app's set reverted.
func (c *Client) Down(ctx context.Context) error {
	for _, l := range slices.Backward(c.migrator.Layers()) {
		if err := l.Down(ctx, len(l.Migrations())); err != nil {
			return err
		}
	}
	return nil
}

// Reset reverts both sets as Down does and drops their history tables, so
// the database returns to its state before the first Up.
func (c *Client) Reset(ctx context.Context) error {
	return c.migrator.Reset(ctx)
}

// Status reads both sets' state bottom first, without a lock.
func (c *Client) Status(ctx context.Context) ([]migrate.SetStatus, error) {
	return c.migrator.Status(ctx)
}
