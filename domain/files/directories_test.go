package files_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/blobfs"
	bfdata "github.com/standards-lab/blobfs/data"
	blobfspg "github.com/standards-lab/blobfs/postgres"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/JaimeStill/spike-cli-architecture/domain/files"
)

// The Store's directory operations over the scripted driver, built as the
// composition root builds it: blobfs's Postgres engine, and a dialect that
// renders the returning commands as single statements, as Postgres's does.
// No test here reaches a network.

const (
	dirID   = "00000000-0000-7000-8000-000000000001"
	otherID = "00000000-0000-7000-8000-000000000002"
	fileID  = "00000000-0000-7000-8000-000000000003"
)

// open builds the Store over a scripted pool that answers with responses.
func open(t *testing.T, responses ...sqltest.Response) (*files.Store, *sqltest.Recorder) {
	t.Helper()
	pool, rec := sqltest.Open(t, responses...)
	s, err := files.New(sqlate.Wrap(pool, sqltest.ReturningDialect{}), bfdata.WithEngine(blobfspg.Engine))
	if err != nil {
		t.Fatal(err)
	}
	return s, rec
}

var (
	directoryColumns = []string{"id", "parent_id", "name", "status", "version", "created_at", "updated_at"}
	fileColumns      = []string{"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at"}
	stamp            = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
)

// directoryRow is one blobfs_directory row, active at version 1; a nil
// parent is the root's.
func directoryRow(id string, parent any, name string) []driver.Value {
	return []driver.Value{id, parent, name, "active", int64(1), stamp, stamp}
}

// fileRow is one available blobfs_file row at version 2.
func fileRow(id, directory, name string, size int64) []driver.Value {
	return []driver.Value{id, directory, name, "available", id + "/" + name, size, "text/plain", `"etag"`, int64(2), stamp, stamp}
}

// resolved is the engine's path resolution reaching the directory at
// depth, the number of segments it matched.
func resolved(id string, parent any, name string, depth int64) sqltest.Response {
	return sqltest.Response{
		Columns: append(slices.Clone(directoryColumns), "depth"),
		Rows:    [][]driver.Value{append(directoryRow(id, parent, name), depth)},
	}
}

// resolvedRoot is the engine's path resolution of the empty path: the root.
func resolvedRoot() sqltest.Response {
	return resolved(blobfs.RootID, nil, "/", 0)
}

// listed is the directory read that follows each half's page, which
// blobfs runs to refuse a listing of a deleting directory: the directory
// with id, active.
func listed(id string) sqltest.Response {
	return directories(directoryRow(id, nil, "listed"))
}

func directories(rows ...[]driver.Value) sqltest.Response {
	return sqltest.Response{Columns: directoryColumns, Rows: rows}
}

func fileRows(rows ...[]driver.Value) sqltest.Response {
	return sqltest.Response{Columns: fileColumns, Rows: rows}
}

func TestVerify_PreparesBlobfsTheEnginesAndTheDomainsStatements(t *testing.T) {
	s, rec := open(t)

	if err := s.Verify(context.Background()); err != nil {
		t.Fatalf("Verify() = %v", err)
	}

	prepared := rec.SQL(sqltest.OpPrepare)
	if len(prepared) == 0 {
		t.Fatal("Verify prepared nothing")
	}
	if !slices.ContainsFunc(prepared, func(q string) bool { return strings.Contains(q, "pg_advisory_xact_lock") }) {
		t.Errorf("Verify did not prepare the engine's tree lock; prepared:\n%s", strings.Join(prepared, "\n---\n"))
	}
	for _, table := range []string{"INSERT INTO directory_owner", "JOIN directory_owner", "INSERT INTO bookmark", "FROM bookmark b"} {
		if !slices.ContainsFunc(prepared, func(q string) bool { return strings.Contains(q, table) }) {
			t.Errorf("Verify did not prepare a statement with %q", table)
		}
	}
	if ops := rec.Ops(); slices.ContainsFunc(ops, func(op sqltest.Op) bool { return op != sqltest.OpPrepare }) {
		t.Errorf("ops = %v, want prepares only", ops)
	}
}

func TestVerify_AnUnappliedSchemaIsErrVerify(t *testing.T) {
	s, rec := open(t)
	rec.FailPrepare = func(string) error { return errors.New(`relation "blobfs_directory" does not exist`) }

	err := s.Verify(context.Background())

	if !errors.Is(err, files.ErrVerify) {
		t.Fatalf("Verify() = %v, want ErrVerify", err)
	}
	if !strings.Contains(err.Error(), "run blobfs schema up") || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Verify() = %q, want the fix and the cause", err)
	}
}

