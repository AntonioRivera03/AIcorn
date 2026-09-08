package environments

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CLI output is bounded even when a build or Kubernetes error is very large.
type tailBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
	log   func(string)
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = append([]byte(nil), b.data[len(b.data)-b.limit:]...)
	}
	if b.log != nil {
		b.log(string(p))
	}
	return len(p), nil
}
func (b *tailBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

func command(ctx context.Context, input []byte, log func(string), name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	stdout := &tailBuffer{limit: 2 << 20, log: log}
	stderr := &tailBuffer{limit: 64 << 10, log: log}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 2048 {
			message = "… " + message[len(message)-2048:]
		}
		return nil, fmt.Errorf("%s: %w: %s", name, err, message)
	}
	return []byte(stdout.String()), nil
}
