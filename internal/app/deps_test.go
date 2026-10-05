package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
	"github.com/standards-lab/go-core/process"
)

// recorder is a pair of fake openers and the services they open. It records
// every opener call (where configuration is read), Start, and Shutdown, in
// order, and fails whichever the test sets an error for.
type recorder struct {
	mu        sync.Mutex
	events    []string
	stopCtxOK []bool // whether each Shutdown's context was still live

	openErr, startErr, closeErr map[string]error
}

func newRecorder() *recorder {
	return &recorder{
		openErr:  map[string]error{},
		startErr: map[string]error{},
		closeErr: map[string]error{},
	}
}

func (r *recorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) log() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// fakeService is one opened dependency.
type fakeService struct {
	r    *recorder
	name string
	pool *sql.DB // the pool Conn returns, made on first use
}

// errNoConnection is what every connection the fake database's pool dials
// fails with.
var errNoConnection = errors.New("fake database: no connection")

// failingConnector is a database/sql connector whose every dial fails, so a
// body handed the fake database's pool gets as far as its first query.
type failingConnector struct{}

func (failingConnector) Connect(context.Context) (driver.Conn, error) { return nil, errNoConnection }
func (failingConnector) Driver() driver.Driver                        { return failingDriver{} }

type failingDriver struct{}

func (failingDriver) Open(string) (driver.Conn, error) { return nil, errNoConnection }

// Conn returns a pool whose every query fails with errNoConnection. Shutdown
// closes it.
func (f *fakeService) Conn() *sql.DB {
	if f.pool == nil {
		f.pool = sql.OpenDB(failingConnector{})
	}
	return f.pool
}

func (f *fakeService) Start(context.Context) error {
	f.r.record("start " + f.name)
	return f.r.startErr[f.name]
}

func (f *fakeService) Shutdown(ctx context.Context) error {
	f.r.record("shutdown " + f.name)
	if f.pool != nil {
		_ = f.pool.Close()
	}
	f.r.mu.Lock()
	f.r.stopCtxOK = append(f.r.stopCtxOK, ctx.Err() == nil)
	f.r.mu.Unlock()
	return f.r.closeErr[f.name]
}

func (r *recorder) open(name string) (*fakeService, error) {
	r.record("open " + name)
	if err := r.openErr[name]; err != nil {
		return nil, err
	}
	return &fakeService{r: r, name: name}, nil
}

// probeApp returns blobfs with r's openers and a probe group, "probe" with
// one leaf "run", declaring need. The leaf takes no arguments and runs body
// inside the group's deps.Run.
func probeApp(r *recorder, stdout, stderr *bytes.Buffer, body func(context.Context, *app.Deps) error, need ...app.Dependency) *app.App {
	a := app.New(stdout, stderr)
	app.SetOpeners(a,
		func() (app.Database, error) { return r.open("postgres") },
		func() (app.Store, error) { return r.open("store") },
	)
	app.Mount(a, func(d *app.Deps) *cli.Command {
		return (&cli.Command{Name: "probe", Summary: "Probe dependencies"}).Add(&cli.Command{
			Name:    "run",
			Summary: "Run the probe",
			Args:    cli.NoArgs,
			Run: d.Run(func(ctx context.Context, _ *cli.Invocation) error {
				return body(ctx, d)
			}),
		})
	}, need...)
	return a
}

// askBoth asks for Postgres and then the object store, returning the first
// failure.
func askBoth(ctx context.Context, d *app.Deps) error {
	if _, err := d.Postgres(ctx); err != nil {
		return err
	}
	_, err := d.Store(ctx)
	return err
}

var fullCycle = []string{
	"open postgres", "start postgres",
	"open store", "start store",
	"shutdown store", "shutdown postgres",
}

func TestRun_OpensNothingWithoutARequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"root --help", []string{"--help"}},
		{"leaf --help", []string{"probe", "run", "--help"}},
		{"parent alone", []string{"probe"}},
		{"unknown command", []string{"probe", "bogus"}},
		{"usage error", []string{"probe", "run", "extra"}},
		{"version", []string{"version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRecorder()
			var out, errOut bytes.Buffer
			a := probeApp(r, &out, &errOut, askBoth, app.DepPostgres, app.DepStore)

			a.Run(context.Background(), tt.args)

			if got := r.log(); len(got) != 0 {
				t.Errorf("events = %q, want no opener called", got)
			}
		})
	}
}

