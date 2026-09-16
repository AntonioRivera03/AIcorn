package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/harness/fleet"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (h *Codex) sessionConfig(spec RunSpec) map[string]any {
	tools := []string{"read_task"}
	env := map[string]string{"AYCORN_DB": h.DBPath, "AYCORN_RUN_TASK": fmt.Sprint(spec.TaskID)}
	if spec.JobID > 0 {
		env["AYCORN_RUN_JOB"] = fmt.Sprint(spec.JobID)
		tools = append(tools, "list_task_links", "add_task_link", "remove_task_link")
	}
	if spec.Request.Conductor != nil {
		tools = append(tools, "search_tasks", "project_context", "read_project_document")
		env["AYCORN_CONDUCTOR_PROJECT"] = fmt.Sprint(spec.Request.ProjectID)
	}
	if c := spec.Request.ProjectChat; c != nil {
		env = map[string]string{"AYCORN_DB": h.DBPath, "AYCORN_CHAT_TURN": fmt.Sprint(c.TurnID), "AYCORN_CHAT_PROJECT": fmt.Sprint(spec.Request.ProjectID)}
		tools = []string{"project_context", "read_task", "search_tasks", "create_task", "update_task", "move_task_stage", "list_task_links", "add_task_link", "remove_task_link", "read_project_document", "request_task_work"}
	}
	config := map[string]any{
		"agents.enabled": true,
		"agents.max_concurrent_threads_per_session": 4,
		"approval_policy":                        "never",
		"sandbox_workspace_write.network_access": false,
		"mcp_servers.aycorn.command":             h.MCPExecutable,
		"mcp_servers.aycorn.env":                 env,
		"mcp_servers.aycorn.required":            true,
		"mcp_servers.aycorn.enabled_tools":       tools,
	}
	// These scoped local tools are authorized by starting the Aycorn operation;
	// their server-side transaction guards still enforce ownership and turn scope.
	for _, name := range tools {
		config["mcp_servers.aycorn.tools."+name+".approval_mode"] = "approve"
	}
	return config
}

// The root session and its native children share the same scoped MCP server.
// Register executable agent profiles, not just role names in the prompt.
func (h *Codex) agentConfig(spec RunSpec, fleetPath string) map[string]any {
	config := h.sessionConfig(spec)
	for _, role := range fleet.All() {
		config["agents."+role.Role+".config_file"] = filepath.Join(fleetPath, role.Role+".toml")
		config["agents."+role.Role+".description"] = role.Description
	}
	config["skills.config"] = []map[string]any{{"path": filepath.Join(fleetPath, fleet.WorkflowPath), "enabled": true}}
	return config
}

func codexEnvironment(cwd string) []string {
	env := []string{}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if key == "PWD" || key == "OLDPWD" || key == "OPENAI_BASE_URL" || key == "OPENAI_API_BASE" || strings.HasPrefix(key, "AYCORN_") || strings.HasPrefix(key, "OPENCODE_") {
			continue
		}
		env = append(env, value)
	}
	// Preserve CODEX_HOME: local authentication and session history belong to the
	// installed Codex, so the same conversations can be resumed in Codex.
	return append(env, "PWD="+cwd)
}

