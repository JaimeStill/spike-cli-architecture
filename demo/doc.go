// Package demo holds blobfs's scenarios, the narrated tours that blobfs
// demo runs and blobfs list prints, over the files domain's API: each step
// calls the Store or the Objects, as a direct command does, and shows the
// result as that command prints it.
//
// Each tour declares the composition root's nodes it reads, as any command
// does: [Directories] the files node alone, so it builds Postgres and never
// the object store, and [Files] the files and objects nodes, so it builds
// both. Each works in its own top-level directory, [DirectoriesArea] or
// [FilesArea], clears it first when an earlier run left it behind, and
// removes it last, so it succeeds twice in a row.
//
// The package exports:
//
//   - [Scenarios], the tours in presentation order, and [Commands], the
//     demo parent with a leaf per tour
//   - [Directories] and [Files], the two tours
//   - [DirectoriesArea], [FilesArea], and [Unit], the working areas and
//     the unit the directories tour owns its area as
package demo
