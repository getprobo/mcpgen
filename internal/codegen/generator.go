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
	"golang.org/x/mod/modfile"
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

	if err := g.generateResolverStruct(); err != nil {
		return fmt.Errorf("failed to generate resolver struct: %w", err)
	}

	if err := g.generateResolverImplementations(); err != nil {
		return fmt.Errorf("failed to generate resolver implementations: %w", err)
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
	code, err := g.typeGen.Generate(g.config.Model.Package)
	if err != nil {
		return err
	}

	modelsFile := "models.go"
	if g.config.Model.Filename != "" {
		modelsFile = g.config.Model.Filename
	}
	modelsPath := filepath.Join(g.config.Output, modelsFile)

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

	serverFile := filepath.Join(g.config.Output, "server.go")

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

// generateResolverStruct creates the main resolver.go file with the Resolver struct
// This file is only generated once and users can edit it freely
func (g *Generator) generateResolverStruct() error {
	resolverFile := filepath.Join(g.config.Output, "resolver.go")

	// Only generate if file doesn't exist
	if _, err := os.Stat(resolverFile); err == nil {
		fmt.Printf("Resolver struct already exists, skipping: %s\n", resolverFile)
		return nil
	}

	tmpl, err := template.ParseFS(templates, "templates/resolver_struct.gotpl")
	if err != nil {
		return fmt.Errorf("failed to parse resolver_struct template: %w", err)
	}

	data := map[string]interface{}{
		"Package":      g.config.Resolver.Package,
		"ResolverType": g.config.Resolver.Type,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute resolver_struct template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format resolver struct code: %w\n%s", err, buf.String())
	}

	if err := os.WriteFile(resolverFile, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write resolver struct file: %w", err)
	}

	fmt.Printf("Generated resolver struct: %s\n", resolverFile)
	return nil
}

// generateResolverImplementations creates/updates schema.resolvers.go with tool/prompt/resource implementations
func (g *Generator) generateResolverImplementations() error {
	resolverFile := filepath.Join(g.config.Output, "schema.resolvers.go")

	fileExists := false
	if _, err := os.Stat(resolverFile); err == nil {
		fileExists = true
	}

	// If file doesn't exist, generate from template (initial generation)
	if !fileExists {
		return g.generateResolverFromTemplate(resolverFile)
	}

	// File exists and preserve is enabled - do incremental update (gqlgen-style)
	if g.config.Resolver.Preserve {
		return g.updateResolverIncremental(resolverFile)
	}

	// File exists but preserve is disabled - regenerate completely
	return g.generateResolverFromTemplate(resolverFile)
}

func (g *Generator) generateResolverFromTemplate(resolverFile string) error {
	tmpl, err := template.ParseFS(templates, "templates/resolver.gotpl")
	if err != nil {
		return fmt.Errorf("failed to parse resolver template: %w", err)
	}

	data := g.buildResolverTemplateData()

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute resolver template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format resolver code: %w\n%s", err, buf.String())
	}

	if err := os.WriteFile(resolverFile, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write resolver file: %w", err)
	}

	fmt.Printf("Generated resolver implementations: %s\n", resolverFile)
	return nil
}

