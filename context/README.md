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
   dependency-free commands work with the stack down; schema, directory, and bookmark commands
   open Postgres only; object commands open Postgres and the object store.
4. Each declared dependency comes up once per run, and dependencies close in reverse order on
   success, on error, and on signal cancellation.
5. A dependency that fails to come up is reported once as the command's error and exits
   non-zero, and whatever had already opened is closed.
6. A written record of each cobra-specific convention in the current layout (silenced errors,
   parent RunE printing help, layers reading Config at run time, PersistentPreRunE flag checks)
   and what replaced it or why it is no longer needed.
7. The layout's test conventions hold without cobra: `internal/app` tests run the command tree
   over buffers, and domain tests run over go-storage's `storagetest.Fake` with no network.
8. A scenario package mounts beside the direct commands and declares its dependencies like
   any other command.
9. The dependency graph expresses go-web-service's stage table, and the CLI and a service run
   on one Coordinator.

## Capabilities

- **Dispatcher** (package `cli`): flag parsing, the command tree, help, and usage exits. `cli.Run`
  takes the process's stdin, stdout, and stderr as one `cli.Streams`. A command's `Use` method
  declares the graph nodes it needs, inherited along its path; `cli.WithGraph` gives `cli.Run` the
  graph to build them from, and the body reads each declared value with `inv.Get`, which panics
  on an undeclared node. A leaf's `Validate` checks its arguments and flag values before anything
  is built, and any error it returns is a usage error. A command's `Footer` appends its own text
  to its generated help.
- **Dependency graph** (package `graph`): a typed graph whose nodes are defined inertly and
  whose Build constructs only what its roots reach through `Scope.Use`, into a System of
  computed layers. A `Ref` names any node without its type, through `Ref.Name`, and
  `Graph.Observe` reports each node a Build begins constructing. A node takes part in a
  lifecycle only through its value's methods; the graph registers no hooks. It imports only the
  standard library and is a go-core promotion candidate.
- **Lifecycle** (package `lifecycle`): a Coordinator over a built System. `Exec` starts the
  System's values layer by layer, runs a function, and shuts them down in reverse; `Run` serves
  until its context ends instead. A value starts if it implements `Starter` and shuts down if it
  implements `Stopper`; `Subsystem` embeds both. `Config` carries the shutdown timeout. It
  imports the standard library, go-core, and `graph`, and is a go-core promotion candidate.
- **Composition root** (package `internal/app`): New/Run, and the graph nodes for configuration,
  the database, the `sql` node (the database's pool in sqlate's Postgres dialect), the object
  store, the schema migrator, and the files domain's `files` and `storage` nodes. One exported
  `Nodes` value describes them: each layer file fills its own part and mounts its own commands
  with one `root.Add(pkg.Commands(...)...)`. It has no initializer: each command declares its
  nodes with `Use`, and the dispatcher builds and runs them. The App publishes its graph, its
  nodes, and its root command (`Graph`, `Nodes`, `Root`) for tests to observe and replace.
- **Infrastructure**: Postgres through go-database with sqlate migrations, the object store
  through go-storage/azureblob, and the compose stack of Postgres and Azurite: the development
  project on 5436 and 10010 (`mise run up`), and the integration project on 5437 and 10011.
- **Domain and admin commands** (evidence 2): package `admin/schema` runs status, up, down, and
  reset on sqlate's `migrate.Migrator`, which is the migrator node's value. Package
  `domain/files` rebuilds spike-blobfs's files surface on published blobfs. Its `files` node, a
  `files.Service` over the `sql` node, serves the directory commands (mkdir, ls, stat, mv,
  rmdir) and bookmark add, ls, and rm. Its `storage` node, a `files.Storage` over `files` and
  the object store, serves put, cat, cp, rm, and rm --recursive. Each operation takes a
  `files.Ref`, a path or an id, so a command accepts `id:<uuid>` wherever an id can name the
  target.
- **Output and scenarios** (evidence 8): package `output` provides two layouts, `Table` and
  `Record`; each domain package renders its own results over them in its `output.go`. Package
  `scenario` holds the runner and the `directories` and `files` tours, mounted under
  `blobfs scenario`; each tour's command declares its nodes with `Use`. `blobfs scenario` alone
  prints its help ending with the listing of the tours, and the root's help ends with the same
  listing.
