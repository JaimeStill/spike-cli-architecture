# goal · experiment.cli-architecture.spike-cli-architecture

- **State:** building
- **Task:** files
- **Branch:** files

## Tasks

1. [x] dispatcher
2. [x] composition
3. [ ] files
4. [ ] validate

## Task brief · files

```
Problem       blobfs serves only schema and version. Evidence 2, 7, and 8 need
              spike-blobfs's files and bookmark surface rebuilt on published
              blobfs, go-storage/azureblob, and Postgres. Each command declares
              only the dependencies it uses. The work is tested in the layout's
              tiers and joined by a scenario package whose commands declare
              their dependencies like any command. Without it, the experiment
              can't show the dispatcher and the graph holding up in a real CLI.
Behaviors     1. mkdir, ls, stat, mv, and rmdir behave as spike-blobfs's do:
                 absolute paths and id:<uuid> refs, cp/mv taking two paths or
                 two ids. ls has --page, --size, repeatable --sort and --filter,
                 --total exact|none, --after-dirs/--after-files, --cursors, and
                 --unit; mkdir has --unit. Each declares Postgres alone and
                 succeeds with the object store unreachable.
              2. bookmark add, ls, and rm behave as spike-blobfs's do, each
                 with a required --unit, add with --active, and at most one
                 active bookmark per unit. Each declares Postgres alone.
              3. put uploads a local file, or stdin for `-`, to a path or into
                 a directory id. Its content type comes from --content-type,
                 then the file's extension, then application/octet-stream. A
                 put onto a pending row resumes it. A name an available file
                 holds is refused.
              4. cat streams an available file's content to stdout. cp copies
                 an available file to a new path or into a directory, refusing
                 a name already taken. rm deletes a file, and refuses a file a
                 unit has bookmarked before touching anything.
              5. rm --recursive takes a path, not an id. It marks the branch
                 deleting, sweeps until no work remains, and prints the files
                 and directories removed as totals. A rerun after an
                 interruption finishes the branch. The root is refused.
              6. put, cat, cp, and rm declare Postgres and the object store. An
                 unreachable store fails them once, naming the store, with the
                 database shut down. The directory and bookmark commands are
                 unaffected.
              7. A files command run against a schema that isn't applied fails
                 at start, naming the files node, before its body runs.
              8. blobfs runs on blobfs's Postgres engine, fixed in the
                 composition root. It has no --dsn, --variant, or --fail-after,
                 and --recursive has no -r shorthand. No module in its graph is
                 cobra.
              9. The dispatcher hands a command stdin beside stdout and stderr.
                 cli.Run and app.New take it as a parameter, a command reads it
                 as Invocation.Stdin, and main passes the process's stdin.
              10. `blobfs list` prints each scenario's name, its summary, and
                  the names of the nodes it declares. It builds nothing and
                  succeeds with the stack down.
              11. `blobfs demo directories` declares Postgres alone, and
                  `blobfs demo files` declares Postgres and the store. Each
                  narrates every step before doing it, brings up only what it
                  declares, reports an unreachable dependency as the start
                  error naming its node, and succeeds twice in a row.
              12. Hermetic tier: app tests drive the command tree over
                  buffers; domain tests run over sqltest and storagetest.Fake
                  with no network.
              13. Integration tier: under `mise run integration`, on its
                  isolated compose project (5437, 10011), the built binary runs
                  as a child process through one ordered script over every
                  command, each test with its own throwaway database and
                  container. A directory command succeeds with the store
                  endpoint unreachable.
Test seams    the blobfs program's arguments, stdin, stdout, stderr, and exit
              code: App.Run over buffers in the hermetic tier, the built binary
              in the integration tier. The domain's own API over sqltest and
              storagetest.Fake.
Slices        1. Directory commands: mkdir, ls, stat, mv, rmdir over published
                 blobfs on its Postgres engine. blobfs v0.5.0 enters as a direct
                 requirement. Adds the files node over the database, the
                 statement check at start, ls's listing output, and the
                 black-box harness over the built binary. Demo: mkdir and ls
                 with Azurite stopped. (Behaviors 1, 7, 8, 12, 13)
              2. Object commands: stdin through the dispatcher; put, cat, cp,
                 rm, and rm --recursive over the store node. Demo: `put -`,
                 cat round-trips the bytes, then rm --recursive prints its
                 totals. (Behaviors 3–6, 9)
              3. Ownership and bookmarks: --unit on mkdir and ls, bookmark
                 add, ls, and rm, and rm's bookmark refusal. Demo: bookmark
                 add --active, then bookmark ls, then a refused rm.
                 (Behaviors 1, 2, 4)
              4. Scenarios: the scenario package, `list` at the root, and the
                 `directories` and `files` tours under `demo`. Demo: `blobfs
                 list` with the stack down, then `blobfs demo files` twice.
                 (Behaviors 10, 11)
              (No upgrade slice: `mise run currency` reports nothing.)
Out of scope  promoting graph, lifecycle, or cli to go-core or go-cli-sdk; the
              evidence-6 record and the README's answer (the validate task); a
              background or standalone sweep command; shorthand flags; the
              root's help listing the scenarios; a blobfs variant other than
              Postgres
Door          two-way: the spike repository only, no release
```

