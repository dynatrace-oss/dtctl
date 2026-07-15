// Package session is the dtctl session layer: everything a Dynatrace CLI
// tool needs to go from "the user's machine" to "an authenticated client for
// the right tenant".
//
// It owns the shared state contract (docs/dev/CONFIG_CONTRACT.md):
//
//   - the config-file model — contexts, tokens, preferences, safety levels —
//     with discovery, env expansion, schema versioning, and round-trip
//     preservation of unknown fields (golden fixtures in testdata/contract)
//   - credential resolution — OS keyring (service "dtctl"), file-based OAuth
//     store fallback, inline config tokens
//
// dtctl and dtctl-* plugins all consume this package, which is what makes
// "drop into any of them wherever dtctl points" work. Configuration
// management (creating contexts, login flows) is dtctl's job alone; other
// consumers treat the config file as read-only.
//
// The alias, hook, and spill fields carried by Config are CLI-owned schema
// data: this package round-trips and exposes them, but never executes them —
// alias expansion and hook execution live in dtctl's cmd layer.
package session
