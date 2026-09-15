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

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) Bytes() []byte { return b.buf.Bytes() }
func (b *cappedBuffer) String() string { return b.buf.String() }
