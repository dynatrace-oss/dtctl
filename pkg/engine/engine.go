package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/shlex"

	"github.com/dynatrace-oss/dtctl/cmd"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// Request is one dtctl invocation for one tenant.
type Request struct {
	// Command is the command line exactly as a user would type it after
	// "dtctl", e.g. `get workflows -o json`. It is split into arguments with
	// POSIX shell rules (quotes and escapes honored; no variable expansion,
	// globbing, pipes, or redirection — dtctl is not a shell). Ignored when
	// Argv is set.
	Command string
	// Argv is the pre-split argument vector (os.Args[1:] shape). Callers that
	// already have discrete arguments should prefer it over Command — no
	// quoting round-trip. Takes precedence over Command.
	Argv []string

	// EnvironmentURL is the Dynatrace environment to run against
	// (e.g. https://abc12345.apps.dynatrace.com). Required.
	EnvironmentURL string
	// Token authenticates every API call of this request. Required.
	Token string
	// SafetyLevel bounds mutating operations for this request
	// (readonly | readwrite-mine | readwrite-all | dangerously-unrestricted).
	// Empty selects the dtctl default (readwrite-all).
	SafetyLevel string
	// Profile selects a built-in command profile (e.g. "query") that reduces
	// the visible command surface for this request. Empty is the full
	// (service-supported) surface.
	Profile string

	// Files is the request's virtual filesystem: every file argument
	// (`-f x.yaml`, `--data-file`, ...) resolves against it, and files the
	// command writes (e.g. `apply --write-id` stamping an id back into the
	// source) land in it. Paths are normalized, so "x.yaml", "./x.yaml" and
	// "/x.yaml" are the same file. nil is an empty filesystem — not the host
	// disk: a user-named path resolves here or not at all, which the
	// TestUserFilePathsGoThroughVFS guard enforces across cmd/ and pkg/.
	// (Host state dtctl owns — a temp file, a cache — is a separate matter;
	// the commands that use it are blocked or ungranted in a service.)
	Files map[string][]byte
	// Stdin feeds commands that read standard input (`-f -`). nil is an empty
	// stream (immediate EOF) — never the host process's stdin.
	Stdin []byte

	// Env sets additional environment variables for the run (e.g.
	// DTCTL_OUTPUT). Applied after credential scrubbing, so entries here win;
	// grant with care — the engine deliberately does not expose this in its
	// HTTP reference wrapper. Profile, when set, overrides an Env entry for
	// DTCTL_PROFILE.
	Env map[string]string
}

// Result is the outcome of one execution. Stdout and Stderr are byte-identical
// to what the dtctl CLI would print for the same command line, credentials,
// and files — including --agent envelopes, table layouts, and error messages.
type Result struct {
	// ExitCode is the CLI process exit code (0 success; sysexits-style codes
	// for errors, e.g. 64 usage, 77 permission).
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	// Files is the complete final state of the request's virtual filesystem:
	// the input files plus anything the command wrote or rewrote.
	Files map[string][]byte
}

// Execute runs one dtctl invocation and returns its outcome.
//
// A non-nil error means the request never ran: it is malformed (no command,
// missing tenant credentials, an unparsable command string) or its context
// was already done. Everything after the run starts — including command
// failures — is expressed CLI-style in Result: exit code plus stdout/stderr.
func Execute(ctx context.Context, req Request) (*Result, error) {
	argv, err := req.argv()
	if err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}
	// The context gates the start only: a request cancelled while waiting in
	// line must not run, but a run in progress cannot be killed (see the
	// package documentation on cancellation).
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	env := make(map[string]string, len(req.Env)+1)
	for k, v := range req.Env {
		env[k] = v
	}
	if req.Profile != "" {
		env[config.ProfileEnvVar] = req.Profile
	}

	// Always a reader, never nil: a nil RunOptions.Stdin leaves the *host*
	// process's stdin in place, which a request must never see (and which can
	// block forever on an open pipe, wedging the serialized engine). A nil
	// req.Stdin yields an empty reader, i.e. immediate EOF.
	stdin := io.Reader(bytes.NewReader(req.Stdin))

	files := vfs.NewMapFS(req.Files)
	var stdout, stderr bytes.Buffer
	code := cmd.Run(argv, cmd.RunOptions{
		// Grant nothing: no plugins, shell aliases, hooks, editors, or
		// browser opens. Everything a request needs happens in-process.
		Capabilities: &cmd.Capabilities{},
		Session: &cmd.Session{
			EnvironmentURL: req.EnvironmentURL,
			Token:          req.Token,
			SafetyLevel:    config.SafetyLevel(req.SafetyLevel),
		},
		Env:             env,
		FS:              files,
		Stdin:           stdin,
		Stdout:          &stdout,
		Stderr:          &stderr,
		BlockedCommands: unsupportedCommands,
	})

	return &Result{
		ExitCode: code,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		Files:    files.Files(),
	}, nil
}

// argv resolves the request's argument vector: Argv verbatim when set,
// otherwise Command split with POSIX shell rules.
func (r *Request) argv() ([]string, error) {
	if len(r.Argv) > 0 {
		return r.Argv, nil
	}
	if r.Command == "" {
		return nil, errors.New("engine: request has no command (set Command or Argv)")
	}
	argv, err := shlex.Split(r.Command)
	if err != nil {
		return nil, fmt.Errorf("engine: parse command: %w", err)
	}
	if len(argv) == 0 {
		return nil, errors.New("engine: request has no command (set Command or Argv)")
	}
	return argv, nil
}

// validate fails fast on request-shape problems, so malformed requests
// surface as Go errors instead of a CLI error transcript. The same
// constraints are enforced again inside the run (cmd.Session.validate) —
// this is the caller-friendly first line.
func (r *Request) validate() error {
	if r.EnvironmentURL == "" {
		return errors.New("engine: EnvironmentURL is required")
	}
	if r.Token == "" {
		return errors.New("engine: Token is required")
	}
	if r.SafetyLevel != "" {
		valid := false
		for _, l := range config.ValidSafetyLevels() {
			if config.SafetyLevel(r.SafetyLevel) == l {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("engine: invalid safety level %q", r.SafetyLevel)
		}
	}
	return nil
}
