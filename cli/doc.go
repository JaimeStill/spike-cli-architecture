// Package cli is a command dispatcher on the standard library's flag package.
// A program builds a tree of [Command] values, each owning a *flag.FlagSet,
// and hands the root to [Run] with the process arguments; Run walks the tree
// to the selected command, parses its flags, runs it, and returns the exit
// code the program passes to os.Exit, following go-core's process
// convention. Wiring mistakes in the tree panic when it is built or at the
// start of its first dispatch, as each symbol's documentation states.
//
// The package exports:
//
//   - [Command], one node of a command tree, a parent or a leaf
//   - [Command.Add], which attaches subcommands
//   - [Command.Flags], which returns the command's flag set; flags defined
//     on the root are root flags, accepted at any depth
//   - [Command.Require], which marks a leaf's flags as required
//   - [Command.Exclusive], which declares a mutually exclusive flag group
//   - [Command].Uses, the graph nodes a command needs
//   - [Invocation], what a running command receives, and
//     [Invocation].System, the System built for its Uses
//   - [Invocation.Changed], which reports whether a flag was given
//   - [Run], which dispatches the arguments and returns the exit code
//   - [Option], which configures one Run, and [WithGraph], the Option that
//     gives Run the graph that Uses are built from
//   - [StringsVar], which defines a repeatable string flag
//   - [NoArgs] and [ExactArgs], the positional-argument validators
//   - [UsageError], an error reported with the command's usage
//   - [Usagef], which returns a formatted UsageError
//
// # Dependencies
//
// A command's dependencies are part of the command: [Command].Uses names
// the [graph.Node] values it needs. A leaf needs the union of the Uses on
// its path, from the root to itself, so Uses on a parent is inherited by
// every leaf below it, a leaf adds its own, and a node named at several
// levels is built once. A leaf whose path has no Uses runs with no Build
// and no lifecycle.
//
// A dispatch to a leaf goes in this order, stopping at the first failure:
// validate its arguments, check its required flags, check its exclusive
// groups, run the root's PreRun, and then, when the union of Uses is not
// empty, build it with the lifecycle configuration node from the
// [WithGraph] graph, start the System with a [lifecycle.Coordinator], run
// the leaf with [Invocation].System set, and shut the System down in
// reverse. Help, a usage error, or a PreRun error ends the dispatch before
// anything is built. A Build, start, or shutdown error, or the leaf's own,
// is reported once on stderr with ExitFailure.
//
// The package imports the standard library, go-core, and the spike's graph
// and lifecycle packages.
package cli
