package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/hashcott/ghostline/internal/store"
)

// Write is one file an import replaces.
type Write struct {
	Path string
	Data []byte
}

// WriteError names the file whose write failed; every earlier write was
// rolled back.
type WriteError struct {
	Path string
	Err  error
}

func (e *WriteError) Error() string { return fmt.Sprintf("backup: write %s: %v", e.Path, e.Err) }
func (e *WriteError) Unwrap() error { return e.Err }

const bakSuffix = ".bak-import"

// Apply writes every file atomically, first copying an existing file to
// <path>.bak-import. If any write fails, every file is put back as it was
// (files the import created are removed) and no backup is left.
func Apply(ws []Write) error {
	type done struct {
		path    string
		existed bool
	}
	var did []done
	rollback := func() {
		for i := len(did) - 1; i >= 0; i-- {
			d := did[i]
			if d.existed {
				if b, err := os.ReadFile(d.path + bakSuffix); err == nil {
					_ = store.WriteFileAtomic(d.path, b)
				}
				_ = os.Remove(d.path + bakSuffix)
			} else {
				_ = os.Remove(d.path)
			}
		}
	}
	for _, w := range ws {
		old, err := os.ReadFile(w.Path)
		existed := err == nil
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			rollback()
			return &WriteError{Path: w.Path, Err: err}
		}
		if existed {
			if err := store.WriteFileAtomic(w.Path+bakSuffix, old); err != nil {
				rollback()
				return &WriteError{Path: w.Path, Err: err}
			}
		}
		did = append(did, done{w.Path, existed})
		if err := store.WriteFileAtomic(w.Path, w.Data); err != nil {
			rollback()
			return &WriteError{Path: w.Path, Err: err}
		}
	}
	return nil
}
