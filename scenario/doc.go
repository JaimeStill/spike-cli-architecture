// Package scenario defines what a blobfs scenario is, the reporter it
// narrates through, the command that runs one, and the listing that prints
// them. It is blobfs's own package, shaped like slab's scenario package on
// package cli in place of cobra, and without slab's needs: a scenario's
// dependencies are the graph nodes it declares, and nothing else. There is
// no registry: [Command] builds one leaf from a scenario a caller hands it,
// the same way a domain package's Commands is called.
//
// A scenario's command declares its nodes with Use, like any command, so
// the dispatcher builds and starts only those, under the lifecycle, before
// the first step runs; a dependency that cannot be reached fails the run
// at start, reported once and naming its node, before anything is
// narrated. The scenario has no probe of its own and names no task that
// would start a dependency.
//
// The package exports:
//
//   - [Scenario], an ordered list of [Step]s over the [Node]s it declares
//   - [Run], which runs a scenario's steps over a built System, narrating
//     each before doing it
//   - [Reporter] and [NewReporter], the channel a step narrates through
//   - [Command], the leaf command that runs one scenario
//   - [WriteListing], which prints the scenarios and the nodes each
//     declares
package scenario
