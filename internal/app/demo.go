package app

import (
	"context"
	"io"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/demo"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// mountDemo builds the scenarios' commands at the root over the domain's
// files and objects nodes: list, and the demo parent, in which each
// scenario's leaf declares the nodes its tour reads, so the dispatcher
// builds only those when it runs.
func mountDemo(d *domain) []*cli.Command {
	return []*cli.Command{listCommand(d), demo.Commands(d.files, d.objects)}
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
			writeScenarios(inv.Stdout, d)
			return nil
		},
	}
}

// scenariosFooter returns the root's help footer: the scenario listing list
// prints, under a Scenarios heading, as spike-blobfs's root help appended
// it. Like list, it declares no node, so the help builds nothing.
func scenariosFooter(d *domain) func(w io.Writer) {
	return func(w io.Writer) {
		_, _ = io.WriteString(w, "Scenarios:\n")
		writeScenarios(w, d)
	}
}

// writeScenarios writes the scenario listing over the domain's files and
// objects nodes to w.
func writeScenarios(w io.Writer, d *domain) {
	scenario.WriteListing(w, demo.Scenarios(d.files, d.objects))
}
