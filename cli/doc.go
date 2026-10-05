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
//   - [Invocation], what a running command receives
//   - [Invocation.Changed], which reports whether a flag was given
//   - [Run], which dispatches the arguments and returns the exit code
//   - [StringsVar], which defines a repeatable string flag
//   - [NoArgs] and [ExactArgs], the positional-argument validators
//   - [UsageError], an error reported with the command's usage
//   - [Usagef], which returns a formatted UsageError
//
// The package imports only the standard library and go-core.
package cli
