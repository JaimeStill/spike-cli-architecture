// Package output renders a command's result to the writer the dispatcher
// hands it, so every command family prints the same way. [Line] writes a
// one-line success, so a command that changes state is never silent;
// [Table] writes rows as aligned columns under a header; [Record] writes
// one row as aligned label and value lines; [Listing] writes a directory
// listing, its entries and then a summary of each half's page; and
// [Bookmarks] writes a unit's bookmarks and a summary of their page.
// Failures are not rendered here: a command returns its error and package
// cli reports it.
//
// The package imports only the standard library.
package output
