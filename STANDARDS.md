# Standards

The judgement calls the check can't enforce, one line each, for the standards-reviewer. What
`mise run check` enforces is never restated here.

The architecture pages that apply, in the architecture repository:

- `principles/composition-root.md`
- `principles/tool-beside-library.md`
- `principles/minimal-footprint.md`
- `standards/go-elemental/principles/topology-and-naming.md`
- `standards/go-elemental/principles/tests-and-docs.md`
- the domain-file ontology, a go-elemental principles page carried as a pending edit in the goal
  record; until it lands, the ontology line below is the rule

## Types and files

- A type or layer is introduced only when it is needed: no pass-through wrapper, no handle type
  over a graph node, and no exported symbol that nothing outside its package reads.
- A file holds one primary type or subject and is named for it, as `cli/invocation.go` holds
  `Invocation`.
- A domain package's files follow the domain-file ontology: `service.go` the exported `Service`,
  built by `New`; `database.go` the unexported `store` and its queries; `storage.go` the blob
  protocol and, in a CLI, the second API type its dependency profile splits off (`Storage`,
  `NewStorage`); `entities.go` the shapes; `errors.go` the errors; `commands.go` the commands,
  their flag structs, and the CLI-syntax parsers; `output.go` the writers and record layouts.

## Commands

- A domain mounts through one `Commands` call, and each command is a plain function of the nodes
  it declares with `Use`, reading them with `inv.Get`.
- `Args` only counts positionals; every argument and flag value, the domain's form rules
  included, is checked in `Validate`, so bad input builds nothing.
- An operation that names an entry takes the domain's reference type (`files.Ref`); one that
  takes a single form refuses the other with a typed form error before any I/O.
- A domain error is worded in the domain's terms; the command adds flag or command wording where
  it reports the error.
- A command that changes state is never silent: it prints one success line with `fmt.Fprintf`.

## Composition

- A value takes part in a lifecycle only through the single-method interfaces it implements,
  `lifecycle.Starter` and `lifecycle.Stopper`; nothing registers hooks.
- A wiring mistake panics at construction or dispatch with its package's prefix; a failure that
  input or the environment can cause returns an error.
- Configuration is a graph node, finalized in its constructor, so the environment is read only
  for a built System and never for help, a usage error, or a command that declares nothing.
