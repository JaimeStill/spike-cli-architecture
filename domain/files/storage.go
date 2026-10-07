package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/standards-lab/blobfs"
	bfdata "github.com/standards-lab/blobfs/data"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/sqlate"
)

// This file holds the Storage and the blob protocol: the object
// operations, Put, Open, Copy, Remove, and RemoveTree, composed from
// blobfs's protocols over the database and the object
// store. Every write runs blobfs's two-phase write and every delete its
// two-phase delete, so the steps, their transactions, and their cleanup
// are the ones blobfs's conformance suite proves; the domain adds the path
// resolution and the refusals of its own. It holds the adapter that is
// blobfs's ObjectStore over go-storage's Store.

// Storage is the domain's second API, over the database and the object
// store: the [Service]'s data access, which it shares, and the object
// store blobfs's protocols put to, read from, and delete from. Its methods
// are the object operations. Only the commands that touch objects run on
// it, so only they declare the object store, and the Service's commands
// never build it.
type Storage struct {
	store   *store
	objects objectStore
}

// NewStorage returns the object operations over svc's data access and st,
// the started object store. It does no I/O.
func NewStorage(svc *Service, st *storage.Store) *Storage {
	return &Storage{store: svc.store, objects: objectStore{st}}
}

// objectStore is the object store as blobfs's protocols call it, its
// bfdata.ObjectStore: the key check, the put, and the delete over
// go-storage's Store, and the read cat and cp make.
type objectStore struct {
	store *storage.Store
}

// ValidateKey reports whether the store's provider accepts key, by the
// provider's own rule.
func (o objectStore) ValidateKey(key string) error {
	if validate := o.store.Capabilities().ValidateKey; validate != nil {
		return validate(key)
	}
	return nil
}

// PutObject stores size bytes of body under key in contentType, or the
// whole body when size is 0, and reports the stored object as blobfs
// records it.
func (o objectStore) PutObject(ctx context.Context, key string, body io.Reader, contentType string, size int64) (blobfs.Object, error) {
	obj, err := o.store.Put(ctx, key, body, storage.PutOptions{ContentType: contentType, Size: size})
	if err != nil {
		return blobfs.Object{}, err
	}
	return blobfs.Object{Size: obj.Size, ContentType: obj.ContentType, ETag: obj.ETag}, nil
}

// DeleteObject removes the object under key. go-storage's Delete treats a
// missing object as success, as blobfs's protocols require of it.
func (o objectStore) DeleteObject(ctx context.Context, key string) error {
	return o.store.Delete(ctx, key)
}

// open opens the object under key for reading. The caller closes it.
func (o objectStore) open(ctx context.Context, key string) (io.ReadCloser, error) {
	blob, err := o.store.Get(ctx, key, storage.GetOptions{})
	if err != nil {
		return nil, err
	}
	return blob.Body, nil
}

// Put writes c as a new file and returns the row available: the file at
// dst's path, whose parent must exist, or the file named c.Name in the
// directory with dst's id. The parent is resolved on the pool. The root
// is blobfs.ErrRootDirectory and a relative path blobfs.ErrInvalidPath,
// and a directory id with no c.Name a [FormError], all before any I/O; a
// directory that does not exist is blobfs.ErrNotFound.
//
// The name is looked up first. A pending row that holds it, which a put
// that stopped after its first step left, is resumed: blobfs's EnsureFile
// runs under the row's own id, so Files.Ensure finds the row, and the
// object is put under its key in the content type it declared, and the row
// completed. Any other put runs blobfs's WriteFile, whose Files.Create
// commits the pending row before any byte is put; a name an available row
// holds is blobfs.ErrNameTaken, and one a deleting row holds that row's
// blobfs.DeletingError, both before the store is reached. A put or a
// completion that fails abandons the write, so the name is free for a
// retry.
func (o *Storage) Put(ctx context.Context, dst Ref, c Content) (PutResult, error) {
	at := dst.Path
	if dst.ID != "" {
		at = c.Name + " in directory " + dst.ID
	}
	res, err := o.put(ctx, dst, c)
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", at, err)
	}
	return res, nil
}

// put is Put's body, its errors unlabelled.
func (o *Storage) put(ctx context.Context, dst Ref, c Content) (PutResult, error) {
	if dst.ID != "" {
		if c.Name == "" {
			return PutResult{}, &FormError{Reason: "a file put into a directory by id takes a name"}
		}
		return o.write(ctx, dst.ID, c.Name, c)
	}
	parent, name, err := splitParent(dst.Path)
	if err != nil {
		return PutResult{}, err
	}
	dir, err := o.store.resolve(ctx, o.store.db, parent)
	if err != nil {
		return PutResult{}, err
	}
	return o.write(ctx, dir.ID, name, c)
}

