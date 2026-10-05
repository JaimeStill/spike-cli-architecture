package app_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"
	"github.com/standards-lab/go-storage"

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
)

// The real object store opener, driven through App.Run. These tests are
// hermetic: they clear every BLOBFS_STORAGE_* and BLOBFS_DATABASE_*
// variable themselves, so the compose stack's defaults in mise's
// environment never reach them. The stack-backed tests are in
// store_integration_test.go.

// clearStorageEnv empties every variable the object store opener reads,
// which go-storage treats as unset, for the duration of t: the named
// settings and every provider option under the options prefix.
func clearStorageEnv(t *testing.T) {
	t.Helper()
	e := storage.NewEnv("BLOBFS")
	for _, name := range []string{
		e.Endpoint, e.Container, e.Account, e.Key,
		e.MaxObjectSize, e.ListPageSize, e.RequestTimeout, e.ReadIdleTimeout,
	} {
		t.Setenv(name, "")
	}
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, e.Options+"_") {
			t.Setenv(name, "")
		}
	}
}

// storeProbeApp returns blobfs with its production openers and a probe
// group, "probe" with one leaf "run", declaring the object store. The leaf
// asks for the store and returns the bring-up's error.
func storeProbeApp(stdout, stderr *bytes.Buffer) *app.App {
	a := app.New(stdout, stderr)
	app.Mount(a, func(d *app.Deps) *cli.Command {
		return (&cli.Command{Name: "probe", Summary: "Probe dependencies"}).Add(&cli.Command{
			Name:    "run",
			Summary: "Run the probe",
			Args:    cli.NoArgs,
			Run: d.Run(func(ctx context.Context, _ *cli.Invocation) error {
				_, err := d.Store(ctx)
				return err
			}),
		})
	}, app.DepStore)
	return a
}

func TestStore_EmptyEnvironmentFailsBringUp(t *testing.T) {
	clearDatabaseEnv(t)
	clearStorageEnv(t)
	var out, errOut bytes.Buffer

	code := storeProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	if want := "blobfs probe run: object store: storage: container is required\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestStore_ConfigurationReadOnlyOnRequest(t *testing.T) {
	// An unparseable size fails the opener with exit 1, so a run that
	// exits otherwise never read the configuration.
	clearDatabaseEnv(t)
	clearStorageEnv(t)
	t.Setenv(storage.NewEnv("BLOBFS").MaxObjectSize, "not-a-size")
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

			code := storeProbeApp(&out, &errOut).Run(context.Background(), tt.args)

			if code != tt.wantCode {
				t.Errorf("code = %d, want %d; stderr = %q", code, tt.wantCode, errOut.String())
			}
		})
	}

	t.Run("request", func(t *testing.T) {
		var out, errOut bytes.Buffer

		code := storeProbeApp(&out, &errOut).Run(context.Background(), []string{"probe", "run"})

		if code != process.ExitFailure {
			t.Errorf("code = %d, want %d: the request should read the bad size", code, process.ExitFailure)
		}
		if want := "BLOBFS_STORAGE_MAX_OBJECT_SIZE"; !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr = %q, want it to name %s", errOut.String(), want)
		}
	})
}
