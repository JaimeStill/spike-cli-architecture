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
