package output_test

import (
	"bytes"
	"testing"

	"github.com/JaimeStill/spike-cli-architecture/output"
)

func TestTable_AlignsColumns(t *testing.T) {
	var b bytes.Buffer

	err := output.Table(&b, []string{"set", "version"}, [][]string{
		{"blobfs", "2"},
		{"app", "10"},
	})

	if err != nil {
		t.Fatal(err)
	}
	want := "set     version\n" +
		"blobfs  2\n" +
		"app     10\n"
	if b.String() != want {
		t.Errorf("output =\n%s\nwant\n%s", b.String(), want)
	}
}

func TestTable_NoRowsWritesTheHeader(t *testing.T) {
	var b bytes.Buffer

	if err := output.Table(&b, []string{"set", "version"}, nil); err != nil {
		t.Fatal(err)
	}

	if want := "set  version\n"; b.String() != want {
		t.Errorf("output = %q, want %q", b.String(), want)
	}
}

func TestRecord_AlignsLabels(t *testing.T) {
	var b bytes.Buffer

	err := output.Record(&b, []output.Field{{Name: "id", Value: "1"}, {Name: "version", Value: "2"}})

	if err != nil {
		t.Fatal(err)
	}
	if want := "id:      1\nversion: 2\n"; b.String() != want {
		t.Errorf("output =\n%s\nwant\n%s", b.String(), want)
	}
}
