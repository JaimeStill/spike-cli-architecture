//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/standards-lab/blobfs"

	"github.com/JaimeStill/spike-cli-architecture/internal/livetest"
)

// binary is the path of cmd/blobfs, built once by TestMain.
var binary string

// TestMain builds the binary into a temporary directory and removes the
// directory after the tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "blobfs-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "blobfs")
	build := exec.Command("go", "build", "-o", binary, "github.com/JaimeStill/spike-cli-architecture/cmd/blobfs")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// target is what one test's runs of the binary point at: its throwaway
// database, through BLOBFS_DATABASE_NAME, and any further environment the
// test sets. Everything else comes from the process's environment, which
// `mise run integration` points at its isolated compose project.
type target struct {
	database string
	env      []string
	db       *sql.DB
}

// open creates the test's database and returns the target over it, with
// the test's own pool on the database for reads and writes the binary
// does not make.
func open(t *testing.T, env ...string) target {
	t.Helper()
	name, db := livetest.Database(t)
	t.Logf("database %s", name)
	return target{database: name, env: env, db: db}
}

// run executes the binary with args against tg, logs the run as a shell
// line with its output, and returns its stdout, stderr, and exit code.
func run(t *testing.T, tg target, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(append(os.Environ(), "BLOBFS_DATABASE_NAME="+tg.database), tg.env...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatalf("run %v: %v", args, err)
	}
	var b strings.Builder
	b.WriteString("$ blobfs " + strings.Join(args, " "))
	if out.Len() > 0 {
		b.WriteString("\n" + strings.TrimRight(out.String(), "\n"))
	}
	if code != 0 {
		fmt.Fprintf(&b, "\nexit %d: %s", code, strings.TrimRight(errOut.String(), "\n"))
	}
	t.Log(b.String())
	return out.String(), errOut.String(), code
}

// ok runs the binary and fails the test unless it exits zero with nothing
// on stderr, returning its stdout.
func ok(t *testing.T, tg target, args ...string) string {
	t.Helper()
	out, errOut, code := run(t, tg, args...)
	if code != 0 || errOut != "" {
		t.Fatalf("%v exited %d: %s", args, code, errOut)
	}
	return out
}

// refused runs the binary and fails the test unless it exits one with
// nothing on stdout and want on stderr.
func refused(t *testing.T, tg target, want string, args ...string) {
	t.Helper()
	failed(t, tg, 1, want, args...)
}

// misused runs the binary and fails the test unless it exits two, a usage
// error, with nothing on stdout and want on stderr.
func misused(t *testing.T, tg target, want string, args ...string) {
	t.Helper()
	failed(t, tg, 2, want, args...)
}

func failed(t *testing.T, tg target, wantCode int, want string, args ...string) {
	t.Helper()
	out, errOut, code := run(t, tg, args...)
	if code != wantCode {
		t.Errorf("%v exited %d, want %d", args, code, wantCode)
	}
	if out != "" {
		t.Errorf("%v: stdout = %q, want nothing", args, out)
	}
	if !strings.Contains(errOut, want) {
		t.Errorf("%v: stderr = %q, want %q", args, errOut, want)
	}
}

// lines splits stdout into its lines.
func lines(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// names returns the second column of every entry line of a listing, the
// names, directories first, in the order printed, joined by spaces.
func names(out string) string {
	var got []string
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "dir ") || strings.HasPrefix(line, "file ") {
			got = append(got, strings.Fields(line)[1])
		}
	}
	return strings.Join(got, " ")
}

// ids returns the id of every entry line of a listing by its name: the last
// column.
func ids(out string) map[string]string {
	m := map[string]string{}
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "dir ") || strings.HasPrefix(line, "file ") {
			f := strings.Fields(line)
			m[f[1]] = f[len(f)-1]
		}
	}
	return m
}

