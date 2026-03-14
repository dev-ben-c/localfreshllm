package engram

import (
	"context"
	"fmt"
	"strings"
)

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is the output returned to the LLM after executing a tool.
type ToolResult struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

type toolDef struct {
	Name        string
	Description string
	Properties  map[string]map[string]string
	Required    []string
}

var allTools = []toolDef{
	{
		Name:        "engram_recall",
		Description: "Search persistent memory for stored knowledge. Returns ranked results combining keyword relevance, recency, and confidence. Use this to find facts, past decisions, configurations, and context about any topic.",
		Properties: map[string]map[string]string{
			"query":    {"type": "string", "description": "Natural language search query (e.g. 'network configuration', 'proxmox GPU passthrough')"},
			"category": {"type": "string", "description": "Optional category filter (e.g. 'nas', 'proxmox', 'network', 'projects')"},
			"limit":    {"type": "number", "description": "Max results to return (default 10)"},
		},
		Required: []string{"query"},
	},
	{
		Name:        "engram_remember",
		Description: "Store a new memory or update an existing one. Use memory_type 'fact' for stable knowledge (configs, IPs, architecture), 'episode' for experiential context (debugging sessions, decisions), 'preference' for user preferences. Facts with the same category+key are auto-deduplicated (upserted).",
		Properties: map[string]map[string]string{
			"content":     {"type": "string", "description": "The memory content to store"},
			"memory_type": {"type": "string", "description": "Type: fact, episode, or preference (default: fact)"},
			"category":    {"type": "string", "description": "Category for organization (e.g. 'network', 'nas', 'proxmox')"},
			"key":         {"type": "string", "description": "Unique key within category. Same category+key = automatic update of existing memory"},
			"confidence":  {"type": "number", "description": "Confidence level 0.0-1.0 (default: 1.0)"},
			"context":     {"type": "string", "description": "Your reasoning for storing this memory — explain WHY you concluded this"},
		},
		Required: []string{"content"},
	},
	{
		Name:        "engram_get_context",
		Description: "Get bootstrapping context: user preferences, recently updated facts, and optionally topic-specific memories. Use when you need broad context about the user or system.",
		Properties: map[string]map[string]string{
			"topic": {"type": "string", "description": "Optional topic to focus context retrieval on"},
			"limit": {"type": "number", "description": "Max results per section (default 10)"},
		},
	},
}

func buildProps(props map[string]map[string]string) map[string]any {
	out := make(map[string]any, len(props))
	for name, p := range props {
		out[name] = map[string]any{"type": p["type"], "description": p["description"]}
	}
	return out
}

// OllamaToolDefs returns tool definitions in Ollama's format.
func OllamaToolDefs() []any {
	out := make([]any, 0, len(allTools))
	for _, t := range allTools {
		params := map[string]any{
			"type":       "object",
			"properties": buildProps(t.Properties),
		}
		if len(t.Required) > 0 {
			params["required"] = t.Required
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

// AnthropicToolDefs returns tool definitions in Anthropic's format.
func AnthropicToolDefs() []any {
	out := make([]any, 0, len(allTools))
	for _, t := range allTools {
		schema := map[string]any{
			"type":       "object",
			"properties": buildProps(t.Properties),
		}
		if len(t.Required) > 0 {
			schema["required"] = t.Required
		}
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": schema,
		})
	}
	return out
}

// ToolNames returns the set of tool names provided by this package.
func ToolNames() map[string]bool {
	names := make(map[string]bool, len(allTools))
	for _, t := range allTools {
		names[t.Name] = true
	}
	return names
}

// Executor runs engram tool calls and returns results.
type Executor struct {
	store *Store
	model string // LLM model name, auto-injected into stored memories
}

// NewExecutor creates a tool executor with the given store and model name.
func NewExecutor(store *Store, model string) *Executor {
	return &Executor{store: store, model: model}
}

// Close releases the database connection.
func (e *Executor) Close() {
	if e.store != nil {
		e.store.Close()
	}
}

// Execute runs a single tool call and returns the result.
func (e *Executor) Execute(ctx context.Context, call ToolCall) ToolResult {
	var result ToolResult
	switch call.Name {
	case "engram_recall":
		result = e.execRecall(ctx, call)
	case "engram_remember":
		result = e.execRemember(ctx, call)
	case "engram_get_context":
		result = e.execGetContext(ctx, call)
	default:
		return ToolResult{
			ID:      call.ID,
			Name:    call.Name,
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}
	}

	if !result.IsError {
		result.Content = "[TOOL RESULT: " + result.Name + "]\n" + result.Content + "\n[END TOOL RESULT]"
	}
	return result
}

func (e *Executor) execRecall(_ context.Context, call ToolCall) ToolResult {
	query, _ := call.Args["query"].(string)
	if query == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: query", IsError: true}
	}

	category, _ := call.Args["category"].(string)
	limit := intArg(call.Args, "limit", 10)

	memories, err := e.store.Recall(query, category, limit)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("recall error: %v", err), IsError: true}
	}

	if len(memories) == 0 {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("No memories found for query: %s", query)}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(memories)))
	for _, m := range memories {
		keyStr := ""
		if m.Key != "" {
			keyStr = "/" + m.Key
		}
		sb.WriteString(fmt.Sprintf("[%s] (%s/%s%s) confidence=%.1f score=%.3f\n",
			m.ID, m.MemoryType, m.Category, keyStr, m.Confidence, m.Score))
		sb.WriteString(m.Content)
		sb.WriteString(fmt.Sprintf("\nupdated: %s | model: %s\n\n", m.UpdatedAt, m.Model))
	}

	return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String()}
}

func (e *Executor) execRemember(_ context.Context, call ToolCall) ToolResult {
	content, _ := call.Args["content"].(string)
	if content == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: content", IsError: true}
	}

	memoryType, _ := call.Args["memory_type"].(string)
	category, _ := call.Args["category"].(string)
	key, _ := call.Args["key"].(string)
	confidence := floatArg(call.Args, "confidence", 1.0)
	memCtx, _ := call.Args["context"].(string)

	id, err := e.store.Remember(content, memoryType, category, key, "[]", confidence, e.model, memCtx)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("remember error: %v", err), IsError: true}
	}

	catKey := category
	if key != "" {
		catKey += "/" + key
	}
	return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("Stored memory [%s] (%s)", id, catKey)}
}

func (e *Executor) execGetContext(_ context.Context, call ToolCall) ToolResult {
	topic, _ := call.Args["topic"].(string)
	limit := intArg(call.Args, "limit", 10)

	result, err := e.store.GetContext(topic, limit)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("get_context error: %v", err), IsError: true}
	}

	return ToolResult{ID: call.ID, Name: call.Name, Content: result}
}

func intArg(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

func floatArg(args map[string]any, key string, defaultVal float64) float64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return defaultVal
}
