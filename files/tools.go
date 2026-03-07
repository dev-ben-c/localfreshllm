package files

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

const maxReadBytes = 512 * 1024 // 512 KB

var allTools = []toolDef{
	{
		Name:        "file_read",
		Description: "Read the contents of a text file. Returns numbered lines. For large files, use offset and limit to read specific line ranges.",
		Properties: map[string]map[string]string{
			"path":   {"type": "string", "description": "Absolute path to the file to read"},
			"offset": {"type": "integer", "description": "Line number to start reading from (1-based, optional, default 1)"},
			"limit":  {"type": "integer", "description": "Maximum number of lines to return (optional, default 200)"},
		},
		Required: []string{"path"},
	},
	{
		Name:        "file_list",
		Description: "List files and directories at the given path. Returns names, sizes, types, and modification times.",
		Properties: map[string]map[string]string{
			"path": {"type": "string", "description": "Absolute path to the directory to list"},
		},
		Required: []string{"path"},
	},
	{
		Name:        "file_write",
		Description: "Write text content to a file. Creates the file if it doesn't exist, or overwrites it. Parent directories must already exist.",
		Properties: map[string]map[string]string{
			"path":    {"type": "string", "description": "Absolute path to the file to write"},
			"content": {"type": "string", "description": "The text content to write to the file"},
		},
		Required: []string{"path", "content"},
	},
	{
		Name:        "file_info",
		Description: "Get metadata about a file or directory: size, permissions, modification time, and whether it is a file, directory, or symlink.",
		Properties: map[string]map[string]string{
			"path": {"type": "string", "description": "Absolute path to the file or directory"},
		},
		Required: []string{"path"},
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

// Executor runs file tool calls and returns results.
type Executor struct {
	allowedPaths []string // resolved absolute paths
}

// NewExecutor creates a file tool executor.
func NewExecutor(allowedPaths []string) *Executor {
	resolved := make([]string, 0, len(allowedPaths))
	for _, p := range allowedPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		resolved = append(resolved, abs)
	}
	return &Executor{allowedPaths: resolved}
}

// NewExecutorFromEnv creates an Executor using FILE_ALLOWED_PATHS env var.
// Paths are colon-separated. Defaults to $HOME if unset.
// Returns nil if no valid paths can be resolved.
func NewExecutorFromEnv() *Executor {
	paths := os.Getenv("FILE_ALLOWED_PATHS")
	if paths == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return nil
		}
		return NewExecutor([]string{home})
	}
	var cleaned []string
	for _, p := range strings.Split(paths, ":") {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return NewExecutor(cleaned)
}

// validatePath checks that the resolved path is under an allowed base directory.
func (e *Executor) validatePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %q", path)
	}

	cleaned := filepath.Clean(path)

	// Resolve symlinks if the path exists to prevent traversal.
	resolved := cleaned
	if r, err := filepath.EvalSymlinks(cleaned); err == nil {
		resolved = r
	}

	for _, base := range e.allowedPaths {
		if resolved == base || strings.HasPrefix(resolved, base+string(filepath.Separator)) {
			return cleaned, nil
		}
	}

	return "", fmt.Errorf("path %q is outside allowed directories", path)
}

// Execute runs a single tool call and returns the result.
func (e *Executor) Execute(ctx context.Context, call ToolCall) ToolResult {
	var result ToolResult
	switch call.Name {
	case "file_read":
		result = e.execRead(call)
	case "file_list":
		result = e.execList(call)
	case "file_write":
		result = e.execWrite(call)
	case "file_info":
		result = e.execInfo(call)
	default:
		return ToolResult{
			ID:      call.ID,
			Name:    call.Name,
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}
	}

	result.Content = sanitize(result.Content)
	if !result.IsError {
		result.Content = "[TOOL RESULT: " + result.Name + "]\n" + result.Content + "\n[END TOOL RESULT]"
	}
	return result
}

