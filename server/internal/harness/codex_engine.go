package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Codex connects to the installed CLI app-server using the user's local login.
type Codex struct{ MCPExecutable, DBPath, FleetDir string }

type EngineHealth struct {
	Ready      bool   `json:"ready"`
	Executable string `json:"executable"`
	Version    string `json:"version"`
	Error      string `json:"error,omitempty"`
}

// ResolveExecutable bypasses mise shims without executing their install/update wrapper.
func ResolveExecutable(configured string) (string, error) {
	if configured != "" {
		return resolvedExecutable(configured)
	}
	if mise, err := exec.LookPath("mise"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if raw, err := exec.CommandContext(ctx, mise, "which", "codex").Output(); err == nil {
			path := strings.TrimSpace(string(raw))
			if filepath.IsAbs(path) {
				if info, err := os.Stat(path); err == nil && !info.IsDir() {
					return resolvedExecutable(path)
				}
			}
		}
	}
	return resolvedExecutable("codex")
}
func resolvedExecutable(path string) (string, error) {
	path, err := exec.LookPath(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
func CheckEngine(ctx context.Context, configured string) EngineHealth {
	h := EngineHealth{}
	path, err := ResolveExecutable(strings.TrimSpace(configured))
	if err != nil {
		h.Error = "Codex was not found. Set its executable path in AI settings."
		return h
	}
	h.Executable = path
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	configureProcess(cmd)
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Run(); err != nil {
		h.Error = "Codex did not pass its startup check: " + err.Error()
		return h
	}
	h.Version = strings.TrimSpace(output.String())
	if h.Version == "" || len(h.Version) > 100 {
		h.Error = "Codex returned an invalid version response."
		return h
	}
	// Probe the transport actually used by this adapter, not codex exec.
	help := exec.CommandContext(ctx, path, "app-server", "--help")
	configureProcess(help)
	var capabilities limitedBuffer
	help.Stdout = &capabilities
	help.Stderr = &capabilities
	if err := help.Run(); err != nil || !strings.Contains(capabilities.String(), "--listen") {
		h.Error = "Codex app-server is unavailable. Update Codex CLI."
		return h
	}
	h.Ready = true
	return h
}

// JSON encoding supplies the escaping needed by TOML basic strings, including
// newlines, quotes, and control characters. No shell interpolation is involved.
func tomlString(value string) string     { raw, _ := json.Marshal(value); return string(raw) }
func tomlStrings(values []string) string { raw, _ := json.Marshal(values); return string(raw) }
func conductorSchema(phase string) []byte {
	props := map[string]any{"completed": map[string]string{"type": "boolean"}, "summary": map[string]string{"type": "string"}, "blocker": map[string]string{"type": "string"}}
	required := []string{"completed", "summary", "blocker"}
	if phase == "planning" {
		props = map[string]any{"ready": map[string]string{"type": "boolean"}, "context": map[string]string{"type": "string"}, "missingContext": map[string]string{"type": "string"}}
		required = []string{"ready", "context", "missingContext"}
	}
	raw, _ := json.Marshal(map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false})
	return raw
}

const outputLimit = 2 * 1024 * 1024

type limitedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := outputLimit - b.Len()
	if n > remaining {
		b.truncated = true
	}
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
