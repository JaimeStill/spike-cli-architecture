// Package cli is a command dispatcher on the standard library's flag package.
// A program builds a tree of [Command] values, each owning a *flag.FlagSet,
// and hands the root to [Run] with the process arguments; Run walks the tree
// to the selected command, parses its flags, runs it, and returns the exit
// code the program passes to os.Exit.
//
// A leaf parses its flags wherever they fall among its positional
// arguments, up to a "--" after which every argument is positional; a
// parent's flags precede its subcommand. Flags defined on the root are root
// flags, accepted by every command at any depth; flags defined on a nested
// parent stay local to it. A leaf can validate its positional count with
// [Command.Args], ask whether a flag was given with [Invocation.Changed], and
// take a repeatable flag with [StringsVar].
//
// Run owns every line the dispatcher prints, and the exit codes follow
// go-core's process convention:
//
//   - a parent command run with no subcommand, or any command given -h or
//     --help, prints its generated help to stdout and returns ExitUsage
//   - an unknown subcommand, an unknown flag, a malformed flag value, or a
//     [UsageError] a command returns is reported on stderr with the command's
//     usage and returns ExitUsage
//   - any other error a command returns is reported once on stderr and
//     returns ExitFailure
//
// Wiring mistakes, such as two subcommands with one name, a flag defined
// twice, or a command redefining a root flag, panic while the tree is built
// or at the start of its first dispatch, so they surface the first time the
// program starts rather than on the path a user happens to take.
//
// The package imports only the standard library and go-core.
package cli