func (e *Executor) execRead(call ToolCall) ToolResult {
	path, _ := call.Args["path"].(string)
	if path == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: path", IsError: true}
	}

	validated, err := e.validatePath(path)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}

	info, err := os.Stat(validated)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error: %v", err), IsError: true}
	}
	if info.IsDir() {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "path is a directory — use file_list instead", IsError: true}
	}

	// Read up to maxReadBytes.
	f, err := os.Open(validated)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error opening file: %v", err), IsError: true}
	}
	defer f.Close()

	data := make([]byte, maxReadBytes+1)
	n, readErr := io.ReadFull(f, data)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error reading file: %v", readErr), IsError: true}
	}
	truncated := n > maxReadBytes
	if truncated {
		n = maxReadBytes
	}
	data = data[:n]

	// Check for binary content.
	if !utf8.Valid(data) || containsNull(data) {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("binary file (%d bytes) — cannot display as text", info.Size()), IsError: true}
	}

	lines := strings.Split(string(data), "\n")

	offset := 1
	limit := 200
	if o, ok := toInt(call.Args["offset"]); ok && o > 0 {
		offset = o
	}
	if l, ok := toInt(call.Args["limit"]); ok && l > 0 {
		limit = l
	}

	startIdx := offset - 1
	if startIdx >= len(lines) {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("offset %d exceeds file length (%d lines)", offset, len(lines))}
	}
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + limit
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "File: %s (%d lines", validated, len(lines))
	if truncated {
		sb.WriteString(", file truncated at 512 KB")
	}
	fmt.Fprintf(&sb, ", showing %d–%d)\n", startIdx+1, endIdx)

	for i := startIdx; i < endIdx; i++ {
		fmt.Fprintf(&sb, "%4d| %s\n", i+1, lines[i])
	}

	if endIdx < len(lines) {
		fmt.Fprintf(&sb, "\n... %d more lines, use offset=%d to continue", len(lines)-endIdx, endIdx+1)
	}

	return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String()}
}

func (e *Executor) execList(call ToolCall) ToolResult {
	path, _ := call.Args["path"].(string)
	if path == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: path", IsError: true}
	}

	validated, err := e.validatePath(path)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}

	info, err := os.Stat(validated)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error: %v", err), IsError: true}
	}
	if !info.IsDir() {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "path is a file — use file_read or file_info instead", IsError: true}
	}

	entries, err := os.ReadDir(validated)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error reading directory: %v", err), IsError: true}
	}

	if len(entries) == 0 {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("Directory: %s (empty)", validated)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Directory: %s (%d entries)\n", validated, len(entries))

	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			fmt.Fprintf(&sb, "  ? %s (error reading info)\n", entry.Name())
			continue
		}

		typeStr := "file"
		if entry.IsDir() {
			typeStr = "dir"
		} else if entry.Type()&os.ModeSymlink != 0 {
			typeStr = "link"
		}

		sizeStr := formatSize(entryInfo.Size())
		if entry.IsDir() {
			sizeStr = "-"
		}

		modTime := entryInfo.ModTime().Format("2006-01-02 15:04")
		fmt.Fprintf(&sb, "  %-5s %8s  %s  %s\n", typeStr, sizeStr, modTime, entry.Name())
	}

	return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String()}
}

func (e *Executor) execWrite(call ToolCall) ToolResult {
	path, _ := call.Args["path"].(string)
	if path == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: path", IsError: true}
	}
	content, _ := call.Args["content"].(string)
	if content == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: content", IsError: true}
	}

	validated, err := e.validatePath(path)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}

	// Check parent directory exists.
	dir := filepath.Dir(validated)
	if _, err := os.Stat(dir); err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("parent directory does not exist: %s", dir), IsError: true}
	}

	existed := false
	if _, err := os.Stat(validated); err == nil {
		existed = true
	}

	if err := os.WriteFile(validated, []byte(content), 0644); err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error writing file: %v", err), IsError: true}
	}

	action := "Created"
	if existed {
		action = "Updated"
	}
	return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("%s %s (%s)", action, validated, formatSize(int64(len(content))))}
}

func (e *Executor) execInfo(call ToolCall) ToolResult {
	path, _ := call.Args["path"].(string)
	if path == "" {
		return ToolResult{ID: call.ID, Name: call.Name, Content: "missing required parameter: path", IsError: true}
	}

	validated, err := e.validatePath(path)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}

	// Use Lstat to detect symlinks.
	linfo, err := os.Lstat(validated)
	if err != nil {
		return ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("error: %v", err), IsError: true}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Path: %s\n", validated)

	typeStr := "file"
	if linfo.IsDir() {
		typeStr = "directory"
	} else if linfo.Mode()&os.ModeSymlink != 0 {
		typeStr = "symlink"
		if target, err := os.Readlink(validated); err == nil {
			fmt.Fprintf(&sb, "Target: %s\n", target)
		}
	}
	fmt.Fprintf(&sb, "Type: %s\n", typeStr)
	fmt.Fprintf(&sb, "Size: %s (%d bytes)\n", formatSize(linfo.Size()), linfo.Size())
	fmt.Fprintf(&sb, "Permissions: %s\n", linfo.Mode().Perm())
	fmt.Fprintf(&sb, "Modified: %s\n", linfo.ModTime().Format(time.RFC3339))

	return ToolResult{ID: call.ID, Name: call.Name, Content: sb.String()}
}

func containsNull(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
