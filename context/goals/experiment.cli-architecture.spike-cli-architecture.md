# goal · experiment.cli-architecture.spike-cli-architecture

- **State:** idle
- **Task:** none
- **Branch:** none

## Tasks

1. [ ] dispatcher
2. [ ] composition
3. [ ] files
4. [ ] validate

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

## Pending edits

(none)
