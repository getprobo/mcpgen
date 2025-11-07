package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.probo.inc/mcpgen/internal/codegen"
	"go.probo.inc/mcpgen/internal/config"
)

var version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "mcpgen",
	Short: "A code generator for Model Context Protocol (MCP) servers",
	Long: `mcpgen is a gqlgen-like code generator for building MCP servers in Go.
It generates type-safe Go code from JSON Schema definitions for tools, resources, and prompts.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of mcpgen",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("mcpgen %s\n", version)
	},
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Go code from mcpgen configuration",
	Long: `Reads mcpgen.yaml (or mcpgen.yml) configuration file and generates:
  - Type-safe Go structs from JSON Schemas
  - MCP server boilerplate code
  - Handler function stubs for tools, resources, and prompts`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")
		return runGenerate(configFile)
	},
}

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new MCP server project",
	Long:  `Creates a new MCP server project with example configuration and file structure.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := "my-mcp-server"
		if len(args) > 0 {
			name = args[0]
		}
		return runInit(name)
	},
}

func init() {
	generateCmd.Flags().StringP("config", "c", "mcpgen.yaml", "Path to config file")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(initCmd)
}

func runGenerate(configFile string) error {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if configFile == "mcpgen.yaml" {
			if _, err := os.Stat("mcpgen.yml"); err == nil {
				configFile = "mcpgen.yml"
			}
		}
	}

	fmt.Printf("Loading configuration from %s...\n", configFile)

	cfg, spec, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("Generating code for %s v%s...\n", spec.Info.Title, spec.Info.Version)

	gen := codegen.New(cfg, spec)

	if err := gen.Generate(); err != nil {
		return fmt.Errorf("code generation failed: %w", err)
	}

	fmt.Println("✓ Code generation completed successfully!")
	return nil
}

func runInit(name string) error {
	fmt.Printf("Initializing new MCP server project: %s\n", name)

	if err := os.MkdirAll(name, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	configContent := `# mcpgen Generation Configuration
# This file contains only code generation settings

# Path to MCP API specification
spec: mcp.yaml

# Code generation settings
generate:
  # Output directory for generated code
  output: generated

  # Package name for generated code
  package: generated

  # Resolver configuration
  resolver:
    filename: resolver.go
    package: generated
    type: Resolver
    preserve: true

  # Optional: Model-specific settings
  models:
    filename: models.go
    package: generated

# Optional: Additional generation options
options:
  skipValidation: false
  verboseComments: false
`

	configPath := filepath.Join(name, "mcpgen.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	mcpContent := fmt.Sprintf(`# MCP API Specification
# This file contains the pure MCP API definition

info:
  title: %s
  version: 1.0.0
  description: An example MCP server

# Reusable schema components
components:
  schemas:
    ExampleInput:
      type: object
      properties:
        message:
          type: string
          description: The message to process
      required: [message]

# MCP Tools
tools:
  - name: example_tool
    description: An example tool that processes messages
    readonly: false
    destructive: false
    idempotent: true
    inputSchema:
      $ref: "#/components/schemas/ExampleInput"

# MCP Resources
resources: []

# MCP Prompts
prompts: []
`, name)

	mcpPath := filepath.Join(name, "mcp.yaml")
	if err := os.WriteFile(mcpPath, []byte(mcpContent), 0644); err != nil {
		return fmt.Errorf("failed to write MCP spec file: %w", err)
	}

	mainContent := fmt.Sprintf(`package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"%s/generated"
)

func main() {
	// Create resolver
	resolver := generated.NewResolver()

	// Create server
	server := generated.New(resolver)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Start server
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Server error: %%v", err)
	}
}
`, name)

	mainPath := filepath.Join(name, "main.go")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		return fmt.Errorf("failed to write main.go: %w", err)
	}

	readmeContent := fmt.Sprintf(`# %s

MCP server generated with mcpgen.

## Getting Started

1. Edit the API specification in mcp.yaml (define tools, resources, prompts)
2. Adjust code generation settings in mcpgen.yaml if needed
3. Generate code:
   ` + "```bash" + `
   mcpgen generate
   ` + "```" + `
4. Implement your handlers in resolver.go
5. Build and run:
   ` + "```bash" + `
   go mod init %s
   go mod tidy
   go build -o server
   ./server
   ` + "```" + `

## Project Structure

- mcpgen.yaml - Code generation configuration
- mcp.yaml - MCP API specification (tools, resources, prompts)
- generated/ - Generated code (server, models, resolver stubs)
- main.go - Entry point
`, name, name, name)

	readmePath := filepath.Join(name, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to write README: %w", err)
	}

	fmt.Printf("\n✓ Project initialized successfully!\n\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  cd %s\n", name)
	fmt.Printf("  mcpgen generate\n")
	fmt.Printf("  go mod init %s\n", name)
	fmt.Printf("  go mod tidy\n")

	return nil
}
