package engine

import "bytes"

// cappedBuffer is a bytes.Buffer that silently discards writes once the byte
// limit is reached. Excess bytes are dropped (never error), so the writer
// inside cmd.Run always sees a healthy io.Writer. When truncation occurs,
// Truncated is set to true.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int64
	truncated bool
}

// Write always reports success for the caller's full-length write, even when
// the excess bytes never reach buf — the doc comment's contract. Returning a
// short count here (n < len(p) with err == nil) would violate io.Writer and
// abort the caller's io.Copy with io.ErrShortWrite (e.g. the stdout/stderr
// pipe drain in cmd/stdio.go), exactly the kind of failure this type exists
// to prevent.
func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.truncated = true
		return n, nil
	}
	if int64(len(p)) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, err := b.buf.Write(p)
	return n, err
}

func (b *cappedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *cappedBuffer) String() string { return b.buf.String() }
