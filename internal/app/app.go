// Package app is blobfs's composition root. It builds the command tree on
// package cli and mounts each command; [App.Run] dispatches the process
// arguments over that tree.
package app

import (
	"context"
	"io"

	"github.com/JaimeStill/spike-cli-architecture/cli"
)

// App is the blobfs program: its command tree and the writers it reports to.
type App struct {
	root   *cli.Command
	stdout io.Writer
	stderr io.Writer
}

// New builds the command tree and returns the App. It is cold: it opens
// nothing and writes nothing until [App.Run].
func New(stdout, stderr io.Writer) *App {
	root := &cli.Command{
		Name:    "blobfs",
		Summary: "blobfs manages files in a blob store.",
	}
	root.Add(versionCommand())
	return &App{root: root, stdout: stdout, stderr: stderr}
}

// Run dispatches args, the program arguments without the program name, and
// returns the process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	return cli.Run(ctx, a.root, args, a.stdout, a.stderr)
}
