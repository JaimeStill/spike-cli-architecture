package app_test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/blobfs"
	"github.com/standards-lab/go-core/process"
	godatabase "github.com/standards-lab/go-database"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/JaimeStill/spike-cli-architecture/internal/apptest"
)

// The object commands driven through App.Run over buffers: the nodes they
// declare, standard input reaching put through the dispatcher, and a store
// that cannot be reached failing them at start. The object store is
// go-storage's Store over its in-memory Fake; the stack-backed runs are
// the integration package's, over the built binary.

// objectVerbs are the object commands, each as a run that reaches its
// body.
var objectVerbs = [][]string{
	{"put", "-", "/a.txt"},
	{"put", "local.txt", "id:" + reportsID},
	{"cat", "/a.txt"},
	{"cat", "id:" + planID},
	{"cp", "/a.txt", "/b.txt"},
	{"cp", "id:" + planID, "id:" + reportsID},
	{"rm", "/a.txt"},
	{"rm", "id:" + planID},
	{"rm", "--recursive", "/reports"},
}

func TestObjects_CommandsBuildTheDatabaseAndTheStore(t *testing.T) {
	for _, args := range objectVerbs {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			clearEnv(t)
			t.Setenv(godatabase.NewEnv("BLOBFS").Name, "app")
			var out, errOut bytes.Buffer
			// The production constructors run but the store's, which is
			// built over a Fake, and none does I/O; the lifecycle
			// configuration, the Build's last root, is halted, so the Build
			// constructs the storage node and everything it reaches, and
			// stops before anything starts.
			a, built := haltedApp(&out, &errOut)
			apptest.FakeStore(t, a, storagetest.NewFake())

			code := a.Run(context.Background(), args)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			if want := "blobfs " + args[0] + ": lifecycle config: halted\n"; errOut.String() != want {
				t.Errorf("stderr = %q, want %q", errOut.String(), want)
			}
			wantBuilt := []string{"storage", "files", "database", "database config", "store", "lifecycle config"}
			if got := built.Log(); !slices.Equal(got, wantBuilt) {
				t.Errorf("nodes built = %q, want %q", got, wantBuilt)
			}
		})
	}
}

// objectApp is scriptedAppIn with the store node built over fake, by
// apptest.FakeStore. It returns the scripted driver's recorder, a run of
// the App, and a ping of the scripted pool.
func objectApp(t *testing.T, fake *storagetest.Fake, stdin io.Reader, stdout, stderr *bytes.Buffer, responses ...sqltest.Response) (*sqltest.Recorder, func(args ...string) int, func() error) {
	t.Helper()
	a, _, rec, pool := scriptedAppIn(t, stdin, stdout, stderr, responses...)
	apptest.FakeStore(t, a, fake)
	run := func(args ...string) int { return a.Run(context.Background(), args) }
	return rec, run, func() error { return pool.Ping() }
}

func TestObjects_PutDashReadsTheInvocationsStdin(t *testing.T) {
	const fileID = "00000000-0000-7000-8000-000000000003"
	fake := storagetest.NewFake()
	stdin := strings.NewReader("piped bytes")
	var out, errOut bytes.Buffer
	rec, run, _ := objectApp(t, fake, stdin, &out, &errOut,
		resolvedRoot(),
		sqltest.Response{Columns: fileColumns},
		sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{
			{fileID, blobfs.RootID, "a.txt", "pending", fileID + "/a.txt", nil, "application/octet-stream", nil, int64(1), stamp, stamp},
		}},
		sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{
			{fileID, blobfs.RootID, "a.txt", "available", fileID + "/a.txt", int64(11), "application/octet-stream", `"e"`, int64(2), stamp, stamp},
		}},
	)

	code := run("put", "-", "/a.txt")

	if code != process.ExitOK {
		t.Fatalf("code = %d, want %d; stderr = %q", code, process.ExitOK, errOut.String())
	}
	if want := "put: /a.txt (id " + fileID + ", 11 bytes, etag \"e\")\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if stdin.Len() != 0 {
		t.Errorf("stdin has %d bytes unread, want the whole body read", stdin.Len())
	}
	blob, err := fake.Get(context.Background(), fileID+"/a.txt", storage.GetOptions{})
	if err != nil {
		t.Fatalf("the store holds no object: %v", err)
	}
	if b, _ := io.ReadAll(blob.Body); string(b) != "piped bytes" {
		t.Errorf("the object = %q, want stdin's bytes", b)
	}
	if opts, _ := fake.LastPut(); opts.ContentType != "application/octet-stream" || opts.Size != 0 {
		t.Errorf("the put's options = %+v, want octet-stream and an unknown size", opts)
	}
	if n := rec.Pending(); n != 0 {
		t.Errorf("%d scripted responses unconsumed", n)
	}
}

func TestObjects_AnUnreachableStoreFailsOnceNamingTheStore(t *testing.T) {
	for _, args := range objectVerbs {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := storagetest.NewFake()
			fake.SetDown(true)
			stdin := strings.NewReader("never read")
			var out, errOut bytes.Buffer
			rec, run, ping := objectApp(t, fake, stdin, &out, &errOut)

			code := run(args...)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			prefix := "blobfs " + args[0] + ": store: "
			if !strings.HasPrefix(errOut.String(), prefix) || strings.Count(errOut.String(), "\n") != 1 {
				t.Errorf("stderr = %q, want one line starting %q", errOut.String(), prefix)
			}
			if !strings.Contains(errOut.String(), storage.ErrUnavailable.Error()) {
				t.Errorf("stderr = %q, want the store's unavailability", errOut.String())
			}
			if out.Len() != 0 || stdin.Len() != len("never read") || fake.Puts() != 0 {
				t.Errorf("stdout = %q, stdin left %d, puts %d: want the body never run", out.String(), stdin.Len(), fake.Puts())
			}
			// Only the statement check reached the database, and the
			// database was shut down with the command.
			if ops := rec.Ops(); slices.ContainsFunc(ops, func(op sqltest.Op) bool { return op != sqltest.OpPrepare }) {
				t.Errorf("ops = %v, want prepares only", ops)
			}
			if err := ping(); err == nil {
				t.Error("the database's pool answers a ping after the run, want it closed")
			}
		})
	}
}

func TestObjects_UsageErrorsBuildNothing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"put - into a directory id", []string{"put", "-", "id:" + reportsID}, "stdin has no name to store under"},
		{"rm --recursive by id", []string{"rm", "--recursive", "id:" + reportsID}, "a branch is removed by path, not by id"},
		{"rm -r", []string{"rm", "-r", "/reports"}, "flag provided but not defined: -r"},
		{"cp a path and an id", []string{"cp", "/a.txt", "id:" + reportsID}, "two paths, or two ids"},
		{"cat with no argument", []string{"cat"}, "accepts 1 argument, got 0"},
		{"put with one argument", []string{"put", "-"}, "accepts 2 arguments, got 1"},
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
			if !strings.Contains(errOut.String(), tt.want) {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.want)
			}
			if got := built.Log(); len(got) != 0 {
				t.Errorf("nodes built = %q, want none", got)
			}
		})
	}
}

func TestObjects_HelpListsTheObjectCommands(t *testing.T) {
	code, stdout, _ := run(t)

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	for _, want := range []string{
		"  put        Upload a local file, or stdin for -, as the file at a path or into a directory\n",
		"  cat        Write an available file's content to stdout\n",
		"  cp         Copy an available file into a directory or to a new path\n",
		"  rm         Delete a file, or with --recursive a directory and everything beneath it\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant it to contain %q", stdout, want)
		}
	}
}
