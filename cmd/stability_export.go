package cmd

import (
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// StabilityManifest renders the stability manifest for the complete command
// tree — every development-tier feature registered, whether or not this
// environment opted into it.
//
// It is exported for the manifest generator and its CI gate, which live outside
// this package: they must see `dtctl serve` too, and serve is wired in main
// because pkg/serve imports pkg/engine, which imports cmd.
func StabilityManifest() string {
	return withCompleteCommandTree(func() string { return stability.Manifest(rootCmd) })
}

// StabilityLint reports declarations that are internally inconsistent across
// the complete command tree. Same reason for being exported as
// StabilityManifest.
func StabilityLint() []error {
	return withCompleteCommandTree(func() []error { return stability.Lint(rootCmd) })
}

// withCompleteCommandTree registers every development feature, runs fn, and
// restores the tree. The restore matters: these helpers run in-process
// alongside ordinary command execution, and leaving a development feature
// attached would hand the next invocation surface it never opted into.
func withCompleteCommandTree[T any](fn func() T) T {
	applyDevelopmentRegistration(map[string]bool{config.DevelopmentAll: true})
	defer applyDevelopmentRegistration(nil)
	return fn()
}