func TestList_ResolvesAndReadsBothHalvesInOneReadOnlySnapshot(t *testing.T) {
	s, rec := open(t,
		resolved(dirID, blobfs.RootID, "reports", 1),
		sqltest.WithTotal(directories(directoryRow(otherID, dirID, "2026")), 1),
		listed(dirID),
		sqltest.WithTotal(fileRows(fileRow(fileID, dirID, "a.txt", 20)), 1),
		listed(dirID),
	)

	c, err := s.List(context.Background(), "/reports", files.Listing{Page: 1, Size: 20})

	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if c.Path != "/reports" || len(c.Directories.Rows) != 1 || c.Directories.Rows[0].Name != "2026" ||
		len(c.Files.Rows) != 1 || c.Files.Rows[0].Name != "a.txt" || c.Directories.Total != 1 || c.Files.Total != 1 {
		t.Errorf("List() = %+v", c)
	}
	want := []sqltest.Op{sqltest.OpBegin, sqltest.OpQuery, sqltest.OpQuery, sqltest.OpQuery, sqltest.OpQuery, sqltest.OpQuery, sqltest.OpCommit}
	if got := rec.Ops(); !slices.Equal(got, want) {
		t.Errorf("ops = %v, want %v", got, want)
	}
	if opts := rec.Calls()[0].TxOptions; !opts.ReadOnly || sql.IsolationLevel(opts.Isolation) != sql.LevelRepeatableRead {
		t.Errorf("transaction options = %+v, want read-only repeatable read", opts)
	}
	if args := rec.Calls()[1].Args; len(args) != 2 || args[0] != blobfs.RootID {
		t.Errorf("the resolution bound %v, want the root and the segments", args)
	}
}

func TestList_FileTermsLeaveTheDirectoryHalf(t *testing.T) {
	// A sort by size and a filter on status name fields the directory
	// listing does not declare or does not share; blobfs would refuse them
	// before any SQL, so the listing succeeding shows they were kept off
	// the directory half, and the SQL shows they reached the file half.
	s, rec := open(t,
		resolvedRoot(),
		sqltest.WithTotal(directories(), 0),
		listed(blobfs.RootID),
		sqltest.WithTotal(fileRows(), 0),
		listed(blobfs.RootID),
	)
	l := files.Listing{
		Page: 1, Size: 20,
		Sort:    []files.Sort{{Field: "size", Descending: true}, {Field: "name"}},
		Filters: []files.Filter{{Field: "status", Op: "in", Value: []any{"available"}}, {Field: "name", Op: "like", Value: "a%"}},
	}

	if _, err := s.List(context.Background(), "/", l); err != nil {
		t.Fatalf("List() = %v", err)
	}

	queries := rec.SQL(sqltest.OpQuery)
	if len(queries) != 5 {
		t.Fatalf("queries = %d, want 5", len(queries))
	}
	if dirs := queries[1]; strings.Contains(dirs, "size DESC") || !strings.Contains(dirs, "LIKE") {
		t.Errorf("the directory half's statement:\n%s\nwant the name filter and no size sort", dirs)
	}
	if fs := queries[3]; !strings.Contains(fs, "size DESC") || !strings.Contains(fs, "LIKE") {
		t.Errorf("the file half's statement:\n%s\nwant the size sort and the name filter", fs)
	}
}

func TestList_TotalNoneCountsNothing(t *testing.T) {
	s, _ := open(t, resolvedRoot(), directories(), listed(blobfs.RootID), fileRows(), listed(blobfs.RootID))

	c, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 20, Total: files.TotalNone})

	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if c.Directories.Total != files.NoTotal || c.Files.Total != files.NoTotal {
		t.Errorf("totals = %d, %d, want NoTotal for both", c.Directories.Total, c.Files.Total)
	}
}

