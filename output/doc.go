// Package output renders a command's result to the writer the dispatcher
// hands it, so every command family prints the same way. [Line] writes a
// one-line success, so a command that changes state is never silent;
// [Table] writes rows as aligned columns under a header. Failures are not
// rendered here: a command returns its error and package cli reports it.
//
// The package imports only the standard library. The planned file commands
// will extend it with the listings they share.
package output
