package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Step is one participant in a phase: a Name that labels its errors, and an
// optional Start and Stop. A nil Start counts as started, so a stop-only step
// is always unwound; a nil Stop is skipped.
type Step struct {
	Name        string
	Start, Stop func(context.Context) error
}

// Stack records the phases that started, in order, and unwinds them in
// reverse. The zero value is ready to use, and a Stack is safe for
// concurrent use.
type Stack struct {
	mu     sync.Mutex
	phases [][]Step
}

// Start runs one phase: steps concurrently (a single step runs on the
// caller's goroutine), each under a child of ctx that the phase's first
// failure cancels. It pushes the steps whose Start succeeded as a new phase
// for [Stack.Unwind], pushing nothing when none did, and returns the
// failures joined, each labelled "name: err". An error wrapping
// context.Canceled that arrives once a failure is on record is dropped: it
// is that failure's consequence. Start may be called any number of times.
func (s *Stack) Start(ctx context.Context, steps ...Step) error {
	phaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu      sync.Mutex
		errs    []error
		started = make([]bool, len(steps))
	)
	run := func(i int) {
		step := steps[i]
		if step.Start == nil {
			started[i] = true
			return
		}
		err := step.Start(phaseCtx)
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
		errs = append(errs, fmt.Errorf("%s: %w", step.Name, err))
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

	var phase []Step
	for i, step := range steps {
		if started[i] {
			phase = append(phase, step)
		}
	}
	if len(phase) > 0 {
		s.mu.Lock()
		s.phases = append(s.phases, phase)
		s.mu.Unlock()
	}
	return errors.Join(errs...)
}

// Unwind stops every pushed phase, the last first, each phase's steps
// concurrently, under one context derived from context.Background and
// bounded by timeout, so cleanup has its whole budget whatever became of the
// contexts Start ran under. Stop errors are labelled "name: err" and joined.
// The first phase that outlives the deadline adds one error wrapping
// context.DeadlineExceeded; its unfinished Stops continue on the expired
// context and their late errors are dropped. Each remaining phase still
// starts its Stops on the expired context, and Unwind does not wait for
// them. Unwind leaves the stack empty, so a second call returns nil.
func (s *Stack) Unwind(timeout time.Duration) error {
	s.mu.Lock()
	phases := s.phases
	s.phases = nil
	s.mu.Unlock()
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
		for _, step := range phase {
			if step.Stop == nil {
				continue
			}
			wg.Go(func() {
				if err := step.Stop(ctx); err != nil {
					mu.Lock()
					phaseErr = append(phaseErr, fmt.Errorf("%s: %w", step.Name, err))
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
				"unwind timeout after %v: %w", timeout, ctx.Err(),
			))
		}
	}
	return errors.Join(errs...)
}
