package harness

// OpenCode's local server uses HTTP for sessions and SSE for notifications.
// These protocol operations are prepared for the second provider; Registry keeps
// it disabled until permission handling and UI interaction are enabled together.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type openCodeClient struct {
	baseURL, directory, password string
	http                         *http.Client
}

type openCodeMessage struct {
	Info struct {
		ID     string          `json:"id"`
		Role   string          `json:"role"`
		Error  json.RawMessage `json:"error"`
		Tokens json.RawMessage `json:"tokens"`
	} `json:"info"`
	Parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"parts"`
}

func newOpenCodeClient(address, directory, password string) (*openCodeClient, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	// Only attach to the harness on this machine, never a remote model API.
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("OpenCode requires a loopback HTTP server URL")
	}
	return &openCodeClient{baseURL: strings.TrimRight(address, "/"), directory: directory, password: password, http: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *openCodeClient) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path+"?directory="+url.QueryEscape(c.directory), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.password != "" {
		req.SetBasicAuth("opencode", c.password)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("OpenCode %s %s: HTTP %d", method, path, response.StatusCode)
	}
	return response, nil
}

func (c *openCodeClient) call(ctx context.Context, method, path string, body, result any) error {
	response, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if result == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, outputLimit))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, outputLimit+1)).Decode(result)
}

func (c *openCodeClient) startSession(ctx context.Context, title string) (string, error) {
	var session struct {
		ID string `json:"id"`
	}
	err := c.call(ctx, "POST", "/session", map[string]string{"title": title}, &session)
	if err == nil && session.ID == "" {
		err = fmt.Errorf("OpenCode returned no session ID")
	}
	return session.ID, err
}

func (c *openCodeClient) sendTurn(ctx context.Context, id string, spec RunSpec) (openCodeMessage, error) {
	var message openCodeMessage
	provider, model, ok := strings.Cut(spec.Request.Model, "/")
	if !ok || provider == "" || model == "" {
		return message, fmt.Errorf("OpenCode requires provider/model")
	}
	developer, prompt := BuildContext(spec)
	err := c.call(ctx, "POST", "/session/"+url.PathEscape(id)+"/message", map[string]any{"system": developer, "model": map[string]string{"providerID": provider, "modelID": model}, "parts": []map[string]string{{"type": "text", "text": prompt}}}, &message)
	if err == nil && len(message.Info.Error) > 0 && string(message.Info.Error) != "null" {
		err = fmt.Errorf("OpenCode turn failed: %s", message.Info.Error)
	}
	return message, err
}

func (c *openCodeClient) abort(ctx context.Context, id string) error {
	return c.call(ctx, "POST", "/session/"+url.PathEscape(id)+"/abort", nil, nil)
}

func (c *openCodeClient) readMessages(ctx context.Context, id string) ([]openCodeMessage, error) {
	var messages []openCodeMessage
	err := c.call(ctx, "GET", "/session/"+url.PathEscape(id)+"/message", nil, &messages)
	return messages, err
}

func (c *openCodeClient) events(ctx context.Context, consume func(json.RawMessage) error) error {
	response, err := c.request(ctx, "GET", "/event", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), outputLimit)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 {
				raw := json.RawMessage(strings.TrimSuffix(data.String(), "\n"))
				if !json.Valid(raw) {
					return fmt.Errorf("invalid OpenCode event")
				}
				if err = consume(raw); err != nil {
					return err
				}
				data.Reset()
			}
		} else if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			data.WriteByte('\n')
		}
		if data.Len() > outputLimit {
			return fmt.Errorf("OpenCode event exceeds storage limit")
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF // a disconnected event stream is never completion
}
