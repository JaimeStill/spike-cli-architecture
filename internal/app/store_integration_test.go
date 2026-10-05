//go:build integration

package app_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/standards-lab/go-core/process"
	"github.com/standards-lab/go-storage"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
)

// The real object store opener against the compose stack, alongside the
// real Postgres opener: `mise run up`, then `mise run integration`, which
// sets BLOBFS_DATABASE_* and BLOBFS_STORAGE_* to the stack's services.

// liveRecorder wraps the production openers and records each real
// dependency's Start and Shutdown, in order, with their errors, so a test
// sees the order the initializer drives the live services in.
type liveRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *liveRecorder) record(event string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		event += " failed"
	}
	r.events = append(r.events, event)
}

func (r *liveRecorder) log() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// liveDatabase is the real database with its lifecycle recorded.
type liveDatabase struct {
	app.Database
	r *liveRecorder
}

func (d liveDatabase) Start(ctx context.Context) error {
	err := d.Database.Start(ctx)
	d.r.record("start postgres", err)
	return err
}

func (d liveDatabase) Shutdown(ctx context.Context) error {
	err := d.Database.Shutdown(ctx)
	d.r.record("shutdown postgres", err)
	return err
}

// liveStore is the real object store with its lifecycle recorded.
type liveStore struct {
	app.Store
	r *liveRecorder
}

func (s liveStore) Start(ctx context.Context) error {
	err := s.Store.Start(ctx)
	s.r.record("start store", err)
	return err
}

func (s liveStore) Shutdown(ctx context.Context) error {
	err := s.Store.Shutdown(ctx)
	s.r.record("shutdown store", err)
	return err
}

// bothProbeApp returns blobfs with the production openers, wrapped to
// record into r, and a probe group, "probe" with one leaf "run", declaring
// Postgres and the object store. The leaf asks for both and returns the
// first failure.
func bothProbeApp(r *liveRecorder, stdout, stderr *bytes.Buffer) *app.App {
	a := app.New(stdout, stderr)
	app.SetOpeners(a,
		func() (app.Database, error) {
			db, err := app.OpenPostgres()
			if err != nil {
				return nil, err
			}
			return liveDatabase{db, r}, nil
		},
		func() (app.Store, error) {
			st, err := app.OpenStore()
			if err != nil {
				return nil, err
			}
			return liveStore{st, r}, nil
		},
	)
	app.Mount(a, func(d *app.Deps) *cli.Command {
		return (&cli.Command{Name: "probe", Summary: "Probe dependencies"}).Add(&cli.Command{
			Name:    "run",
			Summary: "Run the probe",
			Args:    cli.NoArgs,
			Run: d.Run(func(ctx context.Context, _ *cli.Invocation) error {
				if _, err := d.Postgres(ctx); err != nil {
					return err
				}
				_, err := d.Store(ctx)
				return err
			}),
		})
	}, app.DepPostgres, app.DepStore)
	return a
}

func TestStoreIntegration_BringsUpAndCloses(t *testing.T) {
	var out, errOut bytes.Buffer

	code := storeProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q (is the stack up? mise run up)", code, process.ExitOK, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}

func TestStoreIntegration_BothBroughtUpAndClosedInReverse(t *testing.T) {
	r := &liveRecorder{}
	var out, errOut bytes.Buffer

	code := bothProbeApp(r, &out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q (is the stack up? mise run up)", code, process.ExitOK, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
	want := []string{"start postgres", "start store", "shutdown store", "shutdown postgres"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestStoreIntegration_UnreachableFailsBringUpAndClosesPostgres(t *testing.T) {
	env := storage.NewEnv("BLOBFS")
	t.Setenv(env.Endpoint, fmt.Sprintf("http://127.0.0.1:%s/devstoreaccount1", strconv.Itoa(closedPort(t))))
	t.Setenv(env.RequestTimeout, "5s")
	// One try: the SDK's default retries back off for seconds against a
	// port that refuses at once.
	t.Setenv(env.Options+"_MAX_RETRIES", "0")
	r := &liveRecorder{}
	var out, errOut bytes.Buffer

	code := bothProbeApp(r, &out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	// One report, under the command's path: no other line opens a report
	// of its own.
	stderr := errOut.String()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if want := "blobfs probe run: object store: "; !strings.HasPrefix(lines[0], want) {
		t.Errorf("stderr = %q, want its first line under %q", stderr, want)
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "blobfs ") {
			t.Errorf("stderr = %q, want one report: line %q starts another", stderr, line)
		}
	}
	if !strings.Contains(stderr, "connection refused") {
		t.Errorf("stderr = %q, want the refused connection", stderr)
	}
	// Postgres, opened first, is closed after the store that failed to
	// start is shut down.
	want := []string{"start postgres", "start store failed", "shutdown store", "shutdown postgres"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}
