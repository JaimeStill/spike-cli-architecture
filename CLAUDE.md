# spike-cli-architecture

A spike for the Standards Lab reference architecture. It tests whether a command-line application
in the Elemental layout can drop cobra for a dispatcher on the standard library's `flag` and
bring up only the dependencies each command declares. The repository is managed with the marathon
workflow; start from `context/README.md`.

- **Standards:** `STANDARDS.md`, with pointers into the architecture repository.
- **Check:** `mise run check` (hermetic). The Postgres and Azurite tests run as
  `mise run integration`.
- **Goals:** this spike's tasks live in the coordinator's roadmap,
  `standards-lab/context/roadmap.toml`, under `experiment.cli-architecture.spike-cli-architecture`.
  Its goal record is `context/goals/experiment.cli-architecture.spike-cli-architecture.md`.
- **Dependencies:** published versions only, never a `replace` directive.
- **References:** the repositories this spike reads are keys in the coordinator's
  `references.toml` and `references.local.toml`. The spike reads them and never writes to them.
