package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Spec     string             `yaml:"spec" json:"spec"`
	Generate GenerateConfig     `yaml:"generate" json:"generate"`
	Options  GenerationOptions  `yaml:"options,omitempty" json:"options,omitempty"`
}

type GenerateConfig struct {
	Output   string         `yaml:"output" json:"output"`
	Package  string         `yaml:"package" json:"package"`
	Resolver ResolverConfig `yaml:"resolver" json:"resolver"`
	Models   ModelsConfig   `yaml:"models,omitempty" json:"models,omitempty"`
}

type ResolverConfig struct {
	Filename string `yaml:"filename" json:"filename"`
	Package  string `yaml:"package" json:"package"`
	Type     string `yaml:"type" json:"type"`
	Preserve bool   `yaml:"preserve" json:"preserve"`
}

type ModelsConfig struct {
	Filename string `yaml:"filename,omitempty" json:"filename,omitempty"`
	Package  string `yaml:"package,omitempty" json:"package,omitempty"`
}

type GenerationOptions struct {
	SkipValidation  bool   `yaml:"skipValidation,omitempty" json:"skipValidation,omitempty"`
	VerboseComments bool   `yaml:"verboseComments,omitempty" json:"verboseComments,omitempty"`
	Templates       string `yaml:"templates,omitempty" json:"templates,omitempty"`
}

type ServerInfo struct {
	Title       string `yaml:"title" json:"title"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type Components struct {
	Schemas map[string]*jsonschema.Schema `yaml:"schemas,omitempty" json:"schemas,omitempty"`
}

type Schema = jsonschema.Schema

type Tool struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	InputSchema *Schema           `yaml:"inputSchema" json:"inputSchema"`
	Readonly    bool              `yaml:"readonly,omitempty" json:"readonly,omitempty"`
	Destructive bool              `yaml:"destructive,omitempty" json:"destructive,omitempty"`
	Idempotent  bool              `yaml:"idempotent,omitempty" json:"idempotent,omitempty"`
	OpenWorld   bool              `yaml:"openWorld,omitempty" json:"openWorld,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Handler     string            `yaml:"handler,omitempty" json:"handler,omitempty"`
}

type Resource struct {
	URI         string            `yaml:"uri,omitempty" json:"uri,omitempty"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	MimeType    string            `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`
	URITemplate string            `yaml:"uriTemplate,omitempty" json:"uriTemplate,omitempty"`
	Schema      *Schema           `yaml:"schema,omitempty" json:"schema,omitempty"`
	Readonly    bool              `yaml:"readonly,omitempty" json:"readonly,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Handler     string            `yaml:"handler,omitempty" json:"handler,omitempty"`
}

type Prompt struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Arguments   []PromptArgument  `yaml:"arguments,omitempty" json:"arguments,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Handler     string            `yaml:"handler,omitempty" json:"handler,omitempty"`
}

type PromptArgument struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

func Load(path string) (*Config, *MCPSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{
		Spec: "mcp.yaml",
		Generate: GenerateConfig{
			Output:  "generated",
			Package: "generated",
			Resolver: ResolverConfig{
				Filename: "resolver.go",
				Package:  "generated",
				Type:     "Resolver",
				Preserve: true,
			},
		},
	}

	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, config); err != nil {
			return nil, nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported config file format: %s (use .yaml, .yml, or .json)", ext)
	}

	if err := config.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	specPath := config.Spec
	if !filepath.IsAbs(specPath) {
		configDir := filepath.Dir(path)
		specPath = filepath.Join(configDir, specPath)
	}

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		basePath := specPath
		for _, ext := range []string{".yaml", ".yml", ".json"} {
			tryPath := basePath
			if filepath.Ext(tryPath) == "" {
				tryPath = basePath + ext
			} else {
				tryPath = basePath[:len(basePath)-len(filepath.Ext(basePath))] + ext
			}
			if _, err := os.Stat(tryPath); err == nil {
				specPath = tryPath
				break
			}
		}
	}

	spec, err := LoadMCPSpec(specPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load MCP spec from %s: %w", specPath, err)
	}

	return config, spec, nil
}

func (c *Config) Validate() error {
	if c.Spec == "" {
		return fmt.Errorf("spec path is required")
	}
	if c.Generate.Output == "" {
		return fmt.Errorf("generate.output is required")
	}
	if c.Generate.Package == "" {
		return fmt.Errorf("generate.package is required")
	}

	return nil
}

func IsSchemaRef(s *Schema) bool {
	return s != nil && s.Ref != ""
}
