package mcp

import "context"

// OAuthScopeGateFunc is called before a tool resolver when the tool declares
// oauthScopes in the MCP specification. It receives the required scope strings
// and returns an error when the caller is not authorized.
type OAuthScopeGateFunc func(ctx context.Context, requiredScopes []string) error

// DefaultOAuthScopeGate is a no-op gate used when the host does not wire
// enforcement. Annotated tools still call the gate; bypass logic belongs in
// the host implementation.
func DefaultOAuthScopeGate(context.Context, []string) error {
	return nil
}

// WithOAuthScopeGate sets the OAuth scope gate for annotated tool handlers.
func WithOAuthScopeGate(fn OAuthScopeGateFunc) Option {
	return func(o *Options) {
		o.OAuthScopeGate = fn
	}
}
