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

**Problem.** Our infrastructure assumes every application describes, constructs, and starts all
its dependencies once, in one place. A CLI needs a different subset per command, and fighting
that assumption produced brittle code. The spike must show three things:
- a typed dependency graph that describes any dependency inertly, with no reflection;
- that building any subset of it gives a System that one Coordinator runs layer by layer;
- that a dispatcher whose commands declare what they use brings up only that, transparently to
  the command body.

The same primitive must also express go-web-service's staged composition. This covers evidence
3, 4, and 5, and it is the candidate for go-core.

### Behaviors

The graph:

1. Defining a node constructs and starts nothing. `Define` returns a typed `Node[T]`. A node can
   be any value, with or without a lifecycle.
2. `Build(roots...)` constructs only what the roots reach through `Use`, cold. A node reached by
   several dependents is constructed once and shared.
3. Layers are computed from the discovered dependencies by longest-path layering, so a node sits
   one layer above its deepest dependency.
   - `After(n)` orders a node after `n` without passing a value.
   - `After` pulls nothing into the System on its own: it orders only nodes that `Use` brought
     in.
4. Wiring mistakes panic, and a constructor error fails `Build` with the node's name, after
   which nothing in that Build starts. The wiring mistakes are:
   - a dependency cycle;
   - `Use` of a node from another graph;
   - `Use` outside a running constructor;
   - `Replace` after the graph has built.
5. `Replace(n, ctor)` swaps a node's constructor before `Build`, and the substitute goes through
   the same build and lifecycle as the original.
6. A black-box test expresses go-web-service's stage table:
   - the infrastructure nodes;
   - the schema stage, which uses the database;
   - the reactors, which run `After` the schema;
   - the server at the root.

   The computed layers reproduce the table's order.

The lifecycle:

7. `New(sys, cfg)` with `Exec(ctx, fn)` does three things:
   - starts the System's subsystems layer by layer, lowest first, concurrently within a layer;
   - runs `fn`;
   - shuts the subsystems down in reverse.

   `Run(ctx)` does the same, but serves until `ctx` ends.
8. A value is a subsystem when it implements `Start` and `Shutdown`. `OnStart` and `OnShutdown`
   hooks override that. Values without either take no part in the lifecycle.
9. On the first start failure, the rest of that layer is cancelled, and no higher layer starts.
   Every subsystem that started is shut down. So is the one whose Start failed, so it leaks
   nothing. The start error is reported once.
10. Shutdown runs every started subsystem, last layer first. It uses a context detached from
    cancellation and bounded by `ShutdownTimeout`, so it also runs on success, on error, and on
    signal cancellation. A shutdown error is joined with the run's result and fails it.
11. The package documentation:
    - maps go-core's Coordinator onto the new shape (`Add` and stages give way to a System; `Run`
      and the readiness parts stay);
    - names the break at promotion;
    - states the promotion criteria.

The dispatcher:

12. `Uses` on a command, inherited as the union along its path, names the nodes it needs. The
    dispatcher's order is Args, required flags, exclusive groups, PreRun, then Build of the
    path's `Uses` plus the lifecycle config node, then `Exec` around Run.
13. Help, `--help`, a parent run alone, an unknown command, usage errors, and `version`:
    - build nothing and read no configuration;
    - succeed under the dispatcher's exit conventions with the stack down and the environment
      empty.
14. Command bodies read typed values through `inv.System.Get(node)`. A Build, start, or shutdown
    failure is reported once under the command's path and exits 1. A `Uses` without
    `cli.WithGraph` panics at the start of dispatch. A CLI with no `Uses` calls `cli.Run` as
    before.

The blobfs composition:

15. The composition root defines these nodes, and has no dependency enum, opener struct, or
    per-dependency accessor:
    - configuration: database, storage, and lifecycle;
    - the database;
    - the object store;
    - the schema migrator.
16. The schema group `Uses` the migrator only, so its verbs open Postgres and never the object
    store. This is proven hermetically: with an invalid storage configuration, schema still
    fails only on Postgres.
17. The schema verbs, configuration, and compose stack behave as the first build proved:
    - status, up, down, and `reset --yes`, with `reset` refused before anything is built;
    - configuration from `BLOBFS_DATABASE_*` and `BLOBFS_STORAGE_*` only, with no `--dsn`;
    - Postgres on 5436 and Azurite on 10010.
18. Against the real stack, a command using Postgres and the store starts both and shuts them
    down in reverse. With either one unreachable, it reports once, exits 1, and closes whatever
    had started.

### Test seams

- `graph` and `lifecycle` exported APIs, black-box, with fake values.
- `cli.Run` over buffers, with a test graph.
- `App.Run(ctx, args)` over buffers. Its graph is reached through an export_test probe for
  `Replace`, and the testpackage comment returns to "clock or probe hook".
- Integration over the compose stack, with it up and with it down.

### Slices

Nothing trails, so there is no upgrade slice.

1. **graph.** The package, on the standard library only and held there by depguard. Black-box
   tests cover behaviors 1–6, including the go-web-service layering test.
2. **lifecycle.** `Subsystem`, `Config`, and `Coordinator`, with `New`, `Exec`, and `Run`, over a
   `graph.System`. The Stack's engine becomes internal. The documentation carries the go-core
   mapping and the promotion criteria. Behaviors 7–11.
3. **cli.** `Uses`, `Invocation.System`, `WithGraph`, and the extended dispatch. The cli depguard
   rule admits `graph` and `lifecycle`. Behaviors 12–14.
4. **Composition root on the graph.** It replaces `deps.go`, the opener hook, and the probe
   groups, and rewires schema, the hermetic tests, and the integration tests. Behaviors 15–18.

### Out of scope

- Any change to go-core, go-web-service, or the other references. Promotion follows the
  experiment.
- Readiness, `OnReady`, and `Monitor` on the spike's Coordinator. They are mapped in its
  documentation, not built.
- Per-node start scheduling. The layer barrier stands.
- The files and bookmark commands, scenarios, and black-box tests over the built binary. These
  belong to the files task.
- The evidence-6 record and the written answer. These belong to the validate task.

### Door

Two-way. The spike publishes nothing and writes only its own repository.

## Progress

redirected at the session brief; slices 4/4 of the new brief · standards ✓ · spec ✓ · editor ✓

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
- composition: the architect's commit: `version` reports v0.0.0 when the binary has no build
  info, and internal/app drops the `godatabase` and `sqlatepostgres` import aliases.

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
