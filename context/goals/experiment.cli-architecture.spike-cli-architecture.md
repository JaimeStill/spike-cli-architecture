# goal · experiment.cli-architecture.spike-cli-architecture

- **State:** building
- **Task:** composition
- **Branch:** composition

## Tasks

1. [x] dispatcher
2. [ ] composition
3. [ ] files
4. [ ] validate

## Task brief · composition

**Problem.** A cobra CLI makes every command pay for the whole stack. The spike must show two
things. First, each command declares its dependencies, and one central initializer brings up
only those and closes them correctly. Second, the initializer sits on a small reverse-order
stack that go-core's lifecycle.Coordinator could also be built on, so a CLI and a service share
one primitive. The schema commands are the first real consumers, run against a local Postgres
and Azurite stack. This is evidence 3, 4 and 5.

### Behaviors

The stack:
1. It starts phases on demand, running each phase's steps concurrently. The first failure
   cancels the rest of its phase, and the cancellations that follow are dropped. Errors are
   labelled with the step name and joined.
2. It unwinds only the steps that started, last phase first. The unwind runs under one context
   derived from Background and bounded by the given timeout. On overrun it adds one deadline
   error, still attempts the remaining phases, and leaves the stack empty.
3. Its package imports only the standard library. The package documentation records the
   phase-by-phase mapping of lifecycle.Coordinator onto the Stack, including the requirement
   that the run context is cancelled before the drain. It also records the anticipated criteria
   for promoting the package to go-core.

Commands and their dependencies:

4. Help, `--help`, a parent run alone, an unknown command, a usage error, and `version` open
   nothing and read no dependency configuration. They succeed with the stack down and the
   environment empty.
5. Commands declare their dependencies where the composition root mounts them. The schema
   commands declare Postgres only and never open the object store.
6. A declared dependency opens at most once per run, the first time the command asks for it.
7. Dependencies close in reverse order after the command: on success, on a command error, and
   when the signal context is cancelled mid-command.
8. A dependency that fails to come up is reported once, as the command's error with the
   command's path, and the run exits 1. Whatever had already opened is closed, and the command
   body doesn't run.
9. A close error is reported once, joined with the command's result, and makes the exit 1.

The schema commands:

10. `schema status` prints one row per migration set: name, table, version, latest, pending,
    dirty. blobfs's set comes first, then the app's.
11. `schema up` applies every pending migration, bottom set first.
12. `schema down` reverts every set, top set first, and keeps the history tables.
13. `schema reset --yes` reverts everything and drops the history tables. `schema reset`
    without `--yes` is refused before anything opens.

Configuration and the local stack:

14. Configuration comes only from the environment: `BLOBFS_DATABASE_*` and `BLOBFS_STORAGE_*`,
    read when a dependency first opens. There is no `--dsn` flag.
15. `mise run up` starts Postgres and Azurite on ports no other workspace stack uses and waits
    until both are healthy. `mise run integration` runs the integration tests against that
    stack.

### Test seams
- The Stack's exported API, tested black-box with recording steps.
- `App.Run(ctx, args)` over buffers. A test-only hook swaps the real openers for recording
  fakes. Integration tests drive the same `Run` against the compose stack, with it up and with
  it down.

### Slices (nothing trails, so there is no upgrade slice)
1. **The Stack.** A root-level `lifecycle` package on the standard library only, held there by
   the import check. Its documentation carries the Coordinator mapping and the promotion
   criteria. Demo: black-box tests for behaviors 1–3.
2. **Declarations and the initializer over the Stack, against fakes.** Demo: hermetic
   `App.Run` tests for behaviors 4 and 6–9.
3. **The compose stack and the Postgres opener.** Postgres opens through go-database from the
   environment. The mise up, down, reset and integration tasks are added. Demo: an integration
   bring-up and close, and one failure message when the stack is down (behaviors 8, 14, 15).
4. **The schema commands.** Both migration sets run over sqlate's migrator, with the output
   table. Demo: `schema up`, `status`, `down` and `reset --yes` against the stack. Hermetic
   tests show that schema opens Postgres only and that `reset` without `--yes` opens nothing
   (behaviors 5, 10–13).
5. **The object store opener.** It uses go-storage/azureblob, configured from
   `BLOBFS_STORAGE_*`. Demo: an integration bring-up of both dependencies, closed in reverse
   order.

### Out of scope
- Any change to go-core or the other references.
- A test-only Coordinator.
- Changes to `cli`.
- The files and bookmark commands, `--variant`, and any command that uses the object store.
- Black-box tests over the built binary, and scenarios.
- The evidence-6 record and the written answer, which belong to the validate task.

**Door.** Two-way. The spike publishes nothing and writes only its own repository. The compose
volumes are local, and `mise run reset` removes them.

## Progress

slices 2/5 committed · standards — · spec — · editor —

## Decisions

- setup: evidence is the eight items in `context/README.md`, "The evidence".
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
  is unchanged.
- composition: bring-up and teardown run on a small reverse-order Stack, not go-core's
  lifecycle.Coordinator, whose Run blocks until a signal, starts a stage's services
  concurrently, and wraps errors "startup:".
- composition: the Stack lives in a root-level `lifecycle` package, stdlib only, shaped as the
  go-core candidate; rejected the usual `sdk` package for this case (architect). Its package
  documentation records the Coordinator-on-Stack mapping and the promotion criteria; no
  test-only Coordinator is built.
- composition: every library promotion the experiment identifies runs once the experiment
  completes and before `cli` builds, so affected infrastructure adapts first (architect).
- composition: Postgres opens through go-database/postgres, whose Start pings; go-database
  joins the spike's reading list.
- composition: configuration comes from the environment only (`BLOBFS_DATABASE_*`,
  `BLOBFS_STORAGE_*`), read at first open; `--dsn` is dropped, a deliberate difference from
  spike-blobfs.
- composition: the schema surface carries blobfs's migration set and the app's own set now,
  over sqlate's migrate.Migrator in place of spike-blobfs's lib/migrator.
- composition: the hermetic proof swaps the real openers through a test-only hook in
  internal/app.
- composition: defaults: Unwind runs with a 10s timeout; one dependency per Start call;
  a signal-cancelled run exits 1; compose uses postgres:18.6-alpine on 5436 and azurite:3.37.0
  on 10010, both overridable.

## Pending edits

- architecture · `standards/go-elemental/principles/topology-and-naming.md`: state that the
  rule "cmd/* imports only internal/*" covers the module's own packages. `cmd/*` may import
  go-core for process setup (as `principles/composition-root.md` has the entrypoint trap
  signals), but no package of its own module outside `internal/`.
- coordinator · `context/roadmap.toml`: add a go-core goal, "lifecycle: promote the
  reverse-order Stack and rebuild Coordinator on it", to `planned` ahead of `cli`, citing the
  spike's answer.
- coordinator · `context/cli-applications.md`: every library promotion the experiment
  identifies runs once the experiment completes and before `cli` builds.
