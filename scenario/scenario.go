package scenario

import (
	"context"
	"flag"
	"fmt"

	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// Scenario is one narrated tour: what it shows, the graph nodes it
// declares, its own flags, and the ordered steps that show it. It states no
// preconditions of its own: the nodes are its dependencies, which its
// command declares with Use, so the dispatcher builds and starts exactly
// those before the first step, and a dependency that cannot be reached
// fails the run at start, naming its node.
type Scenario struct {
	Name    string              // the word after "blobfs demo"
	Summary string              // the line blobfs list and the demo help print
	Nodes   []Node              // the nodes the command declares, in order
	Flags   func(*flag.FlagSet) // the scenario's own flags; nil for none
	Steps   []Step
}

// Node is a graph node a scenario declares: the [graph.Ref] its command
// passes to Use and the name the listing prints, both from one value. Every
// *graph.Node[T] is one, so a scenario holds the composition root's typed
// nodes as they are, and a step reads a node's value from the System with
// the same handle.
type Node interface {
	graph.Ref
	Name() string
}

// Step is one beat of the narration: the sentence saying what is about to
// happen, and the action that does it. The action reads the values of the
// scenario's nodes from sys, the System the dispatcher built and started,
// and reports what it observed through r.
type Step struct {
	Intent string
	Action func(ctx context.Context, sys *graph.System, r *Reporter) error
}

// Run runs s's steps in order over sys, narrating each intent through r
// before its action. It stops at the first action that returns an error,
// which it returns naming the step by its number and its intent, so a
// failed run's report says which beat failed; the dispatcher labels it with
// the command's path, which names the scenario.
func Run(ctx context.Context, s Scenario, sys *graph.System, r *Reporter) error {
	for i, step := range s.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.Intent(i+1, len(s.Steps), step.Intent)
		if step.Action == nil {
			continue
		}
		if err := step.Action(ctx, sys, r); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, step.Intent, err)
		}
	}
	return nil
}

// refs returns s's nodes as the refs Use takes.
func (s Scenario) refs() []graph.Ref {
	refs := make([]graph.Ref, len(s.Nodes))
	for i, n := range s.Nodes {
		refs[i] = n
	}
	return refs
}
