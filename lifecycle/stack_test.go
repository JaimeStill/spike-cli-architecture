package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
)

// patience bounds every wait a test makes on another goroutine, so a broken
// Stack fails the test instead of hanging it.
const patience = 5 * time.Second

// recorder collects the events of recording steps, in the order they happen.
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

// step returns a step whose Start and Stop record "start name" and
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

// unwrap returns the errors joined in err.
func unwrap(t *testing.T, err error) []error {
	t.Helper()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error %v (%T) does not join errors", err, err)
	}
	return joined.Unwrap()
}

func TestUnwindReversesPhases(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	ctx := context.Background()

	for _, phase := range [][]lifecycle.Step{
		{r.step("a")},
		{r.step("b"), r.step("c")},
		{r.step("d")},
	} {
		if err := s.Start(ctx, phase...); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}

	events := r.list()
	if len(events) != 8 {
		t.Fatalf("events = %q, want 8", events)
	}
	starts, stops := events[:4], events[4:]
	if starts[0] != "start a" || starts[3] != "start d" ||
		!sameSet(starts[1:3], "start b", "start c") {
		t.Errorf("starts = %q, want a, {b c}, d", starts)
	}
	if stops[0] != "stop d" || stops[3] != "stop a" ||
		!sameSet(stops[1:3], "stop b", "stop c") {
		t.Errorf("stops = %q, want d, {b c}, a", stops)
	}
}

// sameSet reports whether got holds exactly want, in any order.
func sameSet(got []string, want ...string) bool {
	return len(got) == len(want) &&
		slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want)))
}