func TestList_ContinuesAHalfFromItsCursorWithoutACount(t *testing.T) {
	// The first page holds one of two files, so it has a next page and a
	// cursor; the second listing continues the file half from it, reading
	// the directory half by number again.
	s, _ := open(t,
		resolvedRoot(),
		sqltest.WithTotal(directories(), 0),
		listed(blobfs.RootID),
		sqltest.WithTotal(fileRows(fileRow(fileID, blobfs.RootID, "a.txt", 1), fileRow(otherID, blobfs.RootID, "b.txt", 2)), 2),
		listed(blobfs.RootID),
	)
	first, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 1})
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if !first.Files.More || first.Files.Next == "" || first.Directories.Next != "" {
		t.Fatalf("first page = %+v, want a file cursor and none for directories", first)
	}

	s, rec := open(t,
		resolvedRoot(),
		sqltest.WithTotal(directories(), 0),
		listed(blobfs.RootID),
		fileRows(fileRow(otherID, blobfs.RootID, "b.txt", 2)),
		listed(blobfs.RootID),
	)
	next, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 1, After: files.After{Files: first.Files.Next}})

	if err != nil {
		t.Fatalf("List() after the cursor = %v", err)
	}
	if len(next.Files.Rows) != 1 || next.Files.Rows[0].Name != "b.txt" || next.Files.Total != files.NoTotal || next.Files.More {
		t.Errorf("continued file half = %+v, want b.txt, no total, no more", next.Files)
	}
	if next.Directories.Total != 0 {
		t.Errorf("directory half total = %d, want 0, counted by number", next.Directories.Total)
	}
	if fs := rec.SQL(sqltest.OpQuery)[3]; strings.Contains(fs, "sqlate_total") {
		t.Errorf("the continued half's statement counts:\n%s", fs)
	}
}

func TestList_RefusesAPathBeforeAnyIO(t *testing.T) {
	tests := []struct {
		path string
		want error
	}{
		{"reports", blobfs.ErrInvalidPath},
		{"/reports/", blobfs.ErrInvalidPath},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			s, rec := open(t)

			_, err := s.List(context.Background(), tt.path, files.Listing{Page: 1, Size: 20})

			if !errors.Is(err, tt.want) {
				t.Errorf("List(%q) = %v, want %v", tt.path, err, tt.want)
			}
			if slices.Contains(rec.Ops(), sqltest.OpQuery) {
				t.Errorf("ops = %v, want no query", rec.Ops())
			}
		})
	}
}

func TestListDirectory_ReadsTheDirectoryFirst(t *testing.T) {
	s, rec := open(t, directories())

	_, err := s.ListDirectory(context.Background(), dirID, files.Listing{Page: 1, Size: 20})

	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Fatalf("ListDirectory() = %v, want ErrNotFound", err)
	}
	if want := []sqltest.Op{sqltest.OpBegin, sqltest.OpQuery, sqltest.OpRollback}; !slices.Equal(rec.Ops(), want) {
		t.Errorf("ops = %v, want %v: no listing after the missing directory", rec.Ops(), want)
	}

	s, _ = open(t,
		directories(directoryRow(dirID, blobfs.RootID, "reports")),
		sqltest.WithTotal(directories(), 0),
		listed(dirID),
		sqltest.WithTotal(fileRows(fileRow(fileID, dirID, "a.txt", 3)), 1),
		listed(dirID),
	)
	c, err := s.ListDirectory(context.Background(), dirID, files.Listing{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListDirectory() = %v", err)
	}
	if c.Path != "" || len(c.Files.Rows) != 1 {
		t.Errorf("ListDirectory() = %+v, want no path and the file", c)
	}
}

func TestStat_ResolvesTheParentAndFindsTheFileByName(t *testing.T) {
	s, rec := open(t, resolved(dirID, blobfs.RootID, "reports", 1), fileRows(fileRow(fileID, dirID, "a.txt", 3)))

	f, err := s.Stat(context.Background(), "/reports/a.txt")

	if err != nil || f.ID != fileID {
		t.Fatalf("Stat() = %+v, %v", f, err)
	}
	if args := rec.Calls()[1].Args; len(args) != 2 || args[0] != dirID || args[1] != "a.txt" {
		t.Errorf("the lookup bound %v, want the parent's id and the name", args)
	}
}

func TestStat_TheRootIsNoFile(t *testing.T) {
	s, rec := open(t)

	_, err := s.Stat(context.Background(), "/")

	if !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("Stat(/) = %v, want ErrRootDirectory", err)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("calls = %v, want none", rec.Ops())
	}
}

func TestFind_FileFirstThenDirectory(t *testing.T) {
	s, _ := open(t, fileRows(fileRow(fileID, dirID, "a.txt", 3)))
	e, err := s.Find(context.Background(), fileID)
	if err != nil || e.Kind != files.EntryFile || e.File.ID != fileID {
		t.Errorf("Find(file) = %+v, %v", e, err)
	}

	s, _ = open(t, fileRows(), directories(directoryRow(dirID, blobfs.RootID, "reports")))
	e, err = s.Find(context.Background(), dirID)
	if err != nil || e.Kind != files.EntryDirectory || e.Directory.Name != "reports" {
		t.Errorf("Find(directory) = %+v, %v", e, err)
	}

	s, _ = open(t, fileRows(), directories())
	_, err = s.Find(context.Background(), otherID)
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), "no file or directory has it") {
		t.Errorf("Find(missing) = %v, want ErrNotFound saying neither has it", err)
	}
}

