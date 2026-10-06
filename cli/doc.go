// Package cli is a command dispatcher on the standard library's flag
// package. A program builds a tree of [Command] values, each owning a
// *flag.FlagSet, and hands the root to [Run] with the process arguments and
// its standard input, output, and error streams; Run walks the tree to the
// selected command, parses its flags, runs it, and returns the exit code the
// program passes to os.Exit, following go-core's process convention. Wiring
// mistakes in the tree panic when it is built or when it is dispatched, as
// each symbol's documentation states.
//
// The package exports:
//
//   - [Command], one node of a command tree, a parent or a leaf
//   - [Command.Add], which attaches subcommands
//   - [Command.Flags], which returns the command's flag set; flags defined
//     on the root are root flags, accepted at any depth
//   - [Command.Require], which marks a leaf's flags as required
//   - [Command.Exclusive], which declares a mutually exclusive flag group
//   - [Command.Use], which declares the graph nodes a command needs
//   - [Invocation], what a running command receives: its arguments, the
//     streams Run was given, Stdin among them, and [Invocation].System,
//     the System built for the nodes its path declares
//   - [Invocation.Changed], which reports whether a flag was given
//   - [Run], which dispatches the arguments and returns the exit code
//   - [Option], which configures one Run, and [WithGraph], the Option that
//     gives Run the graph that declared nodes are built from
//   - [StringsVar], which defines a repeatable string flag
//   - [NoArgs] and [ExactArgs], the positional-argument validators
//   - [UsageError], an error reported with the command's usage
//   - [Usagef], which returns a formatted UsageError
//
// # Dependencies
//
// A command's dependencies are part of the command: [Command.Use] declares
// the [graph.Node] values it needs, inherited along the command path, and
// [Run] builds the union a leaf's path declares from the [WithGraph] graph
// and runs the leaf under a [lifecycle.Coordinator] only once the dispatch
// reaches it, so help, a usage error, or a PreRun error builds nothing. A
// leaf whose path declares no nodes runs with no Build and no lifecycle.
//
// The package imports the standard library, go-core, and the spike's graph
// and lifecycle packages.
package cli
