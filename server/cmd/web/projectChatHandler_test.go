package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/projectchat"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type chatEngine struct {
	run func(context.Context, harness.RunSpec) (harness.RunResult, error)
}

func (e chatEngine) Run(c context.Context, s harness.RunSpec) (harness.RunResult, error) {
	return e.run(c, s)
}
func projectChatApp(t *testing.T) *app {
	a := aiTestApp(t, "")
	a.projectRepo = a.aiService.Projects
	a.projectRepo.DB.Exec("UPDATE ai_settings SET model='gpt-5.6-sol'")
	a.projectChatService = &projectchat.Service{Store: projectchat.Store{DB: a.projectRepo.DB}, AI: a.aiService, WorkspaceRoot: filepath.Join(t.TempDir(), "chat")}
	return a
}

func TestProjectChatUsesFrozenChatterModel(t *testing.T) {
	a := projectChatApp(t)
	s := a.projectChatService
	setModel := func(model string) {
		t.Helper()
		if _, err := s.DB.Exec("UPDATE persona SET model=? WHERE builtin_role='chatter'", model); err != nil {
			t.Fatal(err)
		}
	}
	setModel("gpt-5.5")
	if _, err := s.Send(context.Background(), 1, projectchat.Input{Message: "First turn", Key: "frozen-model"}); err != nil {
		t.Fatal(err)
	}
	setModel("gpt-6-astra")
	var seen []string
	s.Engine = chatEngine{func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		seen = append(seen, spec.Request.Model)
		if spec.Request.PresetName != "Chatter" || spec.Request.AgentModels["chatter"] != spec.Request.Model {
			t.Fatalf("inconsistent Chatter snapshot: %+v", spec.Request)
		}
		return harness.RunResult{Output: "Answered"}, nil
	}}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), 1, projectchat.Input{Message: "Next turn", Key: "new-model"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "gpt-5.5" || seen[1] != "gpt-6-astra" {
		t.Fatalf("expected queued and future model preferences, got %v", seen)
	}
}

func TestProjectChatPersistentResumeIdempotencyAndScope(t *testing.T) {
	a := projectChatApp(t)
	s := a.projectChatService
	ctx := context.Background()
	calls := 0
	s.Engine = chatEngine{func(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		calls++
		c := spec.Request.ProjectChat
		if c == nil || c.TurnID <= 0 || spec.TaskID != 0 {
			t.Fatalf("bad chat spec: %+v", spec)
		}
		if calls == 2 && c.SessionID != "native-project-session" {
			t.Fatal("lost native conversation")
		}
		if err := spec.OnSession("native-project-session", "turn-"+fmtInt(calls)); err != nil {
			return harness.RunResult{}, err
		}
		if err := spec.OnProgress("Working", "Partial"); err != nil {
			return harness.RunResult{}, err
		}
		return harness.RunResult{Output: "Completed #1", UsageJson: `{"total":{"totalTokens":12}}`}, nil
	}}
	in := projectchat.Input{Message: "Read #1", Key: "first", TaskIDs: []int{1}}
	one, err := s.Send(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Send(ctx, 1, in)
	if err != nil || again.ID != one.ID {
		t.Fatal("not idempotent", err)
	}
	if _, err = s.Send(ctx, 1, projectchat.Input{Message: "Different", Key: "first"}); !errors.Is(err, projectchat.ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.Send(ctx, 1, projectchat.Input{Message: "Overlap", Key: "overlap"}); !errors.Is(err, projectchat.ErrConflict) {
		t.Fatal(err)
	}
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	two, err := s.Send(ctx, 1, projectchat.Input{Message: "Continue", Key: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.Conversation(1)
	if err != nil || len(got.Turns) != 2 || got.Turns[1].ID != two.ID || got.Turns[0].Output != "Completed #1" || got.Turns[1].Status != "completed" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err = s.Send(ctx, 1, projectchat.Input{Message: "Wrong scope", Key: "wrong", TaskIDs: []int{999}}); !errors.Is(err, projectchat.ErrInvalid) {
		t.Fatal(err)
	}
	if err = taskownership.CheckChat(s.DB, 1, two.ID); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal("completed chat retained tools", err)
	}
	r := httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("GET", "/api/project-chats/project/1", nil))
	if r.Code != 200 || strings.Contains(r.Body.String(), "requestJson") || strings.Contains(r.Body.String(), "executable") {
		t.Fatal(r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("POST", "/api/project-chats/project/1/messages", strings.NewReader(`{"message":"forged","key":"bad","sessionId":"foreign"}`)))
	if r.Code != 400 {
		t.Fatal(r.Code)
	}
}
func TestProjectChatConcurrencyCancelAndRestart(t *testing.T) {
	a := projectChatApp(t)
	s := a.projectChatService
	ctx := context.Background()
	var wg sync.WaitGroup
	outcomes := make([]error, 5)
	for i := range outcomes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, outcomes[i] = s.Send(ctx, 1, projectchat.Input{Message: "One turn", Key: fmtInt(i)})
		}(i)
	}
	wg.Wait()
	success := 0
	for _, err := range outcomes {
		if err == nil {
			success++
		} else if !errors.Is(err, projectchat.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	got, _ := s.Conversation(1)
	id := got.Turns[0].ID
	started := make(chan struct{})
	s.Engine = chatEngine{func(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		spec.OnSession("session", "turn")
		spec.OnProgress("Working", "Preserved partial")
		close(started)
		<-ctx.Done()
		return harness.RunResult{Output: "Preserved partial"}, ctx.Err()
	}}
	done := make(chan error, 1)
	go func() { done <- s.Tick(ctx) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never started")
	}
	if err := s.Cancel(2, id); !errors.Is(err, projectchat.ErrConflict) {
		t.Fatal("cross-project cancel", err)
	}
	if err := s.Cancel(1, id); err != nil {
		t.Fatal(err)
	}
	if err := taskownership.CheckChat(s.DB, 1, id); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal("canceled chat retained tools", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation never reached harness")
	}
	got, _ = s.Conversation(1)
	if got.Turns[0].Status != "canceled" || got.Turns[0].Output != "Preserved partial" {
		t.Fatal(got)
	}
	pending, err := s.Send(ctx, 1, projectchat.Input{Message: "Do not start", Key: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(1, pending.ID); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec("UPDATE project_chat_turn SET status='running' WHERE id=?", pending.ID)
	if err = s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Conversation(1)
	if got.Turns[1].Status != "interrupted" {
		t.Fatal(got)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "session") {
		t.Fatal("lost checkpoint")
	}
}
