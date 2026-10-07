package app

import (
	"io"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// mountScenario mounts the scenarios' commands at root over the files and
// storage nodes: the scenario parent, in which each scenario's leaf
// declares the nodes its tour reads, so the dispatcher builds only those
// when it runs.
func mountScenario(root *cli.Command, n *Nodes) {
	root.Add(scenario.Commands(n.Files, n.Storage)...)
}

// scenarioFooter returns the root's help footer: the scenario listing the
// scenario parent's help ends with, as spike-blobfs's root help appended
// it. It declares no node, so the help builds nothing.
func scenarioFooter(n *Nodes) func(w io.Writer) {
	return func(w io.Writer) { scenario.WriteListing(w, n.Files, n.Storage) }
}
