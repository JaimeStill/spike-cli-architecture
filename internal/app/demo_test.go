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
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
)

// The scenarios driven through App.Run over buffers: list, which builds
// nothing, and the demo tours, each building the nodes it declares and
// narrating each step before doing it. The full tours run against the
// stack in the integration package, over the built binary.

func TestList_PrintsEachScenarioAndTheNodesItDeclaresAndBuildsNothing(t *testing.T) {
	clearEnv(t)
	r := &recorder{}
	var out, errOut bytes.Buffer

	code := recordingApp(r, &out, &errOut).Run(context.Background(), []string{"list"})

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q", code, process.ExitOK, errOut.String())
	}
	want := "" +
		"  directories  Tour the directory commands on Postgres alone: mkdir, ls, stat, mv, rmdir\n" +
		"               uses files\n" +
		"  files        Tour the object commands on Postgres and the store: put, cat, cp, rm, rm --recursive\n" +
		"               uses files, objects\n"
	if out.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), want)
	}
	if got := r.log(); len(got) != 0 {
		t.Errorf("constructors run = %q, want none", got)
	}
}

func TestDemo_HelpAndRefusalsBuildNothing(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"list with an argument", []string{"list", "extra"}},
		{"demo alone", []string{"demo"}},
		{"unknown tour", []string{"demo", "bogus"}},
		{"directories --help", []string{"demo", "directories", "--help"}},
		{"files --help", []string{"demo", "files", "--help"}},
		{"directories with an argument", []string{"demo", "directories", "extra"}},
		{"files with an unknown flag", []string{"demo", "files", "--bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			r := &recorder{}
			var out, errOut bytes.Buffer

			code := recordingApp(r, &out, &errOut).Run(context.Background(), tt.args)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d; stderr = %q", code, process.ExitUsage, errOut.String())
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want none", got)
			}
		})
	}
}

func TestDemo_HelpListsTheTours(t *testing.T) {
	code, out, _ := run(t, "demo")
	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	for _, want := range []string{"Commands:", "  directories ", "  files "} {
		if !strings.Contains(out, want) {
			t.Errorf("demo help lacks %q:\n%s", want, out)
		}
	}
}

// buildsApp is recordingApp with the real constructors of the files node,
// the database, and its configuration, each recorded, so a run's Build
// goes as far as the lifecycle configuration, whose recorder fails it
// before anything starts.
func buildsApp(t *testing.T, r *recorder, stdout, stderr *bytes.Buffer) func(args ...string) int {
	t.Helper()
	clearEnv(t)
	t.Setenv(godatabase.NewEnv("BLOBFS").Name, "app")
	a := recordingApp(r, stdout, stderr)
	g, n := a.Graph(), a.Nodes()
	g.Replace(n.Files, func(s *graph.Scope) (*files.Store, error) {
		r.record("files")
		return a.NewFiles(s)
	})
	g.Replace(n.Database, func(s *graph.Scope) (*godatabase.DB, error) {
		r.record("database")
		return a.NewDatabase(s)
	})
	g.Replace(n.DatabaseConfig, func(*graph.Scope) (godatabase.Config, error) {
		r.record("database config")
		var cfg godatabase.Config
		err := cfg.Finalize("BLOBFS")
		return cfg, err
	})
	g.Replace(n.Objects, func(s *graph.Scope) (*files.Objects, error) {
		r.record("objects")
		return a.NewObjects(s)
	})
	g.Replace(n.Store, func(*graph.Scope) (*storage.Store, error) {
		r.record("store")
		return fakeStore(t, storagetest.NewFake()), nil
	})
	return func(args ...string) int { return a.Run(context.Background(), args) }
}

