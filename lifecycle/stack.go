package lifecycle

import (
	"context"
	"errors"
	"time"
)

// Step is one participant in a [Stack] phase: a Name that labels its errors,
// and an optional Start and Stop. A nil Start counts as started, so a
// stop-only step is always unwound; a nil Stop is skipped.
//
// Step and Stack are the engine the spike's internal/app still runs its
// dependencies on; the composition task's fourth slice moves it onto
// [Coordinator] and removes both. They are not part of the API this package
// proposes for promotion.
type Step struct {
	Name        string
	Start, Stop func(context.Context) error
}

// Stack records the phases that started, in order, and unwinds them in
// reverse. The zero value is ready to use, and a Stack is safe for
// concurrent use. It is the executor under [Coordinator] exposed phase by
// phase, kept only until internal/app stops using it (see [Step]).
type Stack struct {
	e engine
}

// Start runs one phase: steps concurrently, each under a child of ctx that
// the phase's first failure cancels. It pushes the steps whose Start
// succeeded as a new phase for [Stack.Unwind], pushing nothing when none
// did, and returns the failures joined, each labelled "name: err". An error
// wrapping context.Canceled that arrives once a failure is on record is
// dropped: it is that failure's consequence. Start may be called any number
// of times.
func (s *Stack) Start(ctx context.Context, steps ...Step) error {
	converted := make([]step, len(steps))
	for i, st := range steps {
		converted[i] = step{name: st.Name, start: st.Start, stop: st.Stop}
	}
	return errors.Join(s.e.start(ctx, converted)...)
}

// Unwind stops every pushed phase, the last first, each phase's steps
// concurrently, under one context derived from context.Background and
// bounded by timeout. Stop errors are labelled "name: err" and joined; the
// first phase to outlive the deadline adds one error wrapping
// context.DeadlineExceeded, and the remaining phases are still attempted.
// Unwind leaves the stack empty, so a second call returns nil.
func (s *Stack) Unwind(timeout time.Duration) error {
	return errors.Join(s.e.unwind(timeout)...)
}
