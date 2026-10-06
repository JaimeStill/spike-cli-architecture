package graph

import (
	"context"
	"fmt"
	"slices"
)

// Scope is what a constructor receives: the means to reach its
// dependencies and to record its hooks. It is valid only while the
// constructor runs; every method panics once the constructor has returned.
type Scope struct {
	build *build
	entry *entry
	done  bool
}

// check panics when the constructor that received s has returned.
func (s *Scope) check(op string) {
	if s.done {
		panic(fmt.Sprintf("graph: %s called after the constructor of %q returned", op, s.entry.node.name))
	}
}

// Use returns n's value, building n first when this Build has not, and
// records that the node under construction depends on n. When n's
// constructor fails, Use does not return: it aborts the calling constructor,
// and Build reports n's error. Use panics when n was defined on another
// Graph, when n is under construction (a cycle, named by its path), or
// after the constructor has returned.
func (s *Scope) Use[T any](n *Node[T]) T {
	s.check("Use")
	core := s.build.resolve(n, "Use")
	if err := s.build.construct(core); err != nil {
		panic(abort{err: err})
	}
	if !slices.Contains(s.entry.uses, core) {
		s.entry.uses = append(s.entry.uses, core)
	}
	// The comma-ok form returns the zero T for a nil value of an interface
	// type, which a plain assertion would panic on.
	v, _ := s.build.entries[core].value.(T)
	return v
}

// After orders the node under construction after r: it starts after r and
// stops before it. It passes no value and never builds r; the edge holds
// only when r is in the System, as a root or a node some Use reached, and
// is dropped otherwise. After panics when r is nil
// or was defined on another Graph, or after the constructor has returned.
func (s *Scope) After(r Ref) {
	s.check("After")
	core := s.build.resolve(r, "After")
	if !slices.Contains(s.entry.after, core) {
		s.entry.after = append(s.entry.after, core)
	}
}

// OnStart records fn as the start hook of the node under construction,
// carried on its [Dependency]. It panics when the node has a start hook
// already, or after the constructor has returned.
func (s *Scope) OnStart(fn func(context.Context) error) {
	s.check("OnStart")
	if s.entry.onStart != nil {
		panic(fmt.Sprintf("graph: OnStart called twice for %q", s.entry.node.name))
	}
	s.entry.onStart = fn
}

// OnShutdown records fn as the shutdown hook of the node under
// construction, carried on its [Dependency]. It panics when the node has a
// shutdown hook already, or after the constructor has returned.
func (s *Scope) OnShutdown(fn func(context.Context) error) {
	s.check("OnShutdown")
	if s.entry.onShutdown != nil {
		panic(fmt.Sprintf("graph: OnShutdown called twice for %q", s.entry.node.name))
	}
	s.entry.onShutdown = fn
}
