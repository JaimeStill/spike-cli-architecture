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
	// level and is allowed.
	ErrMoveAcrossScopes = errors.New("files: a move stays under one top-level directory")
)
