package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/blobfs"
	"github.com/standards-lab/go-core/process"
	godatabase "github.com/standards-lab/go-database"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/internal/app"
)

// The directory commands driven through App.Run over buffers: the nodes
// they declare, the files node's statement check at start, and their
// output over a scripted database. The stack-backed runs are the
// integration package's, over the built binary.

const (
	reportsID = "00000000-0000-7000-8000-000000000001"
	planID    = "00000000-0000-7000-8000-000000000002"
)

// directoryVerbs are the directory commands, each as a run that reaches
// its body.
var directoryVerbs = [][]string{
	{"mkdir", "/reports"},
	{"ls", "/"},
	{"ls", "id:" + reportsID},
	{"stat", "/reports"},
	{"stat", "id:" + reportsID},
	{"mv", "/reports", "/archive"},
	{"mv", "id:" + planID, "id:" + reportsID},
	{"rmdir", "/reports"},
}

func TestFiles_CommandsBuildTheDatabaseAndNeverTheStore(t *testing.T) {
	for _, args := range directoryVerbs {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			clearEnv(t)
			t.Setenv(godatabase.NewEnv("BLOBFS").Name, "app")
			r := &recorder{}
			var out, errOut bytes.Buffer
			a := recordingApp(r, &out, &errOut)
			// The real files, database, and database configuration
			// constructors run, each recorded; none does I/O. The lifecycle
			// configuration, the Build's last root, keeps its failing
			// recorder, so the Build runs every constructor the files node
			// reaches and stops before anything starts: a store or storage
			// configuration it reached would be recorded.
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

			code := a.Run(context.Background(), args)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			if want := "blobfs " + args[0] + ": lifecycle config: recorded\n"; errOut.String() != want {
				t.Errorf("stderr = %q, want %q", errOut.String(), want)
			}
			wantRun := []string{"files", "database", "database config", "lifecycle config"}
			if got := r.log(); !slices.Equal(got, wantRun) {
				t.Errorf("constructors run = %q, want %q", got, wantRun)
			}
		})
	}
}

// scriptedApp returns blobfs with its database node Replace-d by a pool
// over the scripted driver, answering with responses, and its store and
// storage configuration nodes by recorders on r, so a run that reached
// either would fail with errRecorded and record it. The files node keeps
// its production constructor, statement check included.
func scriptedApp(t *testing.T, r *recorder, stdout, stderr *bytes.Buffer, responses ...sqltest.Response) (*app.App, *sqltest.Recorder) {
	t.Helper()
	a, rec, _ := scriptedAppIn(t, r, strings.NewReader(""), stdout, stderr, responses...)
	return a, rec
}

// scriptedAppIn is scriptedApp reading stdin, and it returns the scripted
// pool too, so a test can see whether the database was shut down.
func scriptedAppIn(t *testing.T, r *recorder, stdin io.Reader, stdout, stderr *bytes.Buffer, responses ...sqltest.Response) (*app.App, *sqltest.Recorder, *sql.DB) {
	t.Helper()
	clearEnv(t)
	pool, rec := sqltest.Open(t, responses...)
	a := app.New(stdin, stdout, stderr)
	g, n := a.Graph(), a.Nodes()
	g.Replace(n.Database, func(*graph.Scope) (*godatabase.DB, error) {
		cfg := godatabase.Config{Name: "app"}
		if err := cfg.Finalize("BLOBFS"); err != nil {
			return nil, err
		}
		return godatabase.New(pool, cfg), nil
	})
	g.Replace(n.Store, recording[*storage.Store](r, "store"))
	g.Replace(n.StorageConfig, recording[storage.Config](r, "storage config"))
	return a, rec, pool
}

func TestFiles_AnUnappliedSchemaFailsAtStartNamingTheFilesNode(t *testing.T) {
	for _, args := range directoryVerbs {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := &recorder{}
			var out, errOut bytes.Buffer
			a, rec := scriptedApp(t, r, &out, &errOut)
			rec.FailPrepare = func(string) error { return errors.New(`relation "blobfs_directory" does not exist`) }

			code := a.Run(context.Background(), args)

			if code != process.ExitFailure {
				t.Errorf("code = %d, want %d", code, process.ExitFailure)
			}
			want := "blobfs " + args[0] + ": files: " + files.ErrVerify.Error()
			if !strings.HasPrefix(errOut.String(), want) {
				t.Errorf("stderr = %q, want it to start with %q", errOut.String(), want)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want empty", out.String())
			}
			// The body never ran: the statement check prepared and nothing
			// else reached the database.
			if ops := rec.Ops(); slices.ContainsFunc(ops, func(op sqltest.Op) bool { return op != sqltest.OpPrepare }) {
				t.Errorf("ops = %v, want prepares only", ops)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want neither the store nor its configuration", got)
			}
		})
	}
}

var (
	directoryColumns = []string{"id", "parent_id", "name", "status", "version", "created_at", "updated_at"}
	fileColumns      = []string{"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at"}
	stamp            = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
)

func directoryRows(rows ...[]driver.Value) sqltest.Response {
	return sqltest.Response{Columns: directoryColumns, Rows: rows}
}

func directoryRow(id string, parent any, name string) []driver.Value {
	return []driver.Value{id, parent, name, "active", int64(1), stamp, stamp}
}

// resolvedRoot is the Postgres engine's path resolution of /.
func resolvedRoot() sqltest.Response {
	return sqltest.Response{
		Columns: append(slices.Clone(directoryColumns), "depth"),
		Rows:    [][]driver.Value{append(directoryRow(blobfs.RootID, nil, "/"), int64(0))},
	}
}

