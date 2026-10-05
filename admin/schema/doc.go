// Package schema is the schema administration layer: the schema command
// group, which reports, applies, reverts, and resets the two migration sets
// blobfs's database holds. It is a sibling of internal/app, which mounts it,
// and of the domain the file commands add, and it imports no domain
// package.
//
// The package has one file per role. database.go builds the [Client] over
// sqlate's multi-set migrator: blobfs's set first, under its own history
// table, and the app's set last, under sqlate's default table, so blobfs's
// schema is at its head before the app's migrations reference it and a
// revert runs in reverse. commands.go builds the schema command and its
// status, up, down, and reset subcommands over [Deps] and renders each
// result through package output. The composition root opens the database
// and constructs the Client; this package never reads configuration or
// names a driver.
//
// The package exports:
//
//   - [Commands], which builds the schema command and its subcommands
//   - [Deps], what the group needs from the composition root
//   - [Body], a leaf command's body, as [Deps.Run] wraps it
//   - [Client], which runs the schema operations over the migrator
//   - [NewClient], which builds the Client over a database
//   - [Client.Up], [Client.Down], [Client.Reset], and [Client.Status], the
//     operations the subcommands run
//   - [Sets], the two migration sets, bottom first
//   - [AppSet], the name of the app's set
package schema