func (g *Generator) updateResolverIncremental(resolverFile string) error {
	parser, err := NewResolverParser(resolverFile)
	if err != nil {
		return fmt.Errorf("failed to parse existing resolver: %w", err)
	}

	existingHandlers, err := parser.ExtractHandlers(g.config.Resolver.Type)
	if err != nil {
		return fmt.Errorf("failed to extract handlers: %w", err)
	}

	requiredHandlers := g.getRequiredHandlerNames()

	// Identify which handlers are new
	newHandlers := []string{}
	for _, required := range requiredHandlers {
		if _, exists := existingHandlers[required]; !exists {
			newHandlers = append(newHandlers, required)
		}
	}

	// Identify orphaned handlers (exist in file but not in spec, excluding already orphaned ones)
	// First, get the list of handlers that were already in the orphaned section
	previouslyOrphanedHandlers := extractOrphanedHandlerNames(resolverFile)

	// Identify which handlers are orphaned (not in required list and not already in orphaned section)
	currentlyOrphanedHandlers := []string{}
	for handlerName := range existingHandlers {
		isRequired := false
		for _, required := range requiredHandlers {
			if required == handlerName {
				isRequired = true
				break
			}
		}
		// Only mark as newly orphaned if not required and not already orphaned
		if !isRequired && !contains(previouslyOrphanedHandlers, handlerName) {
			currentlyOrphanedHandlers = append(currentlyOrphanedHandlers, handlerName)
		}
	}

	// Check if any previously orphaned handlers are now required (should be removed from orphaned)
	orphanedHandlersRemoved := []string{}
	for _, orphanedName := range previouslyOrphanedHandlers {
		if contains(requiredHandlers, orphanedName) {
			orphanedHandlersRemoved = append(orphanedHandlersRemoved, orphanedName)
		}
	}

	// Mark handlers as orphaned for formatting
	IdentifyOrphanedHandlers(existingHandlers, requiredHandlers)
	orphanedHandlers := FormatOrphanedHandlers(existingHandlers)

	// If nothing changed, skip update
	if len(newHandlers) == 0 && len(currentlyOrphanedHandlers) == 0 && len(orphanedHandlersRemoved) == 0 {
		fmt.Printf("Resolver is up to date, skipping: %s\n", resolverFile)
		return nil
	}

	// Generate code for new handlers only
	newHandlersCode, err := g.generateNewHandlersCode(newHandlers)
	if err != nil {
		return fmt.Errorf("failed to generate new handlers: %w", err)
	}

	// Read the existing file
	content, err := os.ReadFile(resolverFile)
	if err != nil {
		return fmt.Errorf("failed to read resolver file: %w", err)
	}

	// Remove any existing orphaned handlers section
	contentStr := string(content)
	if idx := strings.Index(contentStr, "\n// ==============================================================================\n// Orphaned Handlers\n"); idx != -1 {
		contentStr = contentStr[:idx]
	}

	// Build final content: existing code + new handlers + orphaned section
	var buf bytes.Buffer
	buf.WriteString(contentStr)

	if newHandlersCode != "" {
		buf.WriteString("\n")
		buf.WriteString(newHandlersCode)
	}

	if orphanedHandlers != "" {
		buf.WriteString(orphanedHandlers)
	}

	// Format the final code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format resolver code: %w\n%s", err, buf.String())
	}

	if err := os.WriteFile(resolverFile, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write resolver file: %w", err)
	}

	// Build status message
	var updates []string
	if len(newHandlers) > 0 {
		updates = append(updates, fmt.Sprintf("added %d new", len(newHandlers)))
	}
	if len(currentlyOrphanedHandlers) > 0 {
		updates = append(updates, fmt.Sprintf("orphaned %d", len(currentlyOrphanedHandlers)))
	}
	if len(orphanedHandlersRemoved) > 0 {
		updates = append(updates, fmt.Sprintf("restored %d from orphaned", len(orphanedHandlersRemoved)))
	}

	fmt.Printf("Updated resolver: %s: %s\n", strings.Join(updates, ", "), resolverFile)

	return nil
}

func countOrphanedHandlers(orphanedCode string) int {
	return strings.Count(orphanedCode, "// Orphaned:")
}

