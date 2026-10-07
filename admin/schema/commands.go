package schema

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate/migrate"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/output"
)

// Commands builds the schema command with its status, up, down, and reset
// subcommands over migrator, the composition root's node for the migrator
// [NewMigrator] builds. The group declares migrator with Use, and every
// subcommand inherits it, so the dispatcher builds and starts the
// migrator's dependencies before a verb's body runs and shuts them down
// after; the body reads the migrator with the Invocation's Get. Every
// subcommand takes no arguments.
func Commands(migrator *graph.Node[*migrate.Migrator]) *cli.Command {
	g := group{migrator: migrator}
	return (&cli.Command{
		Name:    "schema",
		Summary: "Report, apply, revert, and reset the two migration sets",
	}).Use(migrator).Add(
		g.status(),
		g.leaf("up", "Apply every pending migration, blobfs's set first and then the app's",
			(*migrate.Migrator).Up, "schema up: both sets at head"),
		g.leaf("down", "Revert every applied migration, the app's set first and then blobfs's; the history tables stay",
			func(m *migrate.Migrator, ctx context.Context) error { return Down(ctx, m) }, "schema down: both sets reverted"),
		g.reset(),
	)
}

// group is the schema group's handle on its migrator node.
type group struct {
	migrator *graph.Node[*migrate.Migrator]
}

// leaf builds one subcommand over an operation that takes no input: it
// runs op on the built migrator and prints result on success.
func (g group) leaf(name, summary string, op func(*migrate.Migrator, context.Context) error, result string) *cli.Command {
	return &cli.Command{
		Name:    name,
		Summary: summary,
		Args:    cli.NoArgs,
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			if err := op(inv.Get(g.migrator), ctx); err != nil {
				return err
			}
			_, err := fmt.Fprintln(inv.Stdout, result)
			return err
		},
	}
}

// status builds the status subcommand: one row per set, bottom first, with
// the set's history table, its head, its latest version, the pending
// migrations by number and name, and whether the head is dirty.
func (g group) status() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show each set's head, latest version, pending migrations, and dirty mark",
		Args:    cli.NoArgs,
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			sets, err := inv.Get(g.migrator).Status(ctx)
			if err != nil {
				return err
			}
			return output.Table(inv.Stdout, statusHeader, statusRows(sets))
		},
	}
}

// reset builds the reset subcommand. It is destructive, so --yes is a
// required flag: the dispatcher refuses a reset without it as a usage
// error, before anything is built. A --yes given as false satisfies the
// requirement, so Validate refuses it too: it runs after the required-flag
// check and before the Build, so an unconfirmed reset never starts the
// database.
func (g group) reset() *cli.Command {
	var yes bool
	cmd := &cli.Command{
		Name:    "reset",
		Summary: "Revert every set, the app's first, and drop the history tables; requires --yes",
		Args:    cli.NoArgs,
		Validate: func(inv *cli.Invocation) error {
			// The required-flag check has already refused an absent --yes.
			if inv.Changed("yes") && !yes {
				return cli.Usagef("--yes=false does not confirm the reset")
			}
			return nil
		},
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			if err := inv.Get(g.migrator).Reset(ctx); err != nil {
				return err
			}
			_, err := fmt.Fprintln(inv.Stdout, "schema reset: both sets reverted and their history tables dropped")
			return err
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the reset: every set is reverted and the history tables are dropped")
	cmd.Require("yes")
	return cmd
}
