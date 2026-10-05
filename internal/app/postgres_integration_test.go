//go:build integration

package app_test

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"
	godatabase "github.com/standards-lab/go-database"
)

// The real Postgres opener against the compose stack: `mise run up`, then
// `mise run integration`, which sets BLOBFS_DATABASE_* to the stack's
// Postgres.

func TestPostgresIntegration_BringsUpAndCloses(t *testing.T) {
	var out, errOut bytes.Buffer

	code := postgresProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q (is the stack up? mise run up)", code, process.ExitOK, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}

func TestPostgresIntegration_UnreachableFailsBringUp(t *testing.T) {
	env := godatabase.NewEnv("BLOBFS")
	t.Setenv(env.Host, "127.0.0.1")
	t.Setenv(env.Port, strconv.Itoa(closedPort(t)))
	t.Setenv(env.ConnTimeout, "2s")
	var out, errOut bytes.Buffer

	code := postgresProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	// One report, under the command's path: the line the dispatcher
	// writes, followed only by pgx's own continuation lines, which list
	// each dial attempt indented by a tab.
	stderr := errOut.String()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if want := "blobfs probe run: postgres: "; !strings.HasPrefix(lines[0], want) {
		t.Errorf("stderr = %q, want its first line under %q", stderr, want)
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "\t") {
			t.Errorf("stderr = %q, want one report: line %q starts another", stderr, line)
		}
	}
	if !strings.Contains(stderr, "connection refused") {
		t.Errorf("stderr = %q, want the refused connection", stderr)
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