func TestDemo_EachTourBuildsTheNodesItDeclares(t *testing.T) {
	tests := []struct {
		tour string
		want []string
	}{
		// The files node alone: Postgres, and never the store or its
		// configuration, which recordingApp would record.
		{"directories", []string{"files", "database", "database config", "lifecycle config"}},
		// The files and objects nodes: Postgres and the store.
		{"files", []string{"files", "database", "database config", "objects", "store", "lifecycle config"}},
	}
	for _, tt := range tests {
		t.Run(tt.tour, func(t *testing.T) {
			r := &recorder{}
			var out, errOut bytes.Buffer
			run := buildsApp(t, r, &out, &errOut)

			code := run("demo", tt.tour)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			if want := "blobfs demo " + tt.tour + ": lifecycle config: recorded\n"; errOut.String() != want {
				t.Errorf("stderr = %q, want %q", errOut.String(), want)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing narrated before the start", out.String())
			}
			if got := r.log(); !slices.Equal(got, tt.want) {
				t.Errorf("constructors run = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDemo_FilesWithTheStoreDownFailsOnceNamingTheStore(t *testing.T) {
	fake := storagetest.NewFake()
	fake.SetDown(true)
	var out, errOut bytes.Buffer
	rec, run, ping := objectApp(t, fake, strings.NewReader(""), &out, &errOut)

	code := run("demo", "files")

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	prefix := "blobfs demo files: store: "
	if !strings.HasPrefix(errOut.String(), prefix) || strings.Count(errOut.String(), "\n") != 1 {
		t.Errorf("stderr = %q, want one line starting %q", errOut.String(), prefix)
	}
	if !strings.Contains(errOut.String(), storage.ErrUnavailable.Error()) {
		t.Errorf("stderr = %q, want the store's unavailability", errOut.String())
	}
	if out.Len() != 0 || fake.Puts() != 0 {
		t.Errorf("stdout = %q, puts %d: want no step narrated or run", out.String(), fake.Puts())
	}
	if ops := rec.Ops(); slices.ContainsFunc(ops, func(op sqltest.Op) bool { return op != sqltest.OpPrepare }) {
		t.Errorf("ops = %v, want prepares only", ops)
	}
	if err := ping(); err == nil {
		t.Error("the database's pool answers a ping after the run, want it closed")
	}
}

func TestDemo_DirectoriesNarratesEachStepBeforeDoingIt(t *testing.T) {
	r := &recorder{}
	var out, errOut bytes.Buffer
	// The first step's resolution of the working area finds nothing, so
	// the step notes there is nothing to clear; the second step's first
	// query is unscripted, so it fails after its heading.
	a, rec := scriptedApp(t, r, &out, &errOut, sqltest.Response{Columns: append(slices.Clone(directoryColumns), "depth")})

	code := a.Run(context.Background(), []string{"demo", "directories"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	wantOut := "" +
		"[1/11] Clear /demo-directories if an earlier run left it behind\n" +
		"  Nothing to clear: /demo-directories does not exist, so this run starts clean.\n" +
		"\n" +
		"[2/11] Create /demo-directories as the demo unit's: mkdir --unit\n"
	if !strings.HasPrefix(out.String(), wantOut) {
		t.Errorf("stdout:\n%s\nwant it to start:\n%s", out.String(), wantOut)
	}
	if strings.Contains(out.String(), "[3/11]") {
		t.Errorf("stdout narrates step 3 after step 2 failed:\n%s", out.String())
	}
	prefix := "blobfs demo directories: step 2 (Create /demo-directories as the demo unit's: mkdir --unit): "
	if !strings.HasPrefix(errOut.String(), prefix) || !strings.Contains(errOut.String(), sqltest.ErrUnscripted.Error()) {
		t.Errorf("stderr = %q, want it to start %q and carry the failure", errOut.String(), prefix)
	}
	if n := rec.Pending(); n != 0 {
		t.Errorf("%d scripted responses unconsumed", n)
	}
	if got := r.log(); len(got) != 0 {
		t.Errorf("constructors run = %q, want neither the store nor its configuration", got)
	}
}
