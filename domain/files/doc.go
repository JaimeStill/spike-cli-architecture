// Package files is blobfs's file-system domain over the published blobfs
// library: the directory commands, mkdir, ls, stat, mv, and rmdir, and the
// bookmark command with add, ls, and rm, and the [Store] they run on; and
// the object commands, put, cat, cp, and rm, and the [Objects] they run on.
// It uses blobfs.Directory and blobfs.File as the library defines them and
// does not restate them.
//
// Beside blobfs's tables the domain owns two of its own, which the app's
// migration set creates. An owner row binds a top-level directory to a
// unit: mkdir --unit writes it with the directory, ls --unit checks it at
// the listed path's top-level directory and lists a unit's own top-level
// directories at the root, and rmdir and rm --recursive remove it with the
// directory. mv keeps a move under one top-level directory, so nothing
// crosses from one owner's scope into another's. A bookmark binds a unit to
// a file, at most one of a unit's bookmarks active; rm refuses a file a
// unit bookmarks, and rm --recursive a branch holding one, before anything
// is touched.
//
// The package has one file per role:
//
//   - entities.go holds the request and result shapes the commands build:
//     a Listing and its terms, a Page and the Contents of a listed
//     directory, an Entry found by id, a move's request and result, the
//     Content a put writes, the results of a put, a copy, and a branch's
//     delete, and a Bookmark as bookmark ls reads it.
//   - errors.go holds the errors the domain adds to blobfs's.
//   - database.go builds the [Store] over a database session: the domain's
//     pattern catalog, and blobfs's statements, with whatever engine the
//     caller passes, and the domain's own, embedded from statements/, both
//     compiled against it. It is the only file that imports the query
//     library: it lowers a Listing to its directives and wraps each of the
//     domain's statements in a typed method. [Store.Verify] prepares every
//     statement against the database.
//   - directories.go composes the directory operations from blobfs's
//     methods. Ids are the primary handle: ListDirectory, StatFile,
//     StatDirectory, Find, and MoveEntry take ids, and the path forms List,
//     Stat, Resolve, and Move resolve their paths and then run the same
//     steps. Mkdir and RemoveDirectory take paths, and write and remove
//     a directory's owner row with it.
//   - bookmarks.go composes the bookmark operations, AddBookmark,
//     RemoveBookmark, and ListBookmarks, from blobfs's methods and the
//     domain's bookmark statements; an add holds the file, blobfs's
//     reference-then-delete rule, before it inserts.
//   - objects.go composes the object operations from blobfs's protocols:
//     a put through WriteFile, or EnsureFile when it resumes a pending
//     row; cat's read; a copy through WriteFile; a file's delete through
//     RemoveFile, whose first transaction holds the file and refuses it
//     while a unit bookmarks it; and a branch's delete,
//     Directories.MarkDeleting, refused while a unit bookmarks a file in
//     the branch, and then the sweep run until no work remains, removing
//     each directory's owner row with it. It holds the adapter that is
//     blobfs's ObjectStore over go-storage's Store.
//   - commands.go builds the commands over the Store's and the Objects'
//     graph nodes and renders each result through package output. ls,
//     stat, mv, put's destination, cat, cp, and rm take an argument written
//     as id:<uuid> in place of a path; rm --recursive, mkdir, ls --unit,
//     and the bookmark subcommands take a path alone. Each bookmark
//     subcommand requires --unit.
//
// The Store holds the database alone, never the object store, so the
// directory and bookmark commands declare only the Store's node and run
// with the store unreachable. The Objects hold the Store and the object store, and only
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
//   - [Commands], which builds the directory and bookmark commands over the
//     Store's node, and [ObjectCommands], which builds the object commands
//     over the Objects' node
//   - [Store], [New], and [Store.Verify]; [Objects] and [NewObjects]
//   - the Store's operations: [Store.List], [Store.ListDirectory],
//     [Store.Stat], [Store.StatFile], [Store.Resolve],
//     [Store.StatDirectory], [Store.Find], [Store.Mkdir], [Store.Move],
//     [Store.MoveEntry], [Store.RemoveDirectory], [Store.AddBookmark],
//     [Store.RemoveBookmark], and [Store.ListBookmarks]
//   - the Objects' operations: [Objects.Put], [Objects.PutFile],
//     [Objects.Open], [Objects.OpenFile], [Objects.Copy],
//     [Objects.CopyFile], [Objects.Remove], [Objects.RemoveFile], and
//     [Objects.RemoveTree]
//   - the shapes they take and return, and [ParseRef], [ParseFilter], and
//     [ParseSort], which read the command line's arguments and terms
//   - [WriteContents], [WriteFileRecord], and [WriteDirectoryRecord], which
//     write a listing and a row as ls and stat print them, so the demo's
//     tours show a result as the command does
//   - [ErrVerify], [ErrMoveAcrossScopes], [ErrNotAvailable], [ErrUnitDepth],
//     [ErrNotOwned], [ErrNoCursorAtRoot], [ErrBookmarked],
//     [ErrAlreadyBookmarked], [ErrActiveBookmark], and [ErrNoBookmark], and
//     the names of the bookmark table's constraints they map from
package files
