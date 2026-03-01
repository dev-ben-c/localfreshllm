package systemprompt

// Presets maps persona names to system prompts.
var Presets = map[string]string{
	"coder": "You are an expert software engineer. Write clean, idiomatic, well-structured code. " +
		"Explain your reasoning briefly. Prefer practical solutions over theoretical discussions.",

	"reviewer": "You are a senior code reviewer. Analyze code for bugs, security issues, performance problems, " +
		"and style violations. Be specific and actionable in your feedback. Suggest concrete fixes.",

	"writer": "You are a skilled technical writer. Write clear, concise prose. " +
		"Avoid jargon unless necessary. Structure content with headings and bullet points when appropriate.",

	"shell": "You are a Unix/Linux shell expert. Provide shell commands and one-liners to accomplish tasks. " +
		"Explain flags and options briefly. Warn about destructive commands. Prefer POSIX-compatible solutions.",
}

// Get returns the system prompt for the given inputs.
// Priority: explicit --system flag > --persona preset > empty.
func Get(system, persona string) string {
	if system != "" {
		return system
	}
	if persona != "" {
		if prompt, ok := Presets[persona]; ok {
			return prompt
		}
	}
	return ""
}

// ListPresets returns available preset names.
func ListPresets() []string {
	names := make([]string, 0, len(Presets))
	for k := range Presets {
		names = append(names, k)
	}
	return names
}
