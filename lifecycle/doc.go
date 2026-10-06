// Package lifecycle runs a built [graph.System]. A [Coordinator] starts the
// System's subsystems layer by layer, runs a function or serves until its
// context ends, and shuts them down in reverse. Nothing is registered: the
// subsystems are inferred from the System's dependencies, and the layers come
// from the graph.
//
// The package exports:
//
//   - [Subsystem], the interface a dependency's value implements to take
//     part in the lifecycle
//   - [Config], the Coordinator's configuration, and [Config.Finalize],
//     which applies its default and environment override
//   - [Coordinator], which runs one System once, and [New], which returns
//     one
//   - [Coordinator.Exec], which starts the System, runs a function, and
//     shuts down: the one-shot form a CLI command uses
//   - [Coordinator.Run], which starts the System, serves until its context
//     ends, and shuts down: the long-running form a service uses
//
// # Participation
//
// A [graph.Dependency] takes part when its Value implements [Subsystem], or
// when its constructor recorded a hook. An OnStart or OnShutdown hook
// overrides the matching method, and a hook with no method makes the
// dependency a start-only or stop-only participant. A dependency with
// neither takes no part.
//
// # Startup
//
// The layers start lowest first, as [graph.System.Layers] orders them. A
// layer's participants start concurrently under a child of the caller's
// context, and the next layer starts only once the whole layer has. The
// first start failure cancels the rest of its layer; the context.Canceled
// errors its siblings return in consequence are dropped, and no higher layer
// starts. Start errors are labelled with the dependency's name, as in
// "database: <err>".
//
// A context that ends before or during startup stops it: no further layer
// starts. Run treats that as a clean stop, as it does the context's end
// after startup. Exec returns the cancellation, since its function never
// ran.
//
// # Shutdown
//
// Shutdown runs on every path: after Exec's function returns, whatever its
// error, after Run's context ends, and after a startup failure or
// cancellation. It shuts down every participant of every layer that began
// to start, the last layer first, each layer concurrently. That includes a
// participant whose Start failed, so a dependency constructed but not
// started leaks nothing; the start error stays the error reported.
//
// Shutdown runs under one context derived from context.Background and
// bounded by [Config].ShutdownTimeout, so it keeps its whole budget even
// when the caller's context has ended. The first layer that outlives the
// deadline adds one error wrapping context.DeadlineExceeded. Its unfinished
// shutdowns continue on the expired context, and their late errors are
// dropped. Each remaining layer still starts its shutdowns on the expired
// context, and shutdown does not wait for them. Shutdown errors are labelled
// with the name, joined, prefixed "shutdown: ", and joined with Exec's or
// Run's result, so a failed shutdown fails an otherwise clean run.
//
// # Mapping go-core's Coordinator
//
// The package is a candidate to replace go-core's lifecycle.Coordinator:
//
//   - Add(Service{Stage}) and the numbered stages give way to the System's
//     computed layers
//   - the Service struct gives way to the [Subsystem] interface and the
//     graph's hooks
//   - Service.Check gives way to readiness inferred, at promotion, from a
//     value implementing go-core's ReadinessChecker
//   - Run keeps its serve-until-signal and its shutdown
//   - OnReady, Monitor, and Ready and Checks stay on the Coordinator at
//     promotion; the spike does not build them
//   - Run's drain timeout becomes [Config].ShutdownTimeout
//   - Exec is new
//
// The break at promotion: Add, Service, the stages, and Run's timeout
// parameter go, and shutdown now also runs for a subsystem whose Start
// failed, which loosens go-core's contract that Shutdown is called only on a
// subsystem that started.
//
// # Promotion
//
// The package is promoted to go-core when:
//
//   - its API stays unchanged through the spike's files and validate
//     tasks
//   - the spike's CLI and a graph shaped like go-web-service's both run on
//     it
//   - go-core's Coordinator, rebuilt on it, passes its black-box tests,
//     adapted only for the break above
//
// Promotion follows the experiment's completion and precedes the build of
// the cli goal.
//
// The package imports the standard library, go-core, and graph.
package lifecycle
