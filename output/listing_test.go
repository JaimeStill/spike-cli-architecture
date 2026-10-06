package output_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/JaimeStill/spike-cli-architecture/output"
)

func TestListing_WritesTheEntriesAndEachHalfsPage(t *testing.T) {
	updated := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	size := int64(12)
	tests := []struct {
		name        string
		dirs, files output.Page
		want        string
	}{
		{
			name:  "counted pages",
			dirs:  output.Page{Number: 1, Size: 20, Listed: 1, Total: 1, Counted: true},
			files: output.Page{Number: 1, Size: 20, Listed: 1, Total: 3, Counted: true, More: true},
			want: "directories: 1 on page 1 of size 20, total 1\nmore: no\n" +
				"files: 1 on page 1 of size 20, total 3\nmore: yes\n",
		},
		{
			name:  "not counted, and an empty later page",
			dirs:  output.Page{Number: 5, Size: 20, Listed: 1, Total: output.NoTotal, Counted: true},
			files: output.Page{Number: 5, Size: 20, Listed: 1, Total: output.NoTotal},
			want: "directories: 1 on page 5 of size 20, total unknown (the page is empty)\nmore: no\n" +
				"files: 1 on page 5 of size 20, total not counted\nmore: no\n",
		},
		{
			name:  "cursors",
			dirs:  output.Page{Number: 1, Size: 1, Listed: 1, Total: 2, Counted: true, More: true, Next: "d1"},
			files: output.Page{Size: 1, Listed: 1, Total: output.NoTotal, Cursor: true, More: true, Next: "f2"},
			want: "directories: 1 on page 1 of size 1, total 2\nmore: yes\nnext-dirs: d1\n" +
				"files: 1 after the cursor, size 1, total not counted\nmore: yes\nnext-files: f2\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			entries := []output.Entry{
				{Kind: "dir", Name: "reports", ID: "D", Updated: updated},
				{Kind: "file", Name: "plan.txt", ID: "F", Size: &size, Status: "available", Updated: updated},
			}

			if err := output.Listing(&b, entries, tt.dirs, tt.files); err != nil {
				t.Fatal(err)
			}

			want := "KIND  NAME      SIZE  STATUS     UPDATED              ID\n" +
				"dir   reports   -     -          2026-10-06 12:00:00  D\n" +
				"file  plan.txt  12    available  2026-10-06 12:00:00  F\n" + tt.want
			if b.String() != want {
				t.Errorf("output =\n%s\nwant\n%s", b.String(), want)
			}
		})
	}
}

func TestListing_NoEntriesWritesTheHeader(t *testing.T) {
	var b bytes.Buffer
	empty := output.Page{Number: 1, Size: 20, Counted: true}

	if err := output.Listing(&b, nil, empty, empty); err != nil {
		t.Fatal(err)
	}

	want := "KIND  NAME  SIZE  STATUS  UPDATED  ID\n" +
		"directories: 0 on page 1 of size 20, total 0\nmore: no\n" +
		"files: 0 on page 1 of size 20, total 0\nmore: no\n"
	if b.String() != want {
		t.Errorf("output =\n%s\nwant\n%s", b.String(), want)
	}
}

func TestBookmarks_WritesTheEntriesThenThePage(t *testing.T) {
	updated := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	size := int64(12)
	var b bytes.Buffer

	err := output.Bookmarks(&b, []output.BookmarkEntry{
		{Path: "/reports/plan.txt", Size: &size, Status: "available", Active: true, Updated: updated},
		{Path: "/draft.bin", Status: "pending", Updated: updated},
	}, output.Page{Number: 1, Size: 20, Listed: 2, Total: 2, Counted: true, Next: "ignored"})

	if err != nil {
		t.Fatalf("Bookmarks() = %v", err)
	}
	want := "" +
		"PATH               SIZE  STATUS     ACTIVE  UPDATED\n" +
		"/reports/plan.txt  12    available  active  2026-10-06 12:00:00\n" +
		"/draft.bin         -     pending    -       2026-10-06 12:00:00\n" +
		"bookmarks: 2 on page 1 of size 20, total 2\n" +
		"more: no\n"
	if b.String() != want {
		t.Errorf("Bookmarks() wrote\n%s\nwant\n%s", b.String(), want)
	}
}

func TestBookmarks_NoEntriesWritesTheHeader(t *testing.T) {
	var b bytes.Buffer

	if err := output.Bookmarks(&b, nil, output.Page{Number: 1, Size: 20}); err != nil {
		t.Fatalf("Bookmarks() = %v", err)
	}
	want := "PATH  SIZE  STATUS  ACTIVE  UPDATED\nbookmarks: 0 on page 1 of size 20, total not counted\nmore: no\n"
	if b.String() != want {
		t.Errorf("Bookmarks() wrote\n%q\nwant\n%q", b.String(), want)
	}
}