// field returns the value of one label of a stat record.
func field(out, label string) string {
	for _, line := range lines(out) {
		if rest, found := strings.CutPrefix(line, label+":"); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// seedFile inserts an available file of size bytes named name into the
// directory with id directoryID, as a completed write leaves it at version
// 2, and returns its id. The put that writes files through the binary
// needs the object store, so the directory script seeds the rows the
// listings and moves act on directly.
func seedFile(t *testing.T, tg target, directoryID, name string, size int64) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := tg.db.ExecContext(context.Background(),
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, size, content_type, etag, version) "+
			"VALUES ($1, $2, $3, 'available', $4, $5, 'text/plain', '\"seeded\"', 2)",
		id, directoryID, name, id+"/"+name, size)
	if err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return id
}

// script is one ordered run of the binary over one database, and what its
// steps share.
type script struct {
	tg target
}

// TestScript is the scripted run of the directory commands through the
// built binary, in its own database: the steps below in order, each a
// subtest, and the script stops at the first step that fails. The
// transcript under -v is the record of what the binary does.
func TestScript(t *testing.T) {
	s := &script{tg: open(t)}
	for _, step := range []struct {
		name string
		fn   func(*testing.T)
	}{
		{"schema-up", s.schemaUp},
		{"mkdir", s.mkdir},
		{"ls", s.list},
		{"stat", s.stat},
		{"mv", s.move},
		{"ids", s.ids},
		{"rmdir", s.removeDirectory},
	} {
		if !t.Run(step.name, step.fn) {
			t.Logf("the script stops at step %s", step.name)
			return
		}
	}
}

// schemaUp is the first step: a listing before the schema fails at the
// files node's start and names the fix, and schema up then applies both
// sets.
func (s *script) schemaUp(t *testing.T) {
	refused(t, s.tg, "blobfs ls: files: the database does not satisfy the statements; if the schema is not applied, run blobfs schema up", "ls", "/")
	refused(t, s.tg, "blobfs mkdir: files: ", "mkdir", "/reports")
	if out := ok(t, s.tg, "schema", "up"); out != "schema up: both sets at head\n" {
		t.Errorf("schema up stdout = %q", out)
	}
	if out := ok(t, s.tg, "ls", "/"); names(out) != "" {
		t.Errorf("ls / on a fresh schema:\n%s", out)
	}
}

// mkdir creates the tree the later steps list and move, and the refusals
// mkdir renders.
func (s *script) mkdir(t *testing.T) {
	if out := ok(t, s.tg, "mkdir", "/reports"); !strings.HasPrefix(out, "mkdir: /reports (id ") {
		t.Errorf("mkdir stdout = %q", out)
	}
	for _, p := range []string{"/reports/2026", "/reports/2025", "/archive", "/archive/old"} {
		ok(t, s.tg, "mkdir", p)
	}
	refused(t, s.tg, "blobfs: name taken (constraint blobfs_uq_directory_parent_name)", "mkdir", "/reports")
	refused(t, s.tg, "not found", "mkdir", "/missing/child")
	refused(t, s.tg, "the root directory", "mkdir", "/")
	refused(t, s.tg, "invalid path", "mkdir", "reports")
	misused(t, s.tg, "accepts 1 argument, got 2", "mkdir", "/a", "/b")
}

