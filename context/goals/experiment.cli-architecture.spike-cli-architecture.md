# goal · experiment.cli-architecture.spike-cli-architecture

- **State:** brief ready
- **Task:** dispatcher
- **Branch:** dispatcher

## Tasks

1. [ ] dispatcher
2. [ ] composition
3. [ ] files
4. [ ] validate

## Task brief · dispatcher

Problem       Cobra goes. A dispatcher on the standard library's `flag` must cover go-cli-sdk's
              planned feature set, import only the standard library and go-core, and carry the
              spike's entrypoint and composition root with one dependency-free command.
              Evidence 1.
Behaviors     1. `blobfs version` prints the module version to stdout and exits 0.
              2. A parent run with no subcommand prints its generated help (usage line,
                 subcommands with summaries, own and inherited flags) to stdout and exits 2.
              3. `--help`/`-h` on any command prints that command's help to stdout, exit 2.
              4. An unknown subcommand writes "unknown command" and the parent's usage to
                 stderr and exits 2.
              5. Flags after positionals parse as flags before them; after `--`, everything is
                 positional.
              6. A root flag is accepted at any depth and binds the same value.
              7. The root pre-run hook runs once, after parsing and before the leaf; its error
                 stops the run.
              8. NoArgs and ExactArgs(n) reject a wrong count with a usage error (exit 2).
              9. A missing required flag, or two flags of one mutually exclusive group, is a
                 usage error naming the flags (exit 2).
              10. A command can ask whether a flag was set explicitly rather than defaulted.
              11. A repeatable string flag collects every occurrence, in order.
              12. An unknown flag or malformed value is a usage error (exit 2); a command's own
                  error is reported once on stderr (exit 1).
              13. The check fails when the dispatcher imports anything outside the standard
                  library and go-core.
Test seams    The composition root's New/Run over buffers with explicit args; the feature-set
              tests mount test-only commands on the dispatcher's exported API.
Slices        1. Walking skeleton (behaviors 1-4, 12, 13): tree, dispatch, help, exit mapping,
                 entrypoint, New/Run with `version`, the depguard rule.
              2. Flag semantics (5, 6, 8, 10, 11): interleaved parse, inherited root flags,
                 NoArgs/ExactArgs, set-flag query, repeatable flag.
              3. Flag groups and the pre-run hook (7, 9).
Out of scope  Shorthand flags, completion, Config, per-command dependencies and the
              initializer, schema/files commands, scenarios, the evidence-6 record, env
              fallbacks.
Door          two-way (nothing published or tagged; the spike is archived after intake)

## Progress

slices 3/3 committed · standards ✓ · spec ✓ · editor ✓

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

## Pending edits

- architecture · reconcile `standards/go-elemental/principles/topology-and-naming.md` ("cmd/*
  imports only internal/*") with `principles/composition-root.md` (the entrypoint traps
  signals, which cmd/blobfs does with go-core's `process.SignalContext`): state whether the
  topology rule covers only the module's own packages.
