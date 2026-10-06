package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// step is one participant in a phase: a name that labels its errors, and an
// optional start and stop. A nil start counts as started, so a stop-only step
// is always unwound; a nil stop is skipped.
type step struct {
	name        string
	start, stop func(context.Context) error
}

// engine records the phases that started, in order, and unwinds them in
// reverse. It is the executor under [Coordinator] and the exported [Stack].
// The zero value is ready to use, and an engine is safe for concurrent use.
type engine struct {
	// unwindFailed pushes a failing phase's every step, not only those
	// whose start succeeded, so a step whose start failed is still stopped.
	unwindFailed bool

	mu     sync.Mutex
	phases [][]step
}

// start runs one phase: steps concurrently (a single step runs on the
// caller's goroutine), each under a child of ctx that the phase's first
// failure cancels. It pushes the steps whose start succeeded as a new phase
// for unwind, or every step when unwindFailed is set and one failed, pushing
// nothing when none qualifies, and returns the failures, each labelled
// "name: err". An error wrapping context.Canceled that arrives once a failure
// is on record is dropped: it is that failure's consequence.
func (e *engine) start(ctx context.Context, steps []step) []error {
	phaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu      sync.Mutex
		errs    []error
		started = make([]bool, len(steps))
	)
	run := func(i int) {
		s := steps[i]
		if s.start == nil {
			started[i] = true
			return
		}
		err := s.start(phaseCtx)
		if err == nil {
			started[i] = true
			return
		}
		// The check and the record hold one lock, so two failures cannot
		// both see none on record.
		mu.Lock()
		defer mu.Unlock()
		if errors.Is(err, context.Canceled) && phaseCtx.Err() != nil && len(errs) > 0 {
			return
		}
		errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
		cancel()
	}

	if len(steps) == 1 {
		run(0)
	} else {
		// Each goroutine writes only its own started index, read after Wait.
		var wg sync.WaitGroup
		for i := range steps {
			wg.Go(func() { run(i) })
		}
		wg.Wait()
	}

	var phase []step
	for i, s := range steps {
		if started[i] || e.unwindFailed {
			phase = append(phase, s)
		}
	}
	if len(phase) > 0 {
		e.mu.Lock()
		e.phases = append(e.phases, phase)
		e.mu.Unlock()
	}
	return errs
}

// unwind stops every pushed phase, the last first, each phase's steps
// concurrently, under one context derived from context.Background and
// bounded by timeout, so cleanup has its whole budget whatever became of the
// contexts start ran under. Stop errors are labelled "name: err". The first
// phase that outlives the deadline adds one error wrapping
// context.DeadlineExceeded; its unfinished stops continue on the expired
// context and their late errors are dropped. Each remaining phase still
// starts its stops on the expired context, and unwind does not wait for
// them. unwind leaves the engine empty, so a second call returns nothing.
func (e *engine) unwind(timeout time.Duration) []error {
	e.mu.Lock()
	phases := e.phases
	e.phases = nil
	e.mu.Unlock()
	if len(phases) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var errs []error
	timedOut := false
	for _, phase := range slices.Backward(phases) {
		var (
			mu       sync.Mutex
			phaseErr []error
			wg       sync.WaitGroup
		)
		for _, s := range phase {
			if s.stop == nil {
				continue
			}
			wg.Go(func() {
				if err := s.stop(ctx); err != nil {
					mu.Lock()
					phaseErr = append(phaseErr, fmt.Errorf("%s: %w", s.name, err))
					mu.Unlock()
				}
			})
		}
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		finished := true
		select {
		case <-done:
		case <-ctx.Done():
			select {
			case <-done:
			default:
				finished = false
			}
		}
		// A phase cut short contributes what it recorded by the deadline;
		// its stragglers write to phaseErr after this snapshot, unread.
		mu.Lock()
		errs = append(errs, phaseErr...)
		mu.Unlock()
		if !finished && !timedOut {
			timedOut = true
			errs = append(errs, fmt.Errorf(
				"timeout after %v: %w", timeout, ctx.Err(),
			))
		}
	}
	return errs
}