// write writes c as the file name in the directory with directoryID.
func (o *Storage) write(ctx context.Context, directoryID, name string, c Content) (PutResult, error) {
	fs, db := o.store.blobfs, o.store.db
	held, err := fs.Files.FindByName(ctx, db, directoryID, name)
	switch {
	case err == nil && held.Status == blobfs.StatusPending:
		f, _, err := fs.EnsureFile(ctx, db, o.objects, held.ID, c.Body, c.Size, func(tx *sqlate.Tx) (blobfs.File, bfdata.WriteOutcome, error) {
			return fs.Files.Ensure(ctx, tx, o.objects, directoryID, name, c.ContentType, bfdata.WithID(held.ID))
		})
		if err != nil {
			return PutResult{}, err
		}
		return PutResult{File: f, Resumed: true}, nil
	case err != nil && !errors.Is(err, blobfs.ErrNotFound):
		return PutResult{}, err
	}
	f, err := fs.WriteFile(ctx, db, o.objects, c.Body, c.Size, func(tx *sqlate.Tx) (blobfs.File, error) {
		return fs.Files.Create(ctx, tx, o.objects, directoryID, name, c.ContentType)
	})
	if err != nil {
		return PutResult{}, err
	}
	return PutResult{File: f}, nil
}

// Open opens the content of the file ref names for reading and returns
// the row with it; the caller closes the reader. Only an available file
// has content: a pending or deleting one is ErrNotAvailable, before the
// store is reached.
func (o *Storage) Open(ctx context.Context, ref Ref) (io.ReadCloser, blobfs.File, error) {
	f, err := o.store.file(ctx, o.store.db, ref)
	if err != nil {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: %w", label(ref, "file"), err)
	}
	body, err := o.open(ctx, f)
	if err != nil {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: %w", label(ref, "file"), err)
	}
	return body, f, nil
}

// open checks that f is available and opens its object.
func (o *Storage) open(ctx context.Context, f blobfs.File) (io.ReadCloser, error) {
	if err := available(f); err != nil {
		return nil, err
	}
	return o.objects.open(ctx, f.Key)
}

// available refuses a file that has no content to read or copy: a pending
// file's object is not written yet, and a deleting file's is being
// removed.
func available(f blobfs.File) error {
	if f.Status != blobfs.StatusAvailable {
		return fmt.Errorf("%w: it is %s", ErrNotAvailable, f.Status)
	}
	return nil
}

// Copy copies the available file src names to a new file with its bytes
// and its content type. src and dst are two paths or two ids (a
// [FormError] otherwise, before any I/O). By path, dst is read as Move
// reads it: an existing directory receives the copy under the source's
// name, and any other path is the copy's path, whose parent must exist. By
// id, dst is the directory that receives the copy under the source's name,
// and the paths of the source's directory and of the destination are
// computed before the copy, so a copy by id reports what a copy by path
// does. The source and the destination are read on the pool.
//
// The copy is blobfs's WriteFile: Files.Create commits the copy's pending
// row in the source's content type, which refuses a name a pending or
// available file holds as blobfs.ErrNameTaken, so nothing is overwritten;
// the source's object is then opened, only once the row is committed, and
// streamed through this process under the copy's key, and the row
// completed.
//
// The root as src is blobfs.ErrRootDirectory and a relative dst
// blobfs.ErrInvalidPath, both before any I/O. A source that does not
// exist, or a destination directory that does not, is blobfs.ErrNotFound,
// before any row is written; a source that is not available is
// ErrNotAvailable.
func (o *Storage) Copy(ctx context.Context, src, dst Ref) (CopyResult, error) {
	at := label(src, "file") + " " + label(dst, "directory")
	if err := checkPair(src, dst); err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", at, err)
	}
	if src.ID == "" && !strings.HasPrefix(dst.Path, "/") {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w: %q does not start with /", at, blobfs.ErrInvalidPath, dst.Path)
	}
	res, err := o.copyRef(ctx, src, dst)
	if err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", at, err)
	}
	return res, nil
}

