package app_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"
	godatabase "github.com/standards-lab/go-database"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
)

// The real Postgres opener, driven through App.Run. These tests are
// hermetic: they set every BLOBFS_DATABASE_* variable themselves, so the
// compose stack's defaults in mise's environment never reach them. The
// stack-backed tests are in postgres_integration_test.go.

// databaseEnv returns every environment variable the Postgres opener reads.
func databaseEnv() []string {
	e := godatabase.NewEnv("BLOBFS")
	return []string{
		e.Host, e.Name, e.User, e.Password, e.Port,
		e.MaxOpenConns, e.MaxIdleConns, e.ConnMaxLifetime, e.ConnMaxIdleTime, e.ConnTimeout,
	}
}

// clearDatabaseEnv empties every variable the Postgres opener reads, which
// go-database treats as unset, for the duration of t.
func clearDatabaseEnv(t *testing.T) {
	t.Helper()
	for _, name := range databaseEnv() {
		t.Setenv(name, "")
	}
}

// postgresProbeApp returns blobfs with its production openers and a probe
// group, "probe" with one leaf "run", declaring Postgres. The leaf asks for
// the database and returns the bring-up's error.
func postgresProbeApp(stdout, stderr *bytes.Buffer) *app.App {
	a := app.New(stdout, stderr)
	app.Mount(a, func(d *app.Deps) *cli.Command {
		return (&cli.Command{Name: "probe", Summary: "Probe dependencies"}).Add(&cli.Command{
			Name:    "run",
			Summary: "Run the probe",
			Args:    cli.NoArgs,
			Run: d.Run(func(ctx context.Context, _ *cli.Invocation) error {
				_, err := d.Postgres(ctx)
				return err
			}),
		})
	}, app.DepPostgres)
	return a
}

func TestPostgres_EmptyEnvironmentFailsBringUp(t *testing.T) {
	clearDatabaseEnv(t)
	var out, errOut bytes.Buffer

	code := postgresProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	if want := "blobfs probe run: postgres: database name required\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestPostgres_ConfigurationReadOnlyOnRequest(t *testing.T) {
	// An unparseable port fails the opener with exit 1, so a run that
	// exits otherwise never read the configuration.
	clearDatabaseEnv(t)
	t.Setenv(godatabase.NewEnv("BLOBFS").Port, "not-a-port")
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"version", []string{"version"}, process.ExitOK},
		{"leaf --help", []string{"probe", "run", "--help"}, process.ExitUsage},
		{"usage error", []string{"probe", "run", "extra"}, process.ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			code := postgresProbeApp(&out, &errOut).Run(context.Background(), tt.args)

			if code != tt.wantCode {
				t.Errorf("code = %d, want %d; stderr = %q", code, tt.wantCode, errOut.String())
			}
		})
	}

	t.Run("request", func(t *testing.T) {
		var out, errOut bytes.Buffer

		code := postgresProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

		if code != process.ExitFailure {
			t.Errorf("code = %d, want %d: the request should read the bad port", code, process.ExitFailure)
		}
		if want := godatabase.NewEnv("BLOBFS").Port; !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr = %q, want it to name %s", errOut.String(), want)
		}
	})
}