// extractOrphanedHandlerNames reads the orphaned section and returns list of handler names
func extractOrphanedHandlerNames(resolverFile string) []string {
	content, err := os.ReadFile(resolverFile)
	if err != nil {
		return nil
	}

	contentStr := string(content)
	orphanedSectionStart := strings.Index(contentStr, "\n// ==============================================================================\n// Orphaned Handlers\n")
	if orphanedSectionStart == -1 {
		return nil
	}

	orphanedSection := contentStr[orphanedSectionStart:]
	var names []string

	// Find all "// Orphaned: <HandlerName>" lines
	lines := strings.Split(orphanedSection, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "// Orphaned: ") {
			name := strings.TrimPrefix(line, "// Orphaned: ")
			names = append(names, name)
		}
	}

	return names
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (g *Generator) generateNewHandlersCode(handlerNames []string) (string, error) {
	if len(handlerNames) == 0 {
		return "", nil
	}

	// Parse the resolver template to extract individual handler templates
	tmpl, err := template.ParseFS(templates, "templates/resolver.gotpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse resolver template: %w", err)
	}

	// Build template data with only the new handlers
	data := g.buildResolverTemplateData()

	// Filter to only include new handlers
	handlerSet := make(map[string]bool)
	for _, name := range handlerNames {
		handlerSet[name] = true
	}

	// Filter tools (HandlerName in data matches exactly now)
	if tools, ok := data["Tools"].([]map[string]interface{}); ok {
		filteredTools := []map[string]interface{}{}
		for _, tool := range tools {
			if handlerName, ok := tool["HandlerName"].(string); ok {
				if handlerSet[handlerName] {
					filteredTools = append(filteredTools, tool)
				}
			}
		}
		data["Tools"] = filteredTools
	}

	// Filter resources (HandlerName in data matches exactly now)
	if resources, ok := data["Resources"].([]map[string]interface{}); ok {
		filteredResources := []map[string]interface{}{}
		for _, resource := range resources {
			if handlerName, ok := resource["HandlerName"].(string); ok {
				if handlerSet[handlerName] {
					filteredResources = append(filteredResources, resource)
				}
			}
		}
		data["Resources"] = filteredResources
		data["HasResources"] = len(filteredResources) > 0
	}

	// Filter prompts (HandlerName in data matches exactly now)
	if prompts, ok := data["Prompts"].([]map[string]interface{}); ok {
		filteredPrompts := []map[string]interface{}{}
		for _, prompt := range prompts {
			if handlerName, ok := prompt["HandlerName"].(string); ok {
				if handlerSet[handlerName] {
					filteredPrompts = append(filteredPrompts, prompt)
				}
			}
		}
		data["Prompts"] = filteredPrompts
		data["HasPrompts"] = len(filteredPrompts) > 0
	}

	// Generate only handler methods (not the full file structure)
	// NOTE: Must match the naming in resolver.gotpl template
	handlersOnlyTmpl, err := template.New("handlers").Parse(`
{{- range .Tools }}

// {{ .HandlerName }} handles the {{ .Name }} tool
// {{ .Description }}
func (r *toolResolver) {{ .HandlerName }}(ctx context.Context, req *mcp.CallToolRequest{{ if .HasInputType }}, input *{{ .InputType }}{{ end }}) (*mcp.CallToolResult, map[string]any, error) {
	// r.{{ $.ResolverType }} fields are available here
	return nil, nil, fmt.Errorf("{{ .Name }} not implemented")
}
{{- end }}

{{- range .Resources }}

// {{ .HandlerName }} handles the {{ .Name }} resource
// {{ .Description }}
func (r *resourceResolver) {{ .HandlerName }}(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	// r.{{ $.ResolverType }} fields are available here
	return nil, fmt.Errorf("{{ .Name }} not implemented")
}
{{- end }}

{{- range .Prompts }}

// {{ .HandlerName }} handles the {{ .Name }} prompt
// {{ .Description }}
func (r *promptResolver) {{ .HandlerName }}(ctx context.Context, req *mcp.GetPromptRequest{{ if .HasArgsType }}, args {{ .ArgsType }}{{ end }}) (*mcp.GetPromptResult, error) {
	// r.{{ $.ResolverType }} fields are available here
	return nil, fmt.Errorf("{{ .Name }} not implemented")
}
{{- end }}
`)
	if err != nil {
		return "", fmt.Errorf("failed to create handlers template: %w", err)
	}

	var buf bytes.Buffer
	if err := handlersOnlyTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute handlers template: %w", err)
	}

	_ = tmpl // Keep using template for future enhancements
	return buf.String(), nil
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
	// Compute type prefix if model package is different
	modelPackage := g.config.Model.Package
	resolverPackage := g.config.Resolver.Package
	typePrefix := ""
	modelImportPath := ""

	if modelPackage != resolverPackage {
		parts := strings.Split(modelPackage, "/")
		typePrefix = parts[len(parts)-1] + "."

		// Compute the full import path for the model package
		modelImportPath = g.computeModelImportPath()
	}

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
			inputTypeName := typePrefix + toPascalCase(tool.Name) + "Input"
			toolData["InputType"] = inputTypeName
			toolData["HasInputType"] = true

			// Add schema variable name with proper prefix
			schemaVarName := typePrefix + toHandlerName(tool.Name) + "ToolInputSchema"
			toolData["InputSchemaVar"] = schemaVarName

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
			argsTypeName := typePrefix + toPascalCase(prompt.Name) + "Args"
			promptData["ArgsType"] = argsTypeName
			promptData["HasArgsType"] = true
		}

		prompts = append(prompts, promptData)
	}

	data := map[string]interface{}{
		"Package":       g.config.Resolver.Package,
		"ServerName":    g.spec.Info.Title,
		"ServerVersion": g.spec.Info.Version,
		"ResolverType":  g.config.Resolver.Type,
		"Tools":         tools,
		"Resources":     resources,
		"Prompts":       prompts,
		"HasResources":  len(resources) > 0,
		"HasPrompts":    len(prompts) > 0,
		"HasTypedTools": hasTypedTools,
	}

	// Add model package import if different from resolver package
	if modelPackage != resolverPackage && modelImportPath != "" {
		var imports []string
		imports = append(imports, modelImportPath)
		data["Imports"] = imports
	}

	return data
}

