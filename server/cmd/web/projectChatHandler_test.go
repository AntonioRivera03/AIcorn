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

// Chatter is pinned to GPT-6 Sol on Codex, whatever the default model is,
// and never works in the repository.
func TestProjectChatRunsOnSolWithoutTheRepository(t *testing.T) {
	a := projectChatApp(t)
	s := a.projectChatService
	if _, err := s.DB.Exec("UPDATE project SET repoPath=? WHERE id=1", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	first, err := s.Send(context.Background(), 1, 0, projectchat.Input{Message: "First turn", Key: "first"})
	if err != nil {
		t.Fatal(err)
	}
	s.DB.Exec("UPDATE ai_settings SET model='gpt-6-astra'")
	var seen []harness.RunSpec
	s.Engine = chatEngine{func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		seen = append(seen, spec)
		if spec.Request.PresetName != "Chatter" || spec.Request.AgentModels["chatter"] != spec.Request.Model {
			t.Fatalf("inconsistent Chatter snapshot: %+v", spec.Request)
		}
		return harness.RunResult{Output: "Answered"}, nil
	}}
	if err = s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Send(context.Background(), 1, first.Conversation, projectchat.Input{Message: "Next turn", Key: "next"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("want two runs, got %d", len(seen))
	}
	for _, spec := range seen {
		if spec.Request.Model != "gpt-6-sol" {
			t.Fatalf("Chatter ran on %q", spec.Request.Model)
		}
		if spec.Request.RepoPath != "" || !strings.HasPrefix(spec.WorkDir, a.projectChatService.WorkspaceRoot) {
			t.Fatalf("Chatter got the repository: %q in %q", spec.Request.RepoPath, spec.WorkDir)
		}
	}
}

func TestProjectChatsListRenameDelete(t *testing.T) {
	a := projectChatApp(t)
	s := a.projectChatService
	ctx := context.Background()
	s.Engine = chatEngine{func(_ context.Context, spec harness.RunSpec) (harness.RunResult, error) {
		if err := spec.OnActivity([]harness.Activity{{ID: "r1", Kind: "thinking", Text: "Reading the board", Status: "completed"}}); err != nil {
			return harness.RunResult{}, err
		}
		return harness.RunResult{Output: "Done", Activity: []harness.Activity{
			{ID: "r1", Kind: "thinking", Text: "Reading the board", Status: "completed"},
			{ID: "t1", Kind: "tool", Tool: "create_task", Status: "completed"},
		}}, nil
	}}
	older, err := s.Send(ctx, 1, 0, projectchat.Input{Message: "Plan the launch\nwith details", Key: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec("UPDATE project_chat SET updatedAt='2000-01-01 00:00:00' WHERE id=?", older.Conversation)
	newer, err := s.Send(ctx, 1, 0, projectchat.Input{Message: "What is blocked?", Key: "b"})
	if err != nil || newer.Conversation == older.Conversation {
		t.Fatalf("a new chat wasn't started: %+v %v", newer, err)
	}

	r := httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("GET", "/api/project-chats/project/1", nil))
	var list []projectchat.Summary
	json.Unmarshal(r.Body.Bytes(), &list)
	if r.Code != 200 || len(list) != 2 || list[0].ID != newer.Conversation || list[0].Status != "pending" || list[1].Title != "Plan the launch" {
		t.Fatalf("%d %+v", r.Code, list)
	}

	got, err := s.Conversation(1, older.Conversation)
	if err != nil || !strings.Contains(string(got.Turns[0].Activity), "create_task") {
		t.Fatalf("activity wasn't kept: %+v %v", got, err)
	}

	r = httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("PUT", "/api/project-chats/project/1/chats/"+fmtInt(older.Conversation), strings.NewReader(`{"title":"  Launch   plan "}`)))
	if got, _ = s.Conversation(1, older.Conversation); r.Code != 204 || got.Title != "Launch plan" {
		t.Fatalf("%d %q", r.Code, got.Title)
	}

	// Deleting a working chat stops its turn and revokes its tools.
	s.DB.Exec("UPDATE project_chat_turn SET status='running' WHERE id=?", newer.ID)
	r = httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("DELETE", "/api/project-chats/project/1/chats/"+fmtInt(newer.Conversation), nil))
	if r.Code != 204 {
		t.Fatal(r.Code, r.Body.String())
	}
	if err = taskownership.CheckChat(s.DB, 1, newer.ID); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal("deleted chat kept its tools", err)
	}
	if _, err = s.Conversation(1, newer.Conversation); err == nil {
		t.Fatal("deleted chat still opens")
	}
	if list, _ = s.Chats(1); len(list) != 1 {
		t.Fatalf("deleted chat still listed: %+v", list)
	}
	r = httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("GET", "/api/project-chats/project/2/chats/"+fmtInt(older.Conversation), nil))
	if r.Code != 404 {
		t.Fatal("another project's chat opened", r.Code)
	}
}

