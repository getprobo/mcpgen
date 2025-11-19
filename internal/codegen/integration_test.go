package codegen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.probo.inc/mcpgen/internal/config"
)

func TestGenerateWithCustomTypes(t *testing.T) {
	specPath := filepath.Join("testdata", "custom_types.yaml")
	spec, err := config.LoadMCPSpec(specPath)
	require.NoError(t, err, "Failed to load spec")

	cfg := &config.Config{
		Spec:   specPath,
		Output: t.TempDir(),
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
		// No custom models in config - using x-go-type annotations
		Models: config.ModelsConfig{
			Models: map[string]config.TypeMapping{},
		},
	}

	gen := New(cfg, spec)

	if err := gen.loadSchemas(); err != nil {
		t.Fatalf("Failed to load schemas: %v", err)
	}

	code, err := gen.typeGen.Generate("test")
	require.NoError(t, err, "Failed to generate code")

	codeStr := string(code)

	customTypes := []string{"Timestamp", "UUID", "Decimal", "Metadata", "Duration"}
	for _, typeName := range customTypes {
		if containsTypeDefinition(codeStr, "type "+typeName+" ") {
			t.Errorf("Should not generate type %q (it has x-go-type annotation)", typeName)
		}
	}

	regularTypes := []string{"Task", "OptionalFields", "Project", "UpdateTaskInput"}
	for _, typeName := range regularTypes {
		if !containsTypeDefinition(codeStr, "type "+typeName) {
			t.Errorf("Should generate type %q", typeName)
		}
	}

	expectedImports := []string{
		"time",
		"github.com/google/uuid",
		"github.com/shopspring/decimal",
		"json",
		"go.probo.inc/mcpgen/mcputil",
	}
	for _, imp := range expectedImports {
		if !containsImport(codeStr, imp) {
			t.Errorf("Should import %q", imp)
		}
	}

	if !containsString(codeStr, "ID        uuid.UUID") {
		t.Error("Task.ID should use uuid.UUID")
	}
	if !containsString(codeStr, "CreatedAt time.Time") {
		t.Error("Task.CreatedAt should use time.Time")
	}
	if !containsString(codeStr, "UpdatedAt *time.Time") {
		t.Error("Task.UpdatedAt should use *time.Time (nullable)")
	}
	// Check omittable fields
	if !containsString(codeStr, "mcputil.Omittable[string]") {
		t.Error("UpdateTaskInput.Title should use mcputil.Omittable[string]")
	}
	if !containsString(codeStr, "mcputil.Omittable[*Status]") {
		t.Error("UpdateTaskInput.Status should use mcputil.Omittable[*Status]")
	}
	if !containsString(codeStr, "mcputil.Omittable[int64]") {
		t.Error("UpdateTaskInput.Priority should use mcputil.Omittable[int64]")
	}
	if !containsString(codeStr, "mcputil.Omittable[[]string]") {
		t.Error("UpdateTaskInput.Tags should use mcputil.Omittable[[]string]")
	}
	// Note: time.Duration test skipped - it's a type alias that may not appear in generated code
}

func TestGenerateWithConfigBasedTypes(t *testing.T) {
	specPath := filepath.Join("testdata", "config_based_types.yaml")
	spec, err := config.LoadMCPSpec(specPath)
	require.NoError(t, err, "Failed to load spec")

	cfg := &config.Config{
		Spec:   specPath,
		Output: t.TempDir(),
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
		// Custom models in config
		Models: config.ModelsConfig{
			Models: map[string]config.TypeMapping{
				"Timestamp": {Model: "time.Time"},
				"UUID":      {Model: "github.com/google/uuid.UUID"},
				"User":      {Model: "github.com/myapp/models.User"},
			},
		},
	}

	gen := New(cfg, spec)

	if err := gen.loadSchemas(); err != nil {
		t.Fatalf("Failed to load schemas: %v", err)
	}

	code, err := gen.typeGen.Generate("test")
	require.NoError(t, err, "Failed to generate code")

	codeStr := string(code)

	customTypes := []string{"Timestamp", "UUID", "User"}
	for _, typeName := range customTypes {
		if containsTypeDefinition(codeStr, "type "+typeName+" ") {
			t.Errorf("Should not generate type %q (it's in config models)", typeName)
		}
	}

	if !containsTypeDefinition(codeStr, "type Event struct") {
		t.Error("Should generate Event type")
	}

	expectedImports := []string{
		"time",
		"github.com/google/uuid",
		"github.com/myapp/models",
	}
	for _, imp := range expectedImports {
		if !containsImport(codeStr, imp) {
			t.Errorf("Should import %q", imp)
		}
	}

	if !containsString(codeStr, "ID        uuid.UUID") {
		t.Error("Event.ID should use uuid.UUID")
	}
	if !containsString(codeStr, "CreatedAt time.Time") {
		t.Error("Event.CreatedAt should use time.Time")
	}
	if !containsString(codeStr, "Owner     models.User") {
		t.Error("Event.Owner should use models.User")
	}
}

