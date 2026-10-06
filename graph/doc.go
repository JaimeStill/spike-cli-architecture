// Package graph is a typed dependency graph. A [Graph] describes [Node]
// values inertly, each a name and a constructor; [Graph.Build] constructs
// what its roots reach and returns it as a [System] of [Dependency] values
// in computed layers. The lifecycle package runs a System: it starts the
// layers in order and stops them in reverse.
//
// The package exports:
//
//   - [Graph], the description of the nodes, and [New], which returns an
//     empty one
//   - [Graph.Define], which adds a node with its constructor and runs
//     nothing
//   - [Graph.Replace], which swaps a node's constructor before Build, for a
//     test's substitute
//   - [Graph.Build], which constructs what the roots reach into a System
//   - [Node], a typed handle on one node, and [Node.Name], its name
//   - [Ref], any Node whatever its type, sealed to Node
//   - [Scope], what a constructor receives
//   - [Scope.Use], which returns a dependency's value, building it if
//     needed
//   - [Scope.After], which orders the node after another without its value
//   - [Scope.OnStart] and [Scope.OnShutdown], which record the node's hooks
//   - [System], what one Build constructed
//   - [System.Get], which returns a node's value
//   - [System.Layers], which returns the built nodes in layers
//   - [Dependency], one built node: its name, value, and hooks
//
// # Discovery
//
// A constructor declares its dependencies by using them: [Scope.Use] builds
// the dependency if this Build has not, and records the edge, so the edges
// are discovered as the constructors run, depth-first from the roots, and
// can depend on what the constructors see. Each node is built at most once
// per Build, so a node shared by several dependents is constructed once and
// they all receive its value; a node no Use reaches is never constructed,
// so a Build of a subset of the roots brings up only that subset's
// dependencies. Each Build is independent and constructs fresh values.
//
// [Scope.After] adds an order-only edge: it passes no value and builds
// nothing, and holds only when its target is in the System by some other
// path.
//
// # Layers
//
// The layers are the longest-path layering of the discovered edges: a node
// with no dependencies is in layer 0, and any other node is one layer above
// the highest of its Use and After dependencies. Every node's dependencies
// therefore sit in lower layers, so the nodes of one layer can start
// together once the layers below them have. Within a layer, nodes are in
// definition order.
//
// # Errors and panics
//
// A constructor's error fails the Build, labelled with its node's name and
// wrapping the error; a failing dependency aborts every constructor whose
// Use reached it, and Build reports the dependency's error. Wiring mistakes
// panic with a "graph: " message, as each symbol's documentation states: a
// dependency cycle, a node used on a Graph it was not defined on, a Scope
// used after its constructor returned, a Replace after Build, a Get of a
// node not in the System, and an empty or duplicate name.
//
// # Promotion
//
// The package is a candidate for go-core, shaped like the cli package is
// shaped for go-cli-sdk: its API is standard-library-only and knows nothing
// of lifecycles beyond carrying each node's hooks.
//
// The package imports only the standard library.
package graph
