package app_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JaimeStill/spike-cli-architecture/internal/app"
	"github.com/standards-lab/go-core/process"
)

// run drives blobfs with args over fresh buffers.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = app.New(&out, &errOut).Run(context.Background(), args)
	return code, out.String(), errOut.String()
}

func TestNew_IsCold(t *testing.T) {
	var out, errOut bytes.Buffer

	app.New(&out, &errOut)

	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("New() wrote %q, %q, want nothing", out.String(), errOut.String())
	}
}

func TestRun_VersionPrintsVersion(t *testing.T) {
	code, stdout, stderr := run(t, "version")

	if code != process.ExitOK {
		t.Errorf("code = %d, want %d", code, process.ExitOK)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 1 || strings.TrimSpace(lines[0]) == "" || !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout = %q, want one non-empty line", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRun_NoArgumentsPrintsRootHelp(t *testing.T) {
	code, stdout, stderr := run(t)

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	for _, want := range []string{
		"Usage:\n  blobfs <command> [flags]\n",
		"\nCommands:\n  version   Print the blobfs version\n",
		"\nFlags:\n  --help   Show help for blobfs\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant it to contain %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRun_HelpFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"root --help", []string{"--help"}, "Usage:\n  blobfs <command> [flags]\n"},
		{"root -h", []string{"-h"}, "Usage:\n  blobfs <command> [flags]\n"},
		{"version --help", []string{"version", "--help"}, "Print the blobfs version\n\nUsage:\n  blobfs version [flags]\n"},
		{"version -h", []string{"version", "-h"}, "Print the blobfs version\n\nUsage:\n  blobfs version [flags]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(t, tt.args...)

			if code != process.ExitUsage {
				t.Errorf("code = %d, want %d", code, process.ExitUsage)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Errorf("stdout =\n%s\nwant it to contain %q", stdout, tt.want)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	code, stdout, stderr := run(t, "bogus")

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	if !strings.HasPrefix(stderr, "blobfs: unknown command \"bogus\"\n") {
		t.Errorf("stderr =\n%s\nwant it to start with the unknown command", stderr)
	}
	if !strings.Contains(stderr, "Usage:\n  blobfs <command> [flags]\n") {
		t.Errorf("stderr =\n%s\nwant it to contain the root's usage", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	code, stdout, stderr := run(t, "version", "--bogus")

	if code != process.ExitUsage {
		t.Errorf("code = %d, want %d", code, process.ExitUsage)
	}
	want := "blobfs version: flag provided but not defined: -bogus\n" +
		"Usage: blobfs version [flags]\n" +
		"Run 'blobfs version --help' for details.\n"
	if stderr != want {
		t.Errorf("stderr =\n%s\nwant\n%s", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}
