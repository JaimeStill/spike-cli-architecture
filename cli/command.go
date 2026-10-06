package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// Command is one node of a command tree. A command with subcommands is a
// parent: it selects one of them by the next argument, and run alone it
// prints its help. A command with a Run function and no subcommands is a
// leaf: it parses its flags and runs.
//
// Set the exported fields in a composite literal, then attach subcommands
// with [Command.Add], declare graph dependencies with [Command.Use], and
// define flags on [Command.Flags].
type Command struct {
	// Name is the word that selects the command under its parent. For the
	// root it is the program name shown in usage lines.
	Name string

	// Summary is the one line the parent's help lists beside Name, and the
	// first line of the command's own help.
	Summary string

	// Synopsis describes the positional arguments in the usage line, such
	// as "<container> <path>". It is empty for a command that takes none.
	Synopsis string

	// Args validates a leaf's positional arguments before Run is called,
	// such as [NoArgs] or [ExactArgs]. An error it returns is reported as a
	// usage error, with ExitUsage, and Run is not called. A nil Args accepts
	// any count. Setting Args on a parent panics when the tree is dispatched.
	Args func(args []string) error

	// Run runs a leaf command. An error it returns is reported on stderr:
	// a [UsageError] with the command's usage and ExitUsage, any other
	// error once with ExitFailure. A nil Run makes the command a parent.
	Run func(ctx context.Context, inv *Invocation) error

	// PreRun is a hook on the root that runs once per dispatch to a leaf,
	// after its flags parse and pass their checks and before its Run, with
	// the Invocation Run will receive. It suits work every command shares,
	// such as validating root flags. An error it returns is reported as
	// Run's would be, and Run is not called. It does not run when the
	// dispatch ends earlier: on help, an unknown subcommand, or a usage
	// error. Setting PreRun below the root panics when the tree is
	// dispatched.
	PreRun func(ctx context.Context, inv *Invocation) error

	flags     *flag.FlagSet
	inherited map[string]bool // names of the root flags shared into flags
	parent    *Command
	children  []*Command
	required  []string    // flag names from Require, in order
	exclusive [][]string  // flag groups from Exclusive, in order
	uses      []graph.Ref // graph nodes from Use, in order
}

// Invocation is what a running command receives: its positional arguments
// and the streams the dispatcher was given.
type Invocation struct {
	// Args holds the positional arguments left after flag parsing.
	Args []string

	// Stdin is the reader passed to [Run], such as the content of a
	// command that reads "-" as standard input. The dispatcher never reads
	// it, so a command that does not read it leaves it unconsumed.
	Stdin io.Reader

	// Stdout and Stderr are the writers passed to [Run].
	Stdout io.Writer
	Stderr io.Writer

	// System is the System built for the nodes the leaf's path declares
	// with [Command.Use], set when the path declares any; it is nil for a
	// leaf whose path declares none, and while the root's PreRun runs,
	// since PreRun runs before the Build.
	System *graph.System

	changed map[string]bool
}

// Changed reports whether the flag called name was set on the command
// line, even to its default value, rather than left at its default. It
// covers the command's own flags and the root flags, wherever in the
// command line a root flag was given; it is false for any other name.
func (inv *Invocation) Changed(name string) bool {
	return inv.changed[name]
}

// Flags returns the command's flag set, creating it on first use. The set
// uses flag.ContinueOnError and discards its own output, so the dispatcher
// does all the printing. Defining a flag twice panics, as flag.FlagSet does.
//
// Define flags by their long name only; the dispatcher has no shorthand
// flags, and accepts -h beside --help as the one exception. A leaf accepts
// its flags before, between, and after its positional arguments, up to a
// "--" after which every argument is positional; a parent's flags precede
// its subcommand. Flags defined on the root are root flags:
// every command below it accepts them too, at any depth, and they set the
// same value. A command below the root that defines a flag with a root
// flag's name panics when the tree is dispatched.
func (c *Command) Flags() *flag.FlagSet {
	if c.flags == nil {
		c.flags = flag.NewFlagSet(c.Name, flag.ContinueOnError)
		c.flags.SetOutput(io.Discard)
	}
	return c.flags
}

// Add attaches subs as subcommands of c and returns c, so a tree can be
// built in one expression. It panics on a wiring mistake: a subcommand with
// no name, a name containing whitespace or starting with "-", a name already
// taken under c, or a subcommand already attached to another parent.
func (c *Command) Add(subs ...*Command) *Command {
	for _, sub := range subs {
		switch {
		case sub.Name == "":
			panic(fmt.Sprintf("cli: %s: subcommand with no name", c.path()))
		case strings.ContainsFunc(sub.Name, unicode.IsSpace) || strings.HasPrefix(sub.Name, "-"):
			panic(fmt.Sprintf("cli: %s: invalid subcommand name %q", c.path(), sub.Name))
		case c.child(sub.Name) != nil:
			panic(fmt.Sprintf("cli: %s: duplicate subcommand %q", c.path(), sub.Name))
		case sub.parent != nil:
			panic(fmt.Sprintf("cli: %s: subcommand %q already added to %s", c.path(), sub.Name, sub.parent.path()))
		}
		sub.parent = c
		c.children = append(c.children, sub)
	}
	return c
}

// Use declares the graph nodes the command needs, appended to any already
// declared; it returns the command, so it chains like Add. A leaf needs the
// union of the nodes declared along its path, from the root to itself, so a
// parent's nodes are inherited by every leaf below it, and a node declared
// at several levels counts once. [Run] builds that union and runs the leaf
// under it, with [Invocation].System set.
//
// Use with no refs declares nothing, as Require with no names requires
// nothing. An untyped nil ref panics here, as Add's wiring mistakes do,
// since nothing declared later can make it valid. A nil *graph.Node is not
// caught here: it reaches [graph.Graph.Build], which panics on it when a
// dispatch runs a leaf at or below c, after PreRun and before any node is
// constructed. Use anywhere in a tree requires
// Run's [WithGraph] option, which is checked when the tree is dispatched
// and panics then, whichever command is selected.
func (c *Command) Use(refs ...graph.Ref) *Command {
	if slices.Contains(refs, nil) {
		panic(fmt.Sprintf("cli: %s: Use of a nil node", c.path()))
	}
	c.uses = append(c.uses, refs...)
	return c
}

// child returns the subcommand of c named name, or nil.
func (c *Command) child(name string) *Command {
	for _, sub := range c.children {
		if sub.Name == name {
			return sub
		}
	}
	return nil
}

// isParent reports whether c selects a subcommand rather than running. A
// command with neither subcommands nor Run counts as a parent, so running it
// prints its help instead of doing nothing.
func (c *Command) isParent() bool {
	return len(c.children) > 0 || c.Run == nil
}

// path returns the command's words from the root, such as "blobfs blob get".
func (c *Command) path() string {
	if c.parent == nil {
		return c.Name
	}
	return c.parent.path() + " " + c.Name
}
