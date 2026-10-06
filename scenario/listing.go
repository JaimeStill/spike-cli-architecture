package scenario

import (
	"fmt"
	"io"
	"strings"
)

// WriteListing prints one line per scenario in all, its name and summary,
// followed by a uses line naming the nodes the scenario declares, in the
// order it declares them. It prints what the scenario declares, not the
// nodes those reach: the graph discovers a node's dependencies only by
// running its constructor, and the listing builds nothing. A scenario that
// declares no node has no uses line.
func WriteListing(w io.Writer, all []Scenario) {
	if len(all) == 0 {
		_, _ = fmt.Fprintln(w, "no scenarios")
		return
	}
	width := 0
	for _, s := range all {
		width = max(width, len(s.Name))
	}
	for _, s := range all {
		_, _ = fmt.Fprintf(w, "  %-*s  %s\n", width, s.Name, s.Summary)
		if len(s.Nodes) == 0 {
			continue
		}
		names := make([]string, len(s.Nodes))
		for i, n := range s.Nodes {
			names[i] = n.Name()
		}
		_, _ = fmt.Fprintf(w, "  %-*s  uses %s\n", width, "", strings.Join(names, ", "))
	}
}
