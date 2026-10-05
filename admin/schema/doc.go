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
package schema
