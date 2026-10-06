// Package files is blobfs's file-system domain over the published blobfs
// library: the directory commands, mkdir, ls, stat, mv, and rmdir, and the
// [Store] they run on. It uses blobfs.Directory and blobfs.File as the
// library defines them and does not restate them.
//
// The package has one file per role:
//
//   - entities.go holds the request and result shapes the commands build:
//     a Listing and its terms, a Page and the Contents of a listed
//     directory, an Entry found by id, and a move's request and result.
//   - errors.go holds the errors the domain adds to blobfs's.
//   - database.go builds the [Store] over a database session: the domain's
//     pattern catalog and blobfs's statements compiled against it, with
//     whatever engine the caller passes. It is the only file that imports
//     the query library, and it lowers a Listing to its directives.
//     [Store.Verify] prepares every statement against the database.
//   - directories.go composes the directory operations from blobfs's
//     methods. Ids are the primary handle: ListDirectory, StatFile,
//     StatDirectory, Find, and MoveEntry take ids, and the path forms List,
//     Stat, Resolve, and Move resolve their paths and then run the same
//     steps. Mkdir and RemoveDirectory take paths.
//   - commands.go builds the commands over the Store's graph node and
//     renders each result through package output. ls, stat, and mv take an
//     argument written as id:<uuid> in place of a path.
//
// The Store holds the database alone, never the object store, so the
// directory commands declare only the Store's node and run with the store
// unreachable. The boundary with the composition root is that one graph
// node: the root defines the node that builds the Store over its database,
// fixes blobfs's engine there, and makes [Store.Verify] the node's start,
// so a files command against a schema that is not applied fails at start,
// naming the node, before its body runs. This package never reads
// configuration, names a driver or an engine, or imports the composition
// root or an admin package.
//
// The package exports:
//
//   - [Commands], which builds the directory commands over the Store's node
//   - [Store], [New], and [Store.Verify]
//   - the Store's operations: [Store.List], [Store.ListDirectory],
//     [Store.Stat], [Store.StatFile], [Store.Resolve],
//     [Store.StatDirectory], [Store.Find], [Store.Mkdir], [Store.Move],
//     [Store.MoveEntry], and [Store.RemoveDirectory]
//   - the shapes they take and return, and [ParseRef], [ParseFilter], and
//     [ParseSort], which read the command line's arguments and terms
//   - [ErrVerify] and [ErrMoveAcrossScopes]
package files
