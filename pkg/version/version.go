package version

// Version is the current version of dtctl
// This can be overridden at build time with -ldflags
var Version = "0.7.3"

// Commit is the git commit hash (set at build time)
var Commit = "unknown"

// Date is the build date (set at build time)
var Date = "unknown"
