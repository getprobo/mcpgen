package mcp_v1

// This file will NOT be regenerated automatically.
//
// It serves as a dependency injection container for your resolvers.
// Add any dependencies you need here (database connections, API clients, etc.)
// and they'll be available to all your tool, prompt, and resource resolvers.

// Resolver is the root resolver that holds dependencies for all MCP handlers
type Resolver struct {
	// Add your dependencies here, for example:
	// DB *sql.DB
	// Cache *redis.Client
	// APIClient *http.Client
}

// NewResolver creates a new resolver instance
func NewResolver() *Resolver {
	return &Resolver{
		// Initialize your dependencies here
	}
}