func TestFiles_DirectoryCommandsOverAScriptedDatabase(t *testing.T) {
	root := directoryRows(directoryRow(blobfs.RootID, nil, "/"))
	tests := []struct {
		name      string
		args      []string
		responses []sqltest.Response
		want      string
	}{
		{
			name:      "mkdir",
			args:      []string{"mkdir", "/reports"},
			responses: []sqltest.Response{resolvedRoot(), directoryRows(directoryRow(reportsID, blobfs.RootID, "reports"))},
			want:      "mkdir: /reports (id " + reportsID + ")\n",
		},
		{
			name: "ls",
			args: []string{"ls", "/"},
			responses: []sqltest.Response{
				resolvedRoot(),
				sqltest.WithTotal(directoryRows(directoryRow(reportsID, blobfs.RootID, "reports")), 1),
				root,
				sqltest.WithTotal(sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{
					{planID, blobfs.RootID, "plan.txt", "available", planID + "/plan.txt", int64(12), "text/plain", `"e"`, int64(2), stamp, stamp},
				}}, 1),
				root,
			},
			want: "" +
				"KIND  NAME      SIZE  STATUS     UPDATED              ID\n" +
				"dir   reports   -     -          2026-10-06 12:00:00  " + reportsID + "\n" +
				"file  plan.txt  12    available  2026-10-06 12:00:00  " + planID + "\n" +
				"directories: 1 on page 1 of size 20, total 1\n" +
				"more: no\n" +
				"files: 1 on page 1 of size 20, total 1\n" +
				"more: no\n",
		},
		{
			name:      "stat by id",
			args:      []string{"stat", "id:" + reportsID},
			responses: []sqltest.Response{{Columns: fileColumns}, directoryRows(directoryRow(reportsID, blobfs.RootID, "reports"))},
			want: "" +
				"id:      " + reportsID + "\n" +
				"parent:  " + blobfs.RootID + "\n" +
				"name:    reports\n" +
				"version: 1\n" +
				"created: 2026-10-06T12:00:00Z\n" +
				"updated: 2026-10-06T12:00:00Z\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{}
			var out, errOut bytes.Buffer
			a, rec := scriptedApp(t, r, &out, &errOut, tt.responses...)

			code := a.Run(context.Background(), tt.args)

			if code != process.ExitOK {
				t.Fatalf("code = %d, want %d; stderr = %q", code, process.ExitOK, errOut.String())
			}
			if out.String() != tt.want {
				t.Errorf("stdout =\n%s\nwant\n%s", out.String(), tt.want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
			if n := rec.Pending(); n != 0 {
				t.Errorf("%d scripted responses unconsumed", n)
			}
			// The statement check ran at start, before the body's first
			// statement.
			if ops := rec.Ops(); len(ops) == 0 || ops[0] != sqltest.OpPrepare {
				t.Errorf("ops = %v, want the statement check's prepares first", ops)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want neither the store nor its configuration", got)
			}
		})
	}
}

func TestFiles_HelpListsTheDirectoryCommands(t *testing.T) {
	code, stdout, _ := run(t)

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	want := "" +
		"  mkdir     Create a directory under an existing parent\n" +
		"  ls        List a directory: its directories, then its files, one page each\n" +
		"  stat      Show a file's or a directory's row, one field per line\n" +
		"  mv        Move or rename a directory or a file within its top-level directory\n" +
		"  rmdir     Remove an empty directory\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout =\n%s\nwant it to contain\n%s", stdout, want)
	}
}

func TestFiles_LsHelpListsItsFlags(t *testing.T) {
	code, stdout, _ := run(t, "ls", "--help")

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	for _, flag := range []string{"--page int", "--size int", "--sort string", "--filter string", "--total string", "--after-dirs string", "--after-files string", "--cursors"} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("stdout =\n%s\nwant it to list %s", stdout, flag)
		}
	}
	if !strings.Contains(stdout, "Usage:\n  blobfs ls [flags] <path|id:<uuid>>\n") {
		t.Errorf("stdout =\n%s\nwant the usage line", stdout)
	}
}

func TestFiles_MalformedArgumentsAreUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"an id that is not a UUID", []string{"ls", "id:nope"}, "blobfs ls: blobfs: invalid id \"nope\": must be a UUID\n"},
		{"the root's id", []string{"stat", "id:" + blobfs.RootID}, "the nil UUID is the root's"},
		{"a path and an id", []string{"mv", "/a", "id:" + reportsID}, "give two paths, or two ids"},
		{"a total mode", []string{"ls", "/", "--total", "some"}, "--total \"some\": the mode is exact or none"},
		{"a filter with no operator", []string{"ls", "/", "--filter", "name"}, "write <field>:<op>:<value>"},
		{"a sort direction", []string{"ls", "/", "--sort", "name:up"}, "the direction is asc or desc"},
		{"a page that is not a number", []string{"ls", "/", "--page", "two"}, "invalid value \"two\" for flag -page"},
		{"mkdir without a path", []string{"mkdir"}, "accepts 1 argument, got 0"},
		{"mv with one path", []string{"mv", "/a"}, "accepts 2 arguments, got 1"},
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
			if !strings.Contains(errOut.String(), tt.want) || !strings.Contains(errOut.String(), "Usage: blobfs "+tt.args[0]+" [flags]") {
				t.Errorf("stderr = %q, want %q and the usage line", errOut.String(), tt.want)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want none", got)
			}
		})
	}
}