// copyRef is Copy's body, its errors unlabelled: the source read and
// checked available, the destination's directory, its path, and the
// copy's name found, and the copy written.
func (o *Storage) copyRef(ctx context.Context, src, dst Ref) (CopyResult, error) {
	db := o.store.db
	f, err := o.store.file(ctx, db, src)
	if err != nil {
		return CopyResult{}, err
	}
	if err := available(f); err != nil {
		return CopyResult{}, err
	}
	from, dirID, toDir, name := src.Path, dst.ID, "", f.Name
	if src.ID != "" {
		fromDir, err := o.store.blobfs.Directories.Path(ctx, db, f.DirectoryID)
		if err != nil {
			return CopyResult{}, err
		}
		from = join(fromDir, f.Name)
		if toDir, err = o.store.blobfs.Directories.Path(ctx, db, dst.ID); err != nil {
			return CopyResult{}, err
		}
	} else {
		var parent blobfs.Directory
		if parent, toDir, name, err = o.store.destination(ctx, db, dst.Path, f.Name); err != nil {
			return CopyResult{}, err
		}
		dirID = parent.ID
	}
	made, err := o.copy(ctx, f, dirID, name)
	if err != nil {
		return CopyResult{}, err
	}
	return CopyResult{From: from, To: join(toDir, made.Name), File: made}, nil
}

// copy writes src's object as the new file name in the directory with
// directoryID, through blobfs's WriteFile. The body opens src's object on
// its first read, which WriteFile makes only after the pending row
// commits, so a refused name never reaches the store.
func (o *Storage) copy(ctx context.Context, src blobfs.File, directoryID, name string) (blobfs.File, error) {
	body := &deferredBody{open: func() (io.ReadCloser, error) { return o.objects.open(ctx, src.Key) }}
	defer body.close()
	var size int64
	if src.Size != nil {
		size = *src.Size
	}
	fs := o.store.blobfs
	return fs.WriteFile(ctx, o.store.db, o.objects, body, size, func(tx *sqlate.Tx) (blobfs.File, error) {
		return fs.Files.Create(ctx, tx, o.objects, directoryID, name, src.ContentType)
	})
}

// deferredBody is a body that opens its reader on the first Read.
type deferredBody struct {
	open func() (io.ReadCloser, error)
	rc   io.ReadCloser
}

func (b *deferredBody) Read(p []byte) (int, error) {
	if b.rc == nil {
		rc, err := b.open()
		if err != nil {
			return 0, err
		}
		b.rc = rc
	}
	return b.rc.Read(p)
}

// close closes the reader if one was opened. Its error is dropped: the
// body was read to the end or the write already failed.
func (b *deferredBody) close() {
	if b.rc != nil {
		_ = b.rc.Close()
	}
}

// Remove deletes the file ref names, whatever its status, and returns
// its row as the delete found it. It is blobfs's RemoveFile: the file's
// lookup, the domain's check that the file may be removed, and blobfs's
// Files.Delete run in one transaction, which commits the row deleting; the
// object is then deleted and the row purged. A file a unit has bookmarked
// is refused with ErrBookmarked in that transaction, before anything is
// touched. A delete that stopped after its first step left the row
// deleting, and a later Remove finishes it. The root is
// blobfs.ErrRootDirectory before any I/O, and a file that does not exist
// is blobfs.ErrNotFound.
func (o *Storage) Remove(ctx context.Context, ref Ref) (blobfs.File, error) {
	if ref.ID == "" {
		if _, _, err := splitParent(ref.Path); err != nil {
			return blobfs.File{}, fmt.Errorf("files: rm %s: %w", ref.Path, err)
		}
	}
	f, err := o.remove(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return o.store.file(ctx, tx, ref)
	})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", label(ref, "file"), err)
	}
	return f, nil
}

// remove runs blobfs's RemoveFile of the file find reads in the delete's
// transaction, after the domain's removable check, and returns the row
// find read.
func (o *Storage) remove(ctx context.Context, find func(*sqlate.Tx) (blobfs.File, error)) (blobfs.File, error) {
	var f blobfs.File
	err := o.store.blobfs.RemoveFile(ctx, o.store.db, o.objects, func(tx *sqlate.Tx) (string, error) {
		var err error
		if f, err = find(tx); err != nil {
			return "", err
		}
		if err := o.removable(ctx, tx, f); err != nil {
			return "", err
		}
		return f.ID, nil
	})
	return f, err
}

