package app_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"
	godatabase "github.com/standards-lab/go-database"
	"github.com/standards-lab/go-storage"

	"github.com/JaimeStill/spike-cli-architecture/admin/schema"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// The schema group's Uses, driven through App.Run. The stack-backed tests
// are in schema_integration_test.go.

// schemaVerbs are the schema group's verbs, each as a run that reaches its
// body.
var schemaVerbs = [][]string{
	{"schema", "status"},
	{"schema", "up"},
	{"schema", "down"},
	{"schema", "reset", "--yes"},
}

func TestSchema_VerbsBuildTheDatabaseAndNeverTheStore(t *testing.T) {
	for _, args := range schemaVerbs {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			clearEnv(t)
			r := &recorder{}
			var out, errOut bytes.Buffer
			a := recordingApp(r, &out, &errOut)
			// The real migrator and database constructors run, each recorded,
			// and the database configuration's recorder stops the Build: the
			// events show every node the verb reaches.
			a.Graph().Replace(a.Nodes().Migrator, func(s *graph.Scope) (*schema.Client, error) {
				r.record("migrator")
				return a.NewMigrator(s)
			})
			a.Graph().Replace(a.Nodes().Database, func(s *graph.Scope) (*godatabase.DB, error) {
				r.record("database")
				return a.NewDatabase(s)
			})

			code := a.Run(context.Background(), args)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			want := "blobfs " + strings.Join(args[:2], " ") + ": database config: recorded\n"
			if errOut.String() != want {
				t.Errorf("stderr = %q, want %q", errOut.String(), want)
			}
			if got, want := r.log(), []string{"migrator", "database", "database config"}; !slices.Equal(got, want) {
				t.Errorf("constructors run = %q, want %q", got, want)
			}
		})
	}
}

func TestSchema_FailsOnlyOnTheDatabaseConfig(t *testing.T) {
	// An empty environment leaves the database name unset. A storage value
	// its Finalize rejects changes nothing: no schema verb reads it.
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"empty environment", nil},
		{"invalid storage value", map[string]string{storage.NewEnv("BLOBFS").MaxObjectSize: "not-a-size"}},
	}
	for _, tt := range tests {
		for _, args := range schemaVerbs {
			t.Run(tt.name+"/"+strings.Join(args, " "), func(t *testing.T) {
				clearEnv(t)
				for k, v := range tt.env {
					t.Setenv(k, v)
				}

				code, stdout, stderr := run(t, args...)

				if code != process.ExitFailure {
					t.Errorf("code = %d, want %d", code, process.ExitFailure)
				}
				// One report, under the verb's path.
				want := "blobfs " + strings.Join(args[:2], " ") + ": database config: database name required\n"
				if stderr != want {
					t.Errorf("stderr = %q, want %q", stderr, want)
				}
				if stdout != "" {
					t.Errorf("stdout = %q, want empty", stdout)
				}
			})
		}
	}
}

func TestSchema_ResetWithoutYesBuildsNothing(t *testing.T) {
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
			clearEnv(t)
			r := &recorder{}
			var out, errOut bytes.Buffer

			code := recordingApp(r, &out, &errOut).Run(context.Background(), tt.args)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d", code, process.ExitUsage)
			}
			if !strings.HasPrefix(errOut.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to start with %q", errOut.String(), tt.wantStderr)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want none", got)
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
