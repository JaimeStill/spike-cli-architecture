package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/standards-lab/blobfs"
	"github.com/standards-lab/sqlate"
)

// This file composes the directory operations from blobfs's methods and the
// domain's owner statements. Ids are the primary handle: ListDirectory,
// StatFile, StatDirectory, Find, and MoveEntry take ids, and the path forms
// List, Stat, Resolve, and Move resolve their paths and then run the same
// steps, so a caller that holds an id from a listing acts without a
// resolution. Mkdir and RemoveDirectory take paths. The object store is
// never consulted.

// List returns the contents of the directory at path under l: the
// directories under it and the files in it, each one page of l's size
// with its total when l asks for one, each continued from its own cursor
// in l.After when one is given. The path is resolved and both halves read
// in one read-only repeatable-read transaction, so the resolution and the
// two halves see one snapshot and agree with each other. A path that names
// no directory is blobfs.ErrNotFound, and one that does not start with a
// slash blobfs.ErrInvalidPath.
//
// A unit in l scopes the listing to what the unit owns. Below the root,
// the path's top-level directory is resolved first and its owner row read,
// and a unit that does not own it is refused with ErrNotOwned before the
// rest of the path is resolved. At the root, the listing is the unit's own
// top-level directories, read through the owner read model under the
// terms that name a directory field, and no files: a file in the root has
// no top-level directory and belongs to no unit. That read model pages by
// number only, so a cursor there is ErrNoCursorAtRoot, before any I/O.
func (s *Store) List(ctx context.Context, path string, l Listing) (Contents, error) {
	if path == "/" && l.Unit != "" && l.After != (After{}) {
		return Contents{}, fmt.Errorf("files: ls / as unit %s: %w", l.Unit, ErrNoCursorAtRoot)
	}
	c, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (Contents, error) {
		if l.Unit == "" {
			dir, err := s.resolve(ctx, tx, path)
			if err != nil {
				return Contents{}, err
			}
			return s.contents(ctx, tx, path, dir.ID, l)
		}
		if path == "/" {
			return s.topLevel(ctx, tx, l)
		}
		dir, err := s.resolveOwned(ctx, tx, path, l.Unit)
		if err != nil {
			return Contents{}, err
		}
		return s.contents(ctx, tx, path, dir.ID, l)
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
	if err != nil {
		if l.Unit != "" {
			return Contents{}, fmt.Errorf("files: ls %s as unit %s: %w", path, l.Unit, err)
		}
		return Contents{}, fmt.Errorf("files: ls %s: %w", path, err)
	}
	return c, nil
}

// resolveOwned resolves the directory at path, below the root, through
// sess for the unit: the path's top-level directory first, then its owner
// row, and the rest of the path only once the unit is known to own it, so
// a unit learns nothing of a branch it does not own. A unit that does not
// own the top-level directory is ErrNotOwned.
func (s *Store) resolveOwned(ctx context.Context, sess sqlate.Session, path, unit string) (blobfs.Directory, error) {
	top := topLevelOf(path)
	dir, err := s.resolve(ctx, sess, top)
	if err != nil {
		return blobfs.Directory{}, err
	}
	owned, err := s.owns(ctx, sess, unit, dir.ID)
	if err != nil {
		return blobfs.Directory{}, err
	}
	if !owned {
		return blobfs.Directory{}, fmt.Errorf("%s: %w", top, ErrNotOwned)
	}
	if top == path {
		return dir, nil
	}
	return s.resolve(ctx, sess, path)
}

// topLevel is the listing of the root as the unit l names: the unit's
// top-level directories through the owner read model, one page by number,
// and an empty file half, whose total is 0 when l counts and NoTotal when
// it does not.
func (s *Store) topLevel(ctx context.Context, sess sqlate.Session, l Listing) (Contents, error) {
	dirs, err := s.ownedBy(ctx, sess, l.Unit, l)
	if err != nil {
		return Contents{}, err
	}
	files := Page[blobfs.File]{Total: 0}
	if l.Total == TotalNone {
		files.Total = NoTotal
	}
	return Contents{Path: "/", Directories: dirs, Files: files}, nil
}

// ListDirectory returns the contents of the directory with id under l, as
// List does for a path, in one read-only repeatable-read transaction. The
// directory is read first, since blobfs lists a directory that does not
// exist as empty, so an id no directory holds is blobfs.ErrNotFound. The
// contents' Path is empty: no path is computed for a listing by id. A
// listing by id has no path to derive a unit's scope from, so a unit in l
// is ErrNotOwned before any I/O.
func (s *Store) ListDirectory(ctx context.Context, id string, l Listing) (Contents, error) {
	if l.Unit != "" {
		return Contents{}, fmt.Errorf("files: ls directory %s as unit %s: a listing by id has no path to derive the scope from: %w", id, l.Unit, ErrNotOwned)
	}
	c, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (Contents, error) {
		if _, err := s.blobfs.Directories.Find(ctx, tx, id); err != nil {
			return Contents{}, err
		}
		return s.contents(ctx, tx, "", id, l)
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
	if err != nil {
		return Contents{}, fmt.Errorf("files: ls directory %s: %w", id, err)
	}
	return c, nil
}

// contents reads the two halves of the directory with id through sess:
// the directory half under the terms naming a directory field, the file
// half under every term, each from its own cursor when l carries one. path
// is what the result reports as listed.
func (s *Store) contents(ctx context.Context, sess sqlate.Session, path, id string, l Listing) (Contents, error) {
	dirs, err := half(ctx, sess, s.blobfs.Directories, id, l, directoryFields, l.After.Directories)
	if err != nil {
		return Contents{}, err
	}
	files, err := half(ctx, sess, s.blobfs.Files, id, l, nil, l.After.Files)
	if err != nil {
		return Contents{}, err
	}
	return Contents{Path: path, Directories: dirs, Files: files}, nil
}

// Stat returns the row of the file at path, whatever its status: the
// parent directory is resolved and the last segment looked up among its
// files, on the pool. A file that does not exist, or a parent that does
// not, is blobfs.ErrNotFound; the root, which is no file, is
// blobfs.ErrRootDirectory.
func (s *Store) Stat(ctx context.Context, path string) (blobfs.File, error) {
	f, err := s.stat(ctx, path)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat %s: %w", path, err)
	}
	return f, nil
}

// stat is Stat's body, its errors unlabelled, so the operations that read
// a file by path first label its errors as their own.
func (s *Store) stat(ctx context.Context, path string) (blobfs.File, error) {
	parent, name, err := splitParent(path)
	if err != nil {
		return blobfs.File{}, err
	}
	dir, err := s.resolve(ctx, s.db, parent)
	if err != nil {
		return blobfs.File{}, err
	}
	return s.blobfs.Files.FindByName(ctx, s.db, dir.ID, name)
}

// StatFile returns the row of the file with id, whatever its status, on
// the pool. A file that does not exist is blobfs.ErrNotFound.
func (s *Store) StatFile(ctx context.Context, id string) (blobfs.File, error) {
	f, err := s.blobfs.Files.Find(ctx, s.db, id)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat file %s: %w", id, err)
	}
	return f, nil
}

