package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"strings"
)

var ErrChatConflict = errors.New("the conversation changed; refresh before sending again")
var ErrChatType = errors.New("choose the Chat task type to send a chat message")

func (r *AgentJobRepo) ChatRequest(task int, key string) (*models.AgentJob, error) {
	var j models.AgentJob
	err := scanAgentJob(r.DB.QueryRow("SELECT "+agentJobColumns+" FROM agent_job WHERE task=? AND json_extract(requestJson,'$.chat.clientKey')=?", task, key), &j)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}
func (r *AgentJobRepo) LatestChat(task int) (*models.AgentJob, models.AIRunArtifacts, error) {
	var j models.AgentJob
	var artifacts models.AIRunArtifacts
	err := scanAgentJob(r.DB.QueryRow("SELECT "+agentJobColumns+" FROM agent_job WHERE task=? AND json_type(requestJson,'$.chat')='object' ORDER BY id DESC LIMIT 1", task), &j)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, artifacts, nil
	}
	if err != nil {
		return nil, artifacts, err
	}
	var raw string
	err = r.DB.QueryRow("SELECT artifactJson FROM agent_run WHERE job=? ORDER BY id DESC LIMIT 1", j.ID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &j, artifacts, nil
	}
	if err != nil {
		return nil, artifacts, err
	}
	err = json.Unmarshal([]byte(raw), &artifacts)
	return &j, artifacts, err
}
func (r *AgentJobRepo) EnqueueChat(task int, request models.AIRunRequest) (*models.AgentJob, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Acquire the write lock before checking the current transcript cursor/type.
	if _, err = tx.Exec("UPDATE agent_job SET progress=progress WHERE task=?", task); err != nil {
		return nil, err
	}
	var j models.AgentJob
	err = scanAgentJob(tx.QueryRow("SELECT "+agentJobColumns+" FROM agent_job WHERE task=? AND json_extract(requestJson,'$.chat.clientKey')=?", task, request.Chat.ClientKey), &j)
	if err == nil {
		return &j, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var mode string
	if err = tx.QueryRow("SELECT tt.viewMode FROM task t JOIN task_type tt ON tt.id=t.type WHERE t.id=?", task).Scan(&mode); err != nil {
		return nil, err
	}
	if mode != "chat" {
		return nil, ErrChatType
	}
	var latest int
	if err = tx.QueryRow("SELECT COALESCE(MAX(id),0) FROM agent_job WHERE task=? AND json_type(requestJson,'$.chat')='object'", task).Scan(&latest); err != nil {
		return nil, err
	}
	if latest != request.Chat.PreviousJob {
		return nil, ErrChatConflict
	}
	if err = taskownership.Check(tx, task, 0); err != nil {
		return nil, err
	}
	var matches bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM task t JOIN checklist c ON c.id=t.checklist WHERE t.id=? AND c.project=?)", task, request.ProjectID).Scan(&matches); err != nil {
		return nil, err
	}
	if !matches {
		return nil, ErrChatConflict
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	err = scanAgentJob(tx.QueryRow("INSERT INTO agent_job(task,persona,status,requestJson) VALUES(?,(SELECT id FROM persona WHERE id=?),'pending',?) RETURNING "+agentJobColumns, task, request.AgentID, string(raw)), &j)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, ErrActiveAIRun
		}
		return nil, err
	}
	return &j, tx.Commit()
}
