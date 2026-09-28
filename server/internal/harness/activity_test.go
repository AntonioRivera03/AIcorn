package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionRecordsThinkingAndToolActivity(t *testing.T) {
	var flushed [][]Activity
	s := &sessionState{threadID: "th", turnID: "tu", done: make(chan struct{}), spec: RunSpec{
		OnActivity: func(a []Activity) error { flushed = append(flushed, a); return nil },
	}}
	send := func(method, params string) {
		t.Helper()
		s.notify(rpcMessage{Method: method, Params: json.RawMessage(params)})
	}
	send("item/started", `{"threadId":"th","turnId":"tu","startedAtMs":1000,"item":{"type":"reasoning","id":"r1","summary":[],"content":[]}}`)
	send("item/reasoning/summaryTextDelta", `{"threadId":"th","turnId":"tu","itemId":"r1","delta":"Checking the board","summaryIndex":0}`)
	send("item/reasoning/summaryPartAdded", `{"threadId":"th","turnId":"tu","itemId":"r1","summaryIndex":1}`)
	send("item/reasoning/summaryTextDelta", `{"threadId":"th","turnId":"tu","itemId":"r1","delta":"Creating the task","summaryIndex":1}`)
	send("item/started", `{"threadId":"th","turnId":"tu","startedAtMs":2600,"item":{"type":"mcpToolCall","id":"t1","server":"aycorn","tool":"create_task","status":"inProgress","arguments":{"name":"Export button"},"result":null,"error":null,"durationMs":null}}`)
	send("item/completed", `{"threadId":"th","turnId":"tu","completedAtMs":2500,"item":{"type":"reasoning","id":"r1","summary":["Checking the board","Creating the task"],"content":[]}}`)
	// Another thread's events (a subagent) never reach this turn's log.
	send("item/started", `{"threadId":"other","turnId":"x","startedAtMs":2700,"item":{"type":"mcpToolCall","id":"t9","tool":"update_task","status":"inProgress","arguments":{}}}`)
	send("item/completed", `{"threadId":"th","turnId":"tu","completedAtMs":2800,"item":{"type":"mcpToolCall","id":"t1","server":"aycorn","tool":"create_task","status":"completed","arguments":{"name":"Export button"},"result":{"content":[],"structuredContent":{"ID":42,"Name":"Export button"},"_meta":null},"error":null,"durationMs":180}}`)

	log := s.activity.snapshot()
	if len(log) != 2 {
		t.Fatalf("want thinking and one tool call, got %+v", log)
	}
	thinking, tool := log[0], log[1]
	if thinking.Kind != "thinking" || thinking.Text != "Checking the board\n\nCreating the task" || thinking.DurationMs != 1500 || thinking.Status != "completed" {
		t.Fatalf("thinking %+v", thinking)
	}
	if tool.Kind != "tool" || tool.Tool != "create_task" || tool.DurationMs != 180 || !strings.Contains(string(tool.Result), `"ID":42`) || !strings.Contains(string(tool.Arguments), "Export button") {
		t.Fatalf("tool %+v", tool)
	}
	if len(flushed) == 0 || len(flushed[len(flushed)-1]) != 2 {
		t.Fatalf("activity wasn't flushed as it happened: %d flushes", len(flushed))
	}
}

func TestFailedToolCallKeepsItsError(t *testing.T) {
	var l activityLog
	l.completed(codexItem{ID: "t1", Type: "mcpToolCall", Tool: "move_task_stage", Status: "failed", Error: &struct {
		Message string `json:"message"`
	}{"task moved since you read it"}}, 10)
	got := l.snapshot()
	if len(got) != 1 || got[0].Status != "failed" || got[0].Error != "task moved since you read it" {
		t.Fatalf("%+v", got)
	}
}
