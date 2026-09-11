// Package engine executes dtctl command lines in-process for services.
//
// It is the embedding surface described in docs/dev/SERVICE_ENGINE_DESIGN.md: a
// caller hands over a command string exactly as a user would type it locally,
// plus the tenant to run it against (environment URL + token) and an optional
// set of virtual files, and gets back the exact byte streams the CLI would
// have printed, the exit code, and the final state of the virtual files.
//
//	res, err := engine.Execute(ctx, engine.Request{
//	    Command:        `apply -f workflow.yaml --write-id --agent`,
//	    EnvironmentURL: "https://abc12345.apps.dynatrace.com",
//	    Token:          tenantToken,
//	    Files:          map[string][]byte{"workflow.yaml": src},
//	})
//	// res.Stdout is byte-identical to local CLI output;
//	// res.Files["workflow.yaml"] now carries the stamped id.
//
// Every execution is fully isolated from the host process:
//
//   - Credentials come only from the request. The host's config file,
//     contexts, keyring, and credential environment variables are scrubbed
//     for the duration of the run (cmd.Session).
//   - File arguments (-f and friends, including writebacks like `apply
//     --write-id`) resolve against the request's virtual filesystem, never
//     the host disk (pkg/vfs).
//   - No subprocesses: plugins, shell aliases, apply hooks, editors, and
//     browser opens are all disabled (cmd.Capabilities zero value).
//   - Host-only commands (config, ctx, auth, ...) are removed from the
//     surface entirely — hidden from `dtctl commands` and help, and guarded
//     with the stable agent-mode error code "unsupported_in_service".
//
// Concurrency: Execute is safe to call from multiple goroutines, but
// executions serialize on an internal lock (the command tree is process
// state — see cmd.Run). A service scales by running more engine processes or
// instances, not more goroutines; the design's WASM phase gives each request
// its own instance at 20-47ms overhead.
//
// Cancellation: the context gates the start of an execution (a request
// cancelled while queued never runs). A run already started cannot be killed
// mid-flight — in-process code cannot be preempted safely. Deployments that
// need hard per-request deadlines put the engine behind a process boundary or
// use the WASM instance model, where the host enforces the deadline.
package engine
