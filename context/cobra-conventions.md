# The cobra conventions and what replaced them

This note is evidence 6: a record of each cobra-specific convention in the current CLI layout and
each cobra feature spike-blobfs uses, with what replaced it in this spike or why it is no longer
needed. The layout's conventions come from the coordinator's `context/cli-applications.md`,
sections "The layout" and "Conventions". The cobra features come from a grep of spike-blobfs's
Go source.

The exit codes and help text below are what the two binaries printed when built from their
current commits into a scratch directory outside both repositories and run with the development
stack down. "spike-blobfs" is that repository's `blobfs` and "this spike" is this repository's
`blobfs`. Exit codes are go-core's: 0 is ExitOK, 1 is ExitFailure, and 2 is ExitUsage.

## The layout's cobra conventions

| Convention in the layout | What replaced it in this spike |
| --- | --- |
| Cobra builds the tree before it parses the flags, so a layer that reads configuration holds the `Config` pointer or a function and reads it at run time. | Configuration is a graph node: `Nodes.DatabaseConfig`, `Nodes.StorageConfig`, and `Nodes.LifecycleConfig` in `internal/app`. Each node's constructor reads the `BLOBFS_*` environment when a `graph.Build` reaches it, and a constructor reads it through `Scope.Use`, so nothing holds a pointer that fills in later. The root defines no flags at all, so there is no `Config` struct and no `config.go`. |
| The root sets `SilenceErrors` and `SilenceUsage`, so `App.Run` renders the one error through the output. | `cli.Run` prints every line the dispatcher prints, so there is nothing to silence. A command's error is reported once as `process.Fail(stderr, "<command path>", err)` and a usage error once with the command's usage line. `App.Run` is one call to `cli.Run` and renders nothing itself. |
| A parent sets `RunE: cmd.Help()` so it prints its help when run alone (spike-blobfs's root, `schema`, and `bookmark`). | A `cli.Command` with a nil `Run` is a parent, and a parent run alone prints its generated help. Observed: spike-blobfs's `blobfs`, `blobfs schema`, and `blobfs bookmark` print help and exit 0; this spike's print help and exit 2, as go-core's `process.Usage` documents for requested help. |
| Run with no subcommand, the root prints its help and then the scenario listing. This is slab's convention (`go-web-service/tools/slab`, whose root `RunE` calls `cmd.Help()` and then writes "Scenarios:" and the listing). spike-blobfs has no scenarios. | `cli.Command.Footer`, which appends a command's own text to its generated help wherever the full help prints. The root's Footer writes the `id:<uuid>` line and the listing, and `blobfs scenario`'s Footer writes the same listing; both call `scenario.WriteListing`. Observed: `blobfs`, `blobfs --help`, and `blobfs scenario` end with "Scenarios:" and each scenario's declared nodes, and exit 2. Slab's separate `list` command has no counterpart. |
| A flag that names something the tool can't build fails before any command runs: clutch checks `--harness` in the root's `PersistentPreRunE`. | `cli.Command.PreRun`, a hook on the root that runs once per dispatch after the leaf's checks and before the Build. blobfs has no root flags, so it sets no PreRun. A leaf's own input is checked in `cli.Command.Validate`, which runs before PreRun and the Build, and whose error is always a usage error, so bad input builds nothing. |
| `domain/<name>` builds its commands with `Commands(svc, out) *cobra.Command`. | `files.Commands(svc, storage)` and `schema.Commands(migrator)` return `[]*cli.Command`, and each layer file in `internal/app` mounts them with one `root.Add(pkg.Commands(...)...)`. Each command declares the nodes it reads with `cli.Command.Use` and reads them with `inv.Get`. No output value is passed: a command writes to the `cli.Streams` its Invocation embeds, through package `output`. |
| `config.go` binds the persistent flags into a `Config`; `commands.go` is the list of mounts. | Neither file exists. There are no persistent flags (see the first row), and `app.New` calls each layer file's mount function in turn. |
| `App.Run` executes the tree under the signal context, renders the error, closes what the infrastructure opened, and returns the exit code; `main` calls `app.New(os.Stdout, os.Stderr).Run(ctx)`. | `cli.Run` builds the union of the nodes the leaf's path declares and runs the leaf under `lifecycle.Coordinator.Exec`, which starts the System layer by layer and shuts it down in reverse, so `App.Run` closes nothing. `main` calls `app.New(cli.Streams{...}).Run(ctx, os.Args[1:])`: the arguments are a parameter, where cobra read `os.Args` itself. |
| Tests assert cobra's message text, such as spike-blobfs's `accepts 1 arg` and `required flag(s) "unit" not set`. | The tests assert the dispatcher's own text, which the module owns, and the exit code: `internal/app`'s tests check `accepts 1 argument, got 0` and `required flag --unit not set` with `process.ExitUsage`. |
| A tool beside a service or library lives in its own module, so cobra never enters the service's dependency graph (slab, following `principles/tool-beside-library.md`). | The cobra reason goes: `go mod graph` and `go list -deps ./...` show no cobra or pflag, and depguard holds `cli` to the standard library, go-core, `graph`, and `lifecycle`. This spike is a standalone module, so it does not test whether a tool beside a library still wants its own module for another reason. |

The layout's other conventions are not cobra's and carry over unchanged: subcommands are bare
words (`blobfs scenario files`), results go to stdout and errors to stderr, and the
`internal/app` tests run the command tree over buffers.

## spike-blobfs's cobra features

| Feature in spike-blobfs | What replaced it in this spike |
| --- | --- |
| `cobra.Command{Use, Short, Long}` (19 commands; 14 set `Long`). | `cli.Command{Name, Summary, Synopsis}`. `Long` has no counterpart: a command's help is its Summary, its usage line, and its flags. Observed: spike-blobfs's `ls --help` opens with a paragraph of prose; this spike's opens with the one-line Summary. `USAGE.md` walks each command with its output instead. |
| `RunE func(cmd *cobra.Command, args []string) error` reading the context with `cmd.Context()` (37 calls). | `Run func(ctx context.Context, inv *cli.Invocation) error`: the context is a parameter and the arguments are `inv.Args`. |
| `cmd.InOrStdin()` for `put -`. | `inv.Stdin`. Stdin is the `Stdin` field of `cli.Streams`, which `cli.Run` and `app.New` take beside `Stdout` and `Stderr`, and which the Invocation embeds. |
| `root.SetOut`, `root.SetErr`, `root.SetArgs`, and `root.ExecuteContext(ctx)`; spike-blobfs's app tests set `os.Args`. | `cli.Run(ctx, root, args, streams, opts...)` takes the streams and the arguments, so tests pass both and touch no process state. |
| `root.AddCommand` and `cmd.AddCommand`. | `cli.Command.Add`, which returns the parent for chaining. Help lists the commands in the order they were added; cobra's "Available Commands" sorted them by name. |
| pflag's `StringVar`, `IntVar`, and `BoolVar` on `cmd.Flags()`, with flags accepted among the positionals. | The standard library's `flag.FlagSet` from `cli.Command.Flags`. A leaf's flags may come before, between, or after its positionals, up to a `--`. |
| `StringArrayVar` for the repeatable `--sort` and `--filter`. | `cli.StringsVar`; values are not split on commas, as StringArrayVar's are not. |
| `BoolVarP(&recursive, "recursive", "r", ...)`, the one shorthand flag. | No shorthand flags: `rm --recursive`. Observed: this spike's `rm -r /x` exits 2 with `flag provided but not defined: -r`. |
| `MarkFlagRequired("unit")` on `bookmark add`, `ls`, and `rm`. | `cli.Command.Require("unit")`. Observed: spike-blobfs prints `required flag(s) "unit" not set` and exits 1; this spike prints `blobfs bookmark add: required flag --unit not set`, the usage line, and a `--help` pointer, and exits 2. `schema reset` requires `--yes` the same way, and its Validate refuses `--yes=false` through `inv.Changed`. |
| `cobra.NoArgs` (7) and `cobra.ExactArgs` (11) as `Args`. | `cli.NoArgs` and `cli.ExactArgs` as `cli.Command.Args`, which only counts; what the arguments say is Validate's. Observed: spike-blobfs's `mkdir` prints `accepts 1 arg(s), received 0` and exits 1, and its `bookmark ls /a --unit <uuid>` prints `unknown command "/a" for "blobfs bookmark ls"` and exits 1; this spike prints `accepts 1 argument, got 0` and `accepts no arguments, got 1`, each with the usage line, and exits 2. |
| The root's persistent flags, `--dsn` and `--variant`. | Dropped: configuration comes from the `BLOBFS_*` environment through the configuration nodes, and the composition root fixes blobfs's Postgres engine. Observed: this spike's `blobfs --dsn x ls /` exits 2 with `flag provided but not defined: -dsn`. `cli` still supports root flags (see below). |
| The automatic `help` command. | Not built; `-h` and `--help` are the one way to ask. Observed: spike-blobfs's `blobfs help` and `blobfs help ls` print help and exit 0, and `blobfs help version` prints the root's help and exits 0 although there is no `version`; this spike's `blobfs help` is an unknown command and exits 2. |
| The automatic `completion` command. | Not built; completion is out of the plan's scope. Observed: spike-blobfs's `blobfs completion bash` prints a bash script and exits 0; this spike's `blobfs completion` is an unknown command and exits 2. |
| `-h` and `--help` on any command, exiting 0. | `-h` and `--help` on any command, the one exception to long names only. Help goes to stdout and exits 2 through `process.Usage`. |
| An unknown command or flag exits 1 with only a message: `unknown command "frobnicate" for "blobfs"`, `unknown flag: --bogus`. | A usage error exits 2 on stderr. An unknown subcommand prints `blobfs: unknown command "frobnicate"` and then the parent's full help; a bad flag prints `blobfs ls: flag provided but not defined: -bogus`, keeping the standard library's single-dash wording, then the usage line and `Run 'blobfs ls --help' for details.` |
| An `Infrastructure` struct holding the config, opening the pool on demand through `Database` and the store through `Storage`, and closed by `App.Run` through `Close`. | Graph nodes in `internal/app` (`Nodes.Database`, `Nodes.SQL`, `Nodes.Store`, and the domain's `Nodes.Files` and `Nodes.Storage`), declared per command with `Use`. The dispatcher builds the declared union once per run and `lifecycle.Coordinator.Exec` starts each value that is a `lifecycle.Starter` and shuts down each `lifecycle.Stopper` in reverse, on success, error, and cancellation. Observed: with the stack down, `version` and help exit as above, and `ls /`, `cat /a.txt`, and `schema status` each exit 1 with one report naming the `database` node. |
| `App.Run` renders a command's error through `output.Error` and exits 1. | `cli.Run` reports it once, labelled with the command path, and exits 1. Observed: spike-blobfs's `ls /` prints `no database: set --dsn or BLOBFS_DSN`; this spike's prints `blobfs ls: database: database connection failed: ...`. |
| Tests that walk the tree with `cmd.Commands()` and `cmd.Name()`. | `cli` exports no tree accessors; tests run the tree through `cli.Run` over buffers and read what it prints. |
| No `version` command (observed: `unknown command "version" for "blobfs"`, exit 1). | `blobfs version` prints the module version and exits 0: the dependency-free command, which works with the stack down. |

## Dispatcher features blobfs doesn't use

The dispatcher carries the plan's whole feature set, and three of its features have no user in
blobfs, so only `cli`'s own tests prove them. A grep of the spike outside `cli/` finds no PreRun,
no `Exclusive` call, and no flag defined on the root; the root's help lists only `--help`.

| Feature | Its tests |
| --- | --- |
| Root flags, accepted at any depth and listed as "Global flags" | `cli/flags_test.go` (`TestRun_RootFlagAcceptedAtAnyDepth`, `TestRun_RootFlagLastOccurrenceWins`, `TestRun_HelpListsRootFlagsAsGlobal`), `cli/groups_test.go`, and `cli/cli_test.go` |
| The root's `PreRun` hook | `cli/prerun_test.go`, with `cli/use_test.go` and `cli/validate_test.go` for its place in the dispatch order |
| `Exclusive` flag groups | `cli/groups_test.go` (`TestExclusive`, `TestRequire_CheckedBeforeExclusive`), `cli/use_test.go`, and `cli/validate_test.go` |

## Candidates the work found

| Candidate | For | What the spike built |
| --- | --- | --- |
| The dependency graph | go-core | Package `graph`: nodes defined inertly, `Build` constructing only what its roots reach through `Scope.Use`, into a System of computed layers. |
| `lifecycle` rebuilt as the graph's executor | go-core | Package `lifecycle`: a Coordinator over a built System, `Exec` beside `Run`, participation through `Starter`, `Stopper`, and `Subsystem`, and a subsystem whose Start fails still shut down. |
| `Graph.Observe` | go-core, with `graph` | A tracing point that reports each node a Build begins constructing, since a System is readable only after a successful Build; `internal/apptest.Builds` records runs through it. |
| A one-shot runner beside `processtest.Launch` | go-core | `processtest.Launch` takes no args or stdin and merges stdout with stderr. The integration suite's runner (`start`, `proc.wait`, and `run` in `integration/integration_test.go`) takes args, stdin, and env and returns stdout, stderr, and the exit code, mirroring lifecycle's `Exec` beside `Run`. |
| Stdin as a Run parameter | go-cli-sdk | `cli.Streams{Stdin, Stdout, Stderr}`, taken by `cli.Run` and embedded in `cli.Invocation`; it changes the candidate's `Run` signature. |
| Dependencies as part of the command | go-cli-sdk | `cli.Command.Use`, `cli.WithGraph`, and `inv.Get`: the dispatcher builds the declared nodes and runs the leaf under `Coordinator.Exec`. It replaces the post-run hook the composition task first wanted for closing what a command opened, and it makes `cli` import `graph` and `lifecycle`. |
| Input checks before anything is built | go-cli-sdk | `cli.Command.Validate`, whose error is always a usage error. |
| A help footer | go-cli-sdk | `cli.Command.Footer`, which carries slab's help-plus-scenario-listing convention without a custom help template. |
