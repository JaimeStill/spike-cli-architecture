package schema

import (
	"context"
	"strconv"
	"strings"

	"github.com/standards-lab/sqlate/migrate"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/output"
)

// Body is a leaf command's body, as package cli runs it.
type Body = func(ctx context.Context, inv *cli.Invocation) error

// Deps is what the schema group needs from the composition root.
type Deps struct {
	// Run wraps each leaf's body, so the dependencies the body brings up
	// are closed when it returns.
	Run func(Body) Body

	// Client constructs the Client inside a wrapped body. It opens the
	// database on its first call in a run, so a run that never reaches a
	// body, such as help or a usage error, opens nothing.
	Client func(ctx context.Context) (*Client, error)
}

// Commands builds the schema command with its status, up, down, and reset
// subcommands. Every subcommand takes no arguments.
func Commands(d Deps) *cli.Command {
	return (&cli.Command{
		Name:    "schema",
		Summary: "Report, apply, revert, and reset the two migration sets",
	}).Add(
		d.status(),
		d.leaf("up", "Apply every pending migration, blobfs's set first and then the app's",
			(*Client).Up, "schema up: both sets at head"),
		d.leaf("down", "Revert every applied migration, the app's set first and then blobfs's; the history tables stay",
			(*Client).Down, "schema down: both sets reverted"),
		d.reset(),
	)
}

// leaf builds one subcommand over a Client method that takes no input: it
// constructs the client, runs op, and prints result on success.
func (d Deps) leaf(name, summary string, op func(*Client, context.Context) error, result string) *cli.Command {
	return &cli.Command{
		Name:    name,
		Summary: summary,
		Args:    cli.NoArgs,
		Run: d.Run(func(ctx context.Context, inv *cli.Invocation) error {
			c, err := d.Client(ctx)
			if err != nil {
				return err
			}
			if err := op(c, ctx); err != nil {
				return err
			}
			return output.Line(inv.Stdout, result)
		}),
	}
}

// status builds the status subcommand: one row per set, bottom first, with
// the set's history table, its head, its latest version, the pending
// migrations by number and name, and whether the head is dirty.
func (d Deps) status() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show each set's head, latest version, pending migrations, and dirty mark",
		Args:    cli.NoArgs,
		Run: d.Run(func(ctx context.Context, inv *cli.Invocation) error {
			c, err := d.Client(ctx)
			if err != nil {
				return err
			}
			sets, err := c.Status(ctx)
			if err != nil {
				return err
			}
			return output.Table(inv.Stdout, statusHeader, statusRows(sets))
		}),
	}
}

// reset builds the reset subcommand. It is destructive, so --yes is a
// required flag: the dispatcher refuses a reset without it as a usage
// error, before the body runs and so before the database opens. A --yes
// given as false satisfies the requirement, so the body refuses it too,
// before it asks for the client.
func (d Deps) reset() *cli.Command {
	var yes bool
	cmd := &cli.Command{
		Name:    "reset",
		Summary: "Revert every set, the app's first, and drop the history tables; requires --yes",
		Args:    cli.NoArgs,
		Run: d.Run(func(ctx context.Context, inv *cli.Invocation) error {
			if !yes {
				return cli.Usagef("--yes=false does not confirm the reset")
			}
			c, err := d.Client(ctx)
			if err != nil {
				return err
			}
			if err := c.Reset(ctx); err != nil {
				return err
			}
			return output.Line(inv.Stdout, "schema reset: both sets reverted and their history tables dropped")
		}),
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
