// Package projectchat owns durable project conversations and their independent queue.
package projectchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/knowledge"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
)

var ErrConflict = errors.New("a project chat turn is already active or this message key was reused; refresh and try again")
var ErrInvalid = errors.New("invalid project chat request")

type Store struct{ DB *sql.DB }
type Turn struct {
	ID           int             `json:"id"`
	Conversation int             `json:"conversationId"`
	Message      string          `json:"message"`
	Status       string          `json:"status"`
	Output       string          `json:"output"`
	Progress     string          `json:"progress"`
	Error        string          `json:"error"`
	SessionID    string          `json:"sessionId,omitempty"`
	TurnID       string          `json:"turnId,omitempty"`
	CreatedAt    string          `json:"createdAt"`
	FinishedAt   string          `json:"finishedAt,omitempty"`
	Usage        json.RawMessage `json:"usage"`
	// Activity is the turn's thinking and tool calls (harness.Activity).
	Activity json.RawMessage     `json:"activity"`
	Request  models.AIRunRequest `json:"-"`
}
type Conversation struct {
	ID        int    `json:"id"`
	ProjectID int    `json:"projectId"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	Turns     []Turn `json:"turns"`
}

// Summary is a chat as the chat list shows it.
type Summary struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	// Status is the latest turn's, so the list can show a chat that's working.
	Status string `json:"status"`
}

const maxTitleLength = 120

type Input struct {
	Message string `json:"message"`
	Key     string `json:"key"`
	TaskIDs []int  `json:"taskIds,omitempty"`
}

// turnColumns lists a turn's columns for scan, each qualified with prefix
// (e.g. "t.") for queries that join.
func turnColumns(prefix string) string {
	names := []string{"id", "conversation", "message", "status", "output", "progress", "error", "sessionId", "turnId", "createdAt", "finishedAt", "usageJson", "activityJson", "requestJson"}
	for i, name := range names {
		names[i] = prefix + name
		if name == "finishedAt" {
			names[i] = "COALESCE(" + prefix + name + ",'')"
		}
	}
	return strings.Join(names, ",")
}

var columns = turnColumns("")

func scan(row interface{ Scan(...any) error }) (Turn, error) {
	var t Turn
	var usage, activity, request string
	err := row.Scan(&t.ID, &t.Conversation, &t.Message, &t.Status, &t.Output, &t.Progress, &t.Error, &t.SessionID, &t.TurnID, &t.CreatedAt, &t.FinishedAt, &usage, &activity, &request)
	t.Usage = json.RawMessage(usage)
	t.Activity = json.RawMessage(activity)
	if err == nil {
		err = json.Unmarshal([]byte(request), &t.Request)
	}
	return t, err
}

// Chats lists a project's chats, most recently used first.
func (s Store) Chats(project int) ([]Summary, error) {
	var exists int
	if err := s.DB.QueryRow("SELECT id FROM project WHERE id=?", project).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT c.id,c.title,c.updatedAt,COALESCE((SELECT status FROM project_chat_turn WHERE conversation=c.id ORDER BY id DESC LIMIT 1),'')
FROM project_chat c WHERE c.project=? AND c.archivedAt IS NULL ORDER BY c.updatedAt DESC,c.id DESC`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chats := []Summary{}
	for rows.Next() {
		var c Summary
		if err = rows.Scan(&c.ID, &c.Title, &c.UpdatedAt, &c.Status); err != nil {
			return nil, err
		}
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// Conversation is one chat with all of its turns.
func (s Store) Conversation(project, chat int) (Conversation, error) {
	c := Conversation{ProjectID: project, Turns: []Turn{}}
	if err := s.DB.QueryRow("SELECT id,title,updatedAt FROM project_chat WHERE id=? AND project=? AND archivedAt IS NULL", chat, project).Scan(&c.ID, &c.Title, &c.UpdatedAt); err != nil {
		return c, err
	}
	rows, err := s.DB.Query("SELECT "+columns+" FROM project_chat_turn WHERE conversation=? ORDER BY id", c.ID)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return c, err
		}
		c.Turns = append(c.Turns, t)
	}
	return c, rows.Err()
}

