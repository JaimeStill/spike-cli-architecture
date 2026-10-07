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

	"github.com/JaimeStill/spike-cli-architecture/cli"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
	"github.com/JaimeStill/spike-cli-architecture/internal/apptest"
)

// The scenarios driven through App.Run over buffers: list, which builds
// nothing, and the demo tours, each building the nodes it declares and
// narrating each step before doing it. The full tours run against the
// stack in the integration package, over the built binary.

func TestList_PrintsEachScenarioAndTheNodesItDeclaresAndBuildsNothing(t *testing.T) {
	clearEnv(t)
	var out, errOut bytes.Buffer
	a, built := haltedApp(&out, &errOut)

	code := a.Run(context.Background(), []string{"list"})

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
	if got := built.Log(); len(got) != 0 {
		t.Errorf("nodes built = %q, want none", got)
	}
}

func TestRootHelp_EndsWithTheScenarioListingAndBuildsNothing(t *testing.T) {
	clearEnv(t)
	var listing, listErr bytes.Buffer
	if code := app.New(cli.Streams{Stdin: strings.NewReader(""), Stdout: &listing, Stderr: &listErr}).Run(context.Background(), []string{"list"}); code != process.ExitOK {
		t.Fatalf("list: code = %d, want %d; stderr = %q", code, process.ExitOK, listErr.String())
	}
	tail := "\nRun 'blobfs <command> --help' for help on a command.\n\nScenarios:\n" + listing.String()
	tests := []struct {
		name   string
		args   []string
		stderr bool // whether the help goes to stderr
	}{
		{"--help", []string{"--help"}, false},
		{"no command", nil, false},
		{"unknown command", []string{"bogus"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			a, built := haltedApp(&out, &errOut)

			code := a.Run(context.Background(), tt.args)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d", code, process.ExitUsage)
			}
			help, other := out.String(), errOut.String()
			if tt.stderr {
				help, other = other, help
			}
			if !strings.HasSuffix(help, tail) {
				t.Errorf("help:\n%s\nwant it to end with:\n%s", help, tail)
			}
			if other != "" {
				t.Errorf("other stream = %q, want empty", other)
			}
			if got := built.Log(); len(got) != 0 {
				t.Errorf("nodes built = %q, want none", got)
			}
		})
	}
}

func TestRootHelp_ScenarioListingIsTheRootsOwn(t *testing.T) {
	for _, args := range [][]string{{"demo"}, {"list", "--help"}, {"schema"}} {
		code, out, errOut := run(t, args...)

		if code != process.ExitUsage {
			t.Errorf("%v: code = %d, want %d", args, code, process.ExitUsage)
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("%v: stdout:\n%s\nwant the command's help", args, out)
		}
		if strings.Contains(out+errOut, "Scenarios:") {
			t.Errorf("%v: stdout:\n%s\nstderr:\n%s\nwant no scenario listing", args, out, errOut)
		}
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
			var out, errOut bytes.Buffer
			a, built := haltedApp(&out, &errOut)

			code := a.Run(context.Background(), tt.args)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d; stderr = %q", code, process.ExitUsage, errOut.String())
			}
			if got := built.Log(); len(got) != 0 {
				t.Errorf("nodes built = %q, want none", got)
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

// buildsApp is haltedApp with the store built over a Fake, by
// apptest.FakeStore, and the database's name set, so a run's Build
// constructs, with production constructors elsewhere, everything the tour
// declares, and goes as far as the lifecycle configuration, which halts it
// before anything starts. It returns a run of the App and the Recorder of
// the nodes its runs build.
func buildsApp(t *testing.T, stdout, stderr *bytes.Buffer) (func(args ...string) int, *apptest.Recorder) {
	t.Helper()
	clearEnv(t)
	t.Setenv(godatabase.NewEnv("BLOBFS").Name, "app")
	a, built := haltedApp(stdout, stderr)
	apptest.FakeStore(t, a, storagetest.NewFake())
	return func(args ...string) int { return a.Run(context.Background(), args) }, built
}

func TestDemo_EachTourBuildsTheNodesItDeclares(t *testing.T) {
	tests := []struct {
		tour string
		want []string
	}{
		// The files node alone: Postgres, and never the store or its
		// configuration, which a reach of either would record.
		{"directories", []string{"files", "database", "database config", "lifecycle config"}},
		// The files and objects nodes: Postgres and the store.
		{"files", []string{"files", "database", "database config", "objects", "store", "lifecycle config"}},
	}
	for _, tt := range tests {
		t.Run(tt.tour, func(t *testing.T) {
			var out, errOut bytes.Buffer
			run, built := buildsApp(t, &out, &errOut)

			code := run("demo", tt.tour)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			if want := "blobfs demo " + tt.tour + ": lifecycle config: halted\n"; errOut.String() != want {
				t.Errorf("stderr = %q, want %q", errOut.String(), want)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing narrated before the start", out.String())
			}
			if got := built.Log(); !slices.Equal(got, tt.want) {
				t.Errorf("nodes built = %q, want %q", got, tt.want)
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
	var out, errOut bytes.Buffer
	// The first step's resolution of the working area finds nothing, so
	// the step notes there is nothing to clear; the second step's first
	// query is unscripted, so it fails after its heading.
	a, built, rec := scriptedApp(t, &out, &errOut, sqltest.Response{Columns: append(slices.Clone(directoryColumns), "depth")})

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
	if got := built.Log(); !slices.Equal(got, scriptedBuilt) {
		t.Errorf("nodes built = %q, want %q: neither the store nor its configuration", got, scriptedBuilt)
	}
}
