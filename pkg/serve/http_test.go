package serve

import (
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerHasTimeouts(t *testing.T) {
	opts := ServeOptions{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}
	srv := newServer("127.0.0.1:0", http.NewServeMux(), opts)

	if srv.ReadTimeout == 0 {
		t.Error("ReadTimeout must be non-zero")
	}
	if srv.WriteTimeout == 0 {
		t.Error("WriteTimeout must be non-zero")
	}
	if srv.IdleTimeout == 0 {
		t.Error("IdleTimeout must be non-zero")
	}
}
