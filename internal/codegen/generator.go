package codegen

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"go.probo.inc/mcpgen/internal/config"
	"go.probo.inc/mcpgen/internal/schema"
)

//go:embed templates/*.gotpl
var templates embed.FS

type Generator struct {
	config       *config.Config
	spec         *config.MCPSpec
	schemaLoader *schema.Loader
	typeGen      *TypeGenerator
}

func New(cfg *config.Config, spec *config.MCPSpec) *Generator {
	return &Generator{
		config:       cfg,
		spec:         spec,
		schemaLoader: schema.NewLoader("."),
		typeGen:      NewTypeGenerator(),
	}
}

func (g *Generator) Generate() error {
	if err := g.loadSchemas(); err != nil {
		return fmt.Errorf("failed to load schemas: %w", err)
	}

	if err := g.generateModels(); err != nil {
		return fmt.Errorf("failed to generate models: %w", err)
	}

	if err := g.generateServer(); err != nil {
		return fmt.Errorf("failed to generate server: %w", err)
	}

	if err := g.generateResolver(); err != nil {
		return fmt.Errorf("failed to generate resolver: %w", err)
	}

	return nil
}

func (g *Generator) loadSchemas() error {
	for name, schema := range g.spec.Components.Schemas {
		if config.IsSchemaRef(schema) {
			s, err := g.schemaLoader.Load(schema.Ref)
			if err != nil {
				return fmt.Errorf("failed to load schema %s: %w", name, err)
			}
			g.typeGen.AddSchema(name, s)
		} else {
			g.typeGen.AddSchema(name, schema)
		}
	}

	for _, tool := range g.spec.Tools {
		if tool.InputSchema != nil {
			typeName := toPascalCase(tool.Name) + "Input"
			handlerName := toHandlerName(tool.Name)
			schemaVarName := handlerName + "ToolInputSchema"

			var resolvedSchema *config.Schema
			if config.IsSchemaRef(tool.InputSchema) {
				if len(tool.InputSchema.Ref) > 0 && tool.InputSchema.Ref[0] == '#' {
					resolved, err := g.spec.ResolveSchemaRef(tool.InputSchema.Ref)
					if err != nil {
						return fmt.Errorf("failed to resolve input schema ref for tool %s: %w", tool.Name, err)
					}
					resolvedSchema = resolved
					g.typeGen.AddSchema(typeName, resolvedSchema)
				} else {
					s, err := g.schemaLoader.Load(tool.InputSchema.Ref)
					if err != nil {
						return fmt.Errorf("failed to load input schema for tool %s: %w", tool.Name, err)
					}
					resolvedSchema = s
					g.typeGen.AddSchema(typeName, s)
				}
			} else {
				resolvedSchema = tool.InputSchema
				g.typeGen.AddSchema(typeName, tool.InputSchema)
			}

			// Add schema variable
			if resolvedSchema != nil {
				schemaJSON, err := json.Marshal(resolvedSchema)
				if err == nil {
					g.typeGen.AddSchemaVar(schemaVarName, string(schemaJSON))
				}
			}
		}
	}

	for _, resource := range g.spec.Resources {
		if resource.Schema != nil {
			typeName := toPascalCase(resource.Name) + "Content"
			if config.IsSchemaRef(resource.Schema) {
				if len(resource.Schema.Ref) > 0 && resource.Schema.Ref[0] == '#' {
					resolvedSchema, err := g.spec.ResolveSchemaRef(resource.Schema.Ref)
					if err != nil {
						return fmt.Errorf("failed to resolve schema ref for resource %s: %w", resource.Name, err)
					}
					g.typeGen.AddSchema(typeName, resolvedSchema)
					continue
				}
				s, err := g.schemaLoader.Load(resource.Schema.Ref)
				if err != nil {
					return fmt.Errorf("failed to load schema for resource %s: %w", resource.Name, err)
				}
				g.typeGen.AddSchema(typeName, s)
			} else {
				g.typeGen.AddSchema(typeName, resource.Schema)
			}
		}
	}

	// Generate typed argument structs for prompts
	for _, prompt := range g.spec.Prompts {
		if len(prompt.Arguments) > 0 {
			typeName := toPascalCase(prompt.Name) + "Args"

			// Create a schema from the prompt arguments
			argSchema := &config.Schema{
				Type:       "object",
				Properties: make(map[string]*config.Schema),
				Required:   []string{},
			}

			for _, arg := range prompt.Arguments {
				argSchema.Properties[arg.Name] = &config.Schema{
					Type:        "string",
					Description: arg.Description,
				}
				if arg.Required {
					argSchema.Required = append(argSchema.Required, arg.Name)
				}
			}

			g.typeGen.AddSchema(typeName, argSchema)
		}
	}

	return nil
}

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func (g *Generator) generateModels() error {
	code, err := g.typeGen.Generate(g.config.Generate.Package)
	if err != nil {
		return err
	}

	modelsFile := "models.go"
	if g.config.Generate.Models.Filename != "" {
		modelsFile = g.config.Generate.Models.Filename
	}
	modelsPath := filepath.Join(g.config.Generate.Output, modelsFile)

	dir := filepath.Dir(modelsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(modelsPath, code, 0644); err != nil {
		return fmt.Errorf("failed to write models file: %w", err)
	}

	fmt.Printf("Generated models: %s\n", modelsPath)
	return nil
}

func (g *Generator) generateServer() error {
	tmpl, err := template.ParseFS(templates, "templates/server.gotpl")
	if err != nil {
		return fmt.Errorf("failed to parse server template: %w", err)
	}

	data := g.buildServerTemplateData()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute server template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format server code: %w\n%s", err, buf.String())
	}

	serverFile := filepath.Join(g.config.Generate.Output, "server.go")

	dir := filepath.Dir(serverFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(serverFile, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write server file: %w", err)
	}

	fmt.Printf("Generated server: %s\n", serverFile)
	return nil
}

func (g *Generator) generateResolver() error {
	resolverFile := filepath.Join(g.config.Generate.Output, g.config.Generate.Resolver.Filename)

	fileExists := false
	if _, err := os.Stat(resolverFile); err == nil {
		fileExists = true
	}

	var orphanedHandlers string
	if fileExists && g.config.Generate.Resolver.Preserve {
		parser, err := NewResolverParser(resolverFile)
		if err == nil {
			existingHandlers, err := parser.ExtractHandlers(g.config.Generate.Resolver.Type)
			if err == nil {
				requiredHandlers := g.getRequiredHandlerNames()
				IdentifyOrphanedHandlers(existingHandlers, requiredHandlers)
				orphanedHandlers = FormatOrphanedHandlers(existingHandlers)

				hasNewHandlers := false
				for _, required := range requiredHandlers {
					if _, exists := existingHandlers[required]; !exists {
						hasNewHandlers = true
						break
					}
				}

				if !hasNewHandlers && orphanedHandlers == "" {
					fmt.Printf("Resolver is up to date, skipping: %s\n", resolverFile)
					return nil
				}
			}
		}
	}

	tmpl, err := template.ParseFS(templates, "templates/resolver.gotpl")
	if err != nil {
		return fmt.Errorf("failed to parse resolver template: %w", err)
	}

	data := g.buildResolverTemplateData()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute resolver template: %w", err)
	}

	if orphanedHandlers != "" {
		buf.WriteString(orphanedHandlers)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format resolver code: %w\n%s", err, buf.String())
	}

	if err := os.WriteFile(resolverFile, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write resolver file: %w", err)
	}

	if fileExists {
		if orphanedHandlers != "" {
			fmt.Printf("Updated resolver with orphaned handlers: %s\n", resolverFile)
		} else {
			fmt.Printf("Updated resolver with new handlers: %s\n", resolverFile)
		}
	} else {
		fmt.Printf("Generated resolver stubs: %s\n", resolverFile)
	}

	return nil
}

