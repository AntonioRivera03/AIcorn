// Package markdown converts task bodies between the Plate.js document JSON
// stored in task.body and plain markdown.
//
// The conversion is not reimplemented in Go. Plate's own serializer
// (@platejs/markdown) is the only thing that knows the app's exact node
// inventory, so it is bundled headless into a single Node script
// (app/scripts/md-convert.ts -> server/assets/bin/md-convert.cjs) and driven as
// a subprocess. That keeps one definition of the document format instead of two
// that drift.
package markdown

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/assets/bin"
)

const (
	defaultTimeout = 5 * time.Second
	// perItemTimeout scales the deadline up for large batches (search_tasks can
	// send up to 100 bodies in one call) — Node's own startup cost is fixed, but
	// serializing N Plate documents is not, so a single flat timeout that's
	// comfortable for one item can fail a full batch under load.
	perItemTimeout = 100 * time.Millisecond
)

// Converter runs the bundled Node script. Use it as a pointer: it caches the
// extracted script path across calls.
type Converter struct {
	// NodePath is the node executable to run. Empty means "node" on PATH.
	NodePath string
	// Timeout bounds a single conversion. Zero means defaultTimeout.
	Timeout time.Duration

	extractOnce sync.Once
	scriptPath  string
	extractErr  error
}

type convertRequest struct {
	Direction string   `json:"direction"`
	Items     []string `json:"items"`
}

type convertResponse struct {
	Results []string `json:"results"`
	Error   string   `json:"error"`
}

// ToMarkdown renders each Plate document to markdown, positionally. Bodies that
// are blank (a task created without one) convert to an empty string without
// paying for a subprocess.
func (c *Converter) ToMarkdown(ctx context.Context, bodies []string) ([]string, error) {
	markdowns := make([]string, len(bodies))

	positions := make([]int, 0, len(bodies))
	items := make([]string, 0, len(bodies))
	for i, body := range bodies {
		if strings.TrimSpace(body) == "" {
			continue
		}
		positions = append(positions, i)
		items = append(items, body)
	}
	if len(items) == 0 {
		return markdowns, nil
	}

	results, err := c.run(ctx, "toMarkdown", items)
	if err != nil {
		return nil, err
	}
	for i, result := range results {
		markdowns[positions[i]] = result
	}
	return markdowns, nil
}

// ToBody parses each markdown string into a Plate document, positionally. Blank
// markdown yields an empty document rather than an empty string, since the
// editor cannot open a body with no nodes.
func (c *Converter) ToBody(ctx context.Context, markdowns []string) ([]string, error) {
	if len(markdowns) == 0 {
		return nil, nil
	}
	return c.run(ctx, "toBody", markdowns)
}

func (c *Converter) run(ctx context.Context, direction string, items []string) ([]string, error) {
	script, err := c.script()
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(convertRequest{Direction: direction, Items: items})
	if err != nil {
		return nil, fmt.Errorf("markdown: encoding %s request: %w", direction, err)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
		if scaled := time.Duration(len(items)) * perItemTimeout; scaled > timeout {
			timeout = scaled
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	node := c.NodePath
	if node == "" {
		node = "node"
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, node, script)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("markdown: %s timed out after %s", direction, timeout)
	}

	// The script reports conversion failures as JSON on stdout and exits
	// non-zero, so decode before judging the exit code — the JSON carries the
	// useful message. Anything else (node missing, a crash) leaves stdout
	// unparseable and surfaces as the process error.
	var response convertResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("markdown: running %s: %w: %s", node, runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("markdown: decoding %s response: %w", direction, err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("markdown: %s: %s", direction, response.Error)
	}
	if runErr != nil {
		return nil, fmt.Errorf("markdown: running %s: %w: %s", node, runErr, strings.TrimSpace(stderr.String()))
	}
	if len(response.Results) != len(items) {
		return nil, fmt.Errorf("markdown: %s returned %d results for %d items", direction, len(response.Results), len(items))
	}
	return response.Results, nil
}

// script materializes the embedded bundle on disk, since node needs a real
// file. The name carries a hash of the contents, so an upgraded binary writes a
// new file instead of reusing a stale one, and repeat runs reuse the same file.
func (c *Converter) script() (string, error) {
	c.extractOnce.Do(func() {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			c.extractErr = fmt.Errorf("markdown: locating cache dir: %w", err)
			return
		}
		dir := filepath.Join(cacheDir, "aycorn")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			c.extractErr = fmt.Errorf("markdown: creating %s: %w", dir, err)
			return
		}

		sum := sha256.Sum256(bin.MdConvert)
		path := filepath.Join(dir, fmt.Sprintf("md-convert-%x.cjs", sum[:8]))
		if _, err := os.Stat(path); err == nil {
			c.scriptPath = path
			return
		}

		// Write under a temp name and rename, so a concurrently starting
		// process can never exec a half-written script.
		tmp, err := os.CreateTemp(dir, "md-convert-*.cjs.tmp")
		if err != nil {
			c.extractErr = fmt.Errorf("markdown: creating temp script: %w", err)
			return
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(bin.MdConvert); err != nil {
			tmp.Close()
			c.extractErr = fmt.Errorf("markdown: writing temp script: %w", err)
			return
		}
		if err := tmp.Close(); err != nil {
			c.extractErr = fmt.Errorf("markdown: closing temp script: %w", err)
			return
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			c.extractErr = fmt.Errorf("markdown: installing %s: %w", path, err)
			return
		}
		c.scriptPath = path
	})
	return c.scriptPath, c.extractErr
}
