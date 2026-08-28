package mcptools

import (
	"encoding/json"
	"fmt"
	"os"
)

// FilterTools returns catalog definitions filtered to only those whose name
// is present in allowed. Order follows orderedNames. Unknown names in allowed
// are silently skipped. Empty or nil allowed returns an empty (non-nil) slice
// — restrictive default, never all tools.
func FilterTools(allowed []string) []Definition {
	if len(allowed) == 0 {
		return []Definition{}
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	out := make([]Definition, 0)
	for _, name := range orderedNames {
		if _, ok := allowedSet[string(name)]; ok {
			out = append(out, definition(name))
		}
	}
	return out
}

// IsAllowed reports whether tool is present in allowed (exact match).
func IsAllowed(tool string, allowed []string) bool {
	for _, a := range allowed {
		if a == tool {
			return true
		}
	}
	return false
}

// mcpServerConfig is the per-server entry in the generated config file.
type mcpServerConfig struct {
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	AllowedTools []string `json:"allowedTools"`
	// Tools mirrors AllowedTools for harnesses that expect a top-level
	// "tools" key inside the server entry.
	Tools []string `json:"tools"`
}

// mcpConfigFile is the canonical on-disk shape written by GenerateMCPConfig.
// It includes both the nested mcpServers.aycorn.allowedTools form and a
// top-level "tools" array so harnesses expecting either shape are satisfied.
//
// Canonical shape:
//
//	{
//	  "mcpServers": {
//	    "aycorn": {
//	      "command": "./cmd/mcp",
//	      "args": [],
//	      "allowedTools": ["search_tasks", ...],
//	      "tools": ["search_tasks", ...]
//	    }
//	  },
//	  "tools": ["search_tasks", ...]
//	}
type mcpConfigFile struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
	Tools      []string                   `json:"tools"`
}

// GenerateMCPConfig writes a temporary JSON MCP config file exposing ONLY
// tools named in allowedTools. Unknown names are skipped. Empty allowedTools
// yields an empty tool list (restrictive default — never grants all tools).
//
// The file is created via os.CreateTemp(dir, "mcp-*.json") with mode 0600
// and JSON indentation. dir may be "" to use the OS default temp directory.
//
// It returns the path, a cleanup func that removes the file (safe to call
// multiple times; errors are ignored), and any error. The caller is
// responsible for calling cleanup when the job completes — the file is
// ephemeral per-job and must not be persisted beyond the job lifetime.
func GenerateMCPConfig(allowedTools []string, dir string) (string, func(), error) {
	filtered := FilterTools(allowedTools)
	names := make([]string, 0, len(filtered))
	for _, d := range filtered {
		names = append(names, d.Name)
	}
	// Ensure non-nil empty slice encodes as [] not null.
	if names == nil {
		names = []string{}
	}

	cfg := mcpConfigFile{
		MCPServers: map[string]mcpServerConfig{
			"aycorn": {
				Command:      "./cmd/mcp",
				Args:         []string{},
				AllowedTools: names,
				Tools:        names,
			},
		},
		Tools: names,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", func() {}, fmt.Errorf("marshal mcp config: %w", err)
	}

	f, err := os.CreateTemp(dir, "mcp-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp mcp config: %w", err)
	}
	path := f.Name()

	// Ensure 0600 regardless of umask.
	if err := f.Chmod(0600); err != nil {
		f.Close()
		os.Remove(path)
		return "", func() {}, fmt.Errorf("chmod mcp config: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", func() {}, fmt.Errorf("write mcp config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", func() {}, fmt.Errorf("close mcp config: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(path)
	}

	return path, cleanup, nil
}