// Rename sets a chat's title; a blank one falls back to its first message.
func (s Store) Rename(project, chat int, title string) error {
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > maxTitleLength {
		return fmt.Errorf("%w: titles are at most %d characters", ErrInvalid, maxTitleLength)
	}
	if title == "" {
		var first string
		if err := s.DB.QueryRow("SELECT message FROM project_chat_turn WHERE conversation=? ORDER BY id LIMIT 1", chat).Scan(&first); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		title = TitleFrom(first)
	}
	res, err := s.DB.Exec("UPDATE project_chat SET title=? WHERE id=? AND project=? AND archivedAt IS NULL", title, chat, project)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes a chat from the list and stops its turn if one is working.
// The rows stay, archived, because tasks record which chat turn changed them.
func (s Store) Delete(project, chat int) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE project_chat SET archivedAt=CURRENT_TIMESTAMP WHERE id=? AND project=? AND archivedAt IS NULL", chat, project)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err = tx.Exec(`UPDATE project_chat_turn SET status=CASE WHEN status='pending' THEN 'canceled' ELSE 'canceling' END,finishedAt=CASE WHEN status='pending' THEN CURRENT_TIMESTAMP ELSE NULL END WHERE conversation=? AND status IN ('pending','running')`, chat); err != nil {
		return err
	}
	return tx.Commit()
}

// TitleFrom makes a chat title from its first message: the start of its
// first line, cut at a word.
func TitleFrom(message string) string {
	line := strings.TrimSpace(message)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.Join(strings.Fields(line), " ")
	const max = 60
	if len([]rune(line)) <= max {
		return line
	}
	cut := string([]rune(line)[:max])
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

// referencedTaskExcerptLength bounds each referenced task's body excerpt in
// the turn payload, so a handful of #mentions can't balloon the request.
const referencedTaskExcerptLength = 300

// referencedTaskContext resolves the message's #id references inside the
// same transaction that will enqueue the turn, rejecting an id outside the
// project before anything is queued. Bodies come back as raw Plate.js JSON;
// the caller converts them to markdown in one batched call.
func referencedTaskContext(tx *sql.Tx, project int, ids []int) ([]models.ReferencedTask, []string, error) {
	referenced := make([]models.ReferencedTask, 0, len(ids))
	bodies := make([]string, 0, len(ids))
	for _, id := range ids {
		var rt models.ReferencedTask
		var body string
		err := tx.QueryRow(`SELECT t.id,t.name,s.name,tt.name,t.body FROM task t
JOIN stage s ON s.id=t.stage
JOIN task_type tt ON tt.id=t.type
JOIN checklist c ON c.id=t.checklist
WHERE t.id=? AND c.project=?`, id, project).Scan(&rt.ID, &rt.Title, &rt.Stage, &rt.Type, &body)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("%w: task #%d is outside this project", ErrInvalid, id)
		}
		if err != nil {
			return nil, nil, err
		}
		owner, err := taskownership.Current(tx, id)
		if err != nil {
			return nil, nil, err
		}
		if owner != nil {
			rt.Owner, rt.State = owner.Name, owner.State
		}
		referenced = append(referenced, rt)
		bodies = append(bodies, body)
	}
	return referenced, bodies, nil
}

