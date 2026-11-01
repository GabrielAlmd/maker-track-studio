package diagnostics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var ErrWriterInUse = errors.New("diagnostic directory is in use")

// RotatingWriter is owned exclusively by Logger's worker. Constructor errors
// are fixed messages; the caller can select memory-only diagnostics on failure.
type RotatingWriter struct {
	root     *os.Root
	lock     *os.File
	file     *os.File
	buffer   *bufio.Writer
	size     int64
	limit    int64
	retryAt  time.Time
	failures int
	closed   bool
	// Isolated fault-injection seam; production always uses root.Rename.
	rename func(string, string) error
}

func rejectLinks(path string) error {
	for {
		info, err := os.Lstat(path)
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return ErrSinkUnavailable
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrSinkUnavailable
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func NewRotatingWriter(directory string, config Config) (*RotatingWriter, error) {
	if config.Validate() != nil {
		return nil, initializationError("CONFIG", nil)
	}
	path, err := filepath.Abs(directory)
	if err != nil || directory == "" || rejectLinks(path) != nil {
		return nil, initializationError("DIRECTORY_PATH", err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, initializationError("DIRECTORY_CREATE", err)
	}
	if rejectLinks(path) != nil {
		return nil, initializationError("DIRECTORY_LINK", nil)
	}
	// Restrict the directory before any diagnostic content is created.
	dir, err := os.Open(path)
	if err != nil {
		return nil, initializationError("DIRECTORY_OPEN", err)
	}
	err = protectFile(dir, true)
	dir.Close()
	if err != nil {
		return nil, initializationError("DIRECTORY_PROTECT", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, initializationError("ROOT_OPEN", err)
	}
	w := &RotatingWriter{root: root, limit: config.MaxFileBytes}
	w.rename = root.Rename
	lock, err := w.openPrivate("writer.lock")
	if err != nil {
		root.Close()
		return nil, initializationError("LOCK_OPEN", err)
	}
	if err := lockFile(lock); err != nil {
		lock.Close()
		root.Close()
		if errors.Is(err, ErrWriterInUse) {
			return nil, ErrWriterInUse
		}
		return nil, initializationError("LOCK_ACQUIRE", err)
	}
	w.lock = lock
	// Tighten permissions on retained logs too, without reading their content.
	for i := 1; i <= PreviousLogFiles; i++ {
		name := fmt.Sprintf("application.log.%d", i)
		if _, err := root.Lstat(name); errors.Is(err, os.ErrNotExist) {
			continue
		}
		f, err := w.openPrivate(name)
		if err != nil {
			w.Close()
			return nil, initializationError("ARCHIVE_OPEN", err)
		}
		if err := f.Close(); err != nil {
			w.Close()
			return nil, initializationError("ARCHIVE_CLOSE", err)
		}
	}
	if err := w.openActive(); err != nil {
		w.Close()
		return nil, initializationError("ACTIVE_OPEN", err)
	}
	return w, nil
}

func (w *RotatingWriter) checkFile(name string) error {
	info, err := w.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return ErrSinkUnavailable
	}
	return nil
}

func (w *RotatingWriter) openPrivate(name string) (*os.File, error) {
	if err := w.checkFile(name); err != nil {
		return nil, err
	}
	f, err := w.root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := protectFile(f, false); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (w *RotatingWriter) openActive() error {
	f, err := w.openPrivate("application.log")
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	size := info.Size()
	// Read only a bounded tail. A partial final record cannot exceed one event
	// for files we own. If it does, rotate the oversized/corrupt file untouched.
	if size > 0 && size <= w.limit {
		length := min(size, int64(MaxEventBytes))
		tail := make([]byte, int(length))
		if _, err := f.ReadAt(tail, size-length); err != nil {
			f.Close()
			return err
		}
		if tail[len(tail)-1] != '\n' {
			last := bytes.LastIndexByte(tail, '\n')
			if last < 0 && size > length {
				// No safe boundary: force a rotation before any append.
				size = w.limit
			} else {
				size = size - length + int64(last+1)
				if err := f.Truncate(size); err != nil {
					f.Close()
					return err
				}
			}
		}
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return err
	}
	w.file, w.size = f, size
	w.buffer = bufio.NewWriterSize(f, WriteBufferBytes)
	return nil
}

func (w *RotatingWriter) fail() error {
	if w.file != nil {
		w.file.Close()
	}
	w.file, w.buffer = nil, nil
	delays := [...]time.Duration{time.Second, 5 * time.Second, 15 * time.Second, time.Minute}
	w.retryAt = time.Now().Add(delays[min(w.failures, len(delays)-1)])
	w.failures++
	return ErrSinkUnavailable
}

func (w *RotatingWriter) ready() error {
	if w.closed {
		return ErrSinkUnavailable
	}
	if w.file != nil {
		return nil
	}
	if time.Now().Before(w.retryAt) {
		return ErrSinkUnavailable
	}
	if err := w.openActive(); err != nil {
		return w.fail()
	}
	return nil
}

func (w *RotatingWriter) rotate() error {
	if err := w.buffer.Flush(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file, w.buffer = nil, nil
	// Preflight all owned slots before mutating any of them.
	for i := 1; i <= PreviousLogFiles; i++ {
		if err := w.checkFile(fmt.Sprintf("application.log.%d", i)); err != nil {
			return err
		}
	}
	if err := w.root.Remove("application.log.3"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for i := PreviousLogFiles - 1; i >= 0; i-- {
		from := "application.log"
		if i > 0 {
			from = fmt.Sprintf("application.log.%d", i)
		}
		to := fmt.Sprintf("application.log.%d", i+1)
		if err := w.rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return w.openActive()
}

// WriteEvent accepts one bounded JSON line. The Logger is the privacy boundary;
// this low-level sink must never be wired directly to arbitrary frontend input.
func (w *RotatingWriter) WriteEvent(data []byte) error {
	if len(data) == 0 || len(data) > MaxEventBytes || data[len(data)-1] != '\n' ||
		bytes.ContainsRune(data[:len(data)-1], '\n') || !json.Valid(data[:len(data)-1]) {
		return errors.New("invalid diagnostic record")
	}
	if err := w.ready(); err != nil {
		return err
	}
	if w.size+int64(len(data)) > w.limit {
		if err := w.rotate(); err != nil {
			return w.fail()
		}
	}
	n, err := w.buffer.Write(data)
	if err != nil || n != len(data) {
		return w.fail()
	}
	w.size += int64(n)
	return nil
}

func (w *RotatingWriter) Flush() error {
	if err := w.ready(); err != nil {
		return err
	}
	if err := w.buffer.Flush(); err != nil {
		return w.fail()
	}
	w.failures = 0
	return nil
}

func (w *RotatingWriter) Sync() error {
	if err := w.Flush(); err != nil {
		return err
	}
	if err := w.file.Sync(); err != nil {
		return w.fail()
	}
	return nil
}

func (w *RotatingWriter) Close() error {
	if w.closed {
		return nil
	}
	var failed bool
	if w.file != nil {
		failed = w.buffer.Flush() != nil
		if w.file.Close() != nil {
			failed = true
		}
	}
	w.closed = true
	if w.lock != nil {
		if w.lock.Close() != nil {
			failed = true
		}
	}
	if w.root != nil {
		if w.root.Close() != nil {
			failed = true
		}
	}
	if failed {
		return ErrSinkUnavailable
	}
	return nil
}
