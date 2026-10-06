package scenario_test

import (
	"bytes"
	"context"
	"flag"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-core/config"
	"github.com/standards-lab/go-core/process"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

// tree is a program with one scenario mounted under demo, over a graph of
// three nodes: the scenario declares a and b, and c is defined and never
// declared. Every constructor records its name, and each value is its
// name.
type tree struct {
	root  *cli.Command
	g     *graph.Graph
	cfg   *graph.Node[lifecycle.Config]
	built []string
}

func newTree(t *testing.T, steps func(a, b *graph.Node[string]) []scenario.Step, flags func(*flag.FlagSet)) *tree {
	t.Helper()
	tr := &tree{g: graph.New()}
	tr.cfg = tr.g.Define("lifecycle", func(*graph.Scope) (lifecycle.Config, error) {
		return lifecycle.Config{ShutdownTimeout: config.Duration(5 * time.Second)}, nil
	})
	define := func(name string) *graph.Node[string] {
		return tr.g.Define(name, func(*graph.Scope) (string, error) {
			tr.built = append(tr.built, name)
			return name, nil
		})
	}
	a, b := define("a"), define("b")
	define("c")
	s := scenario.Scenario{
		Name:    "tour",
		Summary: "A stub tour",
		Nodes:   []scenario.Node{a, b},
		Flags:   flags,
		Steps:   steps(a, b),
	}
	tr.root = (&cli.Command{Name: "prog"}).Add((&cli.Command{Name: "demo"}).Add(scenario.Command(s)))
	return tr
}

func (tr *tree) run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Run(context.Background(), tr.root, args, strings.NewReader(""), &out, &errOut, cli.WithGraph(tr.g, tr.cfg))
	return code, out.String(), errOut.String()
}

func TestCommand_BuildsTheDeclaredNodesAndNarratesOverThem(t *testing.T) {
	tr := newTree(t, func(a, b *graph.Node[string]) []scenario.Step {
		return []scenario.Step{{
			Intent: "Read both nodes",
			Action: func(_ context.Context, sys *graph.System, r *scenario.Reporter) error {
				r.Note("read %s and %s", sys.Get(a), sys.Get(b))
				return nil
			},
		}}
	}, nil)

	code, out, errOut := tr.run("demo", "tour")

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q", code, process.ExitOK, errOut)
	}
	if want := "[1/1] Read both nodes\n  read a and b\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if want := []string{"a", "b"}; !slices.Equal(tr.built, want) {
		t.Errorf("built = %q, want %q: the declared nodes and nothing else", tr.built, want)
	}
}

func TestCommand_HelpBuildsNothingAndListsTheScenariosFlags(t *testing.T) {
	var level int
	tr := newTree(t, func(a, b *graph.Node[string]) []scenario.Step { return nil },
		func(fs *flag.FlagSet) { fs.IntVar(&level, "level", 1, "how deep the tour goes") })

	code, out, _ := tr.run("demo", "tour", "--help")

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	for _, want := range []string{"A stub tour", "Usage:\n  prog demo tour [flags]", "--level", "how deep the tour goes"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	if len(tr.built) != 0 {
		t.Errorf("built = %q, want nothing", tr.built)
	}
}

func TestCommand_ReadsTheScenariosFlags(t *testing.T) {
	var level int
	tr := newTree(t, func(a, b *graph.Node[string]) []scenario.Step {
		return []scenario.Step{{
			Intent: "Report the level",
			Action: func(_ context.Context, _ *graph.System, r *scenario.Reporter) error {
				r.Note("level %d", level)
				return nil
			},
		}}
	}, func(fs *flag.FlagSet) { fs.IntVar(&level, "level", 1, "how deep the tour goes") })

	code, out, errOut := tr.run("demo", "tour", "--level", "3")

	if code != process.ExitOK {
		t.Fatalf("code = %d; stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "level 3") {
		t.Errorf("stdout = %q, want the flag's value", out)
	}
}

func TestCommand_TakesNoArgumentsAndBuildsNothingOnRefusal(t *testing.T) {
	tr := newTree(t, func(a, b *graph.Node[string]) []scenario.Step { return nil }, nil)

	code, out, errOut := tr.run("demo", "tour", "extra")

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	if out != "" || !strings.HasPrefix(errOut, "prog demo tour: ") {
		t.Errorf("stdout = %q, stderr = %q: want a usage error on the tour", out, errOut)
	}
	if len(tr.built) != 0 {
		t.Errorf("built = %q, want nothing", tr.built)
	}
}

func TestCommand_AFailedStepIsReportedOnceUnderTheCommandsPath(t *testing.T) {
	tr := newTree(t, func(a, b *graph.Node[string]) []scenario.Step {
		return []scenario.Step{{
			Intent: "Fail",
			Action: func(context.Context, *graph.System, *scenario.Reporter) error {
				return context.DeadlineExceeded
			},
		}}
	}, nil)

	code, _, errOut := tr.run("demo", "tour")

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	if want := "prog demo tour: step 1 (Fail): context deadline exceeded\n"; errOut != want {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}
