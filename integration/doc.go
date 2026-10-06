// Package integration holds the black-box suite over the built blobfs
// binary, run by `mise run integration` against its isolated compose
// project. Its tests, behind the integration build tag, build cmd/blobfs
// once, run it as a child process configured only through its environment,
// arguments, and standard input, and check its output, its exit code, and
// its effect on a throwaway database and a container of each test's own.
// TestScript is one ordered script over the directory and object commands;
// TestTheStoreUnreachable runs them with the object store's endpoint
// closed, where the directory commands succeed and the object commands
// fail naming the store. Every run logs its command line and output, so
// go test -v prints the transcript.
//
// The package has no code outside its tests.
package integration
