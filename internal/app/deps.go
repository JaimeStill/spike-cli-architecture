package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	godatabase "github.com/standards-lab/go-database"
	"github.com/standards-lab/go-database/postgres"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
)

// envPrefix is the prefix of every environment variable blobfs reads its
// dependency configuration from: the libraries compose the rest, such as
// BLOBFS_DATABASE_HOST.
const envPrefix = "BLOBFS"

// unwindTimeout bounds the closing of a run's dependencies. It runs under a
// context detached from the signal context, so a cancelled run still gets
// the whole budget to close what it opened.
const unwindTimeout = 10 * time.Second

// dependency names one member of the closed set of dependencies a command
// group can declare. The constants' order is the order they open in, and
// their reverse is the order they close in.
type dependency int

const (
	depPostgres dependency = iota // the Postgres database
	depStore                      // the object store

	numDependencies = iota
)

// service is the lifecycle both dependencies share: go-database's *DB and
// go-storage's *Store each start (a ping, a probe) and shut down.
type service interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// database is the Postgres dependency as command bodies see it: go-database's
// *DB behind it.
type database interface {
	service
}

// objectStore is the object store dependency as command bodies see it. A
// later slice holds go-storage's *Store behind it.
type objectStore interface {
	service
}

// openers construct each dependency, unstarted. An opener is where the
// dependency's configuration is read, so configuration is read only when a
// command first asks for the dependency, never in [New] and never for a run
// that opens nothing.
type openers struct {
	postgres func() (database, error)
	store    func() (objectStore, error)
}

// errNoOpener is what a dependency without an opener yet fails to come up
// with: the object store, until a later slice adds its real opener.
var errNoOpener = errors.New("no opener in this build")

// defaultOpeners returns the production openers.
func defaultOpeners() openers {
	return openers{
		postgres: openPostgres,
		store:    func() (objectStore, error) { return nil, errNoOpener },
	}
}

// openPostgres reads the database configuration from the environment alone,
// BLOBFS_DATABASE_HOST, _PORT, _NAME, _USER, _PASSWORD, and the pool and
// timeout settings go-database names, and constructs the pool. It does no
// I/O: the pool first connects in Start, the ping bounded by the
// configuration's conn_timeout.
func openPostgres() (database, error) {
	var cfg godatabase.Config
	if err := cfg.Finalize(envPrefix); err != nil {
		return nil, err
	}
	db, err := postgres.New(cfg)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// initializer brings up the dependencies a run's command declared, on its
// first request and at most once per run, and closes them when the command
// returns. Each dependency opens in its own phase of the Stack, in the fixed
// order of the dependency constants, so the Stack unwinds them in reverse.
type initializer struct {
	openers openers

	mu        sync.Mutex
	stack     lifecycle.Stack
	attempted bool  // bring-up ran this run
	err       error // bring-up's failure, returned to every later request
	db        database
	store     objectStore
}

// bringUp opens the dependencies need marks, unless this run already tried,
// and returns the bring-up's failure. A failure leaves the dependencies
// opened before it on the Stack, for [initializer.unwind] to close.
func (in *initializer) bringUp(ctx context.Context, need *[numDependencies]bool) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.attempted {
		return in.err
	}
	in.attempted = true
	if need[depPostgres] {
		if in.err = in.stack.Start(ctx, step("postgres", in.openers.postgres, &in.db)); in.err != nil {
			return in.err
		}
	}
	if need[depStore] {
		in.err = in.stack.Start(ctx, step("object store", in.openers.store, &in.store))
	}
	return in.err
}

// step returns the Stack step for one dependency: its Start calls open,
// which reads the configuration and constructs the dependency, then starts
// it and records it in *dst; its Stop shuts it down.
//
// A dependency that is constructed but fails to start is shut down within
// Start itself, since the Stack keeps only the steps that started and would
// never stop it: a failed ping must not leak the pool New built. That
// shutdown runs under its own unwindTimeout budget, detached from ctx's
// cancellation, and its error is dropped, so the Start failure stays the
// one error the run reports.
func step[T service](name string, open func() (T, error), dst *T) lifecycle.Step {
	var svc T
	return lifecycle.Step{
		Name: name,
		Start: func(ctx context.Context) error {
			s, err := open()
			if err != nil {
				return err
			}
			if err := s.Start(ctx); err != nil {
				stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unwindTimeout)
				defer cancel()
				_ = s.Shutdown(stopCtx)
				return err
			}
			svc, *dst = s, s
			return nil
		},
		Stop: func(ctx context.Context) error {
			return svc.Shutdown(ctx)
		},
	}
}

// unwind closes whatever this run opened, the last first, and resets the
// initializer for another run. Its error carries the "shutdown:" prefix, so
// a failed close reads apart from a failed open.
func (in *initializer) unwind() error {
	in.mu.Lock()
	defer in.mu.Unlock()
	err := in.stack.Unwind(unwindTimeout)
	in.attempted, in.err, in.db, in.store = false, nil, nil, nil
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// deps is a command group's handle on the dependencies it declared at its
// mount. The group wraps each leaf's body in [deps.Run], and the body asks
// for a dependency with [deps.Postgres] or [deps.Store]; the first request
// brings up the group's whole declared set.
type deps struct {
	init *initializer
	need [numDependencies]bool
}

// runningKey marks a context as inside a [deps.Run] body, the only place a
// dependency can be requested, since only there is it closed afterwards.
type runningKey struct{}

// Run wraps a leaf's body for cli.Command.Run. After the body returns, Run
// closes the dependencies it opened, and joins a close error with the
// body's result, so the dispatcher reports both once, under the command's
// path, and a successful body followed by a failed close exits 1. The
// close runs on success, on failure, and when ctx was cancelled mid-body.
func (d *deps) Run(body func(ctx context.Context, inv *cli.Invocation) error) func(context.Context, *cli.Invocation) error {
	return func(ctx context.Context, inv *cli.Invocation) error {
		err := body(context.WithValue(ctx, runningKey{}, true), inv)
		return errors.Join(err, d.init.unwind())
	}
}

// Postgres returns the Postgres database, bringing up the group's declared
// dependencies on the run's first request. Its error is the bring-up's
// failure, which the body returns as its own. It panics, as a wiring
// mistake, when the group did not declare Postgres or ctx is not a
// [deps.Run] body's.
func (d *deps) Postgres(ctx context.Context) (database, error) {
	if err := d.request(ctx, depPostgres, "Postgres"); err != nil {
		return nil, err
	}
	return d.init.db, nil
}

// Store returns the object store, as [deps.Postgres] returns the database.
func (d *deps) Store(ctx context.Context) (objectStore, error) {
	if err := d.request(ctx, depStore, "the object store"); err != nil {
		return nil, err
	}
	return d.init.store, nil
}

// request checks that dep may be asked for here, then brings up the
// declared set.
func (d *deps) request(ctx context.Context, dep dependency, name string) error {
	if !d.need[dep] {
		panic(fmt.Sprintf("app: %s requested by a command group that did not declare it", name))
	}
	if ctx.Value(runningKey{}) == nil {
		panic(fmt.Sprintf("app: %s requested outside a deps.Run body", name))
	}
	return d.init.bringUp(ctx, &d.need)
}

// mount builds a command group with a handle on the dependencies it
// declares, need, and attaches it under the root. The declaration lives
// here, at the mount, so the composition root lists what each group opens.
func (a *App) mount(build func(*deps) *cli.Command, need ...dependency) {
	d := &deps{init: a.init}
	for _, dep := range need {
		d.need[dep] = true
	}
	a.root.Add(build(d))
}