func (g *Generator) getRequiredHandlerNames() []string {
	var names []string

	for _, tool := range g.spec.Tools {
		names = append(names, toHandlerName(tool.Name))
	}

	for _, resource := range g.spec.Resources {
		names = append(names, toHandlerName(resource.Name))
	}

	for _, prompt := range g.spec.Prompts {
		names = append(names, toHandlerName(prompt.Name))
	}

	return names
}

func (g *Generator) buildServerTemplateData() map[string]interface{} {
	tools := make([]map[string]interface{}, 0, len(g.spec.Tools))
	hasTypedTools := false
	for _, tool := range g.spec.Tools {
		toolData := map[string]interface{}{
			"Name":        tool.Name,
			"Description": tool.Description,
			"HandlerName": toHandlerName(tool.Name),
		}

		// Add input type information and schema code
		if tool.InputSchema != nil {
			inputTypeName := toPascalCase(tool.Name) + "Input"
			toolData["InputType"] = inputTypeName
			toolData["HasInputType"] = true

			resolvedSchema := tool.InputSchema
			if config.IsSchemaRef(tool.InputSchema) && len(tool.InputSchema.Ref) > 0 && tool.InputSchema.Ref[0] == '#' {
				resolved, err := g.spec.ResolveSchemaRef(tool.InputSchema.Ref)
				if err == nil {
					resolvedSchema = resolved
				}
			}

			schemaCode := g.generateSchemaCode(resolvedSchema)
			toolData["InputSchemaCode"] = schemaCode

			hasTypedTools = true
		}

		tools = append(tools, toolData)
	}

	resources := make([]map[string]interface{}, 0, len(g.spec.Resources))
	for _, resource := range g.spec.Resources {
		resData := map[string]interface{}{
			"Name":        resource.Name,
			"Description": resource.Description,
			"HandlerName": toHandlerName(resource.Name),
			"MimeType":    resource.MimeType,
		}

		if resource.URI != "" {
			resData["URI"] = resource.URI
		} else if resource.URITemplate != "" {
			resData["URITemplate"] = resource.URITemplate
			params := extractURIParams(resource.URITemplate)
			resData["URIParams"] = params
		}

		resources = append(resources, resData)
	}

	prompts := make([]map[string]interface{}, 0, len(g.spec.Prompts))
	for _, prompt := range g.spec.Prompts {
		args := make([]map[string]interface{}, 0, len(prompt.Arguments))
		for _, arg := range prompt.Arguments {
			args = append(args, map[string]interface{}{
				"Name":        arg.Name,
				"Description": arg.Description,
				"Required":    arg.Required,
			})
		}

		promptData := map[string]interface{}{
			"Name":        prompt.Name,
			"Description": prompt.Description,
			"HandlerName": toHandlerName(prompt.Name),
			"Arguments":   args,
		}

		// Add args type if there are arguments
		if len(prompt.Arguments) > 0 {
			argsTypeName := toPascalCase(prompt.Name) + "Args"
			promptData["ArgsType"] = argsTypeName
			promptData["HasArgsType"] = true
		}

		prompts = append(prompts, promptData)
	}

	return map[string]interface{}{
		"Package":       g.config.Generate.Package,
		"ServerName":    g.spec.Info.Title,
		"ServerVersion": g.spec.Info.Version,
		"ResolverType":  g.config.Generate.Resolver.Type,
		"Tools":         tools,
		"Resources":     resources,
		"Prompts":       prompts,
		"HasResources":  len(resources) > 0,
		"HasPrompts":    len(prompts) > 0,
		"HasTypedTools": hasTypedTools,
	}
}

func (g *Generator) buildResolverTemplateData() map[string]interface{} {
	return g.buildServerTemplateData()
}

func toHandlerName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})

	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}

	return strings.Join(parts, "")
}

func (g *Generator) generateSchemaCode(s *config.Schema) string {
	schemaJSON, err := json.Marshal(s)
	if err != nil {
		return "nil"
	}

	return fmt.Sprintf("mustUnmarshalSchema(`%s`)", string(schemaJSON))
}

func extractURIParams(uriTemplate string) []map[string]interface{} {
	var params []map[string]interface{}
	start := -1
	for i, ch := range uriTemplate {
		if ch == '{' {
			start = i + 1
		} else if ch == '}' && start >= 0 {
			paramName := uriTemplate[start:i]
			params = append(params, map[string]interface{}{
				"Name":        paramName,
				"Description": fmt.Sprintf("Parameter from URI template: %s", paramName),
			})
			start = -1
		}
	}

	return params
}