// Resolve returns the row of the directory at path, on the pool: the path
// form of StatDirectory. The root is the seeded root row. A segment that
// names no directory is blobfs.ErrNotFound, and a path that does not start
// with a slash, or that has an empty segment or a trailing slash,
// blobfs.ErrInvalidPath.
func (s *Store) Resolve(ctx context.Context, path string) (blobfs.Directory, error) {
	d, err := s.resolve(ctx, s.db, path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: stat %s: %w", path, err)
	}
	return d, nil
}

// StatDirectory returns the row of the directory with id, on the pool. A
// directory that does not exist is blobfs.ErrNotFound.
func (s *Store) StatDirectory(ctx context.Context, id string) (blobfs.Directory, error) {
	d, err := s.blobfs.Directories.Find(ctx, s.db, id)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: stat directory %s: %w", id, err)
	}
	return d, nil
}

// Find returns the row with id as stat and mv find it by id: the file
// first, then the directory when no file has the id. An id neither table
// holds is blobfs.ErrNotFound.
func (s *Store) Find(ctx context.Context, id string) (Entry, error) {
	f, err := s.blobfs.Files.Find(ctx, s.db, id)
	switch {
	case err == nil:
		return Entry{Kind: EntryFile, File: f}, nil
	case !errors.Is(err, blobfs.ErrNotFound):
		return Entry{}, fmt.Errorf("files: id %s: %w", id, err)
	}
	d, err := s.blobfs.Directories.Find(ctx, s.db, id)
	switch {
	case errors.Is(err, blobfs.ErrNotFound):
		return Entry{}, fmt.Errorf("files: id %s: no file or directory has it: %w", id, blobfs.ErrNotFound)
	case err != nil:
		return Entry{}, fmt.Errorf("files: id %s: %w", id, err)
	}
	return Entry{Kind: EntryDirectory, Directory: d}, nil
}