## Progress

slices 4/4 committed · standards ✓ · spec ✓ · editor ✓

## Decisions

- setup: evidence is the eight items in `context/README.md`, "The evidence". *Composition adds a
  ninth (architect): the graph expresses go-web-service's stage table, and the CLI and a
  service run on one Coordinator.*
- setup: kind code; vision and six-area capability map as in `context/README.md`.
- setup: `mise run check` is hermetic, copied from go-core; the dispatcher task adds the import
  check holding the dispatcher to the standard library and go-core. Postgres and Azurite tests
  run as `mise run integration`, cited as evidence in session briefs.
- setup: `mise run currency` uses go-core's `scripts/currency.sh`.
- setup: merge is plain `gh pr merge --merge --delete-branch`; no CI, since `gh pr checks` fails
  in a repository with no checks.
- setup: references live in the coordinator's `references.toml` and `references.local.toml`;
  the spike keeps no references files.
- setup: the spike reads org, architecture, go-core, sqlate, go-storage, blobfs, go-web-service,
  and spike-blobfs.
- setup: the path is dispatcher, composition, files, validate. The binary name, the dispatcher's
  module layout, and the compose ports are settled in the task briefs.

- dispatcher: the binary is `blobfs`, matching spike-blobfs, so later tasks compare the two
  command for command.
- dispatcher: the dispatcher is the root-level package `cli` in the spike module, shaped like
  go-cli-sdk; rejected internal/cli (hides it from the layout it models) and a nested module
  (needs a tag or go.work, against published-versions-only).
- dispatcher: the import check is a depguard rule in .golangci.yml; rejected a `go list -deps`
  script. Direct imports suffice because go-core requires nothing.
- dispatcher: requested help exits 2 through go-core's `process.Usage`, as go-core documents;
  spike-blobfs exits 0 on help and 1 on an unknown command (for the evidence-6 record).
- dispatcher: args reach the composition root as `Run(ctx, args)`; main passes `os.Args[1:]`.
- dispatcher: `version` is the dependency-free command.
- dispatcher: built on stdlib `flag` rather than adopting a CLI library; cobra, urfave/cli, kong
  and ff are third-party and ruled out by the plan in cli-applications.md.
- dispatcher: defaults: process.SignalContext in main; interleaved parse stopping at `--`;
  inherited root flags share one flag.Value; no shorthand flags; a usage-error type maps to 2,
  any other error to 1, reported once; pre-run hook root only; help to stdout, usage errors to
  stderr; wiring mistakes panic at build time.

- dispatcher: help prints the summary, Usage, Commands, Flags, and Global flags. An unknown
  subcommand prints the parent's full help to stderr. A flag or argument usage error prints the
  message, the usage line, and a `--help` pointer; flag error text keeps the standard library's
  single-dash wording.
- dispatcher: a command's error is reported as `process.Fail(stderr, "<command path>", err)`; a
  PreRun error is reported against the leaf.
- dispatcher: Run shares the root's flags into every descendant at its start, not in Add, so
  definition order doesn't matter; a nested parent's flags stay local. Wiring panics (a root flag
  redefined; Args, Require, or Exclusive on a parent; PreRun below the root; an undefined name in
  a group; a group of fewer than two) fire at the start of dispatch over the whole tree, not in
  Add. Add's own checks (subcommand names, duplicates, reparenting) still panic in Add.
- dispatcher: a custom Args validator's error is always a usage error (exit 2).
- dispatcher: Changed covers the leaf's flags and root flags set at any level, not a nested
  parent's local flags.
