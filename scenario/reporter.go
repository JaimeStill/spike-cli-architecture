package scenario

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Reporter is the channel a scenario narrates through: a heading per step,
// prose saying what a step does and why, and the output a step's operation
// produced, each indented under its heading. It writes plain text, with no
// color, to one writer.
type Reporter struct {
	w       io.Writer
	atBlank bool // whether the line just written was blank
	started bool // whether anything has been written
}

// NewReporter returns a Reporter writing to w.
func NewReporter(w io.Writer) *Reporter {
	return &Reporter{w: w}
}

const indent = "  "

// columns is the width Note wraps prose to, the indent included.
const columns = 80

// Intent prints step i of n's intent sentence as a heading, set off from
// the step before it by a blank line.
func (r *Reporter) Intent(i, n int, intent string) {
	r.blank()
	r.printf("[%d/%d] %s\n", i, n, intent)
}

// Note prints prose, wrapped at columns with every line indented: one call
// is one description, however many lines it takes.
func (r *Reporter) Note(format string, args ...any) {
	for _, line := range wrap(fmt.Sprintf(format, args...), columns-len(indent)) {
		if line == "" {
			r.blank()
			continue
		}
		r.printf("%s%s\n", indent, line)
	}
}

// Show prints what render writes as a block indented one level past the
// prose, so a step's result reads apart from its narration: render is the
// same output a direct command writes, such as an output.Record. Nothing
// is printed when render fails, and its error is returned.
func (r *Reporter) Show(render func(io.Writer) error) error {
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		return err
	}
	text := strings.TrimRight(buf.String(), "\n")
	if text == "" {
		return nil
	}
	for line := range strings.SplitSeq(text, "\n") {
		r.printf("%s%s%s\n", indent, indent, line)
	}
	return nil
}

// printf is the one write every channel goes through; a reporter has no
// way to act on a failed write, so the result is discarded here. Only
// blank's own call ever passes the bare "\n" format, so that is what
// atBlank tracks.
func (r *Reporter) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.w, format, args...)
	r.atBlank = format == "\n"
	r.started = true
}

// blank prints one blank line, unless the reporter already sits on one or
// has written nothing yet, so the narration neither opens with a blank
// line nor doubles one.
func (r *Reporter) blank() {
	if r.atBlank || !r.started {
		return
	}
	r.printf("\n")
}
