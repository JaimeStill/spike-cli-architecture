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
	"github.com/standards-lab/go-core/process/processtest"

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
// database, through BLOBFS_DATABASE_NAME, its own container, through
// BLOBFS_STORAGE_CONTAINER, which the store's start creates on the first
// run that builds the store, and any further environment the test sets.
// Everything else comes from the process's environment, which `mise run
// integration` points at its isolated compose project, whose volumes are
// dropped with the containers in it.
type target struct {
	database  string
	container string
	env       []string
	db        *sql.DB
}

// open creates the test's database and returns the target over it and a
// container named after it, with the test's own pool on the database for
// reads and writes the binary does not make.
func open(t *testing.T, env ...string) target {
	t.Helper()
	name, db := livetest.Database(t)
	container := strings.ReplaceAll(name, "_", "-")
	t.Logf("database %s, container %s", name, container)
	return target{database: name, container: container, env: env, db: db}
}

// run executes the binary with args against tg, logs the run as a shell
// line with its output, and returns its stdout, stderr, and exit code. Its
// stdin is empty.
func run(t *testing.T, tg target, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runIn(t, tg, "", args...)
}

// runIn is run with stdin piped to the binary's standard input. A run that
// has not exited within processtest.Failsafe is a stall: it is
// interrupted, killed if it has not exited within Failsafe more, and fails
// the test with its output.
func runIn(t *testing.T, tg target, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), processtest.Failsafe)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = processtest.Failsafe
	cmd.Env = append(append(os.Environ(), "BLOBFS_DATABASE_NAME="+tg.database, "BLOBFS_STORAGE_CONTAINER="+tg.container), tg.env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("blobfs %s did not exit within %s:\nstdout: %s\nstderr: %s", strings.Join(args, " "), processtest.Failsafe, out.String(), errOut.String())
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatalf("run %v: %v", args, err)
	}
	var b strings.Builder
	if stdin != "" {
		fmt.Fprintf(&b, "$ printf %q | blobfs %s", stdin, strings.Join(args, " "))
	} else {
		b.WriteString("$ blobfs " + strings.Join(args, " "))
	}
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
	return okIn(t, tg, "", args...)
}

// okIn is ok with stdin piped to the binary's standard input.
func okIn(t *testing.T, tg target, stdin string, args ...string) string {
	t.Helper()
	out, errOut, code := runIn(t, tg, stdin, args...)
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

// put writes content as the file at path through the binary's put -, with
// any further arguments, and returns the file's id from its result line.
func put(t *testing.T, tg target, path, content string, args ...string) string {
	t.Helper()
	out := okIn(t, tg, content, append([]string{"put", "-", path}, args...)...)
	if !strings.HasPrefix(out, "put: "+path+" (id ") {
		t.Fatalf("put %s stdout = %q", path, out)
	}
	return idOf(t, out)
}

// idOf returns the id a result line reports as "(id <uuid>".
func idOf(t *testing.T, out string) string {
	t.Helper()
	_, rest, found := strings.Cut(out, "(id ")
	if !found || len(rest) < 36 {
		t.Fatalf("no id in %q", out)
	}
	id, err := blobfs.ParseID(rest[:36])
	if err != nil {
		t.Fatalf("the id in %q: %v", out, err)
	}
	return id
}

// seedPending inserts a pending file named name into the directory with id
// directoryID, as a put that stopped after its first step leaves it, and
// returns its id.
func seedPending(t *testing.T, tg target, directoryID, name string) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := tg.db.ExecContext(context.Background(),
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, $3, 'pending', $4, 'text/plain')",
		id, directoryID, name, id+"/"+name)
	if err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return id
}

// markDeleting marks the directories with ids, and every file in them,
// deleting, as blobfs's branch mark leaves a branch whose rm --recursive
// was interrupted before its sweep finished. ids lists the branch's root
// and every directory beneath it.
func markDeleting(t *testing.T, tg target, ids ...string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		if _, err := tg.db.ExecContext(ctx, "UPDATE blobfs_directory SET status = 'deleting', version = version + 1 WHERE id = $1", id); err != nil {
			t.Fatalf("mark directory %s: %v", id, err)
		}
		if _, err := tg.db.ExecContext(ctx, "UPDATE blobfs_file SET status = 'deleting', version = version + 1 WHERE directory_id = $1", id); err != nil {
			t.Fatalf("mark the files of %s: %v", id, err)
		}
	}
}

