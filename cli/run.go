package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/standards-lab/go-core/process"
)

// Run dispatches args, the program arguments without the program name, over
// the tree rooted at root, and returns the process exit code: ExitOK when
// the selected command succeeds, ExitUsage for help and usage errors, and
// ExitFailure for any other error the command returns. Help goes to stdout;
// every error goes to stderr, reported exactly once.
func Run(ctx context.Context, root *Command, args []string, stdout, stderr io.Writer) int {
	bindRootFlags(root)
	cmd := root
	var path []*Command
	for {
		path = append(path, cmd)
		rest, code, ok := parse(cmd, args, stdout, stderr)
		if !ok {
			return code
		}
		if !cmd.isParent() {
			return execute(ctx, path, rest, stdout, stderr)
		}
		if len(rest) == 0 {
			return process.Usage(stdout, help(cmd))
		}
		sub := cmd.child(rest[0])
		if sub == nil {
			msg := fmt.Sprintf("%s: unknown command %q\n\n%s", cmd.path(), rest[0], help(cmd))
			return process.Usage(stderr, msg)
		}
		cmd, args = sub, rest[1:]
	}
}

// parse parses args against cmd's flags and returns the positional
// arguments left. A parent's flags precede its subcommand, so parsing stops
// at the first non-flag argument, which names it; a leaf's flags may come
// anywhere among its positional arguments, up to a "--". When parsing ends
// the dispatch, because help was asked for or a flag was wrong, it reports
// that and returns ok false with the exit code.
func parse(cmd *Command, args []string, stdout, stderr io.Writer) (rest []string, code int, ok bool) {
	fs := cmd.Flags()
	var positional []string
	if !cmd.isParent() {
		args, positional = interleave(fs, args)
	}
	err := fs.Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return nil, process.Usage(stdout, help(cmd)), false
	case err != nil:
		return nil, usageError(cmd, stderr, err), false
	}
	if cmd.isParent() {
		return fs.Args(), process.ExitOK, true
	}
	// interleave left only flags and their values, so fs.Args() is empty.
	return positional, process.ExitOK, true
}

// execute validates the leaf's positional arguments, runs it, and maps its
// error to an exit code. path holds the commands from the root to the leaf.
func execute(ctx context.Context, path []*Command, args []string, stdout, stderr io.Writer) int {
	cmd := path[len(path)-1]
	if cmd.Args != nil {
		if err := cmd.Args(args); err != nil {
			return usageError(cmd, stderr, err)
		}
	}
	inv := &Invocation{Args: args, Stdout: stdout, Stderr: stderr, changed: changed(path)}
	err := cmd.Run(ctx, inv)
	if err == nil {
		return process.ExitOK
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		return usageError(cmd, stderr, err)
	}
	return process.Fail(stderr, cmd.path(), err)
}

// changed returns the names of the flags set on the command line that the
// leaf at the end of path can see: its own, and the root flags, given at
// whichever level they were parsed. A nested parent's local flags are left
// out, so a leaf flag of the same name is not shadowed.
func changed(path []*Command) map[string]bool {
	root, leaf := path[0], path[len(path)-1]
	set := make(map[string]bool)
	for _, cmd := range path {
		cmd.Flags().Visit(func(f *flag.Flag) {
			if cmd == leaf || root.Flags().Lookup(f.Name) != nil {
				set[f.Name] = true
			}
		})
	}
	return set
}

// usageError reports err with cmd's short usage on stderr and returns
// ExitUsage.
func usageError(cmd *Command, stderr io.Writer, err error) int {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %v\n", cmd.path(), err)
	fmt.Fprintf(&b, "Usage: %s\n", usageLine(cmd))
	fmt.Fprintf(&b, "Run '%s --help' for details.", cmd.path())
	return process.Usage(stderr, b.String())
}
