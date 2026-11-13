package codegen

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"go.probo.inc/mcpgen/internal/config"
)

// Titles and descriptions come straight from the spec, so they must reach the
// generated Go code unchanged whatever characters they contain.
var escapingCases = []struct {
	name string
	text string
}{
	{"double quotes", `Find a vendor by name (e.g. "Google Workspace")`},
	{"newline", "Find a vendor by name.\nReturns at most 50 items."},
	{"regex backslash", `Vendor ID, matching ^\d+$`},
	{"backticks", "Pass the `id` returned by `list_vendors`"},
	{"literal backslash n", `One name per line, separated by \n`},
}

func TestGenerateEscapesTitlesAndDescriptions(t *testing.T) {
	for _, tc := range escapingCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			specPath := filepath.Join(dir, "mcp.yaml")
			writeSpec(t, specPath, map[string]any{
				"info": map[string]any{"title": "escaping", "version": "1.0.0"},
				"tools": []any{map[string]any{
					"name":        "find_vendor",
					"title":       tc.text,
					"description": tc.text,
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{"type": "string", "description": tc.text},
						},
					},
				}},
				"resources": []any{
					map[string]any{"uri": "docs://readme", "name": "readme", "description": tc.text},
					map[string]any{"uriTemplate": "vendor://{id}", "name": "vendor", "description": tc.text},
				},
				"prompts": []any{map[string]any{
					"name":        "vendor_help",
					"description": tc.text,
					"arguments":   []any{map[string]any{"name": "topic", "description": tc.text}},
				}},
			})

			serverPath, modelsPath := generateForEscaping(t, specPath, dir)

			// Tool title (twice, with annotations), then the tool, resource,
			// resource template, prompt and prompt argument descriptions.
			assert.Equal(t, []string{tc.text, tc.text}, stringLiterals(t, serverPath, "Title"))
			assert.Equal(t, []string{tc.text, tc.text, tc.text, tc.text, tc.text}, stringLiterals(t, serverPath, "Description"))
			assert.Equal(t, tc.text, schemaPropertyDescription(t, modelsPath, "FindVendorToolInputSchema", "name"))
		})
	}
}

// Specs from a randomized test that broke code generation before descriptions were escaped.
func TestGenerateEscapingFixtures(t *testing.T) {
	fixtures := []string{
		"escaping_tool_quote.yaml",
		"escaping_property_backtick.yaml",
		"escaping_literal_backslash_n.yaml",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			specPath := filepath.Join("testdata", fixture)
			spec, err := config.LoadMCPSpec(specPath)
			require.NoError(t, err, "Failed to load spec")
			require.Len(t, spec.Tools, 1)
			tool := spec.Tools[0]

			serverPath, modelsPath := generateForEscaping(t, specPath, t.TempDir())

			assert.Equal(t, []string{tool.Description}, stringLiterals(t, serverPath, "Description"))
			for name, prop := range tool.InputSchema.Properties {
				assert.Equal(t, prop.Description, schemaPropertyDescription(t, modelsPath, "FindVendorToolInputSchema", name))
			}
		})
	}
}

func writeSpec(t *testing.T, path string, spec map[string]any) {
	t.Helper()
	data, err := yaml.Marshal(spec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// generateForEscaping runs the full generator and returns the server and models file paths.
func generateForEscaping(t *testing.T, specPath, outputDir string) (string, string) {
	t.Helper()
	spec, err := config.LoadMCPSpec(specPath)
	require.NoError(t, err, "Failed to load spec")

	cfg := &config.Config{
		Spec:   specPath,
		Output: outputDir,
		Exec: config.ExecConfig{
			Package:  "test",
			Filename: "server.go",
		},
		Model: config.ModelConfig{
			Package:  "test",
			Filename: "models.go",
		},
		Resolver: config.ResolverConfig{
			Package:  "test",
			Filename: "resolver.go",
			Type:     "Resolver",
			Preserve: false,
		},
	}

	require.NoError(t, New(cfg, spec).Generate(), "Generate() failed")
	return filepath.Join(outputDir, "server.go"), filepath.Join(outputDir, "models.go")
}

// stringLiterals returns, in source order, the value of every string literal
// assigned to the composite-literal field key in the Go file at path.
func stringLiterals(t *testing.T, path, key string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err, "Generated file does not parse")

	var values []string
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
			if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				value, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				values = append(values, value)
			}
		}
		return true
	})
	return values
}

// schemaPropertyDescription reads the schema JSON passed to
// mcp.MustUnmarshalSchema for varName and returns the description of property.
func schemaPropertyDescription(t *testing.T, path, varName, property string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err, "Generated file does not parse")

	var schemaJSON string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, id := range spec.Names {
			if id.Name != varName || i >= len(spec.Values) {
				continue
			}
			call, ok := spec.Values[i].(*ast.CallExpr)
			require.True(t, ok, "%s is not a call", varName)
			require.Len(t, call.Args, 1)
			lit, ok := call.Args[0].(*ast.BasicLit)
			require.True(t, ok, "%s argument is not a literal", varName)
			schemaJSON, err = strconv.Unquote(lit.Value)
			require.NoError(t, err)
		}
		return true
	})
	require.NotEmpty(t, schemaJSON, "%s not found in %s", varName, path)

	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal([]byte(schemaJSON), &schema))
	return schema.Properties[property].Description
}