func TestMkdir_CreatesUnderTheResolvedParent(t *testing.T) {
	s, rec := open(t, resolved(dirID, blobfs.RootID, "reports", 1), directories(directoryRow(otherID, dirID, "2026")))

	d, err := s.Mkdir(context.Background(), "/reports/2026", "")

	if err != nil || d.ID != otherID {
		t.Fatalf("Mkdir() = %+v, %v", d, err)
	}
	insert := rec.Calls()[1]
	if !strings.HasPrefix(insert.SQL, "INSERT INTO blobfs_directory") || !slices.Contains(insert.Args, any(dirID)) || !slices.Contains(insert.Args, any("2026")) {
		t.Errorf("the create ran %q with %v, want the insert under the parent", insert.SQL, insert.Args)
	}
}

func TestMkdir_RefusesBeforeAnyIO(t *testing.T) {
	tests := []struct {
		path string
		want error
	}{
		{"/", blobfs.ErrRootDirectory},
		{"reports", blobfs.ErrInvalidPath},
		{"/reports/", blobfs.ErrInvalidPath},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			s, rec := open(t)

			_, err := s.Mkdir(context.Background(), tt.path, "")

			if !errors.Is(err, tt.want) {
				t.Errorf("Mkdir(%q) = %v, want %v", tt.path, err, tt.want)
			}
			if len(rec.Calls()) != 0 {
				t.Errorf("calls = %v, want none", rec.Ops())
			}
		})
	}
}

func TestMkdir_AMissingParentIsNotFound(t *testing.T) {
	s, rec := open(t, resolvedRoot())

	_, err := s.Mkdir(context.Background(), "/missing/child", "")

	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Mkdir() = %v, want ErrNotFound", err)
	}
	if n := len(rec.SQL(sqltest.OpQuery)); n != 1 {
		t.Errorf("queries = %d, want the resolution alone", n)
	}
}

func TestRemoveDirectory_TheRootIsRefusedBeforeAnyIO(t *testing.T) {
	s, rec := open(t)

	_, err := s.RemoveDirectory(context.Background(), "/")

	if !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("RemoveDirectory(/) = %v, want ErrRootDirectory", err)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("calls = %v, want none", rec.Ops())
	}
}

func TestMove_StaysUnderOneTopLevelDirectory(t *testing.T) {
	tests := []struct {
		name     string
		src, dst string
		// dst is resolved as an existing directory at depth.
		dstID    string
		dstDepth int64
	}{
		{"into another top-level directory", "/a/y", "/b", otherID, 1},
		{"up to the top level", "/a/y", "/", blobfs.RootID, 0},
		{"a top-level directory below another", "/a", "/b", otherID, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, rec := open(t, resolved(tt.dstID, nil, "b", tt.dstDepth))

			_, err := s.Move(context.Background(), tt.src, tt.dst)

			if !errors.Is(err, files.ErrMoveAcrossScopes) {
				t.Fatalf("Move(%s, %s) = %v, want ErrMoveAcrossScopes", tt.src, tt.dst, err)
			}
			want := []sqltest.Op{sqltest.OpBegin, sqltest.OpQuery, sqltest.OpRollback}
			if got := rec.Ops(); !slices.Equal(got, want) {
				t.Errorf("ops = %v, want %v: refused after the destination's resolution alone", got, want)
			}
		})
	}
}

func TestMove_TheRootIsRefusedBeforeAnyIO(t *testing.T) {
	s, rec := open(t)

	_, err := s.Move(context.Background(), "/", "/elsewhere")

	if !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("Move(/) = %v, want ErrRootDirectory", err)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("calls = %v, want none", rec.Ops())
	}
}

func TestMoveEntry_RefusesBeforeAnyIO(t *testing.T) {
	tests := []struct {
		name string
		req  files.MoveRequest
		want error
	}{
		{"the root", files.MoveRequest{Kind: files.EntryDirectory, ID: blobfs.RootID, DirectoryID: dirID}, blobfs.ErrRootDirectory},
		{"an unknown kind", files.MoveRequest{Kind: "link", ID: dirID, DirectoryID: otherID}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, rec := open(t)

			_, err := s.MoveEntry(context.Background(), tt.req)

			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Errorf("MoveEntry() = %v, want %v", err, tt.want)
			}
			if len(rec.Calls()) != 0 {
				t.Errorf("calls = %v, want none", rec.Ops())
			}
		})
	}
}
