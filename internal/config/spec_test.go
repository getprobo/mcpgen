package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPSpecValidateOAuthScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{
			name:   "valid read scope",
			scopes: []string{"v1:third-party:read"},
		},
		{
			name:   "valid write scope",
			scopes: []string{"v1:third-party"},
		},
		{
			name:    "invalid scope format",
			scopes:  []string{"document:read"},
			wantErr: true,
		},
		{
			name:    "empty scope",
			scopes:  []string{""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := &MCPSpec{
				Info: ServerInfo{Title: "test", Version: "1.0.0"},
				Tools: []Tool{
					{
						Name:        "example",
						InputSchema: &Schema{Type: "object"},
						OAuthScopes: tt.scopes,
					},
				},
			}

			err := spec.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