// list is ls with its id column, paging, sorting, filtering, --total none,
// the empty pages, and the cursor under --cursors, over three files seeded
// into /reports with sizes the size sort and the size filter show.
func (s *script) list(t *testing.T) {
	reports := ids(ok(t, s.tg, "ls", "/"))["reports"]
	seedFile(t, s.tg, reports, "c.txt", 30)
	seedFile(t, s.tg, reports, "a.txt", 20)
	seedFile(t, s.tg, reports, "b.txt", 10)

	// Directories then files, one page each, with the totals.
	out := ok(t, s.tg, "ls", "/reports")
	if got := names(out); got != "2025 2026 a.txt b.txt c.txt" {
		t.Errorf("ls /reports names = %s", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total 2\nmore: no\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total 3\nmore: no\n") {
		t.Errorf("ls /reports stdout:\n%s", out)
	}
	if !strings.HasSuffix(lines(out)[0], "  ID") {
		t.Errorf("ls header = %q, want ID as the last column", lines(out)[0])
	}
	for name, id := range ids(out) {
		if _, err := blobfs.ParseID(id); err != nil {
			t.Errorf("ls printed %q as the id of %s: %v", id, name, err)
		}
	}

	// Without --cursors a half with a next page says more: yes and prints
	// no cursor line.
	out = ok(t, s.tg, "ls", "/reports", "--page", "2", "--size", "1", "--sort", "name:desc")
	if got := names(out); got != "2025 b.txt" {
		t.Errorf("ls page 2 of 1 by name desc names = %s", got)
	}
	if !strings.Contains(out, "directories: 1 on page 2 of size 1, total 2\nmore: no\n") || !strings.Contains(out, "files: 1 on page 2 of size 1, total 3\nmore: yes\n") || strings.Contains(out, "next-") {
		t.Errorf("ls page 2 stdout:\n%s", out)
	}

	// A sort by size, a field only files have, leaves the directories in
	// name order, and cannot be continued by a cursor.
	if got := names(ok(t, s.tg, "ls", "/reports", "--sort", "size:desc")); got != "2025 2026 c.txt a.txt b.txt" {
		t.Errorf("ls by size desc names = %s; want directories in name order and files by size", got)
	}
	out = ok(t, s.tg, "ls", "/reports", "--sort", "size:desc", "--size", "2", "--cursors")
	if !strings.Contains(out, "files: 2 on page 1 of size 2, total 3\nmore: yes\n") || strings.Contains(out, "next-files:") {
		t.Errorf("ls by size desc at size 2 stdout:\n%s", out)
	}

	out = ok(t, s.tg, "ls", "/reports", "--total", "none")
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total not counted\nmore: no\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total not counted\nmore: no\n") {
		t.Errorf("ls --total none stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports", "--page", "5")
	if !strings.Contains(out, "directories: 0 on page 5 of size 20, total unknown (the page is empty)\nmore: no\n") || !strings.Contains(out, "files: 0 on page 5 of size 20, total unknown (the page is empty)\nmore: no\n") {
		t.Errorf("an empty later page stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports/2026")
	if !strings.Contains(out, "directories: 0 on page 1 of size 20, total 0\nmore: no\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\nmore: no\n") {
		t.Errorf("an empty first page stdout:\n%s", out)
	}
	refused(t, s.tg, "not found", "ls", "/reports/missing")
	refused(t, s.tg, "unknown sort field \"owner\"", "ls", "/reports", "--sort", "owner")
	misused(t, s.tg, "--total \"some\": the mode is exact or none", "ls", "/reports", "--total", "some")

	// --filter: a shared field predicates both halves, a file-only field
	// the files alone, each operator against the rows.
	for _, tc := range []struct {
		filters []string
		want    string
	}{
		{[]string{"name:like:2%"}, "2025 2026"},
		{[]string{"name:eq:a.txt"}, "a.txt"},
		{[]string{"name:ne:2026"}, "2025 a.txt b.txt c.txt"},
		{[]string{"size:gt:15"}, "2025 2026 a.txt c.txt"},
		{[]string{"size:ge:20", "size:lt:30"}, "2025 2026 a.txt"},
		{[]string{"size:le:10"}, "2025 2026 b.txt"},
		{[]string{"status:in:available,pending", "etag:notnull"}, "2025 2026 a.txt b.txt c.txt"},
		{[]string{"etag:null"}, "2025 2026"},
		{[]string{"created_at:ge:2000-01-01T00:00:00Z", "version:eq:2"}, "a.txt b.txt c.txt"},
		{[]string{"name:like:%.txt", "size:gt:15"}, "a.txt c.txt"},
	} {
		args := []string{"ls", "/reports"}
		for _, f := range tc.filters {
			args = append(args, "--filter", f)
		}
		if got := names(ok(t, s.tg, args...)); got != tc.want {
			t.Errorf("ls --filter %v names = %s, want %s", tc.filters, got, tc.want)
		}
	}
	refused(t, s.tg, "unknown filter field \"owner\"", "ls", "/reports", "--filter", "owner:eq:x")
	refused(t, s.tg, "unknown filter operator \"between\"", "ls", "/reports", "--filter", "name:between:x")
	refused(t, s.tg, "invalid", "ls", "/reports", "--filter", "size:gt:abc")
	misused(t, s.tg, "write <field>:<op>:<value>", "ls", "/reports", "--filter", "name")

	// The cursor: with --cursors a half with a next page prints it, and the
	// flag continues that half alone, without a total.
	out = ok(t, s.tg, "ls", "/reports", "--size", "2", "--cursors")
	if got := names(out); got != "2025 2026 a.txt b.txt" {
		t.Errorf("ls --size 2 names = %s", got)
	}
	cursor := ""
	for _, line := range lines(out) {
		if rest, found := strings.CutPrefix(line, "next-files: "); found {
			cursor = rest
		}
		if strings.HasPrefix(line, "next-dirs:") {
			t.Errorf("the directory half printed a cursor with no next page:\n%s", out)
		}
	}
	if cursor == "" {
		t.Fatalf("ls --size 2 --cursors printed no next-files line:\n%s", out)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 2, total 2\nmore: no\n") || !strings.Contains(out, "files: 2 on page 1 of size 2, total 3\nmore: yes\nnext-files: ") {
		t.Errorf("ls --size 2 --cursors stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports", "--size", "2", "--after-files", cursor, "--cursors")
	if got := names(out); got != "2025 2026 c.txt" {
		t.Errorf("ls --after-files names = %s", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 2, total 2\nmore: no\n") || !strings.Contains(out, "files: 1 after the cursor, size 2, total not counted\nmore: no\n") || strings.Contains(out, "next-") {
		t.Errorf("ls --after-files stdout:\n%s", out)
	}
	refused(t, s.tg, "cursor is malformed", "ls", "/reports", "--after-files", "nonsense")
	refused(t, s.tg, "cursor was issued for another base", "ls", "/reports", "--after-dirs", cursor)
	refused(t, s.tg, "the sort cannot continue from a cursor", "ls", "/reports", "--sort", "size", "--after-files", cursor)
}

// stat shows a directory's row and a file's, by path, and the root.
func (s *script) stat(t *testing.T) {
	out := ok(t, s.tg, "stat", "/reports")
	if field(out, "path") != "/reports" || field(out, "parent") != blobfs.RootID || field(out, "name") != "reports" || field(out, "version") != "1" || strings.Contains(out, "status:") {
		t.Errorf("stat of a directory:\n%s", out)
	}
	out = ok(t, s.tg, "stat", "/")
	if field(out, "path") != "/" || field(out, "id") != blobfs.RootID || field(out, "parent") != "-" || field(out, "name") != "/" {
		t.Errorf("stat /:\n%s", out)
	}
	out = ok(t, s.tg, "stat", "/reports/a.txt")
	if field(out, "path") != "/reports/a.txt" || field(out, "status") != "available" || field(out, "size") != "20" || field(out, "content-type") != "text/plain" || field(out, "etag") != `"seeded"` {
		t.Errorf("stat of a file:\n%s", out)
	}
	refused(t, s.tg, "not found", "stat", "/reports/missing")
	refused(t, s.tg, "not found", "stat", "/missing/a.txt")
}

// move moves and renames files and directories, and the refusals: the
// cycle, the root, the one-top-level-directory rule, the taken name, and
// the missing source and destination.
func (s *script) move(t *testing.T) {
	for _, p := range []string{"/a", "/a/x", "/a/y", "/b"} {
		ok(t, s.tg, "mkdir", p)
	}
	seedFile(t, s.tg, ids(ok(t, s.tg, "ls", "/a"))["x"], "f.txt", 6)

	// A file into a directory, then renamed.
	if out := ok(t, s.tg, "mv", "/a/x/f.txt", "/a/y"); !strings.HasPrefix(out, "mv: /a/x/f.txt -> /a/y/f.txt (id ") {
		t.Errorf("mv stdout = %q", out)
	}
	if out := ok(t, s.tg, "mv", "/a/y/f.txt", "/a/y/g.txt"); !strings.HasPrefix(out, "mv: /a/y/f.txt -> /a/y/g.txt (id ") {
		t.Errorf("mv rename stdout = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/a/y/g.txt"); field(out, "name") != "g.txt" || field(out, "version") != "4" || field(out, "key") == "" || !strings.HasSuffix(field(out, "key"), "/f.txt") {
		t.Errorf("stat after two moves:\n%s\nwant version 4 and the key it was written under", out)
	}
	refused(t, s.tg, "not found", "stat", "/a/x/f.txt")

	// A directory into a directory, then renamed; its contents follow.
	if out := ok(t, s.tg, "mv", "/a/x", "/a/y"); !strings.HasPrefix(out, "mv: /a/x -> /a/y/x (id ") {
		t.Errorf("mv of a directory stdout = %q", out)
	}
	if got := names(ok(t, s.tg, "ls", "/a/y")); got != "x g.txt" {
		t.Errorf("ls /a/y after the move = %s", got)
	}
	ok(t, s.tg, "mv", "/a/y/x", "/a/y/z")
	if got := names(ok(t, s.tg, "ls", "/a/y")); got != "z g.txt" {
		t.Errorf("ls /a/y after the rename = %s", got)
	}
	// A top-level directory may be renamed.
	ok(t, s.tg, "mv", "/b", "/c")
	if got := names(ok(t, s.tg, "ls", "/")); got != "a archive c reports" {
		t.Errorf("ls / after the rename = %s", got)
	}

	refused(t, s.tg, "would create a cycle", "mv", "/a/y", "/a/y/z")
	refused(t, s.tg, "would create a cycle", "mv", "/a/y", "/a/y")
	refused(t, s.tg, "the root directory", "mv", "/", "/elsewhere")
	refused(t, s.tg, "stays under one top-level directory", "mv", "/a/y", "/c/y")
	refused(t, s.tg, "stays under one top-level directory", "mv", "/a", "/c")
	refused(t, s.tg, "stays under one top-level directory", "mv", "/a/y/z", "/z")
	ok(t, s.tg, "mkdir", "/a/held")
	ok(t, s.tg, "mkdir", "/a/y/held")
	refused(t, s.tg, "blobfs: name taken (constraint blobfs_uq_directory_parent_name)", "mv", "/a/held", "/a/y")
	refused(t, s.tg, "not found", "mv", "/a/missing", "/a/y")
	refused(t, s.tg, "not found", "mv", "/a/held", "/a/nope/held")
}

// ids is the id:<uuid> forms of ls, stat, and mv: the record by id is the
// record by path without its path line, and a move by ids reports the
// paths a move by path does.
func (s *script) ids(t *testing.T) {
	for _, p := range []string{"/ids", "/ids/src", "/ids/dst", "/ids/sub"} {
		ok(t, s.tg, "mkdir", p)
	}
	under := ids(ok(t, s.tg, "ls", "/ids"))
	idsDir, srcDir, dstDir, subDir := ids(ok(t, s.tg, "ls", "/"))["ids"], under["src"], under["dst"], under["sub"]
	file := seedFile(t, s.tg, srcDir, "f.txt", 6)

	out := ok(t, s.tg, "stat", "/ids")
	if byID := ok(t, s.tg, "stat", "id:"+idsDir); strings.TrimRight(byID, "\n") != strings.Join(lines(out)[1:], "\n") {
		t.Errorf("stat of a directory by id:\n%s\nwant the record by path without its path line:\n%s", byID, out)
	}
	out = ok(t, s.tg, "stat", "/ids/src/f.txt")
	if field(out, "id") != file {
		t.Errorf("stat of the seeded file:\n%s", out)
	}
	if byID := ok(t, s.tg, "stat", "id:"+file); strings.TrimRight(byID, "\n") != strings.Join(lines(out)[1:], "\n") {
		t.Errorf("stat of a file by id:\n%s\nwant the record by path without its path line:\n%s", byID, out)
	}
	refused(t, s.tg, "no file or directory has it", "stat", "id:"+blobfs.NewID())
	misused(t, s.tg, "must be a UUID", "stat", "id:nope")

	if got := names(ok(t, s.tg, "ls", "id:"+idsDir)); got != "dst src sub" {
		t.Errorf("ls by id names = %s", got)
	}
	if got := names(ok(t, s.tg, "ls", "id:"+srcDir, "--filter", "name:like:f%")); got != "f.txt" {
		t.Errorf("ls by id with a filter names = %s", got)
	}
	refused(t, s.tg, "not found", "ls", "id:"+blobfs.NewID())
	misused(t, s.tg, "must be a UUID", "ls", "id:nope")
	misused(t, s.tg, "the nil UUID is the root's", "ls", "id:"+blobfs.RootID)

	// mv by ids: the file into /ids/dst, then the sub directory into it,
	// each keeping its name, with the paths in the result line.
	if out := ok(t, s.tg, "mv", "id:"+file, "id:"+dstDir); out != "mv: /ids/src/f.txt -> /ids/dst/f.txt (id "+file+")\n" {
		t.Errorf("mv of a file by ids stdout = %q", out)
	}
	if out := ok(t, s.tg, "mv", "id:"+subDir, "id:"+dstDir); out != "mv: /ids/sub -> /ids/dst/sub (id "+subDir+")\n" {
		t.Errorf("mv of a directory by ids stdout = %q", out)
	}
	if got := names(ok(t, s.tg, "ls", "/ids/dst")); got != "sub f.txt" {
		t.Errorf("ls /ids/dst after the moves = %s", got)
	}
	refused(t, s.tg, "stays under one top-level directory", "mv", "id:"+subDir, "id:"+ids(ok(t, s.tg, "ls", "/"))["reports"])
	misused(t, s.tg, "two paths, or two ids", "mv", "id:"+file, "/ids/dst")
	refused(t, s.tg, "no file or directory has it", "mv", "id:"+blobfs.NewID(), "id:"+dstDir)
	misused(t, s.tg, "the nil UUID is the root's", "mv", "id:"+blobfs.RootID, "id:"+dstDir)
}

// removeDirectory removes empty directories and refuses the rest.
func (s *script) removeDirectory(t *testing.T) {
	id := ids(ok(t, s.tg, "ls", "/reports"))["2025"]
	if out := ok(t, s.tg, "rmdir", "/reports/2025"); out != "rmdir: /reports/2025 (id "+id+")\n" {
		t.Errorf("rmdir stdout = %q", out)
	}
	refused(t, s.tg, "not found", "stat", "/reports/2025")
	refused(t, s.tg, "directory not empty", "rmdir", "/reports")
	refused(t, s.tg, "directory not empty", "rmdir", "/archive")
	refused(t, s.tg, "the root directory", "rmdir", "/")
	refused(t, s.tg, "not found", "rmdir", "/reports/missing")
	ok(t, s.tg, "rmdir", "/archive/old")
	ok(t, s.tg, "rmdir", "/archive")
	if got := names(ok(t, s.tg, "ls", "/")); got != "a c ids reports" {
		t.Errorf("ls / after the removals = %s", got)
	}
}

// TestDirectoryCommandsWithTheStoreUnreachable runs every directory
// command with the object store's endpoint on a port nothing listens on:
// the commands declare the database alone, so none reaches the store.
func TestDirectoryCommandsWithTheStoreUnreachable(t *testing.T) {
	tg := open(t, fmt.Sprintf("BLOBFS_STORAGE_ENDPOINT=http://127.0.0.1:%d/devstoreaccount1", closedPort(t)))
	ok(t, tg, "schema", "up")

	ok(t, tg, "mkdir", "/reports")
	ok(t, tg, "mkdir", "/reports/2026")
	id := ids(ok(t, tg, "ls", "/"))["reports"]
	if got := names(ok(t, tg, "ls", "/reports")); got != "2026" {
		t.Errorf("ls /reports names = %s", got)
	}
	ok(t, tg, "ls", "id:"+id)
	ok(t, tg, "stat", "/reports")
	ok(t, tg, "stat", "id:"+id)
	ok(t, tg, "mv", "/reports/2026", "/reports/2027")
	ok(t, tg, "rmdir", "/reports/2027")
}

// closedPort returns a loopback port nothing listens on: one the kernel
// just handed out and that this test has closed again.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