func (g *Generator) buildResolverTemplateData() map[string]interface{} {
	return g.buildServerTemplateData()
}

// computeModelImportPath computes the full import path for the model package
func (g *Generator) computeModelImportPath() string {
	// If the model package is already a full path (contains slashes), use it as-is
	if strings.Contains(g.config.Model.Package, "/") {
		return g.config.Model.Package
	}

	// Make output path absolute
	absOutput, err := filepath.Abs(g.config.Output)
	if err != nil {
		return g.config.Model.Package
	}

	// Find the closest go.mod to the output directory
	modulePath, moduleRoot, err := findClosestGoMod(absOutput)
	if err != nil {
		// If we can't read go.mod, fall back to using the package name directly
		return g.config.Model.Package
	}

	// Compute the relative path from module root to output directory
	relPath, err := filepath.Rel(moduleRoot, absOutput)
	if err != nil {
		// If we can't compute relative path, fall back to package name
		return g.config.Model.Package
	}

	// Compute the import path based on module + relative path + model filename dir
	// Example: demo + generated + types = demo/generated/types
	modelDir := filepath.Dir(g.config.Model.Filename)
	if modelDir == "." {
		// If model filename has no directory component, use the model package name
		return filepath.ToSlash(filepath.Join(modulePath, relPath, g.config.Model.Package))
	}

	// Otherwise, use the directory from the filename
	return filepath.ToSlash(filepath.Join(modulePath, relPath, modelDir))
}

// findClosestGoMod finds the closest go.mod file by walking up from the given directory
// Returns the module path and the directory containing go.mod
func findClosestGoMod(startDir string) (modulePath string, moduleRoot string, err error) {
	// Make startDir absolute
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", "", err
	}

	// Walk up the directory tree looking for go.mod
	currentDir := absDir
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, parse it using the official modfile package
			data, err := os.ReadFile(goModPath)
			if err != nil {
				return "", "", err
			}

			parsed, err := modfile.Parse(goModPath, data, nil)
			if err != nil {
				return "", "", fmt.Errorf("failed to parse %s: %w", goModPath, err)
			}

			if parsed.Module == nil || parsed.Module.Mod.Path == "" {
				return "", "", fmt.Errorf("no module directive found in %s", goModPath)
			}

			return parsed.Module.Mod.Path, currentDir, nil
		}

		// Move up one directory
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached the root directory
			return "", "", fmt.Errorf("no go.mod found in any parent directory of %s", absDir)
		}
		currentDir = parent
	}
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