func TestRun_OpensDeclaredSetOnceInFixedOrder(t *testing.T) {
	r := newRecorder()
	var out, errOut bytes.Buffer
	// The store is asked for first, and everything twice: the declared set
	// still opens once, Postgres first.
	body := func(ctx context.Context, d *app.Deps) error {
		for range 2 {
			if _, err := d.Store(ctx); err != nil {
				return err
			}
			if _, err := d.Postgres(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	a := probeApp(r, &out, &errOut, body, app.DepPostgres, app.DepStore)

	code := a.Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitOK {
		t.Errorf("code = %d, want %d; stderr = %q", code, process.ExitOK, errOut.String())
	}
	if got := r.log(); !slices.Equal(got, fullCycle) {
		t.Errorf("events = %q, want %q", got, fullCycle)
	}
}

func TestRun_OpensOnlyWhatTheGroupDeclares(t *testing.T) {
	r := newRecorder()
	var out, errOut bytes.Buffer
	body := func(ctx context.Context, d *app.Deps) error {
		_, err := d.Postgres(ctx)
		return err
	}
	a := probeApp(r, &out, &errOut, body, app.DepPostgres)

	code := a.Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitOK {
		t.Errorf("code = %d, want %d", code, process.ExitOK)
	}
	want := []string{"open postgres", "start postgres", "shutdown postgres"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestRun_ClosesInReverse(t *testing.T) {
	tests := []struct {
		name       string
		body       func(ctx context.Context, d *app.Deps, cancel context.CancelFunc) error
		wantCode   int
		wantStderr string
	}{
		{
			name: "success",
			body: func(ctx context.Context, d *app.Deps, _ context.CancelFunc) error {
				return askBoth(ctx, d)
			},
			wantCode: process.ExitOK,
		},
		{
			name: "command error",
			body: func(ctx context.Context, d *app.Deps, _ context.CancelFunc) error {
				if err := askBoth(ctx, d); err != nil {
					return err
				}
				return errors.New("body failed")
			},
			wantCode:   process.ExitFailure,
			wantStderr: "blobfs probe run: body failed\n",
		},
		{
			name: "cancelled mid-command",
			body: func(ctx context.Context, d *app.Deps, cancel context.CancelFunc) error {
				if err := askBoth(ctx, d); err != nil {
					return err
				}
				cancel()
				<-ctx.Done()
				return ctx.Err()
			},
			wantCode:   process.ExitFailure,
			wantStderr: "blobfs probe run: context canceled\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRecorder()
			var out, errOut bytes.Buffer
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := func(ctx context.Context, d *app.Deps) error { return tt.body(ctx, d, cancel) }
			a := probeApp(r, &out, &errOut, body, app.DepPostgres, app.DepStore)

			code := a.Run(ctx, []string{"probe", "run"})

			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if errOut.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantStderr)
			}
			if got := r.log(); !slices.Equal(got, fullCycle) {
				t.Errorf("events = %q, want %q", got, fullCycle)
			}
			if slices.Contains(r.stopCtxOK, false) {
				t.Errorf("a Shutdown ran on a done context: %v", r.stopCtxOK)
			}
		})
	}
}

func TestRun_FailedBringUp(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name       string
		fail       func(r *recorder)
		wantEvents []string
		wantStderr string
	}{
		{
			name: "store configuration",
			fail: func(r *recorder) { r.openErr["store"] = boom },
			wantEvents: []string{
				"open postgres", "start postgres", "open store", "shutdown postgres",
			},
			wantStderr: "blobfs probe run: object store: boom\n",
		},
		{
			// The store was constructed, so it is shut down though it never
			// started, and before the Postgres it came up after.
			name: "store start",
			fail: func(r *recorder) { r.startErr["store"] = boom },
			wantEvents: []string{
				"open postgres", "start postgres", "open store", "start store",
				"shutdown store", "shutdown postgres",
			},
			wantStderr: "blobfs probe run: object store: boom\n",
		},
		{
			name: "postgres start",
			fail: func(r *recorder) { r.startErr["postgres"] = boom },
			wantEvents: []string{
				"open postgres", "start postgres", "shutdown postgres",
			},
			wantStderr: "blobfs probe run: postgres: boom\n",
		},
		{
			// The failed start's shutdown error is dropped: the start
			// failure is the one error reported.
			name: "postgres start and its shutdown",
			fail: func(r *recorder) {
				r.startErr["postgres"] = boom
				r.closeErr["postgres"] = errors.New("close boom")
			},
			wantEvents: []string{
				"open postgres", "start postgres", "shutdown postgres",
			},
			wantStderr: "blobfs probe run: postgres: boom\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRecorder()
			tt.fail(r)
			var out, errOut bytes.Buffer
			body := func(ctx context.Context, d *app.Deps) error {
				if err := askBoth(ctx, d); err != nil {
					return err
				}
				r.record("body continued")
				return nil
			}
			a := probeApp(r, &out, &errOut, body, app.DepPostgres, app.DepStore)

			code := a.Run(context.Background(), []string{"probe", "run"})

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			if errOut.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantStderr)
			}
			if got := r.log(); !slices.Equal(got, tt.wantEvents) {
				t.Errorf("events = %q, want %q", got, tt.wantEvents)
			}
			if slices.Contains(r.stopCtxOK, false) {
				t.Errorf("a Shutdown ran on a done context: %v", r.stopCtxOK)
			}
		})
	}
}

