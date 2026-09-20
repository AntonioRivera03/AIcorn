package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/environments"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/worker"
)

func chatTestApp(t *testing.T, root string) *app {
	t.Helper()
	a := aiTestApp(t, root)
	for _, q := range []string{
		`UPDATE task SET type=(SELECT id FROM task_type WHERE viewMode='chat'),body='[{"type":"p","children":[{"text":"Preserved document"}]}]' WHERE id=1`,
		`UPDATE ai_settings SET model='gpt-5.6-sol'`,
	} {
		if _, err := a.aiService.Jobs.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func chatRequest(t *testing.T, a *app, body string, status int) models.AgentJob {
	t.Helper()
	r := httptest.NewRecorder()
	a.routes().ServeHTTP(r, httptest.NewRequest("POST", "/api/ai/tasks/1/chat", strings.NewReader(body)))
	if r.Code != status {
		t.Fatalf("chat returned %d, want %d: %s", r.Code, status, r.Body.String())
	}
	var job models.AgentJob
	if status == 201 {
		if err := json.Unmarshal(r.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
	}
	return job
}

func TestChatRequestBoundariesAndIdempotency(t *testing.T) {
	a := chatTestApp(t, "")
	for _, body := range []string{
		`{"key":"x","message":"Hi","mode":"ask","sessionId":"another-ticket"}`,
		`{"key":"x","message":"Hi","mode":"ask","chat":{"workspace":"/tmp"}}`,
		`{"key":"x","message":"Hi","mode":"ask"} {}`,
		`{"key":"x","message":" ","mode":"ask"}`,
		`{"message":"Hi","mode":"ask"}`,
		`{"key":"x","message":"Hi","mode":"automatic"}`,
	} {
		chatRequest(t, a, body, 400)
	}
	const body = `{"key":"same-request","message":"Explain this","mode":"ask","presetId":1}`
	var wg sync.WaitGroup
	ids := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); ids <- chatRequest(t, a, body, 201).ID }()
	}
	wg.Wait()
	close(ids)
	var id int
	for got := range ids {
		if id != 0 && got != id {
			t.Fatal("retry created another turn")
		}
		id = got
	}
	chatRequest(t, a, `{"key":"overlap","message":"Next","mode":"ask"}`, 409)
	if _, err := a.aiService.Cancel(id); err != nil {
		t.Fatal(err)
	}
	job := chatRequest(t, a, `{"key":"next","message":"Next","mode":"ask"}`, 201)
	if job.Request.Chat.PreviousJob != id || job.Request.Chat.SessionID != "" || job.Request.Conductor != nil || strings.Contains(job.Request.TaskBody, "Preserved document") {
		t.Fatalf("bad chat snapshot: %+v", job.Request)
	}
	if _, err := a.aiService.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.aiService.Jobs.DB.Exec("UPDATE task SET type=1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	chatRequest(t, a, `{"key":"document","message":"Next","mode":"ask"}`, 400)
}

type chatHarness func(context.Context, harness.RunSpec) (harness.RunResult, error)

func (h chatHarness) Run(ctx context.Context, spec harness.RunSpec) (harness.RunResult, error) {
	return h(ctx, spec)
}

func TestChatTurnsReuseWorkspaceAndPreserveTicket(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if raw, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, raw)
		}
	}
	a := chatTestApp(t, root)
	before, err := a.aiService.Tasks.FindOneWithProject(1)
	if err != nil {
		t.Fatal(err)
	}
	first := chatRequest(t, a, `{"key":"one","message":"Create first file","mode":"edit"}`, 201)
	run := func(job models.AgentJob, fn chatHarness) models.AIRunArtifacts {
		t.Helper()
		w := worker.New(a.agentJobService, fn)
		if _, err := w.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		runs, err := a.agentJobService.RunRepo.ListByJob(job.ID)
		if err != nil || len(runs) != 1 {
			t.Fatalf("runs: %+v %v", runs, err)
		}
		return runs[0].Artifacts
	}
	firstArtifacts := run(first, func(_ context.Context, s harness.RunSpec) (harness.RunResult, error) {
		if s.Request.Chat.SessionID != "" || s.Request.Intent != "implement" {
			t.Fatal("first turn unexpectedly resumed")
		}
		if err := s.OnSession("native-chat-1", "turn-1"); err != nil {
			return harness.RunResult{}, err
		}
		err := os.WriteFile(filepath.Join(s.WorkDir, "first.txt"), []byte("first turn\n"), 0600)
		return harness.RunResult{Output: "Created first file"}, err
	})
	previews := &environments.Service{Store: &environments.Store{DB: a.aiService.Jobs.DB}}
	settings := environments.Defaults()
	settings.Context, settings.KindCluster, settings.Profile = "kind-test", "test", "custom"
	rawSettings, _ := json.Marshal(settings)
	if _, err := previews.Store.DB.Exec("INSERT INTO environment_settings(project,settings) VALUES(1,?)", string(rawSettings)); err != nil {
		t.Fatal(err)
	}
	preview, err := previews.Create(context.Background(), 1, environments.CreateInput{TaskID: 1, JobID: first.ID, RequestKey: "first-preview"})
	if err != nil || preview.Branch != firstArtifacts.Branch || !preview.IncludeChanges {
		t.Fatalf("chat cannot use task previews: %+v %v", preview, err)
	}
	second := chatRequest(t, a, `{"key":"two","message":"Create second file","mode":"edit"}`, 201)
	if second.Request.Chat.SessionID != "native-chat-1" || second.Request.Chat.Workspace != firstArtifacts.Workspace {
		t.Fatal("did not use stored session", second.Request.Chat)
	}
	// The old completed turn cannot be merged while the follow-up is queued.
	if _, err := a.aiService.PreviewTaskMerge(context.Background(), 1, first.ID, "main"); err == nil {
		t.Fatal("merge accepted during pending chat")
	}
	if _, err := previews.Create(context.Background(), 1, environments.CreateInput{TaskID: 1, JobID: first.ID, RequestKey: "active-preview"}); !errors.Is(err, environments.ErrConflict) {
		t.Fatalf("captured active chat: %v", err)
	}
	secondArtifacts := run(second, func(_ context.Context, s harness.RunSpec) (harness.RunResult, error) {
		if s.WorkDir != firstArtifacts.Workspace {
			t.Fatal("new checkout replaced conversation")
		}
		if raw, err := os.ReadFile(filepath.Join(s.WorkDir, "first.txt")); err != nil || string(raw) != "first turn\n" {
			t.Fatal("earlier edit lost")
		}
		if err := s.OnSession("native-chat-1", "turn-2"); err != nil {
			return harness.RunResult{}, err
		}
		if err := os.WriteFile(filepath.Join(s.WorkDir, "second.txt"), []byte("second turn\n"), 0600); err != nil {
			return harness.RunResult{}, err
		}
		return harness.RunResult{Output: "Partial second turn"}, errors.New("provider disconnected")
	})
	if secondArtifacts.Branch != firstArtifacts.Branch || len(secondArtifacts.Files) != 2 || len(secondArtifacts.TurnFiles) != 1 || secondArtifacts.TurnFiles[0] != "second.txt" || strings.Contains(secondArtifacts.TurnDiff, "first.txt") {
		t.Fatalf("bad per-turn artifact: %+v", secondArtifacts)
	}
	branches, err := a.aiService.TaskBranches(context.Background(), 1)
	if err != nil || len(branches) != 1 || branches[0].JobID != first.ID || branches[0].Status != "failed" {
		t.Fatalf("duplicate branch: %+v %v", branches, err)
	}
	retained, err := previews.Store.List(1, 1, false)
	if err != nil || len(retained) != 1 || retained[0].JobID != branches[0].JobID {
		t.Fatalf("follow-up hid the earlier preview: %+v %v", retained, err)
	}
	third := chatRequest(t, a, `{"key":"three","message":"Explain the files","mode":"ask","useRepository":true}`, 201)
	if third.Request.Chat.SessionID != "native-chat-1" {
		t.Fatal("failure lost resumability")
	}
	run(third, func(_ context.Context, s harness.RunSpec) (harness.RunResult, error) {
		if s.Request.Intent != "ask" {
			t.Fatal("ask changed edit permission")
		}
		return harness.RunResult{Output: "Explained"}, s.OnSession("native-chat-1", "turn-3")
	})
	// A cancelled queued turn has no artifacts; the subsequent one still resumes.
	canceled := chatRequest(t, a, `{"key":"cancel","message":"Never run","mode":"ask","useRepository":true}`, 201)
	if _, err = a.aiService.Cancel(canceled.ID); err != nil {
		t.Fatal(err)
	}
	continued := chatRequest(t, a, `{"key":"continue","message":"Continue","mode":"ask","useRepository":true}`, 201)
	if continued.Request.Chat.SessionID != "native-chat-1" || continued.Request.Chat.Workspace != firstArtifacts.Workspace {
		t.Fatal("queued cancellation lost conversation")
	}
	if _, err = a.aiService.Cancel(continued.ID); err != nil {
		t.Fatal(err)
	}
	chatRequest(t, a, `{"key":"changed-context","message":"Continue","mode":"ask"}`, 400)
	fresh := chatRequest(t, a, `{"key":"fresh","message":"New start","mode":"edit","newConversation":true}`, 201)
	freshArtifacts := run(fresh, func(_ context.Context, s harness.RunSpec) (harness.RunResult, error) {
		if s.WorkDir == firstArtifacts.Workspace || s.Request.Chat.SessionID != "" {
			t.Fatal("new conversation reused old state")
		}
		return harness.RunResult{Output: "Fresh conversation"}, s.OnSession("native-chat-2", "turn-1")
	})
	if freshArtifacts.Branch == firstArtifacts.Branch {
		t.Fatal("new conversation reused branch")
	}
	after, err := a.aiService.Tasks.FindOneWithProject(1)
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != before.Name || after.Body != before.Body || after.Stage != before.Stage || after.Assignee != before.Assignee || after.Type != before.Type {
		t.Fatalf("chat changed ticket: before %+v after %+v", before, after)
	}
	// Even a stale prepare that finished after another turn may not append.
	stale := *fresh.Request
	stale.Chat = &models.ChatTurn{ClientKey: "stale", PreviousJob: first.ID}
	if _, err = a.aiService.Jobs.EnqueueChat(1, stale); !errors.Is(err, repos.ErrChatConflict) {
		t.Fatalf("stale cursor accepted: %v", err)
	}
}

func TestChatDoesNotTakeOverConductorTicket(t *testing.T) {
	a := chatTestApp(t, "")
	if _, err := a.aiService.Jobs.DB.Exec(`INSERT INTO conductor_task(task,project,state,expectedStage) VALUES(1,1,'waiting',2)`); err != nil {
		t.Fatal(err)
	}
	_, err := a.aiService.StartChat(context.Background(), 1, services.ChatInput{Key: "one", Message: "Hi", Mode: "ask"})
	if !errors.Is(err, repos.ErrActiveAIRun) {
		t.Fatalf("chat took over conductor task: %v", err)
	}
}
