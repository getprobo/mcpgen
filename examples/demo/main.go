package main

import (
	"context"
	"log"

	mcp_v1 "demo/generated"
	"demo/generated/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Create resolver and get fully configured MCP server
	resolver := mcp_v1.NewResolver()
	mcpServer := server.New(resolver)

	// Example 1: Use with stdio transport
	transport := &mcp.StdioTransport{}
	ctx := context.Background()
	log.Println("Starting MCP server with stdio...")
	if err := mcpServer.Run(ctx, transport); err != nil {
		log.Fatalf("Server failed: %v", err)
	}

	// Example 2: Use with HTTP handler (commented out)
	// getServer := func(r *http.Request) *mcp.Server { return mcpServer }
	// eventStore := mcp.NewMemoryEventStore(nil)
	// handler := mcp.NewStreamableHTTPHandler(
	//     getServer,
	//     &mcp.StreamableHTTPOptions{
	//         Stateless:      false,
	//         SessionTimeout: 30 * time.Minute,
	//         EventStore:     eventStore,
	//         Logger:         nil,
	//     },
	// )
	// http.Handle("/mcp", handler)
	// http.ListenAndServe(":8080", nil)
}
