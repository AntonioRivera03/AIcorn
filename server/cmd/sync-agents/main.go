// Command sync-agents mirrors bundled definitions into repository Codex agents.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
)

func main() {
	root := flag.String("root", "..", "repository root (or staging directory)")
	check := flag.Bool("check", false, "check generated files without writing")
	flag.Parse()
	for _, d := range fleet.All() {
		stem := d.Role
		if stem == "researcher" {
			stem = "research"
		} // Existing repository agent file.
		files := map[string][]byte{
			"server/internal/harness/fleet/" + d.Role + ".toml": d.Config("", fleet.WorkflowPath),
			".codex/agents/" + stem + ".toml":                   d.Config("", "../../server/internal/harness/fleet/"+fleet.WorkflowPath),
			".codex/agents/" + stem + ".md":                     []byte(d.Instructions()),
		}
		for name, contents := range files {
			path := filepath.Join(*root, filepath.FromSlash(name))
			if *check {
				existing, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(existing, contents) {
					fail(fmt.Errorf("%s is out of date; run make sync-agents", name))
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				fail(err)
			}
			if err := os.WriteFile(path, contents, 0644); err != nil {
				fail(err)
			}
		}
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
