package app

import (
	"context"
	"io"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// App is the blobfs program: its dependency graph, its command tree, and
// the writers it reports to.
type App struct {
	graph  *graph.Graph
	infra  *infrastructure
	admin  *admin
	domain *domain
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
		graph:  g,
		infra:  infra,
		admin:  defineAdmin(g, infra),
		domain: defineDomain(g, infra),
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
	a.root.Add(files.Commands(a.domain.files)...)
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
	return cli.Run(ctx, a.root, args, a.stdout, a.stderr, cli.WithGraph(a.graph, a.infra.lifecycleConfig))
}
