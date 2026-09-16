package harness

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed fleet
var fleet embed.FS

var fleetRoles = []string{"conductor", "planner", "researcher", "coder", "reviewer", "chatter"}

// Agent files are bundled with the server, so a task in any repository receives
// the same fleet. Versioned paths survive restarts and native Codex resume.
func (h *Codex) installFleet(spec RunSpec) (string, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(fleet, "fleet", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fleet.ReadFile(path)
		if err != nil {
			return err
		}
		files[path[len("fleet/"):]] = data
		return nil
	})
	if err != nil {
		return "", err
	}
	if c := spec.Request.Conductor; c != nil && c.TaskAgent != nil {
		instructions := "You are Conductor's coder. Use Aycorn MCP for ticket context, implement the assigned scope and report actual validation. Do not move tickets or change ownership.\n\nSelected task agent instructions:\n" + c.TaskAgent.Instructions
		files["coder.toml"] = []byte("name = \"coder\"\ndescription = \"Implement the assigned ticket scope\"\nmodel = " + tomlString(c.TaskAgent.Model) + "\ndeveloper_instructions = " + tomlString(instructions) + "\n")
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
