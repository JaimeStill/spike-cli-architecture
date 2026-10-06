// Package files is blobfs's file-system domain over the published blobfs
// library: the directory commands, mkdir, ls, stat, mv, and rmdir, and the
// [Store] they run on; and the object commands, put, cat, cp, and rm, and
// the [Objects] they run on. It uses blobfs.Directory and blobfs.File as the
// library defines them and does not restate them.
//
// The package has one file per role:
//
//   - entities.go holds the request and result shapes the commands build:
//     a Listing and its terms, a Page and the Contents of a listed
//     directory, an Entry found by id, a move's request and result, the
//     Content a put writes, and the results of a put, a copy, and a
//     branch's delete.
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
//   - objects.go composes the object operations from blobfs's protocols:
//     a put through WriteFile, or EnsureFile when it resumes a pending
//     row; cat's read; a copy through WriteFile; a file's delete through
//     RemoveFile; and a branch's delete, Directories.MarkDeleting and then
//     the sweep run until no work remains. It holds the adapter that is
//     blobfs's ObjectStore over go-storage's Store.
//   - commands.go builds the commands over the Store's and the Objects'
//     graph nodes and renders each result through package output. ls,
//     stat, mv, put's destination, cat, cp, and rm take an argument written
//     as id:<uuid> in place of a path; rm --recursive takes a path alone.
//
// The Store holds the database alone, never the object store, so the
// directory commands declare only the Store's node and run with the store
// unreachable. The Objects hold the Store and the object store, and only
// the object commands declare their node, so only they build and start the
// store. The boundary with the composition root is those two graph nodes:
// the root defines the node that builds the Store over its database, fixes
// blobfs's engine there, and makes [Store.Verify] the node's start, so a
// files command against a schema that is not applied fails at start,
// naming the node, before its body runs; and it defines the node that
// builds the Objects over the Store's node and its object store's, whose
// start creates the container and probes it, so an unreachable store fails
// an object command at start, naming the store's node. This package never reads
// configuration, names a driver or an engine, or imports the composition
// root or an admin package.
//
// The package exports:
//
//   - [Commands], which builds the directory commands over the Store's node,
//     and [ObjectCommands], which builds the object commands over the
//     Objects' node
//   - [Store], [New], and [Store.Verify]; [Objects] and [NewObjects]
//   - the Store's operations: [Store.List], [Store.ListDirectory],
//     [Store.Stat], [Store.StatFile], [Store.Resolve],
//     [Store.StatDirectory], [Store.Find], [Store.Mkdir], [Store.Move],
//     [Store.MoveEntry], and [Store.RemoveDirectory]
//   - the Objects' operations: [Objects.Put], [Objects.PutFile],
//     [Objects.Open], [Objects.OpenFile], [Objects.Copy],
//     [Objects.CopyFile], [Objects.Remove], [Objects.RemoveFile], and
//     [Objects.RemoveTree]
//   - the shapes they take and return, and [ParseRef], [ParseFilter], and
//     [ParseSort], which read the command line's arguments and terms
//   - [ErrVerify], [ErrMoveAcrossScopes], and [ErrNotAvailable]
package files
