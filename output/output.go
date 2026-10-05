// Package output renders a command's result to the writer the dispatcher
// hands it, so every command family prints the same way. [Line] writes a
// one-line success, so a command that changes state is never silent;
// [Table] writes rows as aligned columns under a header. Failures are not
// rendered here: a command returns its error and package cli reports it.
//
// The package imports only the standard library. The file commands extend
// it with the listings they share.
package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Line writes line and a newline to w.
func Line(w io.Writer, line string) error {
	_, err := fmt.Fprintln(w, line)
	return err
}

// Table writes header and then each row to w as aligned columns, cells
// separated by two spaces at least. A table with no rows still writes its
// header, so an empty result is visible as one.
func Table(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(header, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
