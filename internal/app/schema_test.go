package app_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"

	"github.com/JaimeStill/spike-cli-architecture/internal/app"
)

// The schema group's dependency declaration, driven through App.Run over
// the recording fakes. The fake database's pool fails every query, so a
// verb that reaches its body brings up the group's declared set and then
// fails at its first statement: the events show what it opened. The
// stack-backed tests are in schema_integration_test.go.

// schemaApp returns blobfs with r's openers and its production groups.
func schemaApp(r *recorder, stdout, stderr *bytes.Buffer) *app.App {
	a := app.New(stdout, stderr)
	app.SetOpeners(a,
		func() (app.Database, error) { return r.open("postgres") },
		func() (app.Store, error) { return r.open("store") },
	)
	return a
}

func TestSchema_VerbsOpenPostgresOnly(t *testing.T) {
	for _, args := range [][]string{
		{"schema", "status"},
		{"schema", "up"},
		{"schema", "down"},
		{"schema", "reset", "--yes"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := newRecorder()
			var out, errOut bytes.Buffer

			code := schemaApp(r, &out, &errOut).Run(context.Background(), args)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d; stderr = %q", code, process.ExitFailure, errOut.String())
			}
			if !strings.Contains(errOut.String(), errNoConnection.Error()) {
				t.Errorf("stderr = %q, want the body to reach the database", errOut.String())
			}
			want := []string{"open postgres", "start postgres", "shutdown postgres"}
			if got := r.log(); !slices.Equal(got, want) {
				t.Errorf("events = %q, want %q", got, want)
			}
		})
	}
}

func TestSchema_ResetWithoutYesOpensNothing(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "no --yes",
			args:       []string{"schema", "reset"},
			wantStderr: "blobfs schema reset: required flag --yes not set\n",
		},
		{
			name:       "--yes=false",
			args:       []string{"schema", "reset", "--yes=false"},
			wantStderr: "blobfs schema reset: --yes=false does not confirm the reset\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRecorder()
			var out, errOut bytes.Buffer

			code := schemaApp(r, &out, &errOut).Run(context.Background(), tt.args)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d", code, process.ExitUsage)
			}
			if !strings.HasPrefix(errOut.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to start with %q", errOut.String(), tt.wantStderr)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("events = %q, want no opener called", got)
			}
		})
	}
}

func TestSchema_HelpAndUsageErrorsOpenNothing(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"group alone", []string{"schema"}},
		{"group --help", []string{"schema", "--help"}},
		{"status --help", []string{"schema", "status", "--help"}},
		{"up --help", []string{"schema", "up", "--help"}},
		{"down --help", []string{"schema", "down", "--help"}},
		{"reset --help", []string{"schema", "reset", "--help"}},
		{"unknown verb", []string{"schema", "bogus"}},
		{"status with an argument", []string{"schema", "status", "extra"}},
		{"up with an argument", []string{"schema", "up", "extra"}},
		{"down with an argument", []string{"schema", "down", "extra"}},
		{"reset with an argument", []string{"schema", "reset", "--yes", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRecorder()
			var out, errOut bytes.Buffer

			code := schemaApp(r, &out, &errOut).Run(context.Background(), tt.args)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d", code, process.ExitUsage)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("events = %q, want no opener called", got)
			}
		})
	}
}

func TestSchema_HelpListsTheVerbs(t *testing.T) {
	code, stdout, _ := run(t, "schema")

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	want := "Commands:\n" +
		"  status   Show each set's head, latest version, pending migrations, and dirty mark\n" +
		"  up       Apply every pending migration, blobfs's set first and then the app's\n" +
		"  down     Revert every applied migration, the app's set first and then blobfs's; the history tables stay\n" +
		"  reset    Revert every set, the app's first, and drop the history tables; requires --yes\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout =\n%s\nwant it to contain\n%s", stdout, want)
	}
}