func TestStartRunsPhaseConcurrently(t *testing.T) {
	a, b := rendezvous()
	var s lifecycle.Stack
	err := s.Start(context.Background(),
		lifecycle.Step{Name: "a", Start: func(context.Context) error { return a() }},
		lifecycle.Step{Name: "b", Start: func(context.Context) error { return b() }},
	)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestUnwindStopsPhaseConcurrently(t *testing.T) {
	a, b := rendezvous()
	var s lifecycle.Stack
	err := s.Start(context.Background(),
		lifecycle.Step{Name: "a", Stop: func(context.Context) error { return a() }},
		lifecycle.Step{Name: "b", Stop: func(context.Context) error { return b() }},
	)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Unwind(2 * patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
}

func TestStartFailureCancelsSiblings(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	errBoom := errors.New("boom")
	waiting := make(chan struct{})

	err := s.Start(context.Background(),
		lifecycle.Step{
			Name: "bad",
			Start: func(context.Context) error {
				select {
				case <-waiting:
					return errBoom
				case <-time.After(patience):
					return errors.New("sibling never began")
				}
			},
			Stop: func(context.Context) error {
				r.record("stop bad")
				return nil
			},
		},
		lifecycle.Step{
			Name: "slow",
			Start: func(ctx context.Context) error {
				close(waiting)
				select {
				case <-ctx.Done():
					r.record("slow cancelled")
					return ctx.Err()
				case <-time.After(patience):
					return errors.New("never cancelled")
				}
			},
			Stop: func(context.Context) error {
				r.record("stop slow")
				return nil
			},
		},
		lifecycle.Step{
			Name: "free",
			Stop: func(context.Context) error {
				r.record("stop free")
				return nil
			},
		},
	)

	if !errors.Is(err, errBoom) {
		t.Fatalf("Start = %v, want errBoom", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("Start = %v, kept the consequent cancellation", err)
	}
	if errs := unwrap(t, err); len(errs) != 1 || errs[0].Error() != "bad: boom" {
		t.Errorf("Start errors = %q, want [bad: boom]", errs)
	}

	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
	want := []string{"slow cancelled", "stop free"}
	if got := r.list(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestStartJoinsIndependentFailures(t *testing.T) {
	a, b := rendezvous()
	errA, errB := errors.New("a failed"), errors.New("b failed")
	var s lifecycle.Stack
	err := s.Start(context.Background(),
		lifecycle.Step{Name: "a", Start: func(context.Context) error {
			if err := a(); err != nil {
				return err
			}
			return errA
		}},
		lifecycle.Step{Name: "b", Start: func(context.Context) error {
			if err := b(); err != nil {
				return err
			}
			return errB
		}},
	)
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Start = %v, want errA and errB", err)
	}
	got := []string{}
	for _, e := range unwrap(t, err) {
		got = append(got, e.Error())
	}
	if !sameSet(got, "a: a failed", "b: b failed") {
		t.Errorf("Start errors = %q, want both labelled", got)
	}
}

func TestStartKeepsCancellationWithoutFailure(t *testing.T) {
	var s lifecycle.Stack
	err := s.Start(context.Background(), lifecycle.Step{
		Name:  "quit",
		Start: func(context.Context) error { return context.Canceled },
	})
	if !errors.Is(err, context.Canceled) || err.Error() != "quit: context canceled" {
		t.Fatalf("Start = %v, want quit: context canceled", err)
	}
}

func TestStartRunsUnderCallerContext(t *testing.T) {
	var s lifecycle.Stack
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Start(ctx, lifecycle.Step{
		Name:  "dep",
		Start: func(ctx context.Context) error { return ctx.Err() },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want the caller's cancellation", err)
	}
}

func TestNilMembers(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	err := s.Start(context.Background(),
		lifecycle.Step{Name: "start-only", Start: func(context.Context) error {
			r.record("start start-only")
			return nil
		}},
		lifecycle.Step{Name: "stop-only", Stop: func(context.Context) error {
			r.record("stop stop-only")
			return nil
		}},
		lifecycle.Step{Name: "empty"},
	)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
	want := []string{"start start-only", "stop stop-only"}
	if got := r.list(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestUnwindLabelsAndJoinsStopErrors(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	errA, errB := errors.New("a stuck"), errors.New("b stuck")
	failing := func(name string, err error) lifecycle.Step {
		return lifecycle.Step{Name: name, Stop: func(context.Context) error {
			r.record("stop " + name)
			return err
		}}
	}
	ctx := context.Background()
	if err := s.Start(ctx, failing("a", errA)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Start(ctx, failing("b", errB)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err := s.Unwind(patience)
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Unwind = %v, want errA and errB", err)
	}
	errs := unwrap(t, err)
	if len(errs) != 2 || errs[0].Error() != "b: b stuck" || errs[1].Error() != "a: a stuck" {
		t.Errorf("Unwind errors = %q, want [b: b stuck a: a stuck]", errs)
	}
	if got, want := r.list(), []string{"stop b", "stop a"}; !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestUnwindContextOutlivesStartContext(t *testing.T) {
	var s lifecycle.Stack
	ctx, cancel := context.WithCancel(context.Background())
	var stopErr error
	var hasDeadline bool
	err := s.Start(ctx, lifecycle.Step{Name: "dep", Stop: func(ctx context.Context) error {
		stopErr = ctx.Err()
		_, hasDeadline = ctx.Deadline()
		return nil
	}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
	if stopErr != nil || !hasDeadline {
		t.Errorf("Stop context: err %v, deadline %v; want live and bounded", stopErr, hasDeadline)
	}
}

func TestUnwindOverrunStillAttemptsRemainingPhases(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	release := make(chan struct{})
	defer close(release)
	firstStopped := make(chan struct{})
	errLast := errors.New("last failed")

	hung1 := make(chan struct{})
	hang := func(name string, begun chan struct{}) lifecycle.Step {
		return lifecycle.Step{Name: name, Stop: func(context.Context) error {
			r.record("stop " + name)
			close(begun)
			<-release
			return errors.New("late")
		}}
	}
	ctx := context.Background()
	for _, step := range []lifecycle.Step{
		{Name: "first", Stop: func(context.Context) error {
			r.record("stop first")
			close(firstStopped)
			return nil
		}},
		hang("hang1", hung1),
		hang("hang2", make(chan struct{})),
		{Name: "last", Stop: func(context.Context) error {
			r.record("stop last")
			return errLast
		}},
	} {
		if err := s.Start(ctx, step); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	err := s.Unwind(20 * time.Millisecond)
	await(t, firstStopped, "the first phase's Stop")
	await(t, hung1, "hang1's Stop")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Unwind = %v, want a deadline error", err)
	}
	if !errors.Is(err, errLast) {
		t.Errorf("Unwind = %v, want errLast from the phase before the overrun", err)
	}
	deadlines := 0
	for _, e := range unwrap(t, err) {
		if errors.Is(e, context.DeadlineExceeded) {
			deadlines++
		}
		if e.Error() == "hang1: late" || e.Error() == "hang2: late" {
			t.Errorf("Unwind kept a straggler's late error: %v", e)
		}
	}
	if deadlines != 1 {
		t.Errorf("Unwind = %v, want exactly one deadline error, got %d", err, deadlines)
	}
	// Past the deadline Unwind no longer waits on a phase, so hang1 and
	// first may begin in either order; both must begin.
	got := r.list()
	if len(got) != 4 || got[0] != "stop last" || got[1] != "stop hang2" ||
		!sameSet(got[2:], "stop hang1", "stop first") {
		t.Errorf("events = %q, want last, hang2, then {hang1 first}", got)
	}
}

func TestUnwindEmptiesStack(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind of a zero Stack = %v, want nil", err)
	}
	errStop := errors.New("stop failed")
	err := s.Start(context.Background(), lifecycle.Step{Name: "dep", Stop: func(context.Context) error {
		r.record("stop dep")
		return errStop
	}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Unwind(patience); !errors.Is(err, errStop) {
		t.Fatalf("first Unwind = %v, want errStop", err)
	}
	if err := s.Unwind(patience); err != nil {
		t.Errorf("second Unwind = %v, want nil", err)
	}
	if got, want := r.list(), []string{"stop dep"}; !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}

	if err := s.Start(context.Background(), r.step("again")); err != nil {
		t.Fatalf("Start after Unwind: %v", err)
	}
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind after restart: %v", err)
	}
	want := []string{"stop dep", "start again", "stop again"}
	if got := r.list(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestStackConcurrentUse(t *testing.T) {
	var r recorder
	var s lifecycle.Stack
	const n = 16
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if err := s.Start(context.Background(), r.step(fmt.Sprint(i))); err != nil {
				t.Errorf("Start: %v", err)
			}
		})
	}
	for range n / 4 {
		wg.Go(func() {
			if err := s.Unwind(patience); err != nil {
				t.Errorf("Unwind: %v", err)
			}
		})
	}
	wg.Wait()
	if err := s.Unwind(patience); err != nil {
		t.Fatalf("Unwind: %v", err)
	}
	stops := 0
	for _, e := range r.list() {
		if strings.HasPrefix(e, "stop ") {
			stops++
		}
	}
	if stops != n {
		t.Errorf("stopped %d steps, want %d", stops, n)
	}
}
