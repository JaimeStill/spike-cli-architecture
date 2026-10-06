//go:build integration

package app_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"
	godatabase "github.com/standards-lab/go-database"
	"github.com/standards-lab/go-storage"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
)

// The real database and store against the compose stack: `mise run
// integration` starts its own isolated project, sets BLOBFS_DATABASE_* and
// BLOBFS_STORAGE_* to that project's services, leaves
// BLOBFS_SHUTDOWN_TIMEOUT at its default, and tears the project down when
// the suite ends.

// probeApp returns blobfs with a test-only command, "probe", that declares
// the database and the store with Use, which no production command
// combines yet. The database and store nodes are Replace-d by their real
// constructors, wrapped to record each value's Start and Shutdown into r,
// and the store is ordered after the database, so the two, one layer in
// production, start and shut down in an order the test can assert.
func probeApp(r *recorder, stdout, stderr *bytes.Buffer) *app.App {
	a := app.New(strings.NewReader(""), stdout, stderr)
	g, n := a.Graph(), a.Nodes()
	g.Replace(n.Database, func(s *graph.Scope) (*godatabase.DB, error) {
		db, err := a.NewDatabase(s)
		if err != nil {
			return nil, err
		}
		recordLifecycle(s, r, "database", db.Start, db.Shutdown)
		return db, nil
	})
	g.Replace(n.Store, func(s *graph.Scope) (*storage.Store, error) {
		s.After(n.Database)
		st, err := a.NewStore(s)
		if err != nil {
			return nil, err
		}
		recordLifecycle(s, r, "store", st.Start, st.Shutdown)
		return st, nil
	})
	a.Root().Add((&cli.Command{
		Name:    "probe",
		Summary: "Start and shut down the database and the store",
		Args:    cli.NoArgs,
		Run:     func(context.Context, *cli.Invocation) error { return nil },
	}).Use(n.Database, n.Store))
	return a
}

// recordLifecycle records hooks that run the value's own start and
// shutdown and record each on r, with " failed" on an error. A hook
// overrides the value's method, so the lifecycle runs these in its place.
func recordLifecycle(s *graph.Scope, r *recorder, name string, start, shutdown func(context.Context) error) {
	s.OnStart(func(ctx context.Context) error {
		err := start(ctx)
		r.record(event("start "+name, err))
		return err
	})
	s.OnShutdown(func(ctx context.Context) error {
		err := shutdown(ctx)
		r.record(event("shutdown "+name, err))
		return err
	})
}

func event(name string, err error) string {
	if err != nil {
		return name + " failed"
	}
	return name
}

// oneReport fails t unless stderr is one report whose first line starts
// with prefix: no later line starts a report of its own.
func oneReport(t *testing.T, stderr, prefix string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if !strings.HasPrefix(lines[0], prefix) {
		t.Errorf("stderr = %q, want its first line under %q", stderr, prefix)
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "blobfs ") {
			t.Errorf("stderr = %q, want one report: line %q starts another", stderr, line)
		}
	}
	if !strings.Contains(stderr, "connection refused") {
		t.Errorf("stderr = %q, want the refused connection", stderr)
	}
}

func TestInfrastructureIntegration_StartsBothAndShutsDownInReverse(t *testing.T) {
	r := &recorder{}
	var out, errOut bytes.Buffer

	code := probeApp(r, &out, &errOut).Run(context.Background(), []string{"probe"})

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q", code, process.ExitOK, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
	want := []string{"start database", "start store", "shutdown store", "shutdown database"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestInfrastructureIntegration_StoreUnreachableClosesTheDatabase(t *testing.T) {
	env := storage.NewEnv("BLOBFS")
	t.Setenv(env.Endpoint, fmt.Sprintf("http://127.0.0.1:%s/devstoreaccount1", strconv.Itoa(closedPort(t))))
	t.Setenv(env.RequestTimeout, "5s")
	// One try: the SDK's default retries back off for seconds against a
	// port that refuses at once.
	t.Setenv(env.Options+"_MAX_RETRIES", "0")
	r := &recorder{}
	var out, errOut bytes.Buffer

	code := probeApp(r, &out, &errOut).Run(context.Background(), []string{"probe"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	oneReport(t, errOut.String(), "blobfs probe: store: ")
	// The store, constructed though it failed to start, is shut down, and
	// then the database it started after.
	want := []string{"start database", "start store failed", "shutdown store", "shutdown database"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestInfrastructureIntegration_DatabaseUnreachable(t *testing.T) {
	env := godatabase.NewEnv("BLOBFS")
	t.Setenv(env.Host, "127.0.0.1")
	t.Setenv(env.Port, strconv.Itoa(closedPort(t)))
	t.Setenv(env.ConnTimeout, "2s")
	r := &recorder{}
	var out, errOut bytes.Buffer

	code := probeApp(r, &out, &errOut).Run(context.Background(), []string{"probe"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	// pgx's own continuation lines list each dial attempt, indented by a
	// tab, after the dispatcher's line.
	oneReport(t, errOut.String(), "blobfs probe: database: ")
	// The store's layer never began to start, so only the database is
	// shut down.
	want := []string{"start database failed", "shutdown database"}
	if got := r.log(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

// closedPort returns a loopback port nothing listens on: one the kernel
// just handed out and that this test has closed again.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