- dispatcher: a StringsVar flag's first command-line use replaces its default; values are not
  split on commas.
- dispatcher: a leaf dispatches in the order Args, required flags, exclusive groups, PreRun, Run.
  Missing required flags are reported together in declaration order; only the first broken
  exclusive group is reported.
- dispatcher: Require and Exclusive are leaf-only, with no persistent-required equivalent; a flag
  every leaf needs is required on each leaf.
- dispatcher: the depguard allow entries are anchored (`go-core$`, `go-core/`, `.../cli$`)
  because depguard matches by prefix; the rule covers cli's tests too.
- dispatcher: spike-blobfs's `rm -r` is a shorthand flag, which this dispatcher doesn't support,
  so the files task uses `--recursive`.
- dispatcher: `cmd/*` may import go-core for process setup (signal context, exit codes); within
  the module it imports only `internal/*` (architect, at the session brief).

- composition: dependencies are declared per command group in the composition root; `cli`
  is unchanged. *Superseded: dependencies are part of the command.*
- composition: bring-up and teardown run on a small reverse-order Stack, not go-core's
  lifecycle.Coordinator, whose Run blocks until a signal, starts a stage's services
  concurrently, and wraps errors "startup:". *Superseded: a Coordinator over a System; the
  Stack is its internal engine.*
- composition: the Stack lives in a root-level `lifecycle` package, stdlib only, shaped as the
  go-core candidate; rejected the usual `sdk` package for this case (architect). Its package
  documentation records the Coordinator-on-Stack mapping and the promotion criteria; no
  test-only Coordinator is built. *Superseded in part: the `lifecycle` package stands and imports
  go-core and graph too; it builds the Coordinator, and its documentation maps go-core's.*
- composition: every library promotion the experiment identifies runs once the experiment
  completes and before `cli` builds, so affected infrastructure adapts first (architect).
- composition: Postgres opens through go-database/postgres, whose Start pings; go-database
  joins the spike's reading list.
- composition: configuration comes from the environment only (`BLOBFS_DATABASE_*`,
  `BLOBFS_STORAGE_*`), read at first open; `--dsn` is dropped, a deliberate difference from
  spike-blobfs. *Superseded in part: configuration nodes read it when a Build reaches them.*
- composition: the schema surface carries blobfs's migration set and the app's own set now,
  over sqlate's migrate.Migrator in place of spike-blobfs's lib/migrator.
- composition: the hermetic proof swaps the real openers through a test-only hook in
  internal/app. *Superseded: tests swap nodes with Graph.Replace.*
- composition: defaults: Unwind runs with a 10s timeout; one dependency per Start call;
  a signal-cancelled run exits 1; compose uses postgres:18.6-alpine on 5436 and azurite:3.37.0
  on 10010, both overridable. *Superseded in part: the timeout is lifecycle.Config's, and a Start
  call takes a whole layer.*
- composition: a leaf's cli Run closes its dependencies through `deps.Run(body)`, since `cli` has
  no post-run hook; a request outside `deps.Run` panics, so nothing opens without being closed.
  A post-run hook is a go-cli-sdk candidate (for the validate task's evidence-6 record).
  *Superseded: the dispatcher runs the leaf under Exec.*
- composition: a request for an undeclared dependency panics as a wiring mistake.
  *Superseded: System.Get of a node not in the System panics.*
- composition: a dependency constructed but failing Start is shut down at once on a detached,
  bounded context; its Start error is the one report. *Superseded: shut down with its layer,
  in reverse.*
- composition: an unreachable-Postgres error is one report spanning several lines (pgx's
  per-attempt continuation lines), passed through unflattened.
- composition: behavior 4's "succeed" is read under the dispatcher decision (requested help
  exits 2, nothing opens); behavior 8's "the body doesn't run" is read under lazy open (the body
  stops at its first request). *Superseded: the brief was renumbered and the lazy open is gone.*
- composition: Stack: a single step runs on the caller's goroutine; a phase with nothing started
  is not pushed; after a deadline overrun Unwind starts the remaining phases without waiting, as
  go-core's drain does. *Superseded in part: the engine pushes every layer that began to start.*
- composition: schema: the app's set is named `app` (spike-blobfs: `consumer`); `--yes=false` is
  refused too, since cli's Require counts an explicit false as set; down reverts each set with
  `Layer.Down(len)`, which keeps the history tables.