- **Tests** (evidence 7): buffer-driven app tests over package `internal/apptest`, whose fixtures
  replace nodes in the App's graph and record what a run builds; domain tests over sqltest and
  `storagetest.Fake`; and, under `mise run integration`, on an isolated compose project it boots
  and tears down, integration tests and a black-box suite (package `integration`) that runs the
  built binary as a child process. The suite reaches every state through production surfaces: it
  kills a `put -` with SIGKILL to leave a pending row, and interrupts rm --recursive through an
  HTTP relay in front of the object store that answers 503 to every blob delete after a set
  number.

## References

The spike reads these repositories and never writes to them. Each is a key in the coordinator's
`references.toml`, with its local checkout in `references.local.toml`: `org`, `architecture`,
`go-core`, `go-database`, `sqlate`, `go-storage`, `blobfs`, `go-web-service` (slab, the layout's
other example), and `spike-blobfs` (the CLI being rebuilt).

## The answer

**Question:** Do a stdlib-`flag` dispatcher with the planned feature set and a per-command
dependency initializer hold up in a real CLI over the `blobfs` library, go-storage, and Postgres?

**Answer:** Yes; a dispatcher on the standard library's `flag` carries spike-blobfs's whole
command surface, each command brings up only the graph nodes it declares, and the same
Coordinator runs go-web-service's staged graph.

1. The planned feature set: proven by the running binary. Root flags, PreRun, and Exclusive
   have no user in blobfs, so only `cli`'s tests prove them.
2. No cobra, spike-blobfs's whole surface: the integration suite over the built binary
   (`TestScript`); `go mod graph` shows no cobra or pflag.
3. Only declared nodes come up: help, version, and a usage error run with the stack down, and
   `internal/app`'s build-recording tests, such as
   `TestSchema_VerbsBuildTheDatabaseAndNeverTheStore` and
   `TestObjects_CommandsBuildTheDatabaseAndTheStore`.
4. Once per run, closed in reverse: `TestUse_NodeNamedTwiceIsBuiltOnce`,
   `TestInfrastructureIntegration_StartsBothAndShutsDownInReverse`, and, on cancellation,
   `TestUse_ShutsDownWhenTheContextEndsMidBody`. The integration test `TestAnInterruptedPut`
   proves a signal through the built binary ends the run with a single report.
5. A failed dependency reported once: `TestUse_FailuresReportedOnce`,
   `TestInfrastructureIntegration_StoreUnreachableClosesTheDatabase`, and the integration test
   `TestTheStoreUnreachable`.
6. The cobra record, [cobra-conventions.md](cobra-conventions.md): checked against both
   binaries' help and exit codes.
7. The test conventions without cobra: proven only by tests, `internal/app`'s over buffers and
   `domain/files`'s over `storagetest.Fake`.
8. A scenario package that declares its nodes: both scenarios run twice against the development
   stack, and the integration test `TestScenarios`.
9. The stage table and one Coordinator: proven only by tests, `TestWebServiceStageOrder` and
   `TestWebServiceSubsetBuild` (graph) and `TestRunServesAGraphShapedLikeTheWebService`
   (lifecycle).

**go-cli-sdk.** The `cli` package's exported API is go-cli-sdk's proven candidate. It adds to
the planned feature set `Use` with `WithGraph`, so a command brings up only what it declares;
`Invocation.Get`, to read a declared value; `Validate`, so bad input builds nothing; `Footer`,
for slab's help-plus-listing convention; and `Streams`, since `put -` reads stdin. It departs
from the plan three times: requested help exits 2 through go-core's `process.Usage`; there is
no `help` or `completion` command; and cobra's `Long` has no counterpart. Once `graph` and
`lifecycle` are promoted, the SDK imports only the standard library and go-core. The intake
decides.

**go-cli-sdk-template.** `internal/app`'s shape is the candidate composition root. It departs
from `cli-applications.md`'s layout: one exported `Nodes` value describes the graph; each layer
file fills its part of it, mounts its commands with one `root.Add(pkg.Commands(...)...)`, or
both; configuration is graph nodes, so there is no `config.go`; there is no `commands.go` and no
central initializer; fixtures live in an internal `apptest` package; and `main` passes
`cli.Streams` and the arguments, so it imports `cli` beside `internal/app`.

**Promotion.** Promote `graph` and `lifecycle` into go-core at the API as it stands after this
task. The files review reshaped both: participation split into the single-method `Starter` and
`Stopper`, which `Subsystem` now embeds; `graph` lost its `Scope` hooks and gained
`Ref.Name`, and kept `Observe` from the files build. The reshaping came from review rulings,
not failures.

See [cobra-conventions.md](cobra-conventions.md) for the cobra record and
[USAGE.md](../USAGE.md) for every command with its output.
