package appengine

import (
	"testing"
)

func TestReadFileOrStdin(t *testing.T) {
	t.Run("read nonexistent file", func(t *testing.T) {
		_, err := ReadFileOrStdin("/nonexistent/file.txt")
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
	})
}