// removable is the check a file's delete runs in its first transaction,
// after the file is read and before anything is touched: a file a unit has
// bookmarked is refused with ErrBookmarked, which rolls the transaction
// back with the row and the object as they were.
//
// blobfs's RemoveFile runs this check before its Files.Delete, whose
// update would take the file's row lock, so the check takes the lock
// itself first, with Files.Hold: bookmark add holds the row before it
// inserts, so an add that held first has committed its bookmark before the
// count reads, and one that arrives later waits for this transaction and
// then refuses the deleting row. A row deleting already, which an earlier
// rm left, cannot be held and needs no hold, since no add can reach it; its
// bookmarks are counted all the same.
func (o *Storage) removable(ctx context.Context, tx *sqlate.Tx, f blobfs.File) error {
	if err := o.store.blobfs.Files.Hold(ctx, tx, f.ID); err != nil && !errors.Is(err, blobfs.ErrDeleting) {
		return err
	}
	n, err := o.store.bookmarksOfFile(ctx, tx, f.ID)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%d unit(s) bookmark the file: %w", n, ErrBookmarked)
	}
	return nil
}

// RemoveTree deletes the directory at ref's path and everything beneath
// it, by blobfs's branch delete: Directories.MarkDeleting marks the branch
// deleting in one transaction, after which the branch takes nothing new,
// and blobfs's sweep then runs passes until no work remains, deleting each
// file's object and purging its row, and removing each directory once it
// is empty. A branch is removed by path alone (a [FormError] for an id,
// before any I/O). Each directory's owner row is removed in the transaction that
// removes the directory, through the sweep's OnRemoveDirectory hook, so an
// owned top-level directory goes with its owner row. The result counts
// what the passes removed.
//
// A branch that holds a file a unit has bookmarked is refused with
// ErrBookmarked in the mark's transaction, after the mark and before it
// commits, so the refusal rolls the mark back and nothing is touched. The
// mark takes every file's row lock, as a file's delete does, so it waits
// on a bookmark add's hold, and the count after it sees every bookmark a
// hold admitted; once the branch is marked, no add can hold its files.
//
// The sweep finishes every marked branch, not only this one, so a branch
// an earlier RemoveTree marked and did not finish, because it was
// interrupted, is finished too, and a RemoveTree of the same path, whose
// directory is still found while it is deleting, marks it again and
// finishes it. A pass's refusals, such as an object the store would not
// delete, leave their rows for a later run; the error of the last pass is
// returned with the counts. The root is blobfs.ErrRootDirectory before any
// I/O.
func (o *Storage) RemoveTree(ctx context.Context, ref Ref) (TreeRemoval, error) {
	if err := checkRemoveTree(ref); err != nil {
		return TreeRemoval{}, fmt.Errorf("files: rm branch %s: %w", label(ref, "directory"), err)
	}
	path := ref.Path
	if _, _, err := splitParent(path); err != nil {
		return TreeRemoval{}, fmt.Errorf("files: rm branch %s: %w", path, err)
	}
	fs, db := o.store.blobfs, o.store.db
	dir, err := o.store.resolve(ctx, db, path)
	if err != nil {
		return TreeRemoval{}, fmt.Errorf("files: rm branch %s: %w", path, err)
	}
	_, err = db.Transact(ctx, func(tx *sqlate.Tx) (bfdata.Marked, error) {
		marked, err := fs.Directories.MarkDeleting(ctx, tx, dir.ID)
		if err != nil {
			return bfdata.Marked{}, err
		}
		n, err := o.store.bookmarksInBranch(ctx, tx, dir.ID)
		if err != nil {
			return bfdata.Marked{}, err
		}
		if n > 0 {
			return bfdata.Marked{}, fmt.Errorf("%d bookmark(s) hold files in the branch: %w", n, ErrBookmarked)
		}
		return marked, nil
	})
	if err != nil {
		return TreeRemoval{}, fmt.Errorf("files: rm branch %s: %w", path, err)
	}
	removeOwner := bfdata.OnRemoveDirectory(func(ctx context.Context, tx *sqlate.Tx, dir blobfs.Directory) error {
		return o.store.deleteOwner(ctx, tx, dir.ID)
	})
	var removed TreeRemoval
	var last error
	err = bfdata.SweepUntilDone(ctx, nil,
		func(ctx context.Context) (bfdata.SweepResult, error) {
			return fs.Sweep(ctx, db, o.objects, removeOwner)
		},
		func(res bfdata.SweepResult, err error) {
			removed.Files += res.Files
			removed.Directories += res.Directories
			last = err
		})
	if err == nil {
		err = last
	}
	if err != nil {
		return removed, fmt.Errorf("files: rm branch %s: removed %d files and %d directories, then: %w", path, removed.Files, removed.Directories, err)
	}
	return removed, nil
}
