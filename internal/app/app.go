// Package app is blobfs's composition root. [New] describes blobfs's
// dependencies as one graph.Graph and builds the command tree on package
// cli over it; [App.Run] dispatches the process arguments over that tree.
//
// The graph's nodes are infrastructure.go's, each configuration finalized
// under the BLOBFS prefix and the database and object store built from
// them, and admin.go's, the migrator built on the database. Describing them
// constructs nothing. A command names the nodes it needs in its Uses: the
// schema group names the migrator, and its verbs inherit it, so a schema
// verb builds the migrator, the database, and its configuration, and never
// the object store. version names none. The dispatcher builds the leaf's
// Uses only when the leaf runs, starts what was built layer by layer, and
// shuts it down in reverse when the leaf returns; a run that builds
// nothing, such as help, a usage error, or version, reads no configuration.
package app

import (
	"context"
	"io"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// App is the blobfs program: its dependency graph, its command tree, and
// the writers it reports to.
type App struct {
	graph  *graph.Graph
	infra  *infrastructure
	admin  *admin
	root   *cli.Command
	stdout io.Writer
	stderr io.Writer
}

// New describes the graph, builds the command tree, and returns the App. It
// is cold: it constructs nothing, reads no configuration, and writes
// nothing until [App.Run].
func New(stdout, stderr io.Writer) *App {
	g := graph.New()
	infra := defineInfrastructure(g)
	a := &App{
		graph: g,
		infra: infra,
		admin: defineAdmin(g, infra),
		root: &cli.Command{
			Name:    "blobfs",
			Summary: "blobfs manages files in a blob store.",
		},
		stdout: stdout,
		stderr: stderr,
	}
	a.root.Add(
		versionCommand(),
		schema.Commands(a.admin.migrator),
	)
	return a
}

// Run dispatches args, the program arguments without the program name, and
// returns the process exit code. The dispatcher builds the selected
// command's Uses from the App's graph, with the lifecycle configuration
// node, and shuts what it built down before Run returns, so a Build, start,
// or shutdown error is reported with the command's result. An App runs one
// command at a time, since a graph.Graph is not safe for concurrent use.
func (a *App) Run(ctx context.Context, args []string) int {
	return cli.Run(ctx, a.root, args, a.stdout, a.stderr, cli.WithGraph(a.graph, a.infra.lifecycleConfig))
}
