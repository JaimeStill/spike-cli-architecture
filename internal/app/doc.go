// Package app is blobfs's composition root. [New] describes blobfs's
// dependencies as one graph.Graph and builds the command tree on package
// cli over it; [App.Run] dispatches the process arguments over that tree.
//
// infrastructure.go defines the configuration nodes, each finalized under
// the BLOBFS prefix, and the database and object store nodes built from
// them. admin.go defines the migrator node, built on the database.
// domain.go defines the files node, the files domain's Store built on the
// database with blobfs's Postgres engine, which the root fixes there, and
// whose start is the Store's statement check; and the objects node, the
// files domain's Objects built on the files node and the object store.
// demo.go mounts the scenarios over the files and objects nodes: the demo
// parent with a leaf per tour, and list at the root, which prints each
// scenario and the nodes it declares. Defining them constructs nothing. A command declares the nodes it needs with Use: the schema
// group declares the migrator, and its verbs inherit it, so a schema verb
// builds the migrator, the database, and their configuration, and never the
// object store; each directory command, mkdir, ls, stat, mv, and rmdir,
// declares the files node, and likewise never builds the object store;
// each object command, put, cat, cp, and rm, declares the objects node, so
// it builds the database and the object store, and a store that cannot be
// reached fails it at start, naming the store's node, with the database
// shut down. Each demo tour declares the nodes it reads: demo directories
// the files node, so it never builds the object store, and demo files the
// files and objects nodes. version and list declare none. [New] takes the process's standard
// input beside its output and error streams, and a command reads it
// through its Invocation, as put - does. The dispatcher builds the nodes the leaf's path
// declares only when the leaf runs, starts what was built layer by layer,
// and shuts it down in reverse when the leaf returns; a run that builds
// nothing, such as help, a usage error, version, or list, reads no
// configuration.
//
// The package exports:
//
//   - [App], the blobfs program, and [New], which describes its graph and
//     builds its command tree
//   - [App.Run], which dispatches the process arguments and returns the
//     exit code
package app
