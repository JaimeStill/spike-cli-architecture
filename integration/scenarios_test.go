//go:build integration

package integration_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The scenarios through the built binary: list with nothing reachable,
// each tour twice in a row against the isolated stack, a tour clearing what
// an interrupted run left, and the directories tour with the object store
// unreachable.

// intent matches a step's heading in a tour's narration, [i/n] and the
// intent sentence.
var intent = regexp.MustCompile(`^\[(\d+)/(\d+)\] `)

// narrated fails t unless out narrates every step of a tour in order: its
// headings number 1 to n, each once, and a heading opens the output.
func narrated(t *testing.T, name, out string) {
	t.Helper()
	var steps []int
	total := 0
	for _, line := range lines(out) {
		m := intent.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		i, _ := strconv.Atoi(m[1])
		total, _ = strconv.Atoi(m[2])
		steps = append(steps, i)
	}
	if total == 0 || len(steps) != total {
		t.Fatalf("demo %s narrated steps %v of %d:\n%s", name, steps, total, out)
	}
	for i, step := range steps {
		if step != i+1 {
			t.Fatalf("demo %s narrated steps %v, want 1 to %d in order:\n%s", name, steps, total, out)
		}
	}
	if !intent.MatchString(out) {
		t.Errorf("demo %s output does not open with its first step's heading:\n%s", name, out)
	}
}

// TestList prints the scenarios with the database and the object store
// both on ports nothing listens on: list declares no node, so it builds
// nothing and reads no configuration.
func TestList(t *testing.T) {
	tg := target{env: []string{
		fmt.Sprintf("BLOBFS_DATABASE_PORT=%d", closedPort(t)),
		fmt.Sprintf("BLOBFS_STORAGE_ENDPOINT=http://127.0.0.1:%d/devstoreaccount1", closedPort(t)),
	}}
	out := ok(t, tg, "list")
	want := "" +
		"  directories  Tour the directory commands on Postgres alone: mkdir, ls, stat, mv, rmdir\n" +
		"               uses files\n" +
		"  files        Tour the object commands on Postgres and the store: put, cat, cp, rm, rm --recursive\n" +
		"               uses files, objects\n"
	if out != want {
		t.Errorf("list stdout:\n%s\nwant:\n%s", out, want)
	}
}

// TestDemos runs each tour twice in a row in one database and container:
// each narrates every step, works in its own area, and removes it, so the
// second run succeeds as the first did and the root is empty after.
func TestDemos(t *testing.T) {
	tg := open(t)
	ok(t, tg, "schema", "up")
	for _, name := range []string{"directories", "files"} {
		for run := 1; run <= 2; run++ {
			out := ok(t, tg, "demo", name)
			narrated(t, name, out)
			if !strings.Contains(out, "Nothing to clear") {
				t.Errorf("demo %s run %d found a working area left behind:\n%s", name, run, out)
			}
		}
	}
	if got := names(ok(t, tg, "ls", "/")); got != "" {
		t.Errorf("ls / after the tours = %s, want nothing left", got)
	}
	if n := owners(t, tg, "0199c0de-0000-7000-8000-0000000000de"); n != 0 {
		t.Errorf("the demo unit owns %d directories after the tours, want 0", n)
	}
}

// TestDemosClearWhatAnInterruptedRunLeft leaves each tour's working area
// behind with content in it, as a run stopped partway would, and each tour
// clears it first and succeeds.
func TestDemosClearWhatAnInterruptedRunLeft(t *testing.T) {
	tg := open(t)
	ok(t, tg, "schema", "up")
	ok(t, tg, "mkdir", "/demo-directories")
	ok(t, tg, "mkdir", "/demo-directories/alpha")
	ok(t, tg, "mkdir", "/demo-directories/alpha/echo")
	ok(t, tg, "mkdir", "/demo-files")
	ok(t, tg, "mkdir", "/demo-files/docs")
	put(t, tg, "/demo-files/docs/hello.txt", "left behind")

	for _, name := range []string{"directories", "files"} {
		out := ok(t, tg, "demo", name)
		narrated(t, name, out)
		if !strings.Contains(out, "An earlier run stopped before its last step") {
			t.Errorf("demo %s did not clear the area left behind:\n%s", name, out)
		}
	}
	if got := names(ok(t, tg, "ls", "/")); got != "" {
		t.Errorf("ls / after the tours = %s, want nothing left", got)
	}
}

// TestDemoDirectoriesWithTheStoreUnreachable runs the tours with the
// object store's endpoint on a port nothing listens on: the directories
// tour declares the files node alone, so it never reaches the store and
// succeeds, while the files tour fails at start, once, naming the store's
// node, before narrating anything.
func TestDemoDirectoriesWithTheStoreUnreachable(t *testing.T) {
	tg := open(t, fmt.Sprintf("BLOBFS_STORAGE_ENDPOINT=http://127.0.0.1:%d/devstoreaccount1", closedPort(t)))
	ok(t, tg, "schema", "up")

	narrated(t, "directories", ok(t, tg, "demo", "directories"))

	out, errOut, code := run(t, tg, "demo", "files")
	if prefix := "blobfs demo files: store: "; code != 1 || !strings.HasPrefix(errOut, prefix) || strings.Count(errOut, "\n") != 1 {
		t.Errorf("demo files exited %d: %q, want one line starting %q", code, errOut, prefix)
	}
	if out != "" {
		t.Errorf("demo files stdout = %q, want nothing narrated", out)
	}
}