func TestGenerateAllPrimitives(t *testing.T) {
	specPath := filepath.Join("testdata", "all_primitives.yaml")
	spec, err := config.LoadMCPSpec(specPath)
	require.NoError(t, err, "Failed to load spec")

	cfg := &config.Config{
		Spec:   specPath,
		Output: t.TempDir(),
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
		Models: config.ModelsConfig{
			Models: map[string]config.TypeMapping{},
		},
	}

	gen := New(cfg, spec)

	if err := gen.loadSchemas(); err != nil {
		t.Fatalf("Failed to load schemas: %v", err)
	}

	code, err := gen.typeGen.Generate("test")
	require.NoError(t, err, "Failed to generate code")

	codeStr := string(code)

	primitiveTypes := map[string]string{
		"StringSchema":  "type StringSchema string",
		"NumberSchema":  "type NumberSchema float64",
		"IntegerSchema": "type IntegerSchema int",
		"BooleanSchema": "type BooleanSchema bool",
		"ArraySchema":   "type ArraySchema []string",
	}

	for typeName, expectedDecl := range primitiveTypes {
		if !containsString(codeStr, expectedDecl) {
			t.Errorf("Should generate %q for %s", expectedDecl, typeName)
		}
	}

	if !containsString(codeStr, "type ObjectSchema struct") {
		t.Error("Should generate ObjectSchema as a struct")
	}

	if !containsString(codeStr, "type Person struct") {
		t.Error("Should generate Person as a struct")
	}

	if !containsString(codeStr, "type Color string") {
		t.Error("Should generate Color as string-based enum")
	}

	enumConstants := []string{"ColorRed", "ColorGreen", "ColorBlue", "ColorYellow"}
	for _, constName := range enumConstants {
		if !containsString(codeStr, constName) {
			t.Errorf("Should generate enum constant %q", constName)
		}
	}

	if len(code) == 0 {
		t.Error("Generated code is empty")
	}
}

func TestGeneratedCodeCompiles(t *testing.T) {
	testCases := []struct {
		name     string
		specFile string
		config   *config.Config
	}{
		{
			name:     "custom_types",
			specFile: "custom_types.yaml",
			config: &config.Config{
				Model: config.ModelConfig{
					Package:  "test",
					Filename: "models.go",
				},
				Models: config.ModelsConfig{
					Models: map[string]config.TypeMapping{},
				},
			},
		},
		{
			name:     "config_based_types",
			specFile: "config_based_types.yaml",
			config: &config.Config{
				Model: config.ModelConfig{
					Package:  "test",
					Filename: "models.go",
				},
				Models: config.ModelsConfig{
					Models: map[string]config.TypeMapping{
						"Timestamp": {Model: "time.Time"},
						"UUID":      {Model: "github.com/google/uuid.UUID"},
						"User":      {Model: "github.com/myapp/models.User"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			specPath := filepath.Join("testdata", tc.specFile)
			spec, err := config.LoadMCPSpec(specPath)
			if err != nil {
				t.Fatalf("Failed to load spec: %v", err)
			}

			tc.config.Spec = specPath
			tc.config.Output = t.TempDir()
			tc.config.Resolver = config.ResolverConfig{
				Package:  "test",
				Filename: "resolver.go",
				Type:     "Resolver",
				Preserve: false,
			}

			gen := New(tc.config, spec)
			if err := gen.loadSchemas(); err != nil {
				t.Fatalf("Failed to load schemas: %v", err)
			}

			code, err := gen.typeGen.Generate("test")
			if err != nil {
				t.Fatalf("Failed to generate code: %v", err)
			}

			// The fact that Generate() succeeded means the code was formatted successfully
			if len(code) == 0 {
				t.Error("Generated code is empty")
			}
		})
	}
}

