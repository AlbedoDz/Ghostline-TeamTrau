// Package logx provides a size-rotated log file writer.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Rotating writes to <dir>/<base>.log and rotates it to <base>.1.log …
// <base>.<keep-1>.log once it would exceed maxBytes.
type Rotating struct {
	mu       sync.Mutex
	dir      string
	base     string
	maxBytes int64
	keep     int
	f        *os.File
	size     int64
}

func NewRotating(dir, base string, maxBytes int64, keep int) (*Rotating, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Rotating{dir: dir, base: base, maxBytes: maxBytes, keep: keep}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rotating) name(i int) string {
	if i == 0 {
		return filepath.Join(r.dir, r.base+".log")
	}
	return filepath.Join(r.dir, fmt.Sprintf("%s.%d.log", r.base, i))
}

func (r *Rotating) open() error {
	f, err := os.OpenFile(r.name(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, fi.Size()
	return nil
}

func (r *Rotating) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	os.Remove(r.name(r.keep - 1))
	for i := r.keep - 2; i >= 0; i-- {
		_ = os.Rename(r.name(i), r.name(i+1)) // older files may not exist yet
	}
	return r.open()
}

func (r *Rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *Rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
