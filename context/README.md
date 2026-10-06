# spike-cli-architecture

spike-cli-architecture tests whether a command-line application in the Elemental layout can drop
cobra for a small dispatcher on the standard library's `flag` and bring up only the dependencies
each command declares, by rebuilding spike-blobfs's CLI on published blobfs, go-storage and
Postgres. It also builds the composition primitive those dependencies come from: a typed
dependency graph and a Coordinator that runs any subset built from it, both go-core candidates.
Its answer is evidence for standards-lab's experiment.cli-architecture intake, which decides
go-cli-sdk's API and go-cli-sdk-template's composition root; the spike decides nothing itself.

The spike is the sub-goal `experiment.cli-architecture.spike-cli-architecture` in the
coordinator's roadmap, `standards-lab/context/roadmap.toml`, where its tasks live. Its answer
lands in `standards-lab/context/cli-applications.md`.

## The question

Can a dispatcher on the standard library's `flag`, with the planned go-cli-sdk feature set and
commands that declare the dependencies they use, hold up in a real CLI over `blobfs`,
go-storage, and Postgres? Can the dependency graph behind it also express go-web-service's
staged composition?

## The evidence

1. A dispatcher on the standard library's `flag` covers go-cli-sdk's whole planned feature set:
   flags after positionals, root flags at any depth, a root pre-run hook, NoArgs and ExactArgs,
   required and mutually exclusive flag groups, a set-flag query, a repeatable string flag,
   generated help (a parent run alone prints it; an unknown subcommand is a usage error), and a
   Run that exits with go-core's ExitUsage (2) on a usage error. It imports only the standard
   library, go-core, and the spike's `graph` and `lifecycle` packages.
2. A CLI with no cobra in its module graph reproduces spike-blobfs's whole command surface over
   published blobfs, go-storage/azureblob, and Postgres.
3. Each command opens only the dependencies it declares: help, usage errors, and
   dependency-free commands work with the stack down; schema commands open Postgres only; file
   commands open Postgres and the object store.
4. Each declared dependency comes up once per run, and dependencies close in reverse order on
   success, on error, and on signal cancellation.
5. A dependency that fails to come up is reported once as the command's error and exits
   non-zero, and whatever had already opened is closed.
6. A written record of each cobra-specific convention in the current layout (silenced errors,
   parent RunE printing help, layers reading Config at run time, PersistentPreRunE flag checks)
   and what replaced it or why it is no longer needed.
7. The layout's test conventions hold without cobra: `internal/app` tests run the command tree
   over buffers, and domain tests run over go-storage's `storagetest.Fake` with no network.
8. A scenario package with `list` mounts beside the direct commands and declares its
   dependencies like any other command.

## Capabilities

- **Dispatcher** (package `cli`): flag parsing, the command tree, help, and usage exits. A
  command's `Uses` names the graph nodes it needs, inherited along its path; `cli.WithGraph`
  gives `cli.Run` the graph to build them from, and the body reads each value through
  `Invocation.System`.
- **Dependency graph** (package `graph`): a typed graph whose nodes are defined inertly and
  whose Build constructs only what its roots reach through `Use`, into a System of computed
  layers. It imports only the standard library and is a go-core promotion candidate.
- **Lifecycle** (package `lifecycle`): a Coordinator over a built System. `Exec` starts the
  System's subsystems layer by layer, runs a function, and shuts them down in reverse; `Run`
  serves until its context ends instead. A value takes part by implementing `Subsystem`, and
  `Config` carries the shutdown timeout. It imports the standard library, go-core, and `graph`,
  and is a go-core promotion candidate.
- **Composition root** (package `internal/app`): New/Run and the graph nodes for configuration,
  the database, the object store, and the schema migrator. It has no initializer: each command
  names its nodes in `Uses`, and the dispatcher builds and runs them.
- **Infrastructure**: Postgres through go-database with sqlate migrations, the object store
  through go-storage/azureblob, and the compose stack of Postgres on 5436 and Azurite on 10010.
- **Domain and admin commands**: schema (package `admin/schema`, with status, up, down, and
  reset); files and bookmarks are planned.
- **Output and scenarios**: package `output` renders a command's result as a line or a table;
  scenarios are planned.
- **Tests**: buffer-driven app tests, fakes, and integration tests over the compose stack;
  black-box tests over the built binary are planned.

## References

The spike reads these repositories and never writes to them. Each is a key in the coordinator's
`references.toml`, with its local checkout in `references.local.toml`: `org`, `architecture`,
`go-core`, `go-database`, `sqlate`, `go-storage`, `blobfs`, `go-web-service` (slab, the layout's
other example), and `spike-blobfs` (the CLI being rebuilt).

## The answer

(written by the validate task)