- composition: the store uses container `cliarch`; Store.Start creates it; the mise env carries
  Azurite's published development key.

- composition: redirected at the session brief (architect): per-command dependency code was
  brittle and more complex than go-web-service's root. The cause is that go-core's lifecycle
  fuses description, construction, and lifecycle; the task now builds a general dependency
  graph and a Coordinator that runs any built subset. Supersedes the per-group mount, the
  lazy initializer, the opener hook, and the exported Stack decisions above.
- composition: no reflection; stdlib plus Go 1.27 generic methods. Rejected uber-go/dig and fx:
  reflection-based frameworks, out on the same no-frameworks line as cobra.
- composition: the dependency strategy is in this spike's scope regardless of CLIs (architect);
  promotion to go-core follows the experiment.
- composition: dependencies are discovered by constructors through Scope.Use, memoized per node,
  so a shared node is built once (architect asked; Build Systems à la Carte's dynamic
  dependencies with a suspending scheduler). Layers are computed by longest-path layering;
  rejected author-declared layer numbers and author-nested layers (a manual topological sort).
- composition: Scope.After is order-only and pulls nothing into a System (systemd's After=, not
  Make's order-only prerequisite).
- composition: a layer barrier on start; configuration is a node; lifecycle inferred from
  Start/Shutdown with Scope.OnStart/OnShutdown overrides; tests swap nodes with Graph.Replace.
- composition: names: package graph (Graph, Define, Node[T], Scope, Use, After, Build, System,
  Get, Replace, Ref); cli.Command.Uses, inherited as the union along the path;
  Invocation.System; cli.WithGraph(g, lifecycleConfig). *Superseded: `cli.Command.Uses` is now
  the method `Use`; see the last composition decision.*
- composition: lifecycle: one Coordinator, New(sys, cfg), Exec (one-shot, architect's name over
  Do) and Run (long-running); the participant interface is Subsystem, rejected Service
  (overloaded), Process (go-core's process package and the OS process), Component and Unit.
- composition: the shutdown timeout is configuration, lifecycle.Config{ShutdownTimeout}, default
  10s, <PREFIX>_SHUTDOWN_TIMEOUT, supplied as a graph node; "drain timeout" becomes "shutdown
  timeout".
- composition: dependencies are part of the command (architect), reopening dispatcher round 1's
  "cli unchanged"; cli may import the spike's graph and lifecycle, both go-core candidates.
- composition: a subsystem whose Start fails is still shut down, so it leaks nothing; loosens
  go-core's started-only Shutdown contract. Readiness, OnReady, and Monitor stay mapped in the
  documentation, not built.

- composition: a built node is a graph.Dependency (name, value, OnStart/OnShutdown hooks), returned
  by System.Layers() [][]Dependency; rejected Member (too generic), Instance, Built (architect).

- composition: a long-running node that nothing Uses (go-web-service's reactors) is its own
  Build root, and the runtime root orders after it with After; layers for the stage table come
  out config → database, store → domain, schema, storage → reactors → server.

- composition: cli's Option is `func(*options)`; WithGraph panics on a nil graph or node; cli
  leaves the lifecycle Config to its node's constructor to finalize.
- composition: Invocation.System is nil while PreRun runs, since PreRun precedes the Build.
- composition: a body's usage error stays a usage error (exit 2) when joined with a shutdown
  error.
- composition: a failing start layer shuts down every participant whose Start was called.
- composition: start errors carry the node's name and no "startup:" prefix.
- composition: Run treats any end of ctx, before, during, or after startup, as a clean stop.
- composition: shutdown's context derives from context.Background.
- composition: graph: a hook recorded twice panics; Define after Build is allowed; Replace is
  locked after the first Build; nodes within a layer keep definition order.
- composition: schema takes the migrator node itself, `schema.Commands(client)`, so one value
  drives both Uses and Get.
- composition: `reset --yes=false` is refused in Args, since the System starts before the body
  runs.
- composition: node names are "database config", "storage config", "lifecycle config",
  "database", "store", and "migrator"; they label errors.
- composition: the integration test orders the store `After` the database to assert reverse
  shutdown; in production the two are independent and share a layer.
- composition: a value constructed by a Build that then fails is not shut down: nothing started,
  and the process exits.
- composition: internal/app's non-test code went from 379 lines to 229.

- composition: redirected at the session brief (architect): `mise run integration` must boot
  its own isolated compose project on its own ports and tear it down with its volumes, as
  go-web-service's does and tests-and-docs.md states; the brief had followed spike-blobfs and
  run against the `mise run up` stack. Project spike-cli-architecture-integration on 5437 and
  10011; compose files drop container_name so the project scopes names.

- composition: `mise run check` also lints with `--build-tags integration`, so the integration
  suite always compiles, as go-web-service's check does (architect).

- composition: a command declares its dependencies with the variadic, chainable method
  `cmd.Use(refs...) *Command` (architect, at the brief), in the style of Require, Exclusive,
  and Add and with Scope.Use's verb; the exported `Uses` field goes and the list is unexported.
  Inheritance along the path and the dispatch order are unchanged.
- composition: `Use` with no refs declares nothing, as Require with no names requires nothing;
  an untyped nil ref panics in `Use`, and a typed nil `*graph.Node` panics in `graph.Build` when
  a dispatch runs the leaf.
- composition: no reflection in cli: `Use` catches only an untyped nil, so cli no longer imports
  reflect.
- composition: `graph.Build` checks every root, for nil or another Graph, before it constructs
  any, so a mistake in one root panics before another root's constructor runs.
- composition: the architect's commit drops internal/app's `godatabase` and `sqlatepostgres`
  import aliases. Its `version` fallback to v0.0.0 was reverted to "(devel)" (architect), since
  go run reports "(devel)" itself and the fallback fired only under go test.

- files: the object store is declared only by the commands that touch objects (put, cat, cp, rm, rm
  --recursive); mkdir, ls, stat, mv, rmdir, and bookmark declare Postgres alone, as spike-blobfs
  never opened the store for them.
- files: rm --recursive marks the branch deleting and sweeps it to completion in the same run,
  blobfs's own branch-delete protocol; it prints totals, not a line per row, and its sweep also
  finishes any branch an earlier run marked (a deliberate difference from spike-blobfs's walk).
- files: --fail-after is dropped from put, cp, and rm; the domain calls blobfs's WriteFile,
  EnsureFile, and RemoveFileID protocols, which blobfs's conformance suite proves; put still resumes
  a pending row (a deliberate difference). *Superseded in part: rm calls RemoveFile, not
  RemoveFileID.*
- files: --variant is dropped; the composition root fixes blobfs's Postgres engine, as
  go-web-service does, so the black-box script runs once (a deliberate difference, like --dsn).
- files: stdin is a parameter of cli.Run and app.New beside stdout and stderr, exposed as
  Invocation.Stdin, replacing cobra's InOrStdin; rejected a WithStdin option.
- files: two scenarios ship, `directories` (Postgres alone) and `files` (Postgres and the store), so
  the listing and a run show scenarios bringing up different subsets.
- files: scenarios mount as leaves under `demo`, with `list` at the root, slab's shape; rejected a
  `scenario` parent with `list` beside them.
- files: a scenario drops slab's Needs; its dependencies come only from Use, the Coordinator reports
  an unreachable one naming its node, and `list` prints each scenario's declared node names.

- files: bad input exits 2 as a usage error, checked before anything is built: a malformed ref,
  sort or filter term, `--total`, or `--unit`; a path mixed with an id; `ls id: --unit`; and
  `rm --recursive id:`. spike-blobfs exited 1.
- files: a half continued from a cursor prints "total not counted"; sort and filter terms reach the
  directory half only for spike-blobfs's set; a directory's new blobfs `status` field is not
  printed.
- files: mv keeps spike-blobfs's rule that a move stays under one top-level directory
  (ErrMoveAcrossScopes); a renamed top-level directory keeps its owner row.
- files: the domain is two graph nodes. `files`, a Store over the database, serves the directory
  and bookmark commands; its start checks blobfs's statements and the domain's own
  (domain/files/statements). `objects`, over `files` and the store, serves put, cat, cp, and rm.
- files: put looks the name up first and resumes a pending row through EnsureFile under that row's
  id, keeping its first content type; any other put calls WriteFile with Files.Create.
- files: cp calls WriteFile with Files.Create, so it refuses any taken name, a pending one included,
  and never resumes (spike-blobfs's cp resumed). Its source opens on the body's first read, so a
  refused name never reaches the store.
- files: rm by path and by id both call RemoveFile with a pick callback; RemoveFileID is unused.
- files: rm holds the file before counting its bookmarks, in the first transaction, so a bookmark
  added concurrently can't lose the object.
- files: rm --recursive counts the branch's bookmarks right after marking it, in the same
  transaction; any bookmark refuses the command and rolls the mark back, because blobfs's sweep
  deletes the object before the foreign key would refuse the row. spike-blobfs stopped partway.
- files: rm --recursive uses blobfs's default batch with no stale reclaim, and reports the last
  pass's error with the counts so far; owner rows are removed through the sweep's OnRemoveDirectory
  hook.
- files: bookmark add --active is refused while another of the unit's bookmarks is active, as in
  spike-blobfs.
- files: --unit is parsed with the standard library's uuid (Go 1.27), not blobfs.ParseID, which
  refuses the nil UUID. The owner listing (`ls / --unit`) pages by number only and hides deleting
  directories.
- files: bookmark ls always prints paths; spike-blobfs's path-less listing, its Paths option, and
  its Scope type are dropped.
- files: an error is labelled once, by the operation the caller ran; sentinels carry no `files:`
  prefix.
- files: scenarios: the runner is package `scenario` and the tours package `demo` (slab's split).
  scenario.Node is `graph.Ref` plus `Name()`, so one typed node drives Use, list's names, and
  System.Get. A step receives (ctx, *graph.System, *Reporter); a failed step reads
  `step N (intent): err`.
- files: `list` prints the nodes a scenario declares, not the transitive set, since finding
  dependencies needs construction and list builds nothing; `demo files` declares files and objects.
- files: the tours work in fixed areas (/demo-directories, /demo-files), clear leftovers first, and
  use a constant demo unit. They hold no bookmarks: a bookmark needs a file, and a file needs the
  store.
- files: each layer file in internal/app mounts its own commands; the demo renders through files'
  exported WriteContents, WriteFileRecord, and WriteDirectoryRecord.
- files: the integration suite runs the binary bounded by go-core's processtest.Failsafe, with its
  own runner, since processtest.Launch takes no args or stdin and merges stdout with stderr. Each
  test gets its own database and container. The unreachable-store test runs two object commands,
  since each waits about 9s on the Azure SDK's retries; the hermetic tests cover all of them.
- files: the integration tests seed a pending row and a deleting branch by SQL, the only way in
  since --fail-after was dropped. Open question for the architect: tests-and-docs.md wants
  production surfaces.
- files: spike-blobfs's root help appended the scenario listing; cli's generated help has no hook
  for it (for the evidence-6 record).
- files: stdin as a cli.Run parameter changes the go-cli-sdk candidate's signature (for the
  evidence-6 record).
- files: go-core's processtest doesn't fit a CLI (no args or stdin, merged streams), a go-core or
  go-cli-sdk candidate (for the evidence-6 record).
- files: internal/app's export_test.go exports production constructors (Nodes.Files,
  Nodes.Objects, NewFiles, NewObjects) for Graph.Replace, beyond the clock or probe hook
  tests-and-docs.md allows, continuing main's pattern. Open question for the architect.

## Pending edits

- architecture · `standards/go-elemental/principles/topology-and-naming.md`: state that the
  rule "cmd/* imports only internal/*" covers the module's own packages. `cmd/*` may import
  go-core for process setup (as `principles/composition-root.md` has the entrypoint trap
  signals), but no package of its own module outside `internal/`.
- coordinator · `context/roadmap.toml`: add a go-core goal, "go-core: the dependency graph,
  and lifecycle rebuilt as its executor (Coordinator over a System; Add and stages retired)", to
  `planned` ahead of `cli`, citing the spike's answer.
- coordinator · `context/roadmap.toml`: widen the spike's and the composition task's summaries
  to the general composition primitive: a dependency graph, a Coordinator over any built
  subset, and commands that declare what they use.
- coordinator · `context/cli-applications.md`: every library promotion the experiment
  identifies runs once the experiment completes and before `cli` builds.
- architecture · a definitions page for the composition ontology (graph, node, system, layer,
  subsystem, coordinator, Exec/Run; "stage" retired; "service" kept for the deployed
  application and the infrastructure tiers), with `standards/go-elemental/principles/
  lifecycle-and-context.md` and `principles/composition-root.md` updated to it.
