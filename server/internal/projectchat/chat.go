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
	ID           int                 `json:"id"`
	Conversation int                 `json:"conversationId"`
	Message      string              `json:"message"`
	Status       string              `json:"status"`
	Output       string              `json:"output"`
	Progress     string              `json:"progress"`
	Error        string              `json:"error"`
	SessionID    string              `json:"sessionId,omitempty"`
	TurnID       string              `json:"turnId,omitempty"`
	CreatedAt    string              `json:"createdAt"`
	FinishedAt   string              `json:"finishedAt,omitempty"`
	Usage        json.RawMessage     `json:"usage"`
	Request      models.AIRunRequest `json:"-"`
}
type Conversation struct {
	ID        int    `json:"id"`
	ProjectID int    `json:"projectId"`
	Turns     []Turn `json:"turns"`
}
type Input struct {
	Message string `json:"message"`
	Key     string `json:"key"`
	TaskIDs []int  `json:"taskIds,omitempty"`
}

const columns = `id,conversation,message,status,output,progress,error,sessionId,turnId,createdAt,COALESCE(finishedAt,''),usageJson,requestJson`

func scan(row interface{ Scan(...any) error }) (Turn, error) {
	var t Turn
	var usage, request string
	err := row.Scan(&t.ID, &t.Conversation, &t.Message, &t.Status, &t.Output, &t.Progress, &t.Error, &t.SessionID, &t.TurnID, &t.CreatedAt, &t.FinishedAt, &usage, &request)
	t.Usage = json.RawMessage(usage)
	if err == nil {
		err = json.Unmarshal([]byte(request), &t.Request)
	}
	return t, err
}
func (s Store) Conversation(project int) (Conversation, error) {
	var c Conversation
	c.ProjectID = project
	c.Turns = []Turn{}
	if _, err := s.DB.Exec("INSERT INTO project_chat(project) SELECT id FROM project WHERE id=? ON CONFLICT DO NOTHING", project); err != nil {
		return c, err
	}
	if err := s.DB.QueryRow("SELECT id FROM project_chat WHERE project=? AND archivedAt IS NULL", project).Scan(&c.ID); err != nil {
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

func (s *Service) Send(ctx context.Context, project int, in Input) (Turn, error) {
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" || len(in.Message) > 32000 || in.Key == "" || len(in.Key) > 128 || len(in.TaskIDs) > 50 {
		return Turn{}, ErrInvalid
	}
	conversation, err := s.Conversation(project)
	if err != nil {
		return Turn{}, err
	}
	// Idempotent retries work even when the provider later becomes unavailable.
	previous, lookup := scan(s.DB.QueryRow("SELECT "+columns+" FROM project_chat_turn WHERE conversation=? AND clientKey=?", conversation.ID, in.Key))
	if lookup == nil {
		if previous.Message != in.Message {
			return Turn{}, ErrConflict
		}
		return previous, nil
	}
	if !errors.Is(lookup, sql.ErrNoRows) {
		return Turn{}, lookup
	}
	p, err := s.AI.Projects.FindOne(project)
	if err != nil {
		return Turn{}, err
	}
	snapshot := &models.TaskWithProject{ProjectID: project}
	snapshot.Name = p.Name
	req, err := s.AI.PrepareSnapshot(ctx, snapshot, services.AIRunInput{Intent: "ask", Instruction: in.Message, UseRepository: p.RepoPath != ""})
	if err != nil {
		return Turn{}, err
	}
	contextData, err := s.Context(project)
	if err != nil {
		return Turn{}, err
	}
	rawContext, _ := json.Marshal(contextData)
	req.ProjectChat = &models.ProjectChatTurn{ConversationID: conversation.ID, Context: json.RawMessage(rawContext), TaskIDs: in.TaskIDs}
	req.PresetName = "Chatter"
	tx, err := taskownership.Begin(s.DB)
	if err != nil {
		return Turn{}, err
	}
	defer tx.Rollback()
	// Scope is rechecked under the same lock as the durable enqueue.
	for _, id := range in.TaskIDs {
		var valid bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=? AND c.project=?)", id, project).Scan(&valid); err != nil {
			return Turn{}, err
		}
		if !valid {
			return Turn{}, fmt.Errorf("%w: task #%d is outside this project", ErrInvalid, id)
		}
	}
	if err = tx.QueryRow("SELECT sessionId FROM project_chat WHERE id=? AND project=? AND archivedAt IS NULL", conversation.ID, project).Scan(&req.ProjectChat.SessionID); err != nil {
		return Turn{}, err
	}
	previous, lookup = scan(tx.QueryRow("SELECT "+columns+" FROM project_chat_turn WHERE conversation=? AND clientKey=?", conversation.ID, in.Key))
	if lookup == nil {
		if previous.Message != in.Message {
			return Turn{}, ErrConflict
		}
		return previous, nil
	}
	if !errors.Is(lookup, sql.ErrNoRows) {
		return Turn{}, lookup
	}
	raw, _ := json.Marshal(req)
	t, err := scan(tx.QueryRow("INSERT INTO project_chat_turn(conversation,clientKey,message,requestJson) VALUES(?,?,?,?) RETURNING "+columns, conversation.ID, in.Key, in.Message, string(raw)))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Turn{}, ErrConflict
		}
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
	work := req.RepoPath
	if work == "" {
		work = filepath.Join(s.WorkspaceRoot, fmt.Sprintf("project-%d", req.ProjectID))
		if err = os.MkdirAll(work, 0700); err != nil {
			return s.finish(t, harness.RunResult{UsageJson: "{}"}, err, parent.Err() != nil)
		}
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
	_, err := s.DB.Exec("UPDATE project_chat_turn SET status=?,output=?,progress='',error=?,usageJson=?,finishedAt=CURRENT_TIMESTAMP WHERE id=? AND status IN ('running','canceling')", status, r.Output, message, r.UsageJson, t.ID)
	return err
}
