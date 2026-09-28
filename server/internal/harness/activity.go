package harness

import (
	"encoding/json"
	"strings"
)

// Activity is one step of an agent's turn, for showing its work: a thinking
// summary, or a tool call with what it was given and what it returned.
type Activity struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // "thinking" or "tool"
	// Tool is the tool's name, e.g. "create_task", or "shell" and "web_search"
	// for the harness's own tools.
	Tool       string          `json:"tool,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Text       string          `json:"text,omitempty"`
	Status     string          `json:"status"` // "inProgress", "completed", or "failed"
	Error      string          `json:"error,omitempty"`
	StartedAt  int64           `json:"startedAt,omitempty"` // Unix milliseconds
	DurationMs int64           `json:"durationMs,omitempty"`
}

const (
	maxActivities     = 300
	maxActivityText   = 20000
	maxActivityResult = 16000
)

// activityLog collects a turn's activity from Codex item events, keyed by
// item ID so deltas and completions land on the right entry.
type activityLog struct {
	items   []Activity
	index   map[string]int
	changed bool
}

// codexItem is the part of a Codex ThreadItem the activity log reads.
type codexItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Summary   []string        `json:"summary"`
	Tool      string          `json:"tool"`
	Status    string          `json:"status"`
	Arguments json.RawMessage `json:"arguments"`
	Command   string          `json:"command"`
	Query     string          `json:"query"`
	Result    *struct {
		Content           []json.RawMessage `json:"content"`
		StructuredContent json.RawMessage   `json:"structuredContent"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	DurationMs *int64 `json:"durationMs"`
}

func (l *activityLog) find(id string) *Activity {
	if i, ok := l.index[id]; ok {
		return &l.items[i]
	}
	return nil
}

func (l *activityLog) started(item codexItem, at int64) {
	if item.ID == "" || l.find(item.ID) != nil || len(l.items) >= maxActivities {
		return
	}
	a := Activity{ID: item.ID, Status: "inProgress", StartedAt: at}
	switch item.Type {
	case "reasoning":
		a.Kind = "thinking"
	case "mcpToolCall":
		a.Kind, a.Tool, a.Arguments = "tool", item.Tool, capJSON(item.Arguments)
	case "commandExecution":
		a.Kind, a.Tool, a.Text = "tool", "shell", item.Command
	case "webSearch":
		a.Kind, a.Tool, a.Text = "tool", "web_search", item.Query
	case "fileChange":
		a.Kind, a.Tool = "tool", "file_change"
	default:
		return
	}
	if l.index == nil {
		l.index = map[string]int{}
	}
	l.index[item.ID] = len(l.items)
	l.items = append(l.items, a)
	l.changed = true
}

func (l *activityLog) thinkingDelta(id, delta string, newPart bool) {
	a := l.find(id)
	if a == nil || len(a.Text) > maxActivityText {
		return
	}
	if newPart && a.Text != "" {
		a.Text += "\n\n"
	}
	a.Text += delta
	l.changed = true
}

func (l *activityLog) completed(item codexItem, at int64) {
	l.started(item, at)
	a := l.find(item.ID)
	if a == nil {
		return
	}
	a.Status = "completed"
	if item.Status == "failed" || item.Error != nil {
		a.Status = "failed"
	}
	if item.Error != nil {
		a.Error = item.Error.Message
	}
	if item.DurationMs != nil {
		a.DurationMs = *item.DurationMs
	} else if a.StartedAt > 0 && at > a.StartedAt {
		a.DurationMs = at - a.StartedAt
	}
	switch item.Type {
	case "reasoning":
		if summary := strings.TrimSpace(strings.Join(item.Summary, "\n\n")); summary != "" {
			a.Text = truncate(summary, maxActivityText)
		}
	case "mcpToolCall":
		if len(item.Arguments) > 0 {
			a.Arguments = capJSON(item.Arguments)
		}
		if item.Result != nil {
			if len(item.Result.StructuredContent) > 0 && string(item.Result.StructuredContent) != "null" {
				a.Result = capJSON(item.Result.StructuredContent)
			} else if raw, err := json.Marshal(item.Result.Content); err == nil {
				a.Result = capJSON(raw)
			}
		}
	}
	l.changed = true
}

// snapshot copies the log and clears the changed flag.
func (l *activityLog) snapshot() []Activity {
	l.changed = false
	return append([]Activity(nil), l.items...)
}

// capJSON drops a value too large to keep; the timeline then shows the step
// without its details.
func capJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(raw) > maxActivityResult || !json.Valid(raw) {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "") + "…"
}
