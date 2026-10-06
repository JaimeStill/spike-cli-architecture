package app

import (
	"context"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/demo"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// mountDemo builds the demo parent over the domain's files and objects
// nodes: each scenario's leaf declares the nodes its tour reads, so the
// dispatcher builds only those when it runs.
func mountDemo(d *domain) *cli.Command {
	return demo.Commands(d.files, d.objects)
}

// listCommand builds list, which prints each scenario, its summary, and the
// names of the nodes it declares. It declares no node, so it builds
// nothing, reads no configuration, and succeeds with the stack down.
func listCommand(d *domain) *cli.Command {
	return &cli.Command{
		Name:    "list",
		Summary: "List the scenarios blobfs demo runs and the nodes each declares",
		Args:    cli.NoArgs,
		Run: func(_ context.Context, inv *cli.Invocation) error {
			scenario.WriteListing(inv.Stdout, demo.Scenarios(d.files, d.objects))
			return nil
		},
	}
}
