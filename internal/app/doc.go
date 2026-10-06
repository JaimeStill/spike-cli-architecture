// Package app is blobfs's composition root. [New] describes blobfs's
// dependencies as one graph.Graph and builds the command tree on package
// cli over it; [App.Run] dispatches the process arguments over that tree.
//
// The graph's nodes are infrastructure.go's, each configuration finalized
// under the BLOBFS prefix and the database and object store built from
// them, and admin.go's, the migrator built on the database. Describing them
// constructs nothing. A command names the nodes it needs in its Uses: the
// schema group names the migrator, and its verbs inherit it, so a schema
// verb builds the migrator, the database, and its configuration, and never
// the object store. version names none. The dispatcher builds the leaf's
// Uses only when the leaf runs, starts what was built layer by layer, and
// shuts it down in reverse when the leaf returns; a run that builds
// nothing, such as help, a usage error, or version, reads no configuration.
//
// The package exports:
//
//   - [App], the blobfs program, and [New], which describes its graph and
//     builds its command tree
//   - [App.Run], which dispatches the process arguments and returns the
//     exit code
package app
