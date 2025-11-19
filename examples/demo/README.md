# mcpgen Demo Example

This is a comprehensive example demonstrating all mcpgen features.

## Features Demonstrated

### 1. Enum Support
- **Operation enum**: `add`, `subtract`, `multiply`, `divide` (in CalculateInput)
- **Priority enum**: `low`, `medium`, `high`, `urgent` (in TaskInput)
- **Filter enum**: `all`, `active`, `completed` (in search tool)
- **Status enum**: `pending`, `in_progress`, `completed`, `cancelled` (in TaskDetails)

### 2. Typed Inputs
- All tool handlers receive typed structs instead of `map[string]any`
- Direct field access without type assertions
- IDE autocomplete for all fields
- Compile-time type checking

### 3. Various Input Types
- **Strings**: title, query
- **Numbers**: a, b, value
- **Integers**: limit
- **Booleans**: completed
- **Arrays**: tags
- **Date-time**: deadline, createdAt
- **Enums**: operation, priority, filter, status

### 4. Tools
- `calculate` - Math operations with enum
- `create_task` - Complex typed input
- `search` - Inline schema with enum
- `get_history` - Simple input

### 5. Resources
- **Static**: `docs://readme` - Documentation
- **Templated**: `task://{id}` - Task details with typed schema
- **Templated**: `result://{operation}` - Result with typed schema

### 6. Prompts
- `task_help` - Help with optional arguments
- `math_help` - Simple help prompt

## Getting Started

1. **Generate code**:
   ```bash
   mcpgen generate
   ```

2. **Check generated files**:
   - `generated/models.go` - Typed structs and enums
   - `generated/resolver.go` - Handler stubs with typed inputs
   - `generated/server.go` - Server setup with automatic type conversion

3. **Implement handlers**:
   Edit `generated/resolver.go` and implement the TODO sections.

## Generated Code Highlights

### Enums (models.go)
```go
type Operation string

const (
    OperationAdd      Operation = "add"
    OperationSubtract Operation = "subtract"
    OperationMultiply Operation = "multiply"
    OperationDivide   Operation = "divide"
)

type Priority string

const (
    PriorityLow    Priority = "low"
    PriorityMedium Priority = "medium"
    PriorityHigh   Priority = "high"
    PriorityUrgent Priority = "urgent"
)
```

### Typed Inputs (models.go)
```go
type CalculateInput struct {
    A         float64   `json:"a"`
    B         float64   `json:"b"`
    Operation Operation `json:"operation"`  // Enum type!
}

type TaskInput struct {
    Title     string    `json:"title"`
    Priority  Priority  `json:"priority"`  // Enum type!
    Deadline  time.Time `json:"deadline,omitempty"`
    Tags      []string  `json:"tags,omitempty"`
    Completed bool      `json:"completed,omitempty"`
}
```

### Typed Handlers (resolver.go)
```go
// Before: func Calculate(ctx, req, args map[string]any)
// After:
func (r *Resolver) Calculate(ctx context.Context, req *mcp.CallToolRequest,
    input *CalculateInput) (*mcp.CallToolResult, error) {

    // Direct access to typed fields:
    result := input.A + input.B

    // Type-safe enum switching:
    switch input.Operation {
    case OperationAdd:
        result = input.A + input.B
    case OperationSubtract:
        result = input.A - input.B
    // ...
    }
}
```

## Benefits

1. **Type Safety**: No runtime type assertion errors
2. **IDE Support**: Full autocomplete for fields and enums
3. **Compile-Time Checks**: Invalid operations caught early
4. **Cleaner Code**: 70% less boilerplate
5. **Better Testing**: Easy to create typed test inputs
6. **Self-Documenting**: Types serve as documentation

## Project Structure

```
demo/
├── mcpgen.yaml       # Generation configuration
├── mcp.yaml          # MCP API specification
├── README.md         # This file
└── generated/        # Generated code (after running mcpgen)
    ├── models.go     # Typed structs and enums
    ├── resolver.go   # Handler implementations
    └── server.go     # MCP server setup
```

## Next Steps

1. Run `mcpgen generate` to create the code
2. Implement handlers in `generated/resolver.go`
3. Build and run your MCP server
4. Enjoy type-safe development!
