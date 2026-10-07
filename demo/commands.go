package demo

import (
	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// Scenarios returns blobfs's scenarios over the composition root's files
// and storage nodes, in presentation order: the directories tour, which
// declares the files node alone, then the files tour, which declares both.
func Scenarios(store *graph.Node[*files.Service], objs *graph.Node[*files.Storage]) []scenario.Scenario {
	return []scenario.Scenario{
		Directories(store),
		Files(store, objs),
	}
}

// Commands builds the demo parent with one leaf per scenario of
// [Scenarios], each declaring its own nodes with Use; the parent declares
// none, so a tour brings up only what it declares.
func Commands(store *graph.Node[*files.Service], objs *graph.Node[*files.Storage]) *cli.Command {
	parent := &cli.Command{
		Name:    "demo",
		Summary: "Run a narrated scenario; blobfs list shows each one and the nodes it declares",
	}
	for _, s := range Scenarios(store, objs) {
		parent.Add(scenario.Command(s))
	}
	return parent
}