func TestRun_FailedStartShutsDownOnACancelledRun(t *testing.T) {
	r := newRecorder()
	r.startErr["postgres"] = context.Canceled
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	// The run is cancelled before bring-up, as by a signal: the constructed
	// Postgres's shutdown still gets a live context.
	body := func(ctx context.Context, d *app.Deps) error {
		cancel()
		_, err := d.Postgres(ctx)
		return err
	}
	a := probeApp(r, &out, &errOut, body, app.DepPostgres)

	code := a.Run(ctx, []string{"probe", "run"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	want := []string{"open postgres", "start postgres", "shutdown postgres"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
	if !slices.Equal(r.stopCtxOK, []bool{true}) {
		t.Errorf("shutdown contexts live = %v, want [true]", r.stopCtxOK)
	}
}

func TestRun_CloseErrorFailsTheRun(t *testing.T) {
	tests := []struct {
		name       string
		bodyErr    error
		wantStderr string
	}{
		{
			name:       "after success",
			wantStderr: "blobfs probe run: shutdown: postgres: close boom\n",
		},
		{
			name:       "joined with command error",
			bodyErr:    errors.New("body failed"),
			wantStderr: "blobfs probe run: body failed\nshutdown: postgres: close boom\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRecorder()
			r.closeErr["postgres"] = errors.New("close boom")
			var out, errOut bytes.Buffer
			body := func(ctx context.Context, d *app.Deps) error {
				if err := askBoth(ctx, d); err != nil {
					return err
				}
				return tt.bodyErr
			}
			a := probeApp(r, &out, &errOut, body, app.DepPostgres, app.DepStore)

			code := a.Run(context.Background(), []string{"probe", "run"})

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			if errOut.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantStderr)
			}
			if got := r.log(); !slices.Equal(got, fullCycle) {
				t.Errorf("events = %q, want %q", got, fullCycle)
			}
		})
	}
}

// mustPanic runs f and returns the panic message, failing the test if f
// does not panic.
func mustPanic(t *testing.T, f func()) (msg string) {
	t.Helper()
	defer func() {
		v := recover()
		if v == nil {
			t.Fatal("did not panic")
		}
		msg, _ = v.(string)
	}()
	f()
	return ""
}

func TestRun_UndeclaredDependencyPanics(t *testing.T) {
	r := newRecorder()
	var out, errOut bytes.Buffer
	body := func(ctx context.Context, d *app.Deps) error {
		_, err := d.Store(ctx)
		return err
	}
	a := probeApp(r, &out, &errOut, body, app.DepPostgres)

	msg := mustPanic(t, func() { a.Run(context.Background(), []string{"probe", "run"}) })

	if !strings.Contains(msg, "did not declare it") {
		t.Errorf("panic = %q, want it to name the undeclared request", msg)
	}
	if got := r.log(); len(got) != 0 {
		t.Errorf("events = %q, want no opener called", got)
	}
}

func TestRun_RequestOutsideDepsRunPanics(t *testing.T) {
	r := newRecorder()
	var out, errOut bytes.Buffer
	a := probeApp(r, &out, &errOut, askBoth, app.DepPostgres)
	app.Mount(a, func(d *app.Deps) *cli.Command {
		return &cli.Command{
			Name:    "bare",
			Summary: "Ask without deps.Run",
			Run: func(ctx context.Context, _ *cli.Invocation) error {
				_, err := d.Postgres(ctx)
				return err
			},
		}
	}, app.DepPostgres)

	msg := mustPanic(t, func() { a.Run(context.Background(), []string{"bare"}) })

	if !strings.Contains(msg, "outside a deps.Run body") {
		t.Errorf("panic = %q, want it to name the unwrapped request", msg)
	}
	if got := r.log(); len(got) != 0 {
		t.Errorf("events = %q, want no opener called", got)
	}
}
