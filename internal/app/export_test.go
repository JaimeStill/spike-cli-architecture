package app

import (
	godatabase "github.com/standards-lab/go-database"
	"github.com/standards-lab/go-storage"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
)

// The probe hook: tests reach an App's graph and its nodes, to Replace a
// node's constructor with a recorder before the App runs, and its root, to
// add a test-only command that declares, with Use, nodes no production
// command combines.

// Nodes is an App's graph nodes.
type Nodes struct {
	DatabaseConfig  *graph.Node[godatabase.Config]
	StorageConfig   *graph.Node[storage.Config]
	LifecycleConfig *graph.Node[lifecycle.Config]
	Database        *graph.Node[*godatabase.DB]
	Store           *graph.Node[*storage.Store]
	Migrator        *graph.Node[*schema.Client]
	Files           *graph.Node[*files.Store]
}

// Graph returns the graph a's commands are built from.
func (a *App) Graph() *graph.Graph { return a.graph }

// Root returns a's root command.
func (a *App) Root() *cli.Command { return a.root }

// Nodes returns a's graph nodes.
func (a *App) Nodes() Nodes {
	return Nodes{
		DatabaseConfig:  a.infra.databaseConfig,
		StorageConfig:   a.infra.storageConfig,
		LifecycleConfig: a.infra.lifecycleConfig,
		Database:        a.infra.database,
		Store:           a.infra.store,
		Migrator:        a.admin.migrator,
		Files:           a.domain.files,
	}
}

// NewDatabase, NewStore, NewMigrator, and NewFiles are a's production
// constructors for the database, store, migrator, and files nodes, for a
// Replace-d constructor that records or wraps the real one.
func (a *App) NewDatabase(s *graph.Scope) (*godatabase.DB, error) { return a.infra.newDatabase(s) }
func (a *App) NewStore(s *graph.Scope) (*storage.Store, error)    { return a.infra.newStore(s) }
func (a *App) NewMigrator(s *graph.Scope) (*schema.Client, error) { return a.admin.newMigrator(s) }
func (a *App) NewFiles(s *graph.Scope) (*files.Store, error)      { return a.domain.newFiles(s) }
