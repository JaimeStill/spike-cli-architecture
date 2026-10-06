package app

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/JaimeStill/spike-cli-architecture/cli"
)

// versionCommand returns the dependency-free version command, which prints
// the main module's version.
func versionCommand() *cli.Command {
	return &cli.Command{
		Name:    "version",
		Summary: "Print the blobfs version",
		Args:    cli.NoArgs,
		Run: func(_ context.Context, inv *cli.Invocation) error {
			_, err := fmt.Fprintln(inv.Stdout, moduleVersion())
			return err
		},
	}
}

// moduleVersion returns the main module's version from the build info, or
// "v0.0.0" when the binary carries none, as with go run or go test.
func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "v0.0.0"
	}
	return info.Main.Version
}
