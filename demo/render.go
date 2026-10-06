package demo

import (
	"fmt"
	"io"
	"strconv"
	"time"

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
	entries := make([]output.Entry, 0, len(c.Directories.Rows)+len(c.Files.Rows))
	for _, d := range c.Directories.Rows {
		entries = append(entries, output.Entry{Kind: "dir", Name: d.Name, ID: d.ID, Updated: d.UpdatedAt})
	}
	for _, f := range c.Files.Rows {
		entries = append(entries, output.Entry{Kind: "file", Name: f.Name, ID: f.ID, Size: f.Size, Status: string(f.Status), Updated: f.UpdatedAt})
	}
	dirs, fls := pageOf(l, l.After.Directories, c.Directories), pageOf(l, l.After.Files, c.Files)
	return r.Show(func(w io.Writer) error { return output.Listing(w, entries, dirs, fls) })
}

// pageOf describes one half's page for output.Listing, as ls does.
func pageOf[T any](l files.Listing, after string, p files.Page[T]) output.Page {
	out := output.Page{
		Number: l.Page, Size: l.Size, Listed: len(p.Rows), Total: p.Total,
		Counted: l.Total == files.TotalExact && after == "", Cursor: after != "", More: p.More, Next: p.Next,
	}
	if p.Total == files.NoTotal {
		out.Total = output.NoTotal
	}
	return out
}

// showDirectory shows a directory's row as stat prints it.
func showDirectory(r *scenario.Reporter, path string, d blobfs.Directory) error {
	parent := "-"
	if d.ParentID != nil {
		parent = *d.ParentID
	}
	return r.Show(func(w io.Writer) error {
		return output.Record(w, []output.Field{
			{Name: "path", Value: path},
			{Name: "id", Value: d.ID},
			{Name: "parent", Value: parent},
			{Name: "name", Value: d.Name},
			{Name: "version", Value: strconv.FormatInt(d.Version, 10)},
			{Name: "created", Value: d.CreatedAt.UTC().Format(time.RFC3339)},
			{Name: "updated", Value: d.UpdatedAt.UTC().Format(time.RFC3339)},
		})
	})
}

// sizeOf returns a file's size, or 0 when the row records none.
func sizeOf(f blobfs.File) int64 {
	if f.Size == nil {
		return 0
	}
	return *f.Size
}
