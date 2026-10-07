package app

import (
	"context"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/sqlate/migrate"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
)

// App is the blobfs program: its dependency graph, its command tree, and
// the streams it reads from and reports to.
type App struct {
	graph   *graph.Graph
	infra   *infrastructure
	admin   *admin
	domain  *domain
	root    *cli.Command
	streams cli.Streams
}

// New describes the graph, builds the command tree, and returns the App. It
// is cold: it constructs nothing, reads no configuration, and writes
// nothing until [App.Run]. streams are what every run reads from and
// writes to: Stdin is what a command reads as standard input, such as
// put's content from -, and nothing reads it but such a command.
func New(streams cli.Streams) *App {
	g := graph.New()
	infra := defineInfrastructure(g)
	a := &App{
		graph:  g,
		infra:  infra,
		admin:  defineAdmin(g, infra),
		domain: defineDomain(g, infra),
		root: &cli.Command{
			Name:    "blobfs",
			Summary: "blobfs manages files in a blob store.",
		},
		streams: streams,
	}
	a.root.Add(versionCommand())
	a.root.Add(mountAdmin(a.admin)...)
	a.root.Add(mountDomain(a.domain)...)
	a.root.Add(mountDemo(a.domain)...)
	a.root.Footer = scenariosFooter(a.domain)
	return a
}

// Run dispatches args, the program arguments without the program name, and
// returns the process exit code. The dispatcher builds the nodes the
// selected command's path declares with Use from the App's graph, with the
// lifecycle configuration node, and shuts what it built down before Run
// returns, so a Build, start, or shutdown error is reported with the
// command's result. An App runs one command at a time, since a graph.Graph
// is not safe for concurrent use.
func (a *App) Run(ctx context.Context, args []string) int {
	return cli.Run(ctx, a.root, args, a.streams, cli.WithGraph(a.graph, a.infra.lifecycleConfig))
}

// Nodes is the App's graph nodes, one handle each, as [App.Nodes] returns
// them: the configurations, the infrastructure built from them, the
// migrator, and the files domain's two nodes. Each field's node is named
// as the dispatcher labels its errors.
type Nodes struct {
	DatabaseConfig  *graph.Node[database.Config]   // "database config"
	StorageConfig   *graph.Node[storage.Config]    // "storage config"
	LifecycleConfig *graph.Node[lifecycle.Config]  // "lifecycle config"
	Database        *graph.Node[*database.DB]      // "database"
	Store           *graph.Node[*storage.Store]    // "store"
	Migrator        *graph.Node[*migrate.Migrator] // "migrator"
	Files           *graph.Node[*files.Service]    // "files"
	Storage         *graph.Node[*files.Storage]    // "storage"
}

// Graph returns the graph a's commands are built from, as [New] described
// it. The program itself never calls it; it is published so a caller
// can, before the App runs, observe what a run builds with
// graph.Graph.Observe, or Replace a node's constructor with a substitute.
// A Replace changes the App itself, and the graph panics on a Replace once
// a run has built from it.
func (a *App) Graph() *graph.Graph { return a.graph }

// Nodes returns a handle on each of a's graph nodes, for a caller's Replace
// or for a command it adds over them.
func (a *App) Nodes() Nodes {
	return Nodes{
		DatabaseConfig:  a.infra.databaseConfig,
		StorageConfig:   a.infra.storageConfig,
		LifecycleConfig: a.infra.lifecycleConfig,
		Database:        a.infra.database,
		Store:           a.infra.store,
		Migrator:        a.admin.migrator,
		Files:           a.domain.files,
		Storage:         a.domain.storage,
	}
}

// Root returns a's root command. A command a caller Adds to it before the
// App runs is part of the App's tree from then on.
func (a *App) Root() *cli.Command { return a.root }
