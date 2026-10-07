package scenario

import (
	"fmt"
	"io"
	"strings"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// scenarios returns blobfs's scenarios over the composition root's files
// and storage nodes, in presentation order: the directories tour, which
// declares the files node alone, then the files tour, which declares both.
func scenarios(svc *graph.Node[*files.Service], st *graph.Node[*files.Storage]) []Scenario {
	return []Scenario{
		directoriesTour(svc),
		filesTour(svc, st),
	}
}

// Commands builds the scenario parent over the composition root's files
// and storage nodes, with one leaf per scenario, each declaring its own
// nodes with Use; the parent declares none, so a tour brings up only what
// it declares. Run alone, the parent prints its help, which ends with the
// listing [WriteListing] writes.
func Commands(svc *graph.Node[*files.Service], st *graph.Node[*files.Storage]) *cli.Command {
	parent := &cli.Command{
		Name:    "scenario",
		Summary: "Run a narrated scenario over the commands",
		Footer:  func(w io.Writer) { WriteListing(w, svc, st) },
	}
	for _, s := range scenarios(svc, st) {
		parent.Add(command(s))
	}
	return parent
}

// WriteListing writes the scenario listing over the files and storage
// nodes to w, as a help Footer prints it: a Scenarios heading, then one
// line per scenario, its name and summary, followed by a uses line naming
// the nodes the scenario declares, in the order it declares them. It is
// the one source of the listing, which the scenario parent's help and the
// root's help both end with. It prints what a scenario declares, not the
// nodes those reach: the graph discovers a node's dependencies only by
// running its constructor, and the listing builds nothing.
func WriteListing(w io.Writer, svc *graph.Node[*files.Service], st *graph.Node[*files.Storage]) {
	all := scenarios(svc, st)
	width := 0
	for _, s := range all {
		width = max(width, len(s.Name))
	}
	_, _ = io.WriteString(w, "Scenarios:\n")
	for _, s := range all {
		_, _ = fmt.Fprintf(w, "  %-*s  %s\n", width, s.Name, s.Summary)
		names := make([]string, len(s.Nodes))
		for i, n := range s.Nodes {
			names[i] = n.Name()
		}
		_, _ = fmt.Fprintf(w, "  %-*s  uses %s\n", width, "", strings.Join(names, ", "))
	}
}
