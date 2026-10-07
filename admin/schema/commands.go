package schema

import (
	"context"
	"strconv"
	"strings"

	"github.com/standards-lab/sqlate/migrate"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/output"
)

// Commands builds the schema command with its status, up, down, and reset
// subcommands over client, the composition root's node for the [Client].
// The group declares client with Use, and every subcommand inherits it, so
// the dispatcher builds and starts the client's dependencies before a
// verb's body runs and shuts them down after; the body reads the client
// with the Invocation's Get. Every subcommand takes no arguments.
func Commands(client *graph.Node[*Client]) *cli.Command {
	g := group{client: client}
	return (&cli.Command{
		Name:    "schema",
		Summary: "Report, apply, revert, and reset the two migration sets",
	}).Use(client).Add(
		g.status(),
		g.leaf("up", "Apply every pending migration, blobfs's set first and then the app's",
			(*Client).Up, "schema up: both sets at head"),
		g.leaf("down", "Revert every applied migration, the app's set first and then blobfs's; the history tables stay",
			(*Client).Down, "schema down: both sets reverted"),
		g.reset(),
	)
}

// group is the schema group's handle on its Client node.
type group struct {
	client *graph.Node[*Client]
}

// leaf builds one subcommand over a Client method that takes no input: it
// runs op on the built client and prints result on success.
func (g group) leaf(name, summary string, op func(*Client, context.Context) error, result string) *cli.Command {
	return &cli.Command{
		Name:    name,
		Summary: summary,
		Args:    cli.NoArgs,
		Run: func(ctx context.Context, inv *cli.Invocation) error {
			if err := op(inv.Get(g.client), ctx); err != nil {
				return err
			}
			return output.Line(inv.Stdout, result)
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
			sets, err := inv.Get(g.client).Status(ctx)
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
			if err := inv.Get(g.client).Reset(ctx); err != nil {
				return err
			}
			return output.Line(inv.Stdout, "schema reset: both sets reverted and their history tables dropped")
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the reset: every set is reverted and the history tables are dropped")
	cmd.Require("yes")
	return cmd
}

var statusHeader = []string{"set", "table", "version", "latest", "pending", "dirty"}

// statusRows renders each set's status as one row under statusHeader. The
// pending column lists the pending migrations as "N name", or none.
func statusRows(sets []migrate.SetStatus) [][]string {
	rows := make([][]string, 0, len(sets))
	for _, s := range sets {
		pending := "none"
		if len(s.Pending) > 0 {
			names := make([]string, 0, len(s.Pending))
			for _, m := range s.Pending {
				names = append(names, strconv.Itoa(m.Version)+" "+m.Name)
			}
			pending = strings.Join(names, ", ")
		}
		rows = append(rows, []string{
			s.Name, s.Table, strconv.Itoa(s.Version), strconv.Itoa(s.Latest), pending, strconv.FormatBool(s.Dirty),
		})
	}
	return rows
}
