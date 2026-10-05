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
// A leaf can require flags with [Command.Require] and declare mutually
// exclusive groups with [Command.Exclusive], over its own flags and the root
// flags; a flag counts as given when Changed reports it. The root's
// [Command.PreRun] hook runs once per dispatch, before the leaf's Run, for
// work every command shares, such as checking the root flags. A dispatch to
// a leaf goes in this order, stopping at the first failure: parse each
// level's flags, select the leaf, validate its arguments, check its
// required flags and then its exclusive groups, run PreRun, run the leaf.
//
// Run owns every line the dispatcher prints, and the exit codes follow
// go-core's process convention:
//
//   - a parent command run with no subcommand, or any command given -h or
//     --help, prints its generated help to stdout and returns ExitUsage
//   - an unknown subcommand, an unknown flag, a malformed flag value, a
//     rejected argument count, a missing required flag, a broken exclusive
//     group, or a [UsageError] that PreRun or the command returns is
//     reported on stderr with the command's usage and returns ExitUsage
//   - any other error PreRun or the command returns is reported once on
//     stderr and returns ExitFailure
//
// Wiring mistakes, such as two subcommands with one name, a flag defined
// twice, a command redefining a root flag, a PreRun below the root, or a
// requirement or group naming a flag the command lacks, panic while the
// tree is built or at the start of its first dispatch, so they surface the
// first time the program starts rather than on the path a user happens to
// take.
//
// The package imports only the standard library and go-core.
package cli
