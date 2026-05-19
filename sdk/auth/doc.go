// Package auth provides Dynatrace token type detection and classification.
//
// It distinguishes between API tokens (dt0c01.* prefix), platform tokens
// (dt0s16.* prefix), and OAuth/JWT bearer tokens, enabling callers to
// set the correct Authorization header scheme.
package auth
