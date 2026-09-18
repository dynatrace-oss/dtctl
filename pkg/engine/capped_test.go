package engine

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCappedBuffer_WriteNeverShortWrites pins the io.Writer contract: Write
// must report the caller's full input length even once truncating, never a
// short count with a nil error.
func TestCappedBuffer_WriteNeverShortWrites(t *testing.T) {
	b := &cappedBuffer{limit: 10}
	chunk := []byte("0123456789ABCDEF") // 16 bytes: crosses the 10-byte cap on write 1
	for i := range 5 {
		n, err := b.Write(chunk)
		require.NoError(t, err)
		require.Equal(t, len(chunk), n,
			"write %d: Write must report the full input length even once truncating", i)
	}
	require.True(t, b.truncated)
	require.LessOrEqual(t, b.buf.Len(), 10)
}

// TestCappedBuffer_SurvivesIOCopy exercises the type through io.Copy, the
// real-world caller (cmd/stdio.go's pipe drain): a short write there aborts
// the copy with io.ErrShortWrite.
func TestCappedBuffer_SurvivesIOCopy(t *testing.T) {
	b := &cappedBuffer{limit: 100}
	src := bytes.NewReader(bytes.Repeat([]byte("x"), 1<<20)) // 1 MiB, far past the cap
	n, err := io.Copy(b, src)
	require.NoError(t, err, "io.Copy must not abort with io.ErrShortWrite")
	require.EqualValues(t, 1<<20, n, "io.Copy must report the full source length copied")
	require.True(t, b.truncated)
	require.LessOrEqual(t, b.buf.Len(), 100)
}
