package demo

import (
	"fmt"
	"io"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/output"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// showLine shows one result line, as a direct command prints its success.
func showLine(r *scenario.Reporter, format string, args ...any) error {
	return r.Show(func(w io.Writer) error { return output.Line(w, fmt.Sprintf(format, args...)) })
}

// showListing shows c as ls prints it under l, cursor lines included, so a
// later step can continue after a cursor the reader has seen.
func showListing(r *scenario.Reporter, l files.Listing, c files.Contents) error {
	return r.Show(func(w io.Writer) error { return files.WriteContents(w, l, c, true) })
}

// showDirectory shows a directory's row as stat prints it.
func showDirectory(r *scenario.Reporter, path string, d blobfs.Directory) error {
	return r.Show(func(w io.Writer) error { return files.WriteDirectoryRecord(w, path, d) })
}

// sizeOf returns a file's size, or 0 when the row records none.
func sizeOf(f blobfs.File) int64 {
	if f.Size == nil {
		return 0
	}
	return *f.Size
}
