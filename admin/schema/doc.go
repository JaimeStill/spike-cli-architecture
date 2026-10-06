// Package schema builds the schema command group, which reports, applies,
// reverts, and resets the two migration sets blobfs's database holds. It is
// a sibling of internal/app, which mounts it, and of the domain packages,
// such as domain/files, and it imports no domain package.
//
// The package has one file per role. database.go builds the [Client] over
// sqlate's multi-set migrator: blobfs's set first, under its own history
// table, and the app's set last, under sqlate's default table, so blobfs's
// schema is at its head before the app's migrations reference it and a
// revert runs in reverse. commands.go builds the schema command and its
// status, up, down, and reset subcommands and renders each result through
// package output.
//
// The group's boundary with the composition root is one graph node: the
// root defines the node that constructs the Client over its database and
// passes it to [Commands]. The group declares that node with Use, so the
// dispatcher builds and starts the database before a verb's body runs and
// shuts it down after, and each body reads the Client from the
// Invocation's System. This package never reads configuration, names a
// driver, or imports the composition root.
//
// The package exports:
//
//   - [Commands], which builds the schema command and its subcommands over
//     the Client's node
//   - [Client], which runs the schema operations over the migrator
//   - [NewClient], which builds the Client over a database
//   - [Client.Up], [Client.Down], [Client.Reset], and [Client.Status], the
//     operations the subcommands run
//   - [Sets], the two migration sets, bottom first
//   - [AppSet], the name of the app's set
package schema
