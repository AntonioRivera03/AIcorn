package harness

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

// ModelInfo is one model a harness can run, as shown in the AI settings
// dropdowns.
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// ModelList says where the models came from: Live is true when the harness
// reported them itself, false for the fixed list below.
type ModelList struct {
	Models []ModelInfo `json:"models"`
	Live   bool        `json:"live"`
}

// fixedModels is used when a harness can't list its own models: always for
// Claude Code (its CLI has no model listing), and for Codex when app-server
// is missing or fails.
//
// TODO: move these lists to an external JSON file so models can be added
// without an Aycorn release.
var fixedModels = map[models.PersonaHarness][]ModelInfo{
	models.PersonaHarnessCodex: {
		{ID: "gpt-6-astra", Name: "GPT-6-Astra", Default: true},
		{ID: "gpt-6-sol", Name: "GPT-6-Sol"},
		{ID: "gpt-6-luna", Name: "GPT-6-Luna"},
		{ID: "gpt-5.6-sol", Name: "GPT-5.6-Sol"},
		{ID: "gpt-5.6-terra", Name: "GPT-5.6-Terra"},
		{ID: "gpt-5.6-luna", Name: "GPT-5.6-Luna"},
		{ID: "gpt-5.5", Name: "GPT-5.5"},
	},
	models.PersonaHarnessClaudeCode: {
		{ID: "claude-opus-5-5", Name: "Claude Opus 5.5", Default: true},
		{ID: "claude-fable-5-1", Name: "Claude Fable 5.1"},
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5"},
	},
}

// DefaultModel is the model a workspace starts on after choosing a harness.
func DefaultModel(harness models.PersonaHarness) string {
	for _, m := range fixedModels[harness] {
		if m.Default {
			return m.ID
		}
	}
	return ""
}

// Listing Codex models starts an app-server, which takes about a second, so
// results are cached per executable. The list depends on the server machine's
// Codex login, which every workspace shares, so the cache is not per workspace.
const modelCacheTTL = 10 * time.Minute

type cachedModels struct {
	models  []ModelInfo
	fetched time.Time
}

var (
	modelCacheMu sync.Mutex
	modelCache   = map[string]cachedModels{}
)

// Models lists the models a harness offers, asking the harness itself where it
// can and falling back to the fixed list otherwise.
func Models(ctx context.Context, harness models.PersonaHarness, executable string) ModelList {
	fixed := ModelList{Models: fixedModels[harness]}
	if harness != models.PersonaHarnessCodex {
		return fixed
	}
	path, err := ResolveExecutable(executable)
	if err != nil {
		return fixed
	}
	modelCacheMu.Lock()
	cached, ok := modelCache[path]
	modelCacheMu.Unlock()
	if ok && time.Since(cached.fetched) < modelCacheTTL {
		return ModelList{Models: cached.models, Live: true}
	}
	live, err := listCodexModels(ctx, path)
	if err != nil || len(live) == 0 {
		return fixed
	}
	modelCacheMu.Lock()
	modelCache[path] = cachedModels{models: live, fetched: time.Now()}
	modelCacheMu.Unlock()
	return ModelList{Models: live, Live: true}
}

// listCodexModels asks Codex app-server for the models this login can use,
// following pagination and skipping models Codex hides from its own picker.
func listCodexModels(ctx context.Context, executable string) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cwd := os.TempDir()
	client, err := startRPC(ctx, executable, []string{"app-server", "--listen", "stdio://"}, codexEnvironment(cwd), cwd, func(rpcMessage) {})
	if err != nil {
		return nil, err
	}
	defer client.close()
	if err = client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aycorn", "title": "Aycorn", "version": "1.0"}}, nil); err != nil {
		return nil, err
	}
	if err = client.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return nil, err
	}
	list := []ModelInfo{}
	params := map[string]any{}
	for {
		var page struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
				Hidden      bool   `json:"hidden"`
				IsDefault   bool   `json:"isDefault"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err = client.call(ctx, "model/list", params, &page); err != nil {
			return nil, err
		}
		for _, m := range page.Data {
			if m.Hidden || !models.IsOpenAIModel(m.ID) {
				continue
			}
			name := m.DisplayName
			if name == "" {
				name = m.ID
			}
			list = append(list, ModelInfo{ID: m.ID, Name: name, Description: m.Description, Default: m.IsDefault})
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return list, nil
		}
		params = map[string]any{"cursor": *page.NextCursor}
	}
}
