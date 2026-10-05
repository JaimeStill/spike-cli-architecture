// Package lifecycle brings dependencies up in phases and unwinds them in
// reverse. A one-shot CLI run brings up only what its command needs, on
// demand, and unwinds it when the command returns; a long-running service
// does the same in stages, and unwinds them when it drains.
//
// The package exports:
//
//   - [Step], one participant: a name for its errors, and optional Start and
//     Stop
//   - [Stack], which records the phases that started and unwinds them; the
//     zero value is ready to use
//   - [Stack.Start], which runs one phase of steps concurrently and pushes
//     the steps that started
//   - [Stack.Unwind], which stops every pushed phase, the last first, under
//     one bounded context
//
// # Phases
//
// Each [Stack.Start] call is one phase. Its steps start concurrently under a
// child of the caller's context that the first failure cancels, so the
// phase's siblings can stop early; the cancellations they return in
// consequence are dropped. Only the steps that started are pushed, so a Stop
// never has to tolerate a dependency that never came up. A failed Start
// leaves the earlier phases and the phase's started steps on the stack for
// the caller to unwind.
//
// # Mapping go-core's Coordinator
//
// The Stack is a candidate for go-core, where lifecycle.Coordinator would be
// rebuilt on it phase by phase:
//
//   - OnShutdown hooks are pushed first, as one phase of stop-only steps, so
//     they are the last phase Unwind runs, and they still run when startup
//     fails
//   - OnStartup hooks, then each numbered stage in ascending order, then
//     StageRoot, become one Start call each, with start-only steps for hooks
//     and a step per service carrying its Start and Shutdown
//   - the drain becomes Unwind(drainTimeout), under the existing "shutdown:"
//     wrap
//   - the Coordinator keeps the "startup:" wrap and its signal-during-startup
//     check, applied to the errors Start returns unwrapped
//   - the state machine, Ready and Checks, OnReady, Monitor, and the
//     registration panics stay in the Coordinator
//
// The one point needing care: Start cancels only its own child context, so
// the Coordinator must still cancel the run context before it drains, as
// go-core's Coordinator does, so no drain runs with the run context live. Only go-core's black-box lifecycle tests can prove the rebuild keeps
// it.
//
// # Promotion
//
// The Stack is promoted to go-core when:
//
//   - its API stays standard-library-only and unchanged through the spike's
//     files and validate tasks
//   - the spike's CLI composition root uses it for every dependency
//   - go-core's Coordinator is rebuilt on it with its existing black-box
//     tests passing unchanged
//
// Promotion follows the experiment's completion and precedes the build of
// the cli goal.
//
// The package imports only the standard library.
package lifecycle
