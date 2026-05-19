// Package sdk is the dt-cli-sdk — a shared library of Dynatrace CLI primitives.
//
// It provides platform-level building blocks that multiple Dynatrace CLIs
// (dtctl, dtmgd, dtwiz, and others) can share instead of re-implementing:
//
//   - urls: Dynatrace environment URL validation and correction
//   - auth: Token type detection and classification
//   - httpclient: HTTP client with retry, pagination, and typed errors
//   - agentmode: AI agent detection and structured JSON envelope
//   - credstore: OS keyring and file-based credential storage
//
// The SDK is designed to be leaf-shaped: it never imports from the CLI
// module, has minimal dependencies, and avoids global state.
//
// See https://github.com/dynatrace-oss/dtctl/tree/main/sdk for details.
package sdk
