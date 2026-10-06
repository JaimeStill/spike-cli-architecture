package scenario_test

import (
	"bytes"
	"testing"

	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// node defines a node named name on g whose constructor fails the test: a
// listing builds nothing.
func node(t *testing.T, g *graph.Graph, name string) *graph.Node[int] {
	return g.Define(name, func(*graph.Scope) (int, error) {
		t.Errorf("the listing built %s", name)
		return 0, nil
	})
}

func TestWriteListing_NamesEachScenarioWithItsSummaryAndDeclaredNodes(t *testing.T) {
	g := graph.New()
	files, objects := node(t, g, "files"), node(t, g, "objects")
	var out bytes.Buffer

	scenario.WriteListing(&out, []scenario.Scenario{
		{Name: "directories", Summary: "the first tour", Nodes: []scenario.Node{files}},
		{Name: "files", Summary: "the second tour", Nodes: []scenario.Node{files, objects}},
		{Name: "bare", Summary: "declares nothing"},
	})

	want := "" +
		"  directories  the first tour\n" +
		"               uses files\n" +
		"  files        the second tour\n" +
		"               uses files, objects\n" +
		"  bare         declares nothing\n"
	if out.String() != want {
		t.Errorf("WriteListing wrote:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestWriteListing_SaysSoWhenThereAreNoScenarios(t *testing.T) {
	var out bytes.Buffer
	scenario.WriteListing(&out, nil)
	if got := out.String(); got != "no scenarios\n" {
		t.Errorf("WriteListing over nothing wrote %q", got)
	}
}
