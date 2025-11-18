package main

import (
	"context"
	"log"

	mcp_v1 "demo/generated"
)

func main() {
	resolver := mcp_v1.NewResolver()
	server := mcp_v1.New(resolver)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
