package app_test

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/blobfs"
	"github.com/standards-lab/go-core/process"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"
)

// Ownership and the bookmark commands driven through App.Run over
// buffers: their output over a scripted database, with the store and its
// configuration never built, and the usage errors a missing or malformed
// --unit makes before anything is built.

// resolved is the Postgres engine's path resolution reaching the
// top-level directory with id and name.
func resolved(id, name string) sqltest.Response {
	return sqltest.Response{
		Columns: append(slices.Clone(directoryColumns), "depth"),
		Rows:    [][]driver.Value{append(directoryRow(id, blobfs.RootID, name), int64(1))},
	}
}

// planRows is the lookup of plan.txt in /reports, available.
func planRows() sqltest.Response {
	return sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{
		{planID, reportsID, "plan.txt", "available", planID + "/plan.txt", int64(12), "text/plain", `"e"`, int64(2), stamp, stamp},
	}}
}

// activeViolation is the violation Postgres raises for a second active
// bookmark of a unit, as sqlate's Postgres dialect maps it.
func activeViolation() error {
	return &sqlate.ConstraintError{
		Constraint: "uq_bookmark_active",
		Class:      sqlate.ErrUniqueViolation,
		Err:        errors.New("duplicate key value violates unique constraint"),
	}
}

func TestBookmarks_CommandsOverAScriptedDatabase(t *testing.T) {
	held := sqltest.Response{Columns: []string{"id"}, Rows: [][]driver.Value{{planID}}}
	tests := []struct {
		name      string
		args      []string
		responses []sqltest.Response
		want      string
	}{
		{
			name:      "mkdir --unit",
			args:      []string{"mkdir", "/reports", "--unit", strings.ToUpper(unitID)},
			responses: []sqltest.Response{resolvedRoot(), directoryRows(directoryRow(reportsID, blobfs.RootID, "reports")), {Affected: 1}},
			want:      "mkdir: /reports (id " + reportsID + ", unit " + unitID + ")\n",
		},
		{
			name: "ls / --unit",
			args: []string{"ls", "/", "--unit", unitID},
			responses: []sqltest.Response{
				sqltest.WithTotal(directoryRows(directoryRow(reportsID, blobfs.RootID, "reports")), 1),
			},
			want: "" +
				"KIND  NAME     SIZE  STATUS  UPDATED              ID\n" +
				"dir   reports  -     -       2026-10-06 12:00:00  " + reportsID + "\n" +
				"directories: 1 on page 1 of size 20, total 1\n" +
				"more: no\n" +
				"files: 0 on page 1 of size 20, total 0\n" +
				"more: no\n",
		},
		{
			name:      "bookmark add --active",
			args:      []string{"bookmark", "add", "/reports/plan.txt", "--unit", unitID, "--active"},
			responses: []sqltest.Response{resolved(reportsID, "reports"), planRows(), held, {Affected: 1}},
			want:      "bookmark add: /reports/plan.txt (file " + planID + ", unit " + unitID + ", active)\n",
		},
		{
			name: "bookmark ls",
			args: []string{"bookmark", "ls", "--unit", unitID},
			responses: []sqltest.Response{sqltest.WithTotal(sqltest.Response{
				Columns: []string{"file_id", "directory_id", "active", "path", "name", "status", "size", "content_type", "created_at", "updated_at"},
				Rows:    [][]driver.Value{{planID, reportsID, true, "/reports/plan.txt", "plan.txt", "available", int64(12), "text/plain", stamp, stamp}},
			}, 1)},
			want: "" +
				"PATH               SIZE  STATUS     ACTIVE  UPDATED\n" +
				"/reports/plan.txt  12    available  active  2026-10-06 12:00:00\n" +
				"bookmarks: 1 on page 1 of size 20, total 1\n" +
				"more: no\n",
		},
		{
			name:      "bookmark rm",
			args:      []string{"bookmark", "rm", "/reports/plan.txt", "--unit", unitID},
			responses: []sqltest.Response{resolved(reportsID, "reports"), planRows(), {Affected: 1}},
			want:      "bookmark rm: /reports/plan.txt (file " + planID + ", unit " + unitID + ")\n",
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
			if n := rec.Pending(); n != 0 {
				t.Errorf("%d scripted responses unconsumed", n)
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want neither the store nor its configuration", got)
			}
		})
	}
}

func TestBookmarks_ARefusalIsReportedOnce(t *testing.T) {
	r := &recorder{}
	var out, errOut bytes.Buffer
	held := sqltest.Response{Columns: []string{"id"}, Rows: [][]driver.Value{{planID}}}
	a, _ := scriptedApp(t, r, &out, &errOut, resolved(reportsID, "reports"), planRows(), held, sqltest.Response{Err: activeViolation()})

	code := a.Run(context.Background(), []string{"bookmark", "add", "/reports/plan.txt", "--unit", unitID, "--active"})

	if code != process.ExitFailure {
		t.Errorf("code = %d, want %d", code, process.ExitFailure)
	}
	if want := "blobfs bookmark add: files: bookmark add /reports/plan.txt as unit " + unitID + ": the unit has an active bookmark already (constraint uq_bookmark_active)\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

func TestBookmarks_AMissingOrMalformedUnitIsAUsageErrorThatBuildsNothing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bookmark add without --unit", []string{"bookmark", "add", "/reports/plan.txt", "--active"}, "required flag --unit not set"},
		{"bookmark ls without --unit", []string{"bookmark", "ls"}, "required flag --unit not set"},
		{"bookmark rm without --unit", []string{"bookmark", "rm", "/reports/plan.txt"}, "required flag --unit not set"},
		{"bookmark add with a malformed unit", []string{"bookmark", "add", "/reports/plan.txt", "--unit", "nope"}, `--unit "nope" is not a UUID`},
		{"bookmark ls with a malformed sort", []string{"bookmark", "ls", "--unit", unitID, "--sort", "path:up"}, "the direction is asc or desc"},
		{"mkdir with a malformed unit", []string{"mkdir", "/reports", "--unit", "nope"}, `--unit "nope" is not a UUID`},
		{"ls with a malformed unit", []string{"ls", "/", "--unit", "nope"}, `--unit "nope" is not a UUID`},
		{"ls by id with a unit", []string{"ls", "id:" + reportsID, "--unit", unitID}, "a listing by id has no path to derive the unit's scope from"},
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
			usage := "Usage: " + commandPath(tt.args) + " [flags]"
			if !strings.Contains(errOut.String(), tt.want) || !strings.Contains(errOut.String(), usage) {
				t.Errorf("stderr = %q, want %q and %q", errOut.String(), tt.want, usage)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want empty", out.String())
			}
			if got := r.log(); len(got) != 0 {
				t.Errorf("constructors run = %q, want none", got)
			}
		})
	}
}