// excerpt shortens converted markdown to a bound for the AI payload, cutting
// at a word boundary like TitleFrom does for chat titles.
func excerpt(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	cut := string(runes[:max])
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

func (s Store) Cancel(project, id int) error {
	res, err := s.DB.Exec(`UPDATE project_chat_turn SET status=CASE WHEN status='pending' THEN 'canceled' ELSE 'canceling' END,finishedAt=CASE WHEN status='pending' THEN CURRENT_TIMESTAMP ELSE NULL END WHERE id=? AND status IN ('pending','running') AND conversation IN (SELECT id FROM project_chat WHERE project=? AND archivedAt IS NULL)`, id, project)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConflict
	}
	return nil
}
func (s Store) Context(project int) (map[string]any, error) {
	p, err := (&repos.ProjectRepo{DB: s.DB}).FindOne(project)
	if err != nil {
		return nil, err
	}
	stages, err := (&repos.StageRepo{DB: s.DB}).ByWorkflowForProject(p.Workflow, project)
	if err != nil {
		return nil, err
	}
	checklists, err := (&repos.ChecklistRepo{DB: s.DB}).AllByProject(project)
	if err != nil {
		return nil, err
	}
	settings, _, err := (&repos.ConductorRepo{DB: s.DB}).Settings(project)
	if err != nil {
		return nil, err
	}
	owners, err := taskownership.ListProject(s.DB, project)
	if err != nil {
		return nil, err
	}
	docs, err := (&knowledge.Store{DB: s.DB}).Documents(project)
	if err != nil {
		return nil, err
	}
	for i := range docs {
		docs[i].Body = nil
	}
	rows, err := s.DB.Query("SELECT tt.id,tt.name FROM task_type tt JOIN project_task_type pt ON pt.task_type=tt.id WHERE pt.project=? ORDER BY tt.id", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	types := []map[string]any{}
	for rows.Next() {
		var id int
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		types = append(types, map[string]any{"id": id, "name": name})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"project": p, "stages": stages, "checklists": checklists, "taskTypes": types, "conductor": settings, "owners": owners, "documents": docs}, nil
}

type Service struct {
	Store
	AI            *services.AIService
	Engine        harness.Harness
	WorkspaceRoot string
}

// previousTurn finds a turn already sent with this client key in the
// project, so a retried send returns it instead of sending twice. Keys are
// random per message, so they're unique across the project's chats too.
func previousTurn(q interface {
	QueryRow(string, ...any) *sql.Row
}, project int, in Input) (Turn, bool, error) {
	t, err := scan(q.QueryRow("SELECT "+turnColumns("t.")+" FROM project_chat_turn t JOIN project_chat c ON c.id=t.conversation WHERE c.project=? AND t.clientKey=?", project, in.Key))
	if errors.Is(err, sql.ErrNoRows) {
		return Turn{}, false, nil
	}
	if err != nil {
		return Turn{}, false, err
	}
	if t.Message != in.Message {
		return Turn{}, false, ErrConflict
	}
	return t, true, nil
}

// Send queues a message to Chatter. chat 0 starts a new chat, titled from the
// message; the returned turn names the chat either way.
func (s *Service) Send(ctx context.Context, project, chat int, in Input) (Turn, error) {
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" || len(in.Message) > 32000 || in.Key == "" || len(in.Key) > 128 || len(in.TaskIDs) > 50 || chat < 0 {
		return Turn{}, ErrInvalid
	}
	// Idempotent retries work even when the provider later becomes unavailable.
	if previous, found, err := previousTurn(s.DB, project, in); err != nil || found {
		return previous, err
	}
	if chat > 0 {
		if _, err := s.Conversation(project, chat); err != nil {
			return Turn{}, err
		}
	}
	p, err := s.AI.Projects.FindOne(project)
	if err != nil {
		return Turn{}, err
	}
	snapshot := &models.TaskWithProject{ProjectID: project}
	snapshot.Name = p.Name
	chatter, err := s.AI.ResolveRole(ctx, "chatter")
	if err != nil {
		return Turn{}, err
	}
	// Never the repository: Chatter works from tasks and documents only.
	req, err := s.AI.PrepareSnapshot(ctx, snapshot, services.AIRunInput{Intent: "ask", Agent: chatter, Instruction: in.Message})
	if err != nil {
		return Turn{}, err
	}
	contextData, err := s.Context(project)
	if err != nil {
		return Turn{}, err
	}
	rawContext, _ := json.Marshal(contextData)
	req.ProjectChat = &models.ProjectChatTurn{Context: json.RawMessage(rawContext), TaskIDs: in.TaskIDs}
	req.PresetName = "Chatter"
	tx, err := taskownership.Begin(s.DB)
	if err != nil {
		return Turn{}, err
	}
	defer tx.Rollback()
	// Scope is rechecked under the same lock as the durable enqueue, and this
	// also gathers what Chatter needs about each reference so it doesn't need
	// a tool round trip for the common case.
	referenced, bodies, err := referencedTaskContext(tx, project, in.TaskIDs)
	if err != nil {
		return Turn{}, err
	}
	if len(bodies) > 0 {
		converted, err := s.AI.Converter.ToMarkdown(ctx, bodies)
		if err != nil {
			return Turn{}, err
		}
		for i := range referenced {
			referenced[i].Excerpt = excerpt(converted[i], referencedTaskExcerptLength)
		}
	}
	req.ProjectChat.ReferencedTasks = referenced
	if previous, found, err := previousTurn(tx, project, in); err != nil || found {
		return previous, err
	}
	if chat == 0 {
		err = tx.QueryRow("INSERT INTO project_chat(project,title,updatedAt) VALUES(?,?,CURRENT_TIMESTAMP) RETURNING id", project, TitleFrom(in.Message)).Scan(&chat)
	} else {
		err = tx.QueryRow("SELECT sessionId FROM project_chat WHERE id=? AND project=? AND archivedAt IS NULL", chat, project).Scan(&req.ProjectChat.SessionID)
	}
	if err != nil {
		return Turn{}, err
	}
	req.ProjectChat.ConversationID = chat
	raw, _ := json.Marshal(req)
	t, err := scan(tx.QueryRow("INSERT INTO project_chat_turn(conversation,clientKey,message,requestJson) VALUES(?,?,?,?) RETURNING "+columns, chat, in.Key, in.Message, string(raw)))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Turn{}, ErrConflict
		}
		return Turn{}, err
	}
	if _, err = tx.Exec("UPDATE project_chat SET updatedAt=CURRENT_TIMESTAMP WHERE id=?", chat); err != nil {
		return Turn{}, err
	}
	return t, tx.Commit()
}
func (s *Service) Start(ctx context.Context) error {
	if _, err := s.DB.Exec("UPDATE project_chat_turn SET status='interrupted',error='Aycorn stopped during this turn. Send another message to continue.',finishedAt=CURRENT_TIMESTAMP WHERE status IN ('running','canceling')"); err != nil {
		return err
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Printf("project chat: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) Tick(parent context.Context) error {
	t, err := scan(s.DB.QueryRow("UPDATE project_chat_turn SET status='running',progress='Starting Chatter' WHERE id=(SELECT id FROM project_chat_turn WHERE status='pending' ORDER BY id LIMIT 1) RETURNING " + columns))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				var status string
				if err := s.DB.QueryRow("SELECT status FROM project_chat_turn WHERE id=?", t.ID).Scan(&status); err != nil || status != "running" {
					cancel()
					return
				}
			}
		}
	}()
	req := t.Request
	req.ProjectChat.TurnID = t.ID
	// An empty folder of its own, never the repository, even for turns
	// queued before Chatter lost repository access.
	req.RepoPath = ""
	work := filepath.Join(s.WorkspaceRoot, fmt.Sprintf("project-%d", req.ProjectID))
	if err = os.MkdirAll(work, 0700); err != nil {
		return s.finish(t, harness.RunResult{UsageJson: "{}"}, err, parent.Err() != nil)
	}
	result, runErr := s.Engine.Run(ctx, harness.RunSpec{Request: &req, WorkDir: work,
		OnProgress: func(progress, output string) error {
			res, err := s.DB.Exec("UPDATE project_chat_turn SET progress=?,output=? WHERE id=? AND status='running'", progress, output, t.ID)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n != 1 {
				return context.Canceled
			}
			return nil
		},
		OnActivity: func(activity []harness.Activity) error {
			raw, err := json.Marshal(activity)
			if err != nil {
				return err
			}
			_, err = s.DB.Exec("UPDATE project_chat_turn SET activityJson=? WHERE id=? AND status='running'", string(raw), t.ID)
			return err
		},
		OnSession: func(session, turn string) error {
			tx, err := taskownership.Begin(s.DB)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			if err = taskownership.CheckChat(tx, req.ProjectID, t.ID); err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE project_chat_turn SET sessionId=?,turnId=? WHERE id=?", session, turn, t.ID); err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE project_chat SET sessionId=? WHERE id=?", session, t.Conversation); err != nil {
				return err
			}
			return tx.Commit()
		}})
	return s.finish(t, result, runErr, parent.Err() != nil)
}
func (s *Service) finish(t Turn, r harness.RunResult, runErr error, shutdown bool) error {
	var current string
	if err := s.DB.QueryRow("SELECT status FROM project_chat_turn WHERE id=?", t.ID).Scan(&current); err != nil {
		return err
	}
	status, message := "completed", ""
	if runErr != nil {
		status = "failed"
		message = runErr.Error()
	}
	if current == "canceling" {
		status = "canceled"
		message = "Stopped by you"
	} else if shutdown {
		status = "interrupted"
		message = "Aycorn stopped during this turn"
	}
	if !json.Valid([]byte(r.UsageJson)) {
		r.UsageJson = "{}"
	}
	// Keep what was recorded while running if the harness returned nothing.
	activity := "activityJson"
	var args []any
	if len(r.Activity) > 0 {
		raw, _ := json.Marshal(r.Activity)
		activity = "?"
		args = append(args, string(raw))
	}
	args = append([]any{status, r.Output, message, r.UsageJson}, append(args, t.ID)...)
	_, err := s.DB.Exec("UPDATE project_chat_turn SET status=?,output=?,progress='',error=?,usageJson=?,activityJson="+activity+",finishedAt=CURRENT_TIMESTAMP WHERE id=? AND status IN ('running','canceling')", args...)
	return err
}