// Mkdir creates the directory at path under its parent, which must exist.
// There is no -p: a missing parent is blobfs.ErrNotFound. The root is
// blobfs.ErrRootDirectory before any I/O, and a name an active directory
// holds in the parent is blobfs.ErrNameTaken.
//
// Without a unit the parent is resolved and the directory created on the
// pool. With one, the path must name a top-level directory (ErrUnitDepth
// otherwise, before any I/O), and the directory and the owner row that
// binds it to the unit are written in one transaction, so a directory
// created with a unit never exists without its owner.
func (s *Store) Mkdir(ctx context.Context, path, unit string) (blobfs.Directory, error) {
	parent, name, err := splitParent(path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: mkdir %s: %w", path, err)
	}
	if unit != "" && parent != "/" {
		return blobfs.Directory{}, fmt.Errorf("files: mkdir %s: %w", path, ErrUnitDepth)
	}
	create := func(sess sqlate.Session) (blobfs.Directory, error) {
		dir, err := s.resolve(ctx, sess, parent)
		if err != nil {
			return blobfs.Directory{}, err
		}
		return s.blobfs.Directories.Create(ctx, sess, dir.ID, name)
	}
	var made blobfs.Directory
	if unit == "" {
		made, err = create(s.db)
	} else {
		made, err = s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
			made, err := create(tx)
			if err != nil {
				return blobfs.Directory{}, err
			}
			return made, s.insertOwner(ctx, tx, made.ID, unit)
		})
	}
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: mkdir %s: %w", path, err)
	}
	return made, nil
}

// RemoveDirectory removes the empty directory at path, with its owner row
// when it has one, in one transaction: the path is resolved, the owner row
// removed, and the directory removed through blobfs, whose refusal rolls
// the owner row back with it. A directory that still has directories or
// files under it is blobfs.ErrNotEmpty, and the root is
// blobfs.ErrRootDirectory before any I/O.
func (s *Store) RemoveDirectory(ctx context.Context, path string) (blobfs.Directory, error) {
	if _, _, err := splitParent(path); err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: rmdir %s: %w", path, err)
	}
	dir, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
		dir, err := s.resolve(ctx, tx, path)
		if err != nil {
			return blobfs.Directory{}, err
		}
		if err := s.deleteOwner(ctx, tx, dir.ID); err != nil {
			return blobfs.Directory{}, err
		}
		return dir, s.blobfs.Directories.Delete(ctx, tx, dir.ID)
	})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: rmdir %s: %w", path, err)
	}
	return dir, nil
}

