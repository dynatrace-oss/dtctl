package main

import (
	"os"

	"github.com/dynatrace-oss/dtctl/cmd"
	"github.com/dynatrace-oss/dtctl/pkg/serve"
)

func main() {
	// Server mode is experimental and off by default: without the opt-in, serve
	// is neither dispatched nor registered, so `dtctl serve` is an unknown
	// command like any other (see serve.ExperimentalEnvVar).
	if serve.Experimental() {
		// `dtctl serve ...` runs outside the normal command pipeline: every
		// request the server accepts becomes an engine execution that must
		// acquire the per-invocation lock, which the pipeline would already be
		// holding for the serve command itself. See serve.Run.
		if len(os.Args) > 1 && os.Args[1] == "serve" {
			os.Exit(serve.Run(os.Args[2:]))
		}

		// serve lives outside package cmd (it imports pkg/engine, which imports
		// cmd); register it here so it appears in help and the command catalog.
		cmd.AddCommand(serve.NewCommand())
	}

	cmd.Execute()
}