func TestChatTitles(t *testing.T) {
	cases := map[string]string{
		"  What is blocked?  ":      "What is blocked?",
		"First line\nsecond line":   "First line",
		strings.Repeat("word ", 30): "word word word word word word word word word word word word…",
	}
	for in, want := range cases {
		if got := projectchat.TitleFrom(in); got != want {
			t.Errorf("TitleFrom(%q) = %q, want %q", in, got, want)
		}
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
	one, err := s.Send(ctx, 1, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	chat := one.Conversation
	// A retry, even one that doesn't know the new chat yet, returns the same turn.
	again, err := s.Send(ctx, 1, 0, in)
	if err != nil || again.ID != one.ID {
		t.Fatal("not idempotent", err)
	}
	if _, err = s.Send(ctx, 1, chat, projectchat.Input{Message: "Different", Key: "first"}); !errors.Is(err, projectchat.ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.Send(ctx, 1, chat, projectchat.Input{Message: "Overlap", Key: "overlap"}); !errors.Is(err, projectchat.ErrConflict) {
		t.Fatal(err)
	}
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	two, err := s.Send(ctx, 1, chat, projectchat.Input{Message: "Continue", Key: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.Conversation(1, chat)
	if err != nil || len(got.Turns) != 2 || got.Turns[1].ID != two.ID || got.Turns[0].Output != "Completed #1" || got.Turns[1].Status != "completed" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err = s.Send(ctx, 1, chat, projectchat.Input{Message: "Wrong scope", Key: "wrong", TaskIDs: []int{999}}); !errors.Is(err, projectchat.ErrInvalid) {
		t.Fatal(err)
	}
	if err = taskownership.CheckChat(s.DB, 1, two.ID); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal("completed chat retained tools", err)
	}
	r := httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("GET", "/api/project-chats/project/1/chats/"+fmtInt(chat), nil))
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
	s.Engine = chatEngine{func(context.Context, harness.RunSpec) (harness.RunResult, error) {
		return harness.RunResult{Output: "Ready"}, nil
	}}
	opened, err := s.Send(ctx, 1, 0, projectchat.Input{Message: "Open a chat", Key: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	chat := opened.Conversation
	var wg sync.WaitGroup
	outcomes := make([]error, 5)
	for i := range outcomes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, outcomes[i] = s.Send(ctx, 1, chat, projectchat.Input{Message: "One turn", Key: fmtInt(i)})
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
	got, _ := s.Conversation(1, chat)
	id := got.Turns[1].ID
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
	got, _ = s.Conversation(1, chat)
	if got.Turns[1].Status != "canceled" || got.Turns[1].Output != "Preserved partial" {
		t.Fatal(got)
	}
	pending, err := s.Send(ctx, 1, chat, projectchat.Input{Message: "Do not start", Key: "pending"})
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
	got, _ = s.Conversation(1, chat)
	if got.Turns[2].Status != "interrupted" {
		t.Fatal(got)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "session") {
		t.Fatal("lost checkpoint")
	}
}
