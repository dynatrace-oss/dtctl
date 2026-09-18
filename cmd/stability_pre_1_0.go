package cmd

// pre10Since is the dtctl version at which the pre-1.0 surface audit demoted a
// flag from stable to experimental.
//
// These flags are not experimental because they are new or unproven — most have
// shipped for months and work correctly. They are experimental because a
// breaking change to each one is already an accepted decision in
// dtctl-contrib's `breaking-changes/` folder, so dtctl cannot honestly promise
// the additive-only contract that `stable` means. Marking them keeps the
// promise and the plan consistent: an operator who pins
// `DTCTL_MIN_STABILITY=stable` today will not have a command line broken by
// 1.0, and the `[Experimental]` badge tells an interactive user which spellings
// are on their way out.
//
// Each mark cites the document that drives it. When a document is implemented
// the flag is either gone or renamed, and its mark goes with it.
const pre10Since = "0.39.0"
