// Package integration holds the black-box suite over the built blobfs
// binary, run by `mise run integration` against its isolated compose
// project. Its tests, behind the integration build tag, build cmd/blobfs
// once, run it as a child process configured only through its environment,
// arguments, and standard input, and check its output, its exit code, and
// its effect on a throwaway database and a container of each test's own.
// Each run is bounded by go-core's processtest.Failsafe, and so is every
// wait on a condition, which polls it through processtest.WaitFor.
//
// The suite reaches every state through the surfaces production has: the
// CLI for state, the environment for configuration, the network for
// faults, and signals and the exit code for lifecycle. It writes nothing
// to the database or the store behind the binary's back, and reads them
// only through the binary.
//
//   - A pending row is a crashed put's: put - runs with its standard input
//     held open on a pipe, so it commits the pending row and waits on the
//     body; once stat in another run shows the row pending, the put is
//     killed with SIGKILL, which it cannot catch.
//   - An interrupted rm --recursive is a store outage mid-sweep: the store
//     is reached through a processtest.Forward relay, its retries off
//     through BLOBFS_STORAGE_OPTIONS_MAX_RETRIES, and the branch holds
//     enough files that its sweep outlasts the notice that it has begun.
//     Once stat in another run finds the branch's first file gone, the
//     relay is severed, so every later delete is refused: the run exits one
//     with the refusal and the counts it reached, and the branch stays
//     deleting. The relay is restored before the rerun that finishes it.
//   - An unreachable store is an endpoint on a port nothing listens on.
//
// TestScript is one ordered script over the directory, object, and
// bookmark commands. TestTheStoreUnreachable closes the object store's
// endpoint: every directory and bookmark command still succeeds, and two
// object commands fail naming the store. scenarios_test.go runs list with
// nothing reachable, each demo tour twice in a row and after an
// interrupted run, and demo directories with the store unreachable, where
// demo files fails naming the store. Every run logs its command line and
// output, so go test -v prints the transcript.
//
// The package has no code outside its tests.
package integration
