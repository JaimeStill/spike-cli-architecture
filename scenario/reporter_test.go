package scenario_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/JaimeStill/spike-cli-architecture/scenario"
)

func TestReporter_IndentsNotesAndShownOutputUnderTheIntent(t *testing.T) {
	var out bytes.Buffer
	r := scenario.NewReporter(&out)

	r.Intent(1, 2, "Make a directory")
	r.Note("Why it is %s.", "made")
	if err := r.Show(func(w io.Writer) error {
		_, err := fmt.Fprint(w, "mkdir: /a\nmkdir: /b\n\n")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r.Intent(2, 2, "List it")

	want := "" +
		"[1/2] Make a directory\n" +
		"  Why it is made.\n" +
		"    mkdir: /a\n" +
		"    mkdir: /b\n" +
		"\n" +
		"[2/2] List it\n"
	if out.String() != want {
		t.Errorf("Reporter wrote:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestReporter_NoteWrapsAtEightyColumns(t *testing.T) {
	var out bytes.Buffer
	r := scenario.NewReporter(&out)

	r.Note("%s", strings.Repeat("word ", 30))

	for line := range strings.SplitSeq(strings.TrimRight(out.String(), "\n"), "\n") {
		if len(line) > 80 || !strings.HasPrefix(line, "  word") {
			t.Errorf("Note line %q: want at most 80 columns, indented", line)
		}
	}
	if n := strings.Count(out.String(), "word"); n != 30 {
		t.Errorf("Note wrote %d words, want 30", n)
	}
}

func TestReporter_ShowWritesNothingWhenRenderFails(t *testing.T) {
	boom := errors.New("render failed")
	var out bytes.Buffer
	r := scenario.NewReporter(&out)

	err := r.Show(func(w io.Writer) error {
		_, _ = fmt.Fprintln(w, "partial")
		return boom
	})

	if !errors.Is(err, boom) {
		t.Errorf("Show error = %v, want the render's", err)
	}
	if out.Len() != 0 {
		t.Errorf("Show wrote %q, want nothing", out.String())
	}
}
