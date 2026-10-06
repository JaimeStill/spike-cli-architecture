package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/blobfs"
	"github.com/standards-lab/sqlate"
)

// This file composes the bookmark operations, add, rm, and ls, from
// blobfs's methods and the domain's bookmark statements. A bookmark binds a
// unit to a file at the file's grain; at most one of a unit's bookmarks is
// active. Each operation runs on the database alone.

// AddBookmark records that the unit bookmarks the file at path and returns
// the file's row. With active, the bookmark becomes the unit's one active
// bookmark, and the add is refused with ErrActiveBookmark while another
// bookmark of the unit is active; the other one is left as it is. The
// parent's resolution, the file's lookup, the hold of the file, and the
// insert run in one transaction.
//
// The hold is blobfs's reference-then-delete rule: Files.Hold locks the
// file's row until the transaction ends, so an rm that begins meanwhile
// waits and then sees the bookmark, and an rm that began first makes the
// hold refuse. A file that does not exist, or a parent that does not, is
// blobfs.ErrNotFound, and so is a file removed between its lookup and the
// insert. A pending file can be bookmarked. A deleting file is refused with
// ErrNotAvailable over blobfs's DeletingError, because its delete is under
// way. A file the unit has bookmarked already is ErrAlreadyBookmarked,
// active or not. The root is blobfs.ErrRootDirectory before any I/O.
func (s *Store) AddBookmark(ctx context.Context, path, unit string, active bool) (blobfs.File, error) {
	parent, name, err := splitParent(path)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: bookmark add %s: %w", path, err)
	}
	f, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		dir, err := s.resolve(ctx, tx, parent)
		if err != nil {
			return blobfs.File{}, err
		}
		f, err := s.blobfs.Files.FindByName(ctx, tx, dir.ID, name)
		if err != nil {
			return blobfs.File{}, err
		}
		if err := s.blobfs.Files.Hold(ctx, tx, f.ID); err != nil {
			if errors.Is(err, blobfs.ErrDeleting) {
				return blobfs.File{}, fmt.Errorf("%w: %w", ErrNotAvailable, err)
			}
			return blobfs.File{}, err
		}
		return f, s.insertBookmark(ctx, tx, unit, f.ID, active)
	})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: bookmark add %s as unit %s: %w", path, unit, err)
	}
	return f, nil
}

// RemoveBookmark removes the unit's bookmark of the file at path, active or
// not, and returns the file's row. The path is resolved and the row deleted
// on the pool: the delete is keyed by the unit and the file's id, so the two
// need not share a snapshot. A file that does not exist is
// blobfs.ErrNotFound; a file the unit has not bookmarked is ErrNoBookmark.
// Removing the active bookmark leaves the unit with none, which a later add
// with active may fill.
func (s *Store) RemoveBookmark(ctx context.Context, path, unit string) (blobfs.File, error) {
	f, err := s.stat(ctx, path)
	if err == nil {
		err = s.deleteBookmark(ctx, s.db, unit, f.ID)
	}
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: bookmark rm %s as unit %s: %w", path, unit, err)
	}
	return f, nil
}

// ListBookmarks returns one page, by number, of the unit's bookmarks under
// l's page, size, sort terms, and total mode, each with its file's full
// path, in path order unless l sorts otherwise. The read model pages by
// number only, so the page carries no cursor, and l's filters and cursors
// are not read. The page and its total are read in one read-only
// repeatable-read transaction.
func (s *Store) ListBookmarks(ctx context.Context, unit string, l Listing) (Page[Bookmark], error) {
	p, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (Page[Bookmark], error) {
		return s.bookmarksOf(ctx, tx, unit, Listing{Page: l.Page, Size: l.Size, Sort: l.Sort, Total: l.Total})
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
	if err != nil {
		return Page[Bookmark]{}, fmt.Errorf("files: bookmark ls as unit %s: %w", unit, err)
	}
	return p, nil
}
