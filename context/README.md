# spike-cli-architecture

spike-cli-architecture tests whether a command-line application in the Elemental layout can drop
cobra for a small dispatcher on the standard library's `flag` and bring up only the dependencies
each command declares, by rebuilding spike-blobfs's CLI on published blobfs, go-storage and
Postgres. Its answer is evidence for standards-lab's experiment.cli-architecture intake, which
decides go-cli-sdk's API and go-cli-sdk-template's composition root; the spike decides nothing
itself.

The spike is the sub-goal `experiment.cli-architecture.spike-cli-architecture` in the
coordinator's roadmap, `standards-lab/context/roadmap.toml`, where its tasks live. Its answer
lands in `standards-lab/context/cli-applications.md`.

## The question

Can a dispatcher on the standard library's `flag`, with the planned go-cli-sdk feature set and a
per-command dependency initializer, hold up in a real CLI over `blobfs`, go-storage, and
Postgres?

## The evidence

1. A dispatcher on the standard library's `flag` covers go-cli-sdk's whole planned feature set:
   flags after positionals, root flags at any depth, a root pre-run hook, NoArgs and ExactArgs,
   required and mutually exclusive flag groups, a set-flag query, a repeatable string flag,
   generated help (a parent run alone prints it; an unknown subcommand is a usage error), and a
   Run that exits with go-core's ExitUsage (2) on a usage error. It imports only the standard
   library and go-core.
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

- **Dispatcher** (package `cli`): flag parsing, the command tree, help, and usage exits.
- **Lifecycle** (package `lifecycle`): the reverse-order Stack, which starts dependencies in
  phases and unwinds them last first; it imports only the standard library and is a go-core
  promotion candidate.
- **Composition root** (package `internal/app`): New/Run, per-command-group dependency
  declaration, and the initializer, which opens a declared dependency on the first request and
  closes it when the command returns.
- **Infrastructure**: Postgres through go-database with sqlate migrations, the object store
  through go-storage/azureblob, and the compose stack of Postgres on 5436 and Azurite on 10010.
- **Domain and admin commands**: schema (package `admin/schema`, with status, up, down, and
  reset); files and bookmarks are planned.
- **Output and scenarios**: package `output` renders a command's result as a line or a table;
  scenarios are planned.
- **Tests**: buffer-driven app tests, fakes, and black-box integration over the built binary.

## References

The spike reads these repositories and never writes to them. Each is a key in the coordinator's
`references.toml`, with its local checkout in `references.local.toml`: `org`, `architecture`,
`go-core`, `go-database`, `sqlate`, `go-storage`, `blobfs`, `go-web-service` (slab, the layout's
other example), and `spike-blobfs` (the CLI being rebuilt).

## The answer

(written by the validate task)
