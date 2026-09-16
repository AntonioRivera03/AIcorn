package harness

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
	"github.com/waseem-polus/aycorn/server/internal/models"
)

// Agent files are bundled with the server, so a task in any repository receives
// the same fleet. Versioned paths survive restarts and native Codex resume.
func (h *Codex) installFleet(spec RunSpec) (string, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(fleet.Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fleet.Files.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = data
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, d := range fleet.All() {
		model := spec.Request.AgentModels[d.Role]
		// Preserve the model of an already queued Conductor cycle or Job while
		// always using the fixed Coder instructions.
		if c := spec.Request.Conductor; c != nil && d.Role == "coder" && c.TaskAgent != nil {
			model = c.TaskAgent.Model
		}
		if model != "" && !models.IsOpenAIModel(model) {
			return "", fmt.Errorf("invalid model for %s", d.Name)
		}
		files[d.Role+".toml"] = d.Config(model, fleet.WorkflowPath)
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		fmt.Fprintf(hash, "%s\x00%s\x00", key, files[key])
	}
	root := h.FleetDir
	if root == "" {
		root = filepath.Join(filepath.Dir(h.DBPath), "harness-fleet")
	}
	root, err = filepath.Abs(filepath.Join(root, fmt.Sprintf("%x", hash.Sum(nil))))
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		path := filepath.Join(root, key)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if current, readErr := os.ReadFile(path); readErr == nil && string(current) == string(files[key]) {
			continue
		}
		// Concurrent jobs may install the same version. Publish complete files
		// atomically, so Codex never observes a partially written agent config.
		temp, createErr := os.CreateTemp(filepath.Dir(path), ".fleet-")
		if createErr != nil {
			return "", createErr
		}
		_, err = temp.Write(files[key])
		closeErr := temp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(temp.Name(), path)
		}
		_ = os.Remove(temp.Name())
		if err != nil {
			return "", err
		}
	}
	return root, nil
}
