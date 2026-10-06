// Package integration holds the black-box suite over the built blobfs
// binary, run by `mise run integration` against its isolated compose
// project. Its tests, behind the integration build tag, build cmd/blobfs
// once, run it as a child process configured only through its environment
// and arguments, and check its output, its exit code, and its effect on a
// throwaway database of each test's own. TestScript is one ordered script
// over the directory commands; TestDirectoryCommandsWithTheStoreUnreachable
// runs them with the object store's endpoint closed. Every run logs its
// command line and output, so go test -v prints the transcript.
//
// The package has no code outside its tests.
package integration
