package scenario_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

func TestRun_NarratesEachStepBeforeItsAction(t *testing.T) {
	var out bytes.Buffer
	act := func(name string) func(context.Context, *graph.System, *scenario.Reporter) error {
		return func(context.Context, *graph.System, *scenario.Reporter) error {
			out.WriteString("acted " + name + "\n")
			return nil
		}
	}
	s := scenario.Scenario{
		Name: "stub",
		Steps: []scenario.Step{
			{Intent: "First", Action: act("first")},
			{Intent: "Narrated only"},
			{Intent: "Third", Action: act("third")},
		},
	}

	err := scenario.Run(context.Background(), s, nil, scenario.NewReporter(&out))

	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	want := "" +
		"[1/3] First\n" +
		"acted first\n" +
		"\n" +
		"[2/3] Narrated only\n" +
		"\n" +
		"[3/3] Third\n" +
		"acted third\n"
	if out.String() != want {
		t.Errorf("Run wrote:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRun_NamesTheFailedStepByNumberAndIntent(t *testing.T) {
	boom := errors.New("name taken")
	var ran bool
	s := scenario.Scenario{
		Name: "stub",
		Steps: []scenario.Step{
			{Intent: "First", Action: func(context.Context, *graph.System, *scenario.Reporter) error { return nil }},
			{Intent: "Make the area", Action: func(context.Context, *graph.System, *scenario.Reporter) error { return boom }},
			{Intent: "Never reached", Action: func(context.Context, *graph.System, *scenario.Reporter) error { ran = true; return nil }},
		},
	}
	var out bytes.Buffer

	err := scenario.Run(context.Background(), s, nil, scenario.NewReporter(&out))

	if !errors.Is(err, boom) {
		t.Fatalf("Run error = %v, want the step's error", err)
	}
	if want := "step 2 (Make the area): name taken"; err.Error() != want {
		t.Errorf("Run error = %q, want %q", err, want)
	}
	if ran {
		t.Error("Run ran a step after one failed")
	}
	if want := "[1/3] First\n\n[2/3] Make the area\n"; out.String() != want {
		t.Errorf("Run wrote %q, want %q: the failed step narrated, the next not", out.String(), want)
	}
}

func TestRun_StopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := scenario.Scenario{
		Name: "stub",
		Steps: []scenario.Step{
			{Intent: "Cancel", Action: func(context.Context, *graph.System, *scenario.Reporter) error { cancel(); return nil }},
			{Intent: "Never narrated"},
		},
	}
	var out bytes.Buffer

	err := scenario.Run(ctx, s, nil, scenario.NewReporter(&out))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want context.Canceled", err)
	}
	if want := "[1/2] Cancel\n"; out.String() != want {
		t.Errorf("Run wrote %q, want %q", out.String(), want)
	}
}
