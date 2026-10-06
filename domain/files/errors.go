package files

import "errors"

// The sentinels the domain adds to blobfs's. A refusal matches one of
// these or one of blobfs's own, so a caller classifies it with errors.Is.
var (
	// ErrVerify reports a database blobfs's statements do not prepare
	// against. The usual cause is a schema that is not applied, so the
	// message says which command applies it. It carries no "files:" prefix
	// because it is reported as the files node's start error, which the
	// lifecycle labels with the node's name.
	ErrVerify = errors.New("the database does not satisfy the statements; if the schema is not applied, run blobfs schema up")

	// ErrMoveAcrossScopes reports a mv whose source and destination lie
	// under different top-level directories, or one of them at the top
	// level and the other below it. A top-level directory is the grain an
	// owner binds, so a move that crossed it would carry an entry out of
	// one owner's scope into another's, or move a top-level directory to
	// another depth. A rename of a top-level directory stays at the top
	// level and is allowed, and the owner row follows it by id.
	ErrMoveAcrossScopes = errors.New("files: a move stays under one top-level directory")

	// ErrNotAvailable reports a cat or a cp of a file that has no content
	// to read: a pending file, whose object is not written yet, or a
	// deleting one, whose object is being removed. The message names the
	// status. It carries no "files:" prefix because the operation that
	// refuses it names itself so.
	ErrNotAvailable = errors.New("the file is not available")

	// ErrUnitDepth reports a mkdir with a unit at a path that is not at
	// depth one. An owner row binds a top-level directory only; every
	// directory below it is in that directory's scope.
	ErrUnitDepth = errors.New("--unit applies to a top-level directory only")

	// ErrNotOwned reports a listing under a unit that does not own the
	// top-level directory of the listed path: another unit owns it, or no
	// unit does.
	ErrNotOwned = errors.New("the unit does not own the directory")

	// ErrNoCursorAtRoot reports a listing of / under a unit that was asked
	// to continue from a cursor. That listing reads the unit's top-level
	// directories through the domain's owner read model, which pages by
	// number only, and lists no files.
	ErrNoCursorAtRoot = errors.New("ls / --unit pages by number only; the owner read model takes no cursor")

	// ErrBookmarked reports an rm refused because a unit bookmarks the file,
	// or an rm --recursive refused because a unit bookmarks a file in the
	// branch. The refusal comes in the delete's first transaction, which
	// rolls back, so nothing is touched; the caller removes the bookmarks
	// and runs rm again.
	ErrBookmarked = errors.New("the file is bookmarked")

	// ErrAlreadyBookmarked reports a bookmark add of a file the unit has
	// bookmarked already, active or not: the violation of the bookmark
	// table's primary key. A bookmark is added once and removed once; there
	// is no activation of an existing one.
	ErrAlreadyBookmarked = errors.New("the unit has bookmarked the file already")

	// ErrActiveBookmark reports a bookmark add with --active while another
	// bookmark of the unit is active: the violation of the partial unique
	// index uq_bookmark_active, which allows one active bookmark per unit.
	// The other bookmark is left as it is; the caller removes it first.
	ErrActiveBookmark = errors.New("the unit has an active bookmark already")

	// ErrNoBookmark reports a bookmark rm of a file the unit has not
	// bookmarked. The file exists; the bookmark does not.
	ErrNoBookmark = errors.New("the unit has no bookmark of the file")
)

// The names of the constraints and the unique index the app's bookmark
// migration declares, which database.go maps to the sentinels above. They
// carry no blobfs_ prefix, so a violation of one is told from one of
// blobfs's.
const (
	// ConstraintPrimaryKeyBookmark is the primary key on bookmark
	// (unit_id, file_id). A violation on an add is ErrAlreadyBookmarked.
	ConstraintPrimaryKeyBookmark = "pk_bookmark"

	// ConstraintUniqueBookmarkActive is the partial unique index on
	// bookmark (unit_id) WHERE active. A violation on an add is
	// ErrActiveBookmark.
	ConstraintUniqueBookmarkActive = "uq_bookmark_active"

	// ConstraintForeignKeyBookmarkFile is the foreign key from
	// bookmark.file_id to blobfs_file.id. A violation on an add is
	// blobfs.ErrNotFound: the file was removed between its resolution and
	// the insert.
	ConstraintForeignKeyBookmarkFile = "fk_bookmark_file"
)
