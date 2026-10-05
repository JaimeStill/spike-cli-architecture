// Package app is blobfs's composition root. It builds the command tree on
// package cli and mounts each command group with the dependencies it
// declares; [App.Run] dispatches the process arguments over that tree.
//
// A group declares its dependencies from a closed set, Postgres and then the
// object store, where it is mounted. Its bodies ask for a dependency through
// the handle the mount gives them; the first request in a run brings up the
// group's declared set, one dependency per phase in that fixed order, and the
// dependencies close in reverse when the body returns. A run that asks for
// none, such as help, a usage error, or version, opens nothing and reads no
// dependency configuration.
package app

import (
	"context"
	"io"

	"github.com/JaimeStill/spike-cli-architecture/cli"
)

// App is the blobfs program: its command tree and the writers it reports to.
type App struct {
	root   *cli.Command
	init   *initializer
	stdout io.Writer
	stderr io.Writer
}

// New builds the command tree and returns the App. It is cold: it opens
// nothing, reads no configuration, and writes nothing until [App.Run].
func New(stdout, stderr io.Writer) *App {
	a := &App{
		root: &cli.Command{
			Name:    "blobfs",
			Summary: "blobfs manages files in a blob store.",
		},
		init:   &initializer{openers: defaultOpeners()},
		stdout: stdout,
		stderr: stderr,
	}
	// version declares no dependencies.
	a.mount(func(*deps) *cli.Command { return versionCommand() })
	return a
}

// Run dispatches args, the program arguments without the program name, and
// returns the process exit code. The command's dependencies are closed
// before Run returns, within the command's own Run (see [deps.Run]), so a
// close error is reported with the command's result. An App runs one
// command at a time.
func (a *App) Run(ctx context.Context, args []string) int {
	return cli.Run(ctx, a.root, args, a.stdout, a.stderr)
}