// Move moves the directory or file at src to dst, in one transaction. dst
// is read the way Unix reads it: when it names an existing directory the
// source moves into it under its own name, and otherwise dst is the new
// path, whose parent must exist and whose last segment is the new name, so
// a move to a new name under the same parent is a rename. src is resolved
// as a directory first and as a file when no directory is at the path; a
// directory and a file may share a name, and the directory wins.
//
// A directory moves through blobfs's Directories.Move, which takes the
// tree lock and runs the cycle check inside this transaction, so a move
// into the directory itself or one of its descendants is blobfs.ErrCycle.
// A file moves through Files.Move. The directory's contents and the file's
// object follow by id: no key encodes a path, so nothing moves in the
// store.
//
// The move stays under one top-level directory (ErrMoveAcrossScopes
// otherwise), checked once both paths resolve and before anything
// changes. The root is blobfs.ErrRootDirectory before any I/O. A source
// that does not exist, or a destination whose parent does not, is
// blobfs.ErrNotFound; a name already held in the destination by an entry
// of the same kind is blobfs.ErrNameTaken.
func (s *Store) Move(ctx context.Context, src, dst string) (MoveResult, error) {
	srcParent, srcName, err := splitParent(src)
	if err != nil {
		return MoveResult{}, fmt.Errorf("files: mv %s: %w", src, err)
	}
	if !strings.HasPrefix(dst, "/") {
		return MoveResult{}, fmt.Errorf("files: mv %s %s: %w: %q does not start with /", src, dst, blobfs.ErrInvalidPath, dst)
	}
	res, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (MoveResult, error) {
		parent, parentPath, name, err := s.destination(ctx, tx, dst, srcName)
		if err != nil {
			return MoveResult{}, err
		}
		to := join(parentPath, name)
		if err := sameScope(src, to); err != nil {
			return MoveResult{}, err
		}
		dir, err := s.resolve(ctx, tx, src)
		switch {
		case err == nil:
			moved, err := s.blobfs.Directories.Move(ctx, tx, dir.ID, parent.ID, name, dir.Version)
			if err != nil {
				return MoveResult{}, err
			}
			return MoveResult{Kind: EntryDirectory, ID: moved.ID, From: src, To: join(parentPath, moved.Name)}, nil
		case !errors.Is(err, blobfs.ErrNotFound):
			return MoveResult{}, err
		}
		srcDir, err := s.resolve(ctx, tx, srcParent)
		if err != nil {
			return MoveResult{}, err
		}
		f, err := s.blobfs.Files.FindByName(ctx, tx, srcDir.ID, srcName)
		if err != nil {
			return MoveResult{}, err
		}
		moved, err := s.blobfs.Files.Move(ctx, tx, f.ID, parent.ID, name, f.Version)
		if err != nil {
			return MoveResult{}, err
		}
		return MoveResult{Kind: EntryFile, ID: moved.ID, From: src, To: join(parentPath, moved.Name)}, nil
	})
	if err != nil {
		return MoveResult{}, fmt.Errorf("files: mv %s %s: %w", src, dst, err)
	}
	return res, nil
}

// MoveEntry moves the directory or file with req.ID into the directory
// with req.DirectoryID, as req.Name or under its own name when req.Name is
// empty, in one transaction: Move with both resolutions replaced by ids.
// The row is read first, for its name, its parent, and its version;
// req.Version, when not 0, guards the move in place of the version read.
// The rules are Move's, the one-top-level-directory rule included, which
// by id needs the paths of the source's parent and of the destination,
// each computed by blobfs's Directories.Path before anything changes;
// those paths fill From and To, so a move by id reports what a move by
// path does.
//
// The root as a directory source is blobfs.ErrRootDirectory, and a kind
// other than EntryDirectory or EntryFile is refused, both before any I/O.
func (s *Store) MoveEntry(ctx context.Context, req MoveRequest) (MoveResult, error) {
	switch req.Kind {
	case EntryDirectory, EntryFile:
	default:
		return MoveResult{}, fmt.Errorf("files: mv %q %s: the kind is %s or %s", req.Kind, req.ID, EntryDirectory, EntryFile)
	}
	if req.Kind == EntryDirectory && req.ID == blobfs.RootID {
		return MoveResult{}, fmt.Errorf("files: mv %s %s: %w", req.Kind, req.ID, blobfs.ErrRootDirectory)
	}
	res, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (MoveResult, error) {
		var parentID, srcName string
		var version int64
		switch req.Kind {
		case EntryDirectory:
			d, err := s.blobfs.Directories.Find(ctx, tx, req.ID)
			if err != nil {
				return MoveResult{}, err
			}
			if d.ParentID == nil {
				return MoveResult{}, blobfs.ErrRootDirectory
			}
			parentID, srcName, version = *d.ParentID, d.Name, d.Version
		case EntryFile:
			f, err := s.blobfs.Files.Find(ctx, tx, req.ID)
			if err != nil {
				return MoveResult{}, err
			}
			parentID, srcName, version = f.DirectoryID, f.Name, f.Version
		}
		if req.Version != 0 {
			version = req.Version
		}
		name := req.Name
		if name == "" {
			name = srcName
		}
		fromDir, err := s.blobfs.Directories.Path(ctx, tx, parentID)
		if err != nil {
			return MoveResult{}, err
		}
		toDir, err := s.blobfs.Directories.Path(ctx, tx, req.DirectoryID)
		if err != nil {
			return MoveResult{}, err
		}
		from := join(fromDir, srcName)
		if err := sameScope(from, join(toDir, name)); err != nil {
			return MoveResult{}, err
		}
		res := MoveResult{Kind: req.Kind, ID: req.ID, From: from}
		switch req.Kind {
		case EntryDirectory:
			moved, err := s.blobfs.Directories.Move(ctx, tx, req.ID, req.DirectoryID, name, version)
			if err != nil {
				return MoveResult{}, err
			}
			res.To = join(toDir, moved.Name)
		case EntryFile:
			moved, err := s.blobfs.Files.Move(ctx, tx, req.ID, req.DirectoryID, name, version)
			if err != nil {
				return MoveResult{}, err
			}
			res.To = join(toDir, moved.Name)
		}
		return res, nil
	})
	if err != nil {
		return MoveResult{}, fmt.Errorf("files: mv %s %s into %s: %w", req.Kind, req.ID, req.DirectoryID, err)
	}
	return res, nil
}

