package output

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

// Entry is one line of a directory listing: a directory or a file in the
// listed directory. Kind is dir or file. ID is the row's id, the handle the
// id:<uuid> argument form takes. Size is nil for a directory and for a file
// whose size is not known yet, and Status is empty for a directory.
type Entry struct {
	Kind    string
	Name    string
	ID      string
	Size    *int64
	Status  string
	Updated time.Time
}

// NoTotal is the Total of a Page whose total is not known: the page is
// empty and is not the first, so nothing on it carried the count.
const NoTotal = -1

// Page describes one half of a directory listing: the page number and size
// the listing asked for, how many rows the page holds, and the total of all
// pages, which is NoTotal when the page carries none and is not reported at
// all when the listing did not count it (Counted false). Cursor says the
// half was read after a cursor rather than by number, so the number is not
// shown. More says whether rows remain after the page, and Next is the
// cursor of the following page, written on its own line when it is not
// empty.
type Page struct {
	Number  int
	Size    int
	Listed  int
	Total   int
	Counted bool
	Cursor  bool
	More    bool
	Next    string
}

// Listing writes a directory listing to w: the entries as aligned columns
// under KIND NAME SIZE STATUS UPDATED ID, directories then files as the
// caller ordered them, with the id last so the name stays second; then for
// each half a line saying what its page holds and its total, a more: line
// saying whether rows remain after it, and, when the half has a next page
// and a cursor to continue it, a next-dirs: or next-files: line with the
// cursor. The caller blanks Next to leave those lines out.
func Listing(w io.Writer, entries []Entry, directories, files Page) error {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		size, status := "-", "-"
		if e.Size != nil {
			size = strconv.FormatInt(*e.Size, 10)
		}
		if e.Status != "" {
			status = e.Status
		}
		rows = append(rows, []string{e.Kind, e.Name, size, status, e.Updated.UTC().Format(time.DateTime), e.ID})
	}
	if err := Table(w, []string{"KIND", "NAME", "SIZE", "STATUS", "UPDATED", "ID"}, rows); err != nil {
		return err
	}
	if err := page(w, "directories", "next-dirs", directories); err != nil {
		return err
	}
	return page(w, "files", "next-files", files)
}

// page writes one half's lines: the rows on the page, the page number and
// size (or the cursor and the size), and the total as counted, not
// counted, or unknown; then more: yes or more: no; then the next cursor
// under label when there is one.
func page(w io.Writer, half, label string, p Page) error {
	total := "not counted"
	switch {
	case p.Counted && p.Total == NoTotal:
		total = "unknown (the page is empty)"
	case p.Counted:
		total = strconv.Itoa(p.Total)
	}
	var err error
	if p.Cursor {
		_, err = fmt.Fprintf(w, "%s: %d after the cursor, size %d, total %s\n", half, p.Listed, p.Size, total)
	} else {
		_, err = fmt.Fprintf(w, "%s: %d on page %d of size %d, total %s\n", half, p.Listed, p.Number, p.Size, total)
	}
	if err != nil {
		return err
	}
	more := "no"
	if p.More {
		more = "yes"
	}
	if _, err := fmt.Fprintf(w, "more: %s\n", more); err != nil {
		return err
	}
	if p.Next != "" {
		_, err = fmt.Fprintf(w, "%s: %s\n", label, p.Next)
	}
	return err
}

// BookmarkEntry is one line of a bookmark listing: a file the unit
// bookmarks, at its full path. Size is nil for a file whose size is not
// known yet. Active marks the unit's one active bookmark. Updated is the
// bookmark's.
type BookmarkEntry struct {
	Path    string
	Size    *int64
	Status  string
	Active  bool
	Updated time.Time
}

// Bookmarks writes a bookmark listing to w: the entries as aligned columns
// under PATH SIZE STATUS ACTIVE UPDATED, in the order given, then a line
// saying what the page holds and its total, and a more: line saying
// whether rows remain after it. The listing pages by number only, so p's
// Next is not read and no cursor line is written.
func Bookmarks(w io.Writer, entries []BookmarkEntry, p Page) error {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		size, active := "-", "-"
		if e.Size != nil {
			size = strconv.FormatInt(*e.Size, 10)
		}
		if e.Active {
			active = "active"
		}
		rows = append(rows, []string{e.Path, size, e.Status, active, e.Updated.UTC().Format(time.DateTime)})
	}
	if err := Table(w, []string{"PATH", "SIZE", "STATUS", "ACTIVE", "UPDATED"}, rows); err != nil {
		return err
	}
	p.Cursor, p.Next = false, ""
	return page(w, "bookmarks", "", p)
}
