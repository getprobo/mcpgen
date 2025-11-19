package main

import (
	"context"
	"log"

	mcp_v1 "demo/generated"
	"demo/generated/server"
)

func main() {
	resolver := mcp_v1.NewResolver()
	srv := server.New(resolver)

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