// script is one ordered run of the binary over one database, and what its
// steps share.
type script struct {
	tg target
}

// TestScript is the scripted run of the directory, object, and bookmark
// commands through the built binary, in its own database and container: the steps below in order, each a
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
		{"put", s.put},
		{"cat", s.cat},
		{"cp", s.copy},
		{"rm", s.remove},
		{"rm-recursive", s.removeTree},
		{"units", s.units},
		{"bookmarks", s.bookmarks},
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
// the empty pages, and the cursor under --cursors, over three files put
// into /reports with sizes the size sort and the size filter show.
func (s *script) list(t *testing.T) {
	for _, f := range []struct {
		name string
		size int
	}{{"c.txt", 30}, {"a.txt", 20}, {"b.txt", 10}} {
		put(t, s.tg, "/reports/"+f.name, strings.Repeat("x", f.size), "--content-type", "text/plain")
	}

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
	if field(out, "path") != "/reports/a.txt" || field(out, "status") != "available" || field(out, "size") != "20" || field(out, "content-type") != "text/plain" || !strings.HasPrefix(field(out, "etag"), `"`) || field(out, "version") != "2" {
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
	put(t, s.tg, "/a/x/f.txt", "f.txt\n")

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
	file := put(t, s.tg, "/ids/src/f.txt", "f.txt\n")

	out := ok(t, s.tg, "stat", "/ids")
	if byID := ok(t, s.tg, "stat", "id:"+idsDir); strings.TrimRight(byID, "\n") != strings.Join(lines(out)[1:], "\n") {
		t.Errorf("stat of a directory by id:\n%s\nwant the record by path without its path line:\n%s", byID, out)
	}
	out = ok(t, s.tg, "stat", "/ids/src/f.txt")
	if field(out, "id") != file {
		t.Errorf("stat of the put file:\n%s", out)
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

// put writes files from stdin and from local files, to paths and into a
// directory by id, with the content type from the flag, the extension, or
// the default, resumes a pending row, and refuses a name a file holds.
func (s *script) put(t *testing.T) {
	ok(t, s.tg, "mkdir", "/objects")
	objects := ids(ok(t, s.tg, "ls", "/"))["objects"]

	// put - reads the binary's stdin, and cat round-trips the bytes.
	out := okIn(t, s.tg, "hello, blobfs\n", "put", "-", "/objects/hello.txt")
	if !strings.HasPrefix(out, "put: /objects/hello.txt (id ") || !strings.Contains(out, ", 14 bytes, etag \"") {
		t.Errorf("put - stdout = %q", out)
	}
	if got := ok(t, s.tg, "cat", "/objects/hello.txt"); got != "hello, blobfs\n" {
		t.Errorf("cat after put - = %q", got)
	}
	out = ok(t, s.tg, "stat", "/objects/hello.txt")
	if field(out, "status") != "available" || field(out, "size") != "14" || field(out, "content-type") != "application/octet-stream" {
		t.Errorf("stat after put -:\n%s", out)
	}

	// A local file: the content type from its extension, by path and into
	// the directory by id under the file's base name.
	dir := t.TempDir()
	report := filepath.Join(dir, "report.json")
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(report, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := ok(t, s.tg, "put", report, "/objects/report.json"); !strings.Contains(out, ", 11 bytes, ") {
		t.Errorf("put of a local file stdout = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/objects/report.json"); field(out, "content-type") != "application/json" {
		t.Errorf("stat of a .json put:\n%s\nwant the type from the extension", out)
	}
	if out := ok(t, s.tg, "put", notes, "id:"+objects); !strings.HasPrefix(out, "put: notes.txt in id:"+objects+" (id ") {
		t.Errorf("put into a directory by id stdout = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/objects/notes.txt"); !strings.HasPrefix(field(out, "content-type"), "text/plain") {
		t.Errorf("stat of a .txt put:\n%s", out)
	}
	if got := ok(t, s.tg, "cat", "/objects/notes.txt"); got != "notes\n" {
		t.Errorf("cat of the file put by id = %q", got)
	}
	put(t, s.tg, "/objects/image.bin", "\x89PNG", "--content-type", "image/png")
	if out := ok(t, s.tg, "stat", "/objects/image.bin"); field(out, "content-type") != "image/png" {
		t.Errorf("stat after --content-type:\n%s", out)
	}

	// A pending row a stopped put left is resumed under its own key.
	pending := seedPending(t, s.tg, objects, "pending.txt")
	if out := okIn(t, s.tg, "resumed\n", "put", "-", "/objects/pending.txt"); out != "put: /objects/pending.txt (id "+pending+", 8 bytes, etag "+field(ok(t, s.tg, "stat", "/objects/pending.txt"), "etag")+", resumed the pending row)\n" {
		t.Errorf("put onto a pending row stdout = %q", out)
	}
	if got := ok(t, s.tg, "cat", "id:"+pending); got != "resumed\n" {
		t.Errorf("cat of the resumed file = %q", got)
	}

	// A name an available file holds is refused, and the file is intact.
	if _, errOut, code := runIn(t, s.tg, "other", "put", "-", "/objects/hello.txt"); code != 1 || !strings.Contains(errOut, "name taken") {
		t.Errorf("put over an available file exited %d: %s", code, errOut)
	}
	if got := ok(t, s.tg, "cat", "/objects/hello.txt"); got != "hello, blobfs\n" {
		t.Errorf("cat after the refused put = %q", got)
	}
	refused(t, s.tg, "not found", "put", report, "/missing/report.json")
	refused(t, s.tg, "no such file or directory", "put", filepath.Join(dir, "missing.txt"), "/objects/missing.txt")
	refused(t, s.tg, "the root directory", "put", "-", "/")
	misused(t, s.tg, "stdin has no name to store under", "put", "-", "id:"+objects)
}

// cat streams files by path and by id, and refuses a pending file, a
// directory, and a missing one.
func (s *script) cat(t *testing.T) {
	hello := ids(ok(t, s.tg, "ls", "/objects"))["hello.txt"]
	if got := ok(t, s.tg, "cat", "id:"+hello); got != "hello, blobfs\n" {
		t.Errorf("cat by id = %q", got)
	}
	seedPending(t, s.tg, ids(ok(t, s.tg, "ls", "/"))["objects"], "stuck.txt")
	refused(t, s.tg, "the file is not available: it is pending", "cat", "/objects/stuck.txt")
	refused(t, s.tg, "not found", "cat", "/objects/missing.txt")
	refused(t, s.tg, "not found", "cat", "/objects")
	refused(t, s.tg, "not found", "cat", "id:"+blobfs.NewID())
}

// copy copies a file to a new path, into a directory, and by ids, and
// refuses a name already taken and a source with no content.
func (s *script) copy(t *testing.T) {
	ok(t, s.tg, "mkdir", "/objects/sub")
	ok(t, s.tg, "mkdir", "/objects/sub2")
	out := ok(t, s.tg, "cp", "/objects/hello.txt", "/objects/copy.txt")
	if !strings.HasPrefix(out, "cp: /objects/hello.txt -> /objects/copy.txt (id ") || !strings.Contains(out, ", 14 bytes, etag ") {
		t.Errorf("cp stdout = %q", out)
	}
	if got := ok(t, s.tg, "cat", "/objects/copy.txt"); got != "hello, blobfs\n" {
		t.Errorf("cat of the copy = %q", got)
	}
	if out := ok(t, s.tg, "stat", "/objects/copy.txt"); field(out, "content-type") != "application/octet-stream" || field(out, "id") == ids(ok(t, s.tg, "ls", "/objects"))["hello.txt"] {
		t.Errorf("stat of the copy:\n%s\nwant the source's type under its own id", out)
	}
	if out := ok(t, s.tg, "cp", "/objects/hello.txt", "/objects/sub"); !strings.HasPrefix(out, "cp: /objects/hello.txt -> /objects/sub/hello.txt (id ") {
		t.Errorf("cp into a directory stdout = %q", out)
	}
	under := ids(ok(t, s.tg, "ls", "/objects"))
	if out := ok(t, s.tg, "cp", "id:"+under["report.json"], "id:"+under["sub2"]); !strings.HasPrefix(out, "cp: /objects/report.json -> /objects/sub2/report.json (id ") {
		t.Errorf("cp by ids stdout = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/objects/sub2/report.json"); field(out, "content-type") != "application/json" {
		t.Errorf("stat of the copy by ids:\n%s", out)
	}
	refused(t, s.tg, "name taken", "cp", "/objects/hello.txt", "/objects/sub")
	refused(t, s.tg, "name taken", "cp", "/objects/hello.txt", "/objects/copy.txt")
	refused(t, s.tg, "name taken", "cp", "/objects/hello.txt", "/objects")
	refused(t, s.tg, "the file is not available: it is pending", "cp", "/objects/stuck.txt", "/objects/sub")
	refused(t, s.tg, "not found", "cp", "/objects/missing.txt", "/objects/sub")
	refused(t, s.tg, "not found", "cp", "/objects/hello.txt", "/objects/nope/hello.txt")
	misused(t, s.tg, "two paths, or two ids", "cp", "/objects/hello.txt", "id:"+under["sub"])
}

// remove deletes single files by path and by id, a pending file among
// them, and refuses a missing one.
func (s *script) remove(t *testing.T) {
	copyID := ids(ok(t, s.tg, "ls", "/objects"))["copy.txt"]
	if out := ok(t, s.tg, "rm", "/objects/copy.txt"); out != "rm: /objects/copy.txt (id "+copyID+")\n" {
		t.Errorf("rm stdout = %q", out)
	}
	refused(t, s.tg, "not found", "stat", "/objects/copy.txt")
	refused(t, s.tg, "not found", "rm", "/objects/copy.txt")
	sub := ids(ok(t, s.tg, "ls", "/objects/sub"))["hello.txt"]
	if out := ok(t, s.tg, "rm", "id:"+sub); out != "rm: id:"+sub+" (id "+sub+")\n" {
		t.Errorf("rm by id stdout = %q", out)
	}
	ok(t, s.tg, "rm", "/objects/stuck.txt")
	if got := names(ok(t, s.tg, "ls", "/objects")); got != "sub sub2 hello.txt image.bin notes.txt pending.txt report.json" {
		t.Errorf("ls /objects after the removals = %s", got)
	}
	refused(t, s.tg, "not found", "rm", "id:"+blobfs.NewID())
	refused(t, s.tg, "the root directory", "rm", "/")
}

// removeTree deletes a branch and prints its totals, finishes a branch
// whose earlier run was interrupted, and, through its sweep, a branch
// another run marked; the root and an id are refused.
func (s *script) removeTree(t *testing.T) {
	for _, p := range []string{"/tree", "/tree/a", "/tree/a/b"} {
		ok(t, s.tg, "mkdir", p)
	}
	put(t, s.tg, "/tree/x.txt", "x")
	put(t, s.tg, "/tree/a/y.txt", "y")
	put(t, s.tg, "/tree/a/b/z.txt", "z")
	if out := ok(t, s.tg, "rm", "--recursive", "/tree"); out != "rm --recursive: /tree (3 files, 3 directories)\n" {
		t.Errorf("rm --recursive stdout = %q", out)
	}
	refused(t, s.tg, "not found", "ls", "/tree")

	// An interrupted run: the branch is marked and its sweep never ran. A
	// rerun finds the deleting directory at its path and finishes it.
	ok(t, s.tg, "mkdir", "/half")
	ok(t, s.tg, "mkdir", "/half/sub")
	put(t, s.tg, "/half/sub/h.txt", "h")
	half := ids(ok(t, s.tg, "ls", "/"))["half"]
	markDeleting(t, s.tg, half, ids(ok(t, s.tg, "ls", "/half"))["sub"])
	refused(t, s.tg, "deleting", "ls", "/half")
	if out := ok(t, s.tg, "rm", "--recursive", "/half"); out != "rm --recursive: /half (1 files, 2 directories)\n" {
		t.Errorf("rm --recursive of an interrupted branch stdout = %q", out)
	}

	// The sweep finishes every marked branch: /orphan, which another run
	// marked, goes with /other, and the totals count both.
	ok(t, s.tg, "mkdir", "/orphan")
	ok(t, s.tg, "mkdir", "/other")
	put(t, s.tg, "/orphan/o.txt", "o")
	markDeleting(t, s.tg, ids(ok(t, s.tg, "ls", "/"))["orphan"])
	if out := ok(t, s.tg, "rm", "--recursive", "/other"); out != "rm --recursive: /other (1 files, 2 directories)\n" {
		t.Errorf("rm --recursive with another marked branch stdout = %q", out)
	}
	if got := names(ok(t, s.tg, "ls", "/")); got != "a c ids objects reports" {
		t.Errorf("ls / after the branch deletes = %s", got)
	}

	refused(t, s.tg, "the root directory", "rm", "--recursive", "/")
	refused(t, s.tg, "not found", "rm", "--recursive", "/missing")
	misused(t, s.tg, "a branch is removed by path, not by id", "rm", "--recursive", "id:"+ids(ok(t, s.tg, "ls", "/"))["objects"])
	misused(t, s.tg, "flag provided but not defined: -r", "rm", "-r", "/objects")
}

// units is mkdir and ls under --unit: the owner row mkdir writes for a
// top-level directory, ls scoped to the unit's top-level directory and, at
// the root, to the unit's own top-level directories, and the refusals;
// then mv carrying the owner row with a renamed top-level directory, and
// rmdir and rm --recursive removing the owner rows with their directories.
func (s *script) units(t *testing.T) {
	unit, other := blobfs.NewID(), blobfs.NewID()
	if out := ok(t, s.tg, "mkdir", "/owned", "--unit", unit); !strings.HasPrefix(out, "mkdir: /owned (id ") || !strings.HasSuffix(out, ", unit "+unit+")\n") {
		t.Errorf("mkdir --unit stdout = %q, want the unit named", out)
	}
	ok(t, s.tg, "mkdir", "/owned/sub")
	ok(t, s.tg, "mkdir", "/theirs", "--unit", strings.ToUpper(other))
	put(t, s.tg, "/owned/sub/f.txt", "f")
	refused(t, s.tg, "--unit applies to a top-level directory only", "mkdir", "/owned/deeper", "--unit", unit)
	misused(t, s.tg, `--unit "nope" is not a UUID`, "mkdir", "/x", "--unit", "nope")

	// ls --unit at the unit's top-level directory and below it.
	if got := names(ok(t, s.tg, "ls", "/owned", "--unit", unit)); got != "sub" {
		t.Errorf("ls /owned as the owner names = %s", got)
	}
	if got := names(ok(t, s.tg, "ls", "/owned/sub", "--unit", unit)); got != "f.txt" {
		t.Errorf("ls /owned/sub as the owner names = %s", got)
	}
	refused(t, s.tg, "the unit does not own the directory", "ls", "/owned", "--unit", other)
	refused(t, s.tg, "the unit does not own the directory", "ls", "/owned/sub", "--unit", other)
	refused(t, s.tg, "the unit does not own the directory", "ls", "/reports", "--unit", unit)

	// ls / --unit: the unit's own top-level directories, and no files.
	out := ok(t, s.tg, "ls", "/", "--unit", unit)
	if got := names(out); got != "owned" {
		t.Errorf("ls / as the unit names = %s", got)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 20, total 1\nmore: no\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\nmore: no\n") {
		t.Errorf("ls / as the unit stdout:\n%s", out)
	}
	if got := names(ok(t, s.tg, "ls", "/", "--unit", other)); got != "theirs" {
		t.Errorf("ls / as the other unit names = %s", got)
	}
	if got := names(ok(t, s.tg, "ls", "/", "--unit", unit, "--filter", "name:like:own%", "--filter", "size:gt:1")); got != "owned" {
		t.Errorf("ls / as the unit with a filter names = %s", got)
	}
	if got := names(ok(t, s.tg, "ls", "/", "--unit", unit, "--filter", "name:like:zzz%")); got != "" {
		t.Errorf("ls / as the unit with a filter nothing matches names = %s", got)
	}
	refused(t, s.tg, "owner read model takes no cursor", "ls", "/", "--unit", unit, "--after-dirs", "x")
	misused(t, s.tg, "a listing by id has no path to derive the unit's scope from", "ls", "id:"+ids(ok(t, s.tg, "ls", "/"))["owned"], "--unit", unit)
	if got := names(ok(t, s.tg, "ls", "/")); got != "a c ids objects owned reports theirs" {
		t.Errorf("ls / names = %s", got)
	}

	// A renamed top-level directory keeps its owner row, which follows it
	// by id; rmdir removes the row with the directory.
	ok(t, s.tg, "mv", "/theirs", "/mine")
	if got := names(ok(t, s.tg, "ls", "/", "--unit", other)); got != "mine" {
		t.Errorf("ls / as the other unit after the rename = %s", got)
	}
	ok(t, s.tg, "rmdir", "/mine")
	if n := owners(t, s.tg, other); n != 0 {
		t.Errorf("owner rows of the other unit after rmdir = %d, want 0", n)
	}

	// rm --recursive removes the owner row of the branch's root with it.
	if out := ok(t, s.tg, "rm", "--recursive", "/owned"); out != "rm --recursive: /owned (1 files, 2 directories)\n" {
		t.Errorf("rm --recursive of an owned branch stdout = %q", out)
	}
	if n := owners(t, s.tg, unit); n != 0 {
		t.Errorf("owner rows of the unit after rm --recursive = %d, want 0", n)
	}
	if got := names(ok(t, s.tg, "ls", "/", "--unit", unit)); got != "" {
		t.Errorf("ls / as the unit after rm --recursive = %s", got)
	}
}

// bookmarks is the bookmark commands: add with --active and the refusal of
// a second active bookmark, ls with the files' full paths and the active
// marker, rm refused for a bookmarked file and for a branch holding one,
// then bookmark rm, after which rm succeeds.
func (s *script) bookmarks(t *testing.T) {
	unit, other := blobfs.NewID(), blobfs.NewID()
	ok(t, s.tg, "mkdir", "/library")
	ok(t, s.tg, "mkdir", "/library/deep")
	x := put(t, s.tg, "/library/x.txt", "x")
	put(t, s.tg, "/library/y.txt", "yy")
	z := put(t, s.tg, "/library/deep/z.txt", "zzz")

	// add: the result lines, the single active bookmark, and the refusals.
	if out := ok(t, s.tg, "bookmark", "add", "/library/deep/z.txt", "--unit", unit); out != "bookmark add: /library/deep/z.txt (file "+z+", unit "+unit+", inactive)\n" {
		t.Errorf("bookmark add stdout = %q", out)
	}
	if out := ok(t, s.tg, "bookmark", "add", "/library/x.txt", "--unit", unit, "--active"); out != "bookmark add: /library/x.txt (file "+x+", unit "+unit+", active)\n" {
		t.Errorf("bookmark add --active stdout = %q", out)
	}
	ok(t, s.tg, "bookmark", "add", "/library/x.txt", "--unit", other, "--active")
	refused(t, s.tg, "the unit has an active bookmark already (constraint uq_bookmark_active)", "bookmark", "add", "/library/y.txt", "--unit", unit, "--active")
	refused(t, s.tg, "the unit has bookmarked the file already (constraint pk_bookmark)", "bookmark", "add", "/library/deep/z.txt", "--unit", unit)
	refused(t, s.tg, "not found", "bookmark", "add", "/library/missing.txt", "--unit", unit)
	refused(t, s.tg, "not found", "bookmark", "add", "/library/deep", "--unit", unit)
	misused(t, s.tg, "required flag --unit not set", "bookmark", "add", "/library/y.txt")
	misused(t, s.tg, `--unit "nope" is not a UUID`, "bookmark", "ls", "--unit", "nope")

	// ls: full paths in path order, the active marker, the page and its
	// total; then paging, sorting, --total none, and another unit's.
	out := ok(t, s.tg, "bookmark", "ls", "--unit", unit)
	if got := bookmarkPaths(out); got != "/library/deep/z.txt /library/x.txt" {
		t.Errorf("bookmark ls paths = %s", got)
	}
	if !strings.HasPrefix(out, "PATH ") || !strings.Contains(out, "bookmarks: 2 on page 1 of size 20, total 2\nmore: no\n") {
		t.Errorf("bookmark ls stdout:\n%s", out)
	}
	for _, line := range lines(out) {
		f := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "/library/x.txt ") && (f[1] != "1" || f[2] != "available" || f[3] != "active"):
			t.Errorf("the active bookmark's line = %q", line)
		case strings.HasPrefix(line, "/library/deep/z.txt ") && (f[1] != "3" || f[3] != "-"):
			t.Errorf("the inactive bookmark's line = %q", line)
		}
	}
	out = ok(t, s.tg, "bookmark", "ls", "--unit", unit, "--size", "1", "--sort", "path:desc")
	if got := bookmarkPaths(out); got != "/library/x.txt" || !strings.Contains(out, "bookmarks: 1 on page 1 of size 1, total 2\nmore: yes\n") {
		t.Errorf("bookmark ls --size 1 --sort path:desc stdout:\n%s", out)
	}
	if out := ok(t, s.tg, "bookmark", "ls", "--unit", unit, "--total", "none"); !strings.Contains(out, "bookmarks: 2 on page 1 of size 20, total not counted\nmore: no\n") {
		t.Errorf("bookmark ls --total none stdout:\n%s", out)
	}
	if got := bookmarkPaths(ok(t, s.tg, "bookmark", "ls", "--unit", other)); got != "/library/x.txt" {
		t.Errorf("bookmark ls of the other unit = %s", got)
	}
	if out := ok(t, s.tg, "bookmark", "ls", "--unit", blobfs.NewID()); !strings.Contains(out, "bookmarks: 0 on page 1 of size 20, total 0\nmore: no\n") {
		t.Errorf("bookmark ls of a unit with none:\n%s", out)
	}
	refused(t, s.tg, "unknown sort field", "bookmark", "ls", "--unit", unit, "--sort", "key")

	// rm of a bookmarked file is refused before anything is touched, and
	// so is rm --recursive of a branch holding one.
	refused(t, s.tg, "2 unit(s) bookmark the file; remove the bookmarks and rerun rm", "rm", "/library/x.txt")
	refused(t, s.tg, "the file is bookmarked", "rm", "id:"+x)
	if out := ok(t, s.tg, "stat", "/library/x.txt"); field(out, "status") != "available" || field(out, "version") != "2" {
		t.Errorf("stat after the refused rm:\n%s\nwant the row untouched", out)
	}
	if got := ok(t, s.tg, "cat", "/library/x.txt"); got != "x" {
		t.Errorf("cat after the refused rm = %q", got)
	}
	refused(t, s.tg, "3 bookmark(s) hold files in the branch; remove the bookmarks and rerun rm --recursive", "rm", "--recursive", "/library")
	if got := names(ok(t, s.tg, "ls", "/library")); got != "deep x.txt y.txt" {
		t.Errorf("ls /library after the refused rm --recursive = %s", got)
	}

	// bookmark rm: the active bookmark goes, another may become active,
	// and once no unit bookmarks the file, rm succeeds.
	if out := ok(t, s.tg, "bookmark", "rm", "/library/x.txt", "--unit", unit); out != "bookmark rm: /library/x.txt (file "+x+", unit "+unit+")\n" {
		t.Errorf("bookmark rm stdout = %q", out)
	}
	refused(t, s.tg, "the unit has no bookmark of the file", "bookmark", "rm", "/library/x.txt", "--unit", unit)
	refused(t, s.tg, "not found", "bookmark", "rm", "/library/missing.txt", "--unit", unit)
	refused(t, s.tg, "1 unit(s) bookmark the file", "rm", "/library/x.txt")
	ok(t, s.tg, "bookmark", "rm", "/library/x.txt", "--unit", other)
	if out := ok(t, s.tg, "rm", "/library/x.txt"); out != "rm: /library/x.txt (id "+x+")\n" {
		t.Errorf("rm after the bookmarks went stdout = %q", out)
	}
	ok(t, s.tg, "bookmark", "add", "/library/y.txt", "--unit", unit, "--active")
	if got := bookmarkPaths(ok(t, s.tg, "bookmark", "ls", "--unit", unit)); got != "/library/deep/z.txt /library/y.txt" {
		t.Errorf("bookmark ls after rm = %s", got)
	}
	ok(t, s.tg, "bookmark", "rm", "/library/y.txt", "--unit", unit)
	ok(t, s.tg, "bookmark", "rm", "/library/deep/z.txt", "--unit", unit)
	if out := ok(t, s.tg, "rm", "--recursive", "/library"); out != "rm --recursive: /library (2 files, 2 directories)\n" {
		t.Errorf("rm --recursive after the bookmarks went stdout = %q", out)
	}
}

// bookmarkPaths returns the first column of every entry line of a bookmark
// listing, the paths, in the order printed, joined by spaces.
func bookmarkPaths(out string) string {
	var paths []string
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "/") {
			paths = append(paths, strings.Fields(line)[0])
		}
	}
	return strings.Join(paths, " ")
}

// owners returns how many directories the unit with id owns, read from the
// owner table on the test's own pool.
func owners(t *testing.T, tg target, unit string) int {
	t.Helper()
	var n int
	if err := tg.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM directory_owner WHERE unit_id = $1", unit).Scan(&n); err != nil {
		t.Fatalf("count the owner rows of %s: %v", unit, err)
	}
	return n
}

// TestTheStoreUnreachable runs the commands with the object store's
// endpoint on a port nothing listens on: every directory and bookmark
// command declares the database alone, so none reaches the store and each
// succeeds, while an object command fails at start, once, naming the
// store's node.
func TestTheStoreUnreachable(t *testing.T) {
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

	// Ownership and the bookmark commands, over a pending file seeded
	// without the store.
	unit := blobfs.NewID()
	ok(t, tg, "mkdir", "/library", "--unit", unit)
	if got := names(ok(t, tg, "ls", "/", "--unit", unit)); got != "library" {
		t.Errorf("ls / --unit names = %s", got)
	}
	seedPending(t, tg, ids(ok(t, tg, "ls", "/"))["library"], "plan.txt")
	ok(t, tg, "ls", "/library", "--unit", unit)
	ok(t, tg, "bookmark", "add", "/library/plan.txt", "--unit", unit, "--active")
	if got := bookmarkPaths(ok(t, tg, "bookmark", "ls", "--unit", unit)); got != "/library/plan.txt" {
		t.Errorf("bookmark ls paths = %s", got)
	}
	ok(t, tg, "bookmark", "rm", "/library/plan.txt", "--unit", unit)

	// Each refusal waits out the provider's retries, so two of the object
	// commands stand for the rest, which the hermetic tier runs over a
	// store that is down.
	for _, args := range [][]string{
		{"put", "-", "/reports/a.txt"},
		{"rm", "--recursive", "/reports"},
	} {
		_, errOut, code := runIn(t, tg, "never stored", args...)
		if prefix := "blobfs " + args[0] + ": store: "; code != 1 || !strings.HasPrefix(errOut, prefix) || strings.Count(errOut, "\n") != 1 {
			t.Errorf("%v exited %d: %q, want one line starting %q", args, code, errOut, prefix)
		}
	}
	// Nothing ran: the directory is as it was, and still listed.
	if got := names(ok(t, tg, "ls", "/reports")); got != "" {
		t.Errorf("ls /reports after the refused object commands = %s", got)
	}
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
