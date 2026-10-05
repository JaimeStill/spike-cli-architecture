// Package cli is a command dispatcher on the standard library's flag package.
// A program builds a tree of [Command] values, each owning a *flag.FlagSet,
// and hands the root to [Run] with the process arguments; Run walks the tree
// to the selected command, parses its flags, runs it, and returns the exit
// code the program passes to os.Exit.
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
// Wiring mistakes, such as two subcommands with one name or a flag defined
// twice, panic while the tree is built, so they surface the first time the
// program starts rather than on the path a user happens to take.
//
// The package imports only the standard library and go-core.
package cli
