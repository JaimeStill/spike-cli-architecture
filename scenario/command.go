package scenario

import (
	"context"

	"github.com/JaimeStill/spike-cli-architecture/cli"
)

// Command builds the leaf command that runs s: named and summarized as s
// is, taking no arguments, declaring s's nodes with Use, and carrying the
// flags s.Flags defines. The dispatcher builds and starts the declared
// nodes before the command runs, so the first step runs over a System
// already up; each step reads the nodes with the command's Invocation, and
// narrates through a Reporter over the command's stdout.
func Command(s Scenario) *cli.Command {
	cmd := &cli.Command{
		Name:    s.Name,
		Summary: s.Summary,
		Args:    cli.NoArgs,
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			return Run(ctx, s, inv, NewReporter(inv.Stdout))
		},
	}
	cmd.Use(s.refs()...)
	if s.Flags != nil {
		s.Flags(cmd.Flags())
	}
	return cmd
}
