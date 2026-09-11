package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/cmd"
	"github.com/dynatrace-oss/dtctl/pkg/engine"
)

// executeRequest is the POST /v1/execute body. Command/argv, files, and stdin
// mirror engine.Request; file contents travel as plain JSON strings (the
// payloads dtctl consumes — YAML, JSON, DQL — are text).
type executeRequest struct {
	// Command is the dtctl command line exactly as typed locally,
	// e.g. "apply -f workflow.yaml --agent". Ignored when argv is set.
	Command string `json:"command,omitempty"`
	// Argv is the pre-split argument vector; takes precedence over command.
	Argv []string `json:"argv,omitempty"`

	EnvironmentURL string `json:"environmentUrl"`
	Token          string `json:"token"`
	SafetyLevel    string `json:"safetyLevel,omitempty"`
	Profile        string `json:"profile,omitempty"`

	Files map[string]string `json:"files,omitempty"`
	Stdin string            `json:"stdin,omitempty"`
}

// executeResponse is the POST /v1/execute response. A command that fails
// still answers 200: exitCode/stderr are the result, exactly as on a local
// shell. Non-200 statuses are reserved for requests that never ran.
type executeResponse struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	// Files is the complete final state of the request's virtual filesystem.
	Files      map[string]string `json:"files,omitempty"`
	DurationMs int64             `json:"durationMs"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Handler returns the HTTP server's http.Handler:
//
//	POST /v1/execute — run one dtctl command line (body: executeRequest)
//	GET  /healthz    — liveness probe
//
// maxRequestBytes bounds the request body (virtual files travel inline).
func Handler(maxRequestBytes int64) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
	})
	mux.HandleFunc("/v1/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

		var req executeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge,
					fmt.Sprintf("request body exceeds %d bytes", tooLarge.Limit))
				return
			}
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}

		files := make(map[string][]byte, len(req.Files))
		for name, content := range req.Files {
			files[name] = []byte(content)
		}
		var stdin []byte
		if req.Stdin != "" {
			stdin = []byte(req.Stdin)
		}

		start := time.Now()
		res, err := engine.Execute(r.Context(), engine.Request{
			Command:        req.Command,
			Argv:           req.Argv,
			EnvironmentURL: req.EnvironmentURL,
			Token:          req.Token,
			SafetyLevel:    req.SafetyLevel,
			Profile:        req.Profile,
			Files:          files,
			Stdin:          stdin,
			// Env is intentionally not exposed over HTTP: arbitrary variables
			// reach proxies, exporters, and other process-level behavior.
			// Embedding hosts that need it use engine.Request.Env directly.
		})
		if err != nil {
			// The request never ran: malformed shape or cancelled while queued.
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		outFiles := make(map[string]string, len(res.Files))
		for name, content := range res.Files {
			outFiles[name] = string(content)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(executeResponse{
			ExitCode:   res.ExitCode,
			Stdout:     string(res.Stdout),
			Stderr:     string(res.Stderr),
			Files:      outFiles,
			DurationMs: time.Since(start).Milliseconds(),
		})
	})
	return mux
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

// newHTTPCommand builds `dtctl serve http`.
func newHTTPCommand() *cobra.Command {
	var (
		addr            string
		maxRequestBytes int64
	)
	c := &cobra.Command{
		Use:   "http",
		Short: "Serve the dtctl execute API over HTTP (reference implementation)",
		Long: `Serve dtctl over HTTP: one request executes one dtctl command line for one
tenant and returns the CLI-identical output.

  POST /v1/execute
    {"command": "get workflows --agent",
     "environmentUrl": "https://abc12345.apps.dynatrace.com",
     "token": "dt0s16....",
     "safetyLevel": "readonly",
     "files": {"x.yaml": "..."}}
  -> {"exitCode": 0, "stdout": "...", "stderr": "...", "files": {...}}

  GET /healthz -> {"status":"ok"}

Each request brings its own environment URL and token; the local dtctl config,
keyring, and credential environment variables are never read. File arguments
resolve against the request's "files", and files written back (e.g. by
apply --write-id) are returned in the response. Requests execute one at a time
per process.

This is a reference implementation: it performs no authentication of its own
(the per-request token only authenticates against Dynatrace) and binds to
localhost by default. Put your own authentication and TLS in front before
exposing it, or embed pkg/engine directly.`,
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if cmd.RunActive() {
				// Reached through the normal pipeline (e.g. `dtctl -v serve
				// http`, which the main dispatch does not intercept): serving
				// here would hold the invocation lock for the server's
				// lifetime and deadlock every request.
				return errors.New("serve must be invoked directly as `dtctl serve http [flags]`, with no arguments before it")
			}
			return runHTTP(command.Context(), addr, maxRequestBytes)
		},
	}
	c.Flags().StringVar(&addr, "addr", "127.0.0.1:7211", "listen address")
	c.Flags().Int64Var(&maxRequestBytes, "max-request-bytes", 10<<20,
		"maximum request body size in bytes (virtual files travel inline)")
	return c
}

// runHTTP serves until the context is cancelled or SIGINT/SIGTERM arrives,
// then shuts down gracefully, letting an in-flight execution finish (a started
// run cannot be interrupted — see the pkg/engine cancellation notes).
func runHTTP(ctx context.Context, addr string, maxRequestBytes int64) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              addr,
		Handler:           Handler(maxRequestBytes),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "dtctl serve http: listening on http://%s\n", addr)
		errc <- server.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "dtctl serve http: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
