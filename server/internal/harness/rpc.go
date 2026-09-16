package harness

// The local app-server speaks newline-delimited JSON-RPC over stdio. Keeping
// transport separate from prompts and workflow rules mirrors T3's adapter boundary.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type rpcClient struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	next    int
	pending map[int]chan rpcMessage
	done    chan struct{}
	err     error
	stderr  limitedBuffer
}

func startRPC(ctx context.Context, executable string, args, env []string, cwd string, notify func(rpcMessage)) (*rpcClient, error) {
	c := &rpcClient{pending: make(map[int]chan rpcMessage), done: make(chan struct{})}
	c.cmd = exec.CommandContext(ctx, executable, args...)
	c.cmd.Dir, c.cmd.Env, c.cmd.Stderr = cwd, env, &c.stderr
	configureProcess(c.cmd)
	var err error
	if c.in, err = c.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	out, err := c.cmd.StdoutPipe()
	if err != nil {
		c.in.Close()
		return nil, err
	}
	if err = c.cmd.Start(); err != nil {
		c.in.Close()
		return nil, err
	}
	go func() {
		defer close(c.done)
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		for scanner.Scan() {
			var m rpcMessage
			if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
				c.err = fmt.Errorf("invalid app-server message: %w", err)
				return
			}
			if m.Method != "" {
				if len(m.ID) > 0 {
					// Background jobs cannot answer interactive questions or authorize
					// a broader sandbox. Return a protocol error instead of hanging.
					_ = c.send(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "Aycorn background runs cannot answer interactive requests"}})
				}
				notify(m)
				continue
			}
			var id int
			if json.Unmarshal(m.ID, &id) != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
		c.err = scanner.Err()
		if c.err == nil {
			c.err = errors.New("Codex app-server disconnected")
		}
	}()
	return c, nil
}

func (c *rpcClient) send(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return json.NewEncoder(c.in).Encode(value)
}

func (c *rpcClient) call(ctx context.Context, method string, params, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan rpcMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err := c.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(m.Result, result)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", method, ctx.Err())
	case <-c.done:
		return c.err
	}
}

func (c *rpcClient) close() {
	_ = c.in.Close()
	// Give app-server time to flush session history after a successful turn.
	select {
	case <-c.done:
	case <-time.After(time.Second):
		_ = c.cmd.Cancel()
	}
	_ = c.cmd.Cancel() // also reap any remaining descendants
	_ = c.cmd.Wait()
	<-c.done
}