// destination reads mv's destination through sess: the directory the
// source moves into, that directory's path, and the name the source takes
// there. dst names an existing directory, in which case the name is the
// source's own, or a new path, in which case the parent must exist and the
// last segment is the name.
func (s *Store) destination(ctx context.Context, sess sqlate.Session, dst, srcName string) (blobfs.Directory, string, string, error) {
	dir, err := s.resolve(ctx, sess, dst)
	switch {
	case err == nil:
		return dir, dst, srcName, nil
	case !errors.Is(err, blobfs.ErrNotFound):
		return blobfs.Directory{}, "", "", err
	}
	parentPath, name, err := splitParent(dst)
	if err != nil {
		return blobfs.Directory{}, "", "", err
	}
	parent, err := s.resolve(ctx, sess, parentPath)
	if err != nil {
		return blobfs.Directory{}, "", "", err
	}
	return parent, parentPath, name, nil
}

// resolve returns the directory at the absolute path through sess: blobfs
// resolves the path below the root, so / is the root itself. A path that
// does not start with a slash is blobfs.ErrInvalidPath before any I/O.
func (s *Store) resolve(ctx context.Context, sess sqlate.Session, path string) (blobfs.Directory, error) {
	rest, ok := strings.CutPrefix(path, "/")
	if !ok {
		return blobfs.Directory{}, fmt.Errorf("%w: %q does not start with /", blobfs.ErrInvalidPath, path)
	}
	return s.blobfs.Directories.FindByPath(ctx, sess, blobfs.RootID, rest)
}

// splitParent splits the path of an entry into its parent's path and its
// name. The root itself is blobfs.ErrRootDirectory, and a relative path or
// one ending with a slash blobfs.ErrInvalidPath. The parent's segments are
// validated when the parent is resolved and the name when the row is
// written.
func splitParent(path string) (parent, name string, err error) {
	if !strings.HasPrefix(path, "/") {
		return "", "", fmt.Errorf("%w: %q does not start with /", blobfs.ErrInvalidPath, path)
	}
	if path == "/" {
		return "", "", blobfs.ErrRootDirectory
	}
	at := strings.LastIndex(path, "/")
	parent, name = path[:at], path[at+1:]
	if name == "" {
		return "", "", fmt.Errorf("%w: %q ends with a slash", blobfs.ErrInvalidPath, path)
	}
	if parent == "" {
		parent = "/"
	}
	return parent, name, nil
}

// join returns the path of name in the directory at dir.
func join(dir, name string) string {
	return strings.TrimSuffix(dir, "/") + "/" + name
}

// sameScope refuses a move from the path from to the path to unless both
// lie under one top-level directory, where an entry at the top level
// counts as lying under the root, with ErrMoveAcrossScopes.
func sameScope(from, to string) error {
	if scopeOf(from) == scopeOf(to) {
		return nil
	}
	return fmt.Errorf("%s is under %s and %s under %s: %w", from, scopePath(from), to, scopePath(to), ErrMoveAcrossScopes)
}

// topLevelOf returns the path of the top-level directory that contains the
// entry at path, the path itself for a top-level entry.
func topLevelOf(path string) string {
	first, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return "/" + first
}

// scopeOf returns the normalized name of the top-level directory that
// contains the entry at path, or the empty string when the entry is itself
// at the top level, so that the root contains it.
func scopeOf(path string) string {
	first, _, below := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !below {
		return ""
	}
	return blobfs.NormalizeName(first)
}

// scopePath renders scopeOf(path) for a message: the top-level directory's
// path, or / for the root.
func scopePath(path string) string {
	if scope := scopeOf(path); scope != "" {
		return "/" + scope
	}
	return "/"
}
