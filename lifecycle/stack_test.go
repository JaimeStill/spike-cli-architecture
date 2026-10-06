package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
)

// patience bounds every wait a test makes on another goroutine, so a broken
// lifecycle fails the test instead of hanging it.
const patience = 5 * time.Second

// recorder collects events, in the order they happen.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// step returns a Stack step whose Start and Stop record "start name" and
// "stop name" and succeed.
func (r *recorder) step(name string) lifecycle.Step {
	return lifecycle.Step{
		Name: name,
		Start: func(context.Context) error {
			r.record("start " + name)
			return nil
		},
		Stop: func(context.Context) error {
			r.record("stop " + name)
			return nil
		},
	}
}

// await fails the test unless ch closes within patience.
func await(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(patience):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// rendezvous returns two functions, each of which marks its side begun and
// waits for the other's, failing with an error when the other never begins:
// the pair completes only when both run at once.
func rendezvous() (a, b func() error) {
	aBegun, bBegun := make(chan struct{}), make(chan struct{})
	meet := func(mine chan struct{}, theirs <-chan struct{}) func() error {
		return func() error {
			close(mine)
			select {
			case <-theirs:
				return nil
			case <-time.After(patience):
				return errors.New("sibling never began")
			}
		}
	}
	return meet(aBegun, bBegun), meet(bBegun, aBegun)
}

// sameSet reports whether got holds exactly want, in any order.
func sameSet(got []string, want ...string) bool {
	return len(got) == len(want) &&
		slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want)))
}

// The Stack tests cover only what internal/app relies on until it moves onto
// the Coordinator: phases unwind in reverse, a failed Start is labelled and
// pushes nothing, and Unwind empties the stack for another run.

func TestStackUnwindsPhasesInReverse(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	for _, name := range []string{"a", "b"} {
		if err := s.Start(context.Background(), r.step(name)); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
	want := []string{"start a", "start b", "stop b", "stop a"}
	if got := r.list(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestStackFailedStartPushesNothing(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	errBoom := errors.New("boom")
	err := s.Start(context.Background(), lifecycle.Step{
		Name:  "dep",
		Start: func(context.Context) error { return errBoom },
		Stop: func(context.Context) error {
			r.record("stop dep")
			return nil
		},
	})
	if !errors.Is(err, errBoom) || err.Error() != "dep: boom" {
		t.Fatalf("Start = %v, want dep: boom", err)
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
	if got := r.list(); len(got) != 0 {
		t.Errorf("events = %q, want the failed step not stopped", got)
	}
}

func TestStackUnwindEmptiesStack(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	if err := s.Start(context.Background(), r.step("dep")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("first Unwind: %v", err)
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("second Unwind: %v", err)
	}
	want := []string{"start dep", "stop dep"}
	if got := r.list(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}