func (h *Codex) Run(parent context.Context, spec RunSpec) (result RunResult, err error) {
	result.UsageJson = "{}"
	defer func() {
		if err != nil {
			result.ExitCode = 1
		}
	}()
	if spec.Request == nil || spec.Request.Engine != "codex" {
		return result, errors.New("missing or incompatible Codex request")
	}
	r := spec.Request
	if !models.IsOpenAIModel(r.Model) {
		return result, errors.New("Codex requires an OpenAI model")
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(r.TimeoutSeconds)*time.Second)
	defer cancel()
	state := &sessionState{spec: spec, done: make(chan struct{}), usage: "{}"}
	client, err := startRPC(ctx, r.Executable, []string{"app-server", "--listen", "stdio://"}, codexEnvironment(spec.WorkDir), spec.WorkDir, state.notify)
	if err != nil {
		return result, err
	}
	defer func() {
		client.close()
		state.mu.Lock()
		defer state.mu.Unlock()
		result.Output, result.UsageJson = state.output.String(), state.usage
		result.SessionID, result.TurnID = state.threadID, state.turnID
	}()
	if err = client.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aycorn", "title": "Aycorn", "version": "1.0"}}, nil); err != nil {
		return result, err
	}
	if err = client.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return result, err
	}
	developer, prompt := BuildContext(spec)
	fleetPath, err := h.installFleet(spec)
	if err != nil {
		return result, fmt.Errorf("install agent fleet: %w", err)
	}
	config := h.agentConfig(spec, fleetPath)
	sandbox := "read-only"
	if r.Intent == "implement" && r.RepoPath != "" {
		sandbox = "workspace-write"
	}
	var opened struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	method := "thread/start"
	threadParams := map[string]any{"cwd": spec.WorkDir, "model": r.Model, "modelProvider": "openai", "approvalPolicy": "never", "sandbox": sandbox, "ephemeral": false, "developerInstructions": developer, "config": config}
	sessionID := ""
	if r.Chat != nil {
		sessionID = r.Chat.SessionID
	}
	if r.ProjectChat != nil {
		sessionID = r.ProjectChat.SessionID
	}
	if sessionID != "" {
		method = "thread/resume"
		delete(threadParams, "ephemeral")
		threadParams["threadId"] = sessionID
		threadParams["excludeTurns"] = true
	}
	if err = client.call(ctx, method, threadParams, &opened); err != nil {
		return result, err
	}
	if opened.Thread.ID == "" {
		return result, errors.New("Codex did not return a thread ID")
	}
	state.mu.Lock()
	state.threadID = opened.Thread.ID
	state.mu.Unlock()
	if spec.OnSession != nil {
		if err = spec.OnSession(opened.Thread.ID, ""); err != nil {
			return result, err
		}
	}
	threadName := fmt.Sprintf("Aycorn #%d · %s", spec.TaskID, r.TaskName)
	if r.ProjectChat != nil {
		threadName = "Aycorn · Chatter · " + r.TaskName
	}
	if err = client.call(ctx, "thread/name/set", map[string]any{"threadId": opened.Thread.ID, "name": threadName}, nil); err != nil {
		return result, err
	}
	params := map[string]any{"threadId": opened.Thread.ID, "input": []map[string]any{{"type": "text", "text": prompt}}}
	if r.Conductor != nil {
		var schema any
		_ = json.Unmarshal(conductorSchema(r.Conductor.Phase), &schema)
		params["outputSchema"] = schema
	}
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = client.call(ctx, "turn/start", params, &started); err != nil {
		return result, err
	}
	if started.Turn.ID == "" {
		return result, errors.New("Codex did not return a turn ID")
	}
	state.mu.Lock()
	state.turnID = started.Turn.ID
	state.mu.Unlock()
	if spec.OnSession != nil {
		if err = spec.OnSession(opened.Thread.ID, started.Turn.ID); err != nil {
			return result, err
		}
	}
	select {
	case <-state.done:
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.failure != "" {
			return result, errors.New(state.failure)
		}
		if state.output.truncated {
			return result, errors.New("Response exceeded the 2 MiB storage limit")
		}
		if strings.TrimSpace(state.output.String()) == "" {
			return result, errors.New("Codex completed without an answer")
		}
		return result, nil
	case <-ctx.Done():
		return result, ctx.Err()
	case <-client.done:
		return result, client.err
	}
}

type sessionState struct {
	mu                               sync.Mutex
	spec                             RunSpec
	threadID, turnID, usage, failure string
	output                           limitedBuffer
	done                             chan struct{}
	once                             sync.Once
	lastFlush                        time.Time
}

func (s *sessionState) notify(m rpcMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(m.ID) > 0 {
		s.fail("Codex needs user input: " + m.Method + ". Continue this conversation in Codex.")
		return
	}
	var p struct {
		ThreadID   string          `json:"threadId"`
		TurnID     string          `json:"turnId"`
		Delta      string          `json:"delta"`
		TokenUsage json.RawMessage `json:"tokenUsage"`
		WillRetry  bool            `json:"willRetry"`
		Error      struct {
			Message string `json:"message"`
		} `json:"error"`
		Item struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Phase string `json:"phase"`
		} `json:"item"`
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	if json.Unmarshal(m.Params, &p) != nil {
		return
	}
	// Child conversations have their own terminal events. Only the root can
	// finish this run; subagent output must not replace its structured handoff.
	if s.threadID == "" || p.ThreadID != s.threadID {
		return
	}
	if s.turnID != "" && p.TurnID != "" && p.TurnID != s.turnID {
		return
	}
	progress := "Thinking"
	switch m.Method {
	case "turn/started":
		s.turnID = p.Turn.ID
	case "item/agentMessage/delta":
		s.output.Write([]byte(p.Delta))
		progress = "Writing response"
	case "item/completed":
		if p.Item.Type == "agentMessage" {
			s.output = limitedBuffer{}
			s.output.Write([]byte(p.Item.Text))
		}
	case "item/started":
		switch p.Item.Type {
		case "agentMessage":
			s.output = limitedBuffer{}
		case "commandExecution":
			progress = "Running project command"
		case "fileChange":
			progress = "Editing files"
		case "mcpToolCall":
			progress = "Using Aycorn tools"
		case "collabAgentToolCall":
			progress = "Coordinating agents"
		}
	case "thread/tokenUsage/updated":
		s.usage = string(p.TokenUsage)
	case "error":
		if !p.WillRetry {
			s.fail(p.Error.Message)
		}
	case "turn/completed":
		if p.Turn.Status != "completed" {
			s.failure = "Codex turn " + p.Turn.Status
			if p.Turn.Error != nil {
				s.failure += ": " + p.Turn.Error.Message
			}
		}
		s.once.Do(func() { close(s.done) })
	}
	if s.spec.OnProgress != nil && time.Since(s.lastFlush) > 500*time.Millisecond {
		if err := s.spec.OnProgress(progress, s.output.String()); err != nil {
			s.fail("Could not persist run progress: " + err.Error())
		}
		s.lastFlush = time.Now()
	}
}

func (s *sessionState) fail(message string) { s.failure = message; s.once.Do(func() { close(s.done) }) }
