package output

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SpillWriter is WriteSpillFile with the write spread over time: rows go to a
// temp file in dir as they arrive and are committed onto the final name by
// Commit. A streamed result only learns that name once the response metadata
// has arrived — after the rows — so the destination cannot be chosen upfront.
//
// The same atomicity holds as for WriteSpillFile: readers never observe a
// half-written file, and an abandoned stream leaves only a .tmp for the TTL
// prune to sweep.
type SpillWriter struct {
	dir       string
	tmp       *os.File
	buf       *bufio.Writer // batches the per-row writes into few syscalls
	counter   *countingWriter
	committed bool
	closed    bool
}

// NewSpillWriter creates the temp file that rows are streamed into. dir is
// created with 0700 if missing; Commit must name a path inside it.
func NewSpillWriter(dir string) (*SpillWriter, error) {
	if err := os.MkdirAll(dir, spillDirMode); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, "stream.*"+tmpSuffix)
	if err != nil {
		return nil, err
	}
	buf := bufio.NewWriterSize(tmp, 64<<10)
	return &SpillWriter{dir: dir, tmp: tmp, buf: buf, counter: &countingWriter{w: buf}}, nil
}

// Writer returns the sink rows are written to.
func (w *SpillWriter) Writer() io.Writer { return w.counter }

// Bytes reports how much has been written so far.
func (w *SpillWriter) Bytes() int64 { return w.counter.n }

// Commit closes the temp file and renames it onto targetPath, which must live
// in the directory the writer was created with (rename(2) is only atomic within
// one filesystem). It returns the number of bytes written.
func (w *SpillWriter) Commit(targetPath string) (int64, error) {
	if filepath.Dir(targetPath) != filepath.Clean(w.dir) {
		return 0, fmt.Errorf("spill target %q is not in the streaming directory %q", targetPath, w.dir)
	}
	tmpName := w.tmp.Name()
	if err := w.buf.Flush(); err != nil {
		_ = w.tmp.Close()
		w.closed = true
		_ = os.Remove(tmpName)
		return 0, err
	}
	if err := w.tmp.Close(); err != nil {
		w.closed = true
		_ = os.Remove(tmpName)
		return 0, err
	}
	w.closed = true
	if err := os.Chmod(tmpName, spillFileMode); err != nil {
		_ = os.Remove(tmpName)
		return 0, err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		_ = os.Remove(tmpName)
		return 0, err
	}
	w.committed = true
	return w.counter.n, nil
}

// Abort discards an uncommitted stream. It is safe to call after Commit, and
// safe to call twice, so it works as a defer.
func (w *SpillWriter) Abort() {
	if w.committed {
		return
	}
	name := w.tmp.Name()
	if !w.closed {
		_ = w.tmp.Close()
		w.closed = true
	}
	_ = os.Remove(name)
}
