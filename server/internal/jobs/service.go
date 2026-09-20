package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
)

var ErrInvalid = errors.New("invalid job configuration")
var ErrConflict = errors.New("job changed or is already running; refresh and try again")

type Template struct {
	ID          int    `json:"id"`
	ProjectID   int    `json:"projectId"`
	Revision    int    `json:"revision"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Body        string `json:"body"` // Markdown, converted to Plate when instantiated.
	ChecklistID int    `json:"checklistId"`
	StageID     int    `json:"stageId"`
	TypeID      int    `json:"typeId"`
	Priority    string `json:"priority"`
	Assignee    string `json:"assignee"`
	Prompt      string `json:"prompt"`
}
type Job struct {
	ID         int    `json:"id"`
	ProjectID  int    `json:"projectId"`
	TemplateID int    `json:"templateId"`
	Name       string `json:"name"`
	AgentID    int    `json:"agentId"`
	Schedule   string `json:"schedule"`
	Timezone   string `json:"timezone"`
	Enabled    bool   `json:"enabled"`
	NextRun    int64  `json:"nextRun"`
	Revision   int    `json:"revision"`
	LastError  string `json:"lastError"`
}
type Run struct {
	ID        int    `json:"id"`
	JobID     int    `json:"jobId"`
	TaskID    int    `json:"taskId"`
	Trigger   string `json:"trigger"`
	CreatedAt string `json:"createdAt"`
	State     string `json:"state"`
	Message   string `json:"message"`
}
type Service struct {
	DB        *sql.DB
	AI        *services.AIService
	Conductor *services.ConductorService
	Now       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalid, message) }
func (s *Service) projectExists(project int) error {
	var id int
	return s.DB.QueryRow("SELECT id FROM project WHERE id=?", project).Scan(&id)
}

func (s *Service) Templates(project int) ([]Template, error) {
	if err := s.projectExists(project); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query("SELECT id,project,data,revision FROM task_template WHERE project=? ORDER BY id DESC", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Template{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanTemplate(row scanner) (Template, error) {
	var t Template
	var raw string
	var id, project, revision int
	if err := row.Scan(&id, &project, &raw, &revision); err != nil {
		return t, err
	}
	err := json.Unmarshal([]byte(raw), &t)
	t.ID, t.ProjectID, t.Revision = id, project, revision
	return t, err
}
func (s *Service) Template(project, id int) (Template, error) {
	return scanTemplate(s.DB.QueryRow("SELECT id,project,data,revision FROM task_template WHERE project=? AND id=?", project, id))
}
func (s *Service) CreateTemplate(project int) (Template, error) {
	t := Template{ProjectID: project, Name: "New Template", Title: "New Task", Priority: "Medium"}
	if err := s.projectExists(project); err != nil {
		return t, err
	}
	// Creation has useful defaults; missing checklist/type is editable in place.
	_ = s.DB.QueryRow("SELECT id FROM checklist WHERE project=? ORDER BY isDefault DESC,id LIMIT 1", project).Scan(&t.ChecklistID)
	_ = s.DB.QueryRow("SELECT s.id FROM stage s JOIN project p ON p.workflow=s.workflow WHERE p.id=? AND s.type='open' ORDER BY s.position,s.id LIMIT 1", project).Scan(&t.StageID)
	_ = s.DB.QueryRow("SELECT tt.id FROM task_type tt JOIN project_task_type pt ON pt.task_type=tt.id WHERE pt.project=? ORDER BY tt.isDefault DESC,tt.id LIMIT 1", project).Scan(&t.TypeID)
	raw, _ := json.Marshal(t)
	var id int
	err := s.DB.QueryRow("INSERT INTO task_template(project,data) VALUES(?,?) RETURNING id", project, string(raw)).Scan(&id)
	if err != nil {
		return t, err
	}
	return s.Template(project, id)
}
func validateTemplateText(t Template) error {
	if len(t.Name) > 300 || len(t.Title) > 500 || len(t.Body) > 256000 || len(t.Prompt) > 16000 || len(t.Assignee) > 300 {
		return invalid("template text exceeds the allowed length")
	}
	switch t.Priority {
	case "Urgent", "High", "Medium", "Low":
	default:
		return invalid("unknown priority")
	}
	if t.ChecklistID < 0 || t.StageID < 0 || t.TypeID < 0 {
		return invalid("invalid template selection")
	}
	return nil
}
func (s *Service) UpdateTemplate(project, id int, t Template) (Template, error) {
	if err := validateTemplateText(t); err != nil {
		return t, err
	}
	if _, err := s.Template(project, id); err != nil {
		return t, err
	}
	if err := validateTemplateRefs(s.DB, project, t, false); err != nil {
		return t, err
	}
	t.ProjectID, t.ID = project, id
	raw, _ := json.Marshal(t)
	res, err := s.DB.Exec("UPDATE task_template SET data=?,revision=revision+1 WHERE id=? AND project=? AND revision=?", string(raw), id, project, t.Revision)
	if err != nil {
		return t, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return t, ErrConflict
	}
	return s.Template(project, id)
}

type querier interface{ QueryRow(string, ...any) *sql.Row }

func validateTemplateRefs(q querier, project int, t Template, required bool) error {
	if required && (t.ChecklistID == 0 || t.StageID == 0 || t.TypeID == 0) {
		return invalid("select a checklist, stage and task type")
	}
	for _, check := range []struct {
		id    int
		query string
	}{
		{t.ChecklistID, "SELECT COUNT(*) FROM checklist WHERE id=? AND project=?"},
		{t.StageID, "SELECT COUNT(*) FROM stage s JOIN project p ON p.workflow=s.workflow WHERE s.id=? AND p.id=?"},
		{t.TypeID, "SELECT COUNT(*) FROM project_task_type WHERE task_type=? AND project=?"},
	} {
		if check.id == 0 {
			continue
		}
		var n int
		if err := q.QueryRow(check.query, check.id, project).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return invalid("template selection does not belong to this project")
		}
	}
	return nil
}
func (s *Service) DeleteTemplate(project, id int) error {
	if _, err := s.Template(project, id); err != nil {
		return err
	}
	res, err := s.DB.Exec("DELETE FROM task_template WHERE id=? AND project=? AND NOT EXISTS(SELECT 1 FROM scheduled_job WHERE template=?)", id, project, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return invalid("remove jobs using this template first")
	}
	return err
}
func insertTask(tx *sql.Tx, t Template, body string) (int, error) {
	var id int
	err := tx.QueryRow("INSERT INTO task(checklist,stage,type,name,body,assignee,priority,timeCompleted) VALUES(?,?,?,?,?,?,?,CASE WHEN (SELECT type FROM stage WHERE id=?)='done' THEN CURRENT_TIMESTAMP ELSE NULL END) RETURNING id", t.ChecklistID, t.StageID, t.TypeID, t.Title, body, t.Assignee, t.Priority, t.StageID).Scan(&id)
	return id, err
}
func (s *Service) Instantiate(ctx context.Context, project, id int) (int, error) {
	t, err := s.Template(project, id)
	if err != nil {
		return 0, err
	}
	if err = validateTemplateRefs(s.DB, project, t, true); err != nil {
		return 0, err
	}
	body, err := s.AI.Converter.ToBody(ctx, []string{t.Body})
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE task_template SET revision=revision WHERE id=? AND revision=?", id, t.Revision)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return 0, ErrConflict
	}
	if err = validateTemplateRefs(tx, project, t, true); err != nil {
		return 0, err
	}
	task, err := insertTask(tx, t, body[0])
	if err != nil {
		return 0, err
	}
	return task, tx.Commit()
}

const jobColumns = "id,project,template,name,COALESCE(agent,0),schedule,timezone,enabled,nextRun,revision,lastError"

func scanJob(row scanner) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.ProjectID, &j.TemplateID, &j.Name, &j.AgentID, &j.Schedule, &j.Timezone, &j.Enabled, &j.NextRun, &j.Revision, &j.LastError)
	return j, err
}
func (s *Service) Job(project, id int) (Job, error) {
	return scanJob(s.DB.QueryRow("SELECT "+jobColumns+" FROM scheduled_job WHERE project=? AND id=?", project, id))
}
func (s *Service) Jobs(project int) ([]Job, error) {
	if err := s.projectExists(project); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query("SELECT "+jobColumns+" FROM scheduled_job WHERE project=? ORDER BY id DESC", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, j)
	}
	return result, rows.Err()
}
func (s *Service) CreateJob(project, template int) (Job, error) {
	t, err := s.Template(project, template)
	if err != nil {
		return Job{}, err
	}
	var id int
	err = s.DB.QueryRow("INSERT INTO scheduled_job(project,template,name) VALUES(?,?,?) RETURNING id", project, template, t.Name).Scan(&id)
	if err != nil {
		return Job{}, err
	}
	return s.Job(project, id)
}
func schedule(expression, zone string) (cron.Schedule, error) {
	if strings.TrimSpace(zone) == "" {
		return nil, invalid("choose an IANA timezone")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return nil, invalid("unknown timezone")
	}
	if len(strings.Fields(expression)) != 5 || strings.Contains(expression, "=") {
		return nil, invalid("schedule must have five cron fields: minute hour day month weekday")
	}
	result, err := cron.ParseStandard("CRON_TZ=" + zone + " " + expression)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return result, nil
}
func (s *Service) UpdateJob(ctx context.Context, project, id int, j Job) (Job, error) {
	old, err := s.Job(project, id)
	if err != nil {
		return j, err
	}
	j.ID, j.ProjectID, j.TemplateID = old.ID, old.ProjectID, old.TemplateID
	if len(j.Name) > 300 || len(j.Schedule) > 200 || len(j.Timezone) > 100 || j.AgentID < 0 {
		return j, invalid("invalid job settings")
	}
	if j.AgentID > 0 {
		if _, err = s.AI.ResolveAgent(ctx, j.AgentID); err != nil {
			return j, err
		}
	}
	next := old.NextRun
	if j.Schedule != "" {
		parsed, err := schedule(j.Schedule, j.Timezone)
		if err != nil {
			return j, err
		}
		if old.Schedule != j.Schedule || old.Timezone != j.Timezone || (!old.Enabled && j.Enabled) || next == 0 {
			next = parsed.Next(s.now()).Unix()
			if next <= 0 {
				return j, invalid("schedule has no future occurrence")
			}
		}
	} else {
		next = 0
		if j.Enabled {
			return j, invalid("set a schedule before enabling it")
		}
	}
	if j.Enabled {
		if j.AgentID == 0 {
			return j, invalid("select a job agent")
		}
		if _, _, err = s.prepare(ctx, j); err != nil {
			return j, err
		}
	}
	var agent any
	if j.AgentID > 0 {
		agent = j.AgentID
	}
	res, err := s.DB.Exec("UPDATE scheduled_job SET name=?,agent=?,schedule=?,timezone=?,enabled=?,nextRun=?,revision=revision+1,lastError='' WHERE project=? AND id=? AND revision=?", j.Name, agent, j.Schedule, j.Timezone, j.Enabled, next, project, id, j.Revision)
	if err != nil {
		return j, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return j, ErrConflict
	}
	return s.Job(project, id)
}
func (s *Service) prepare(ctx context.Context, j Job) (Template, *models.AIRunRequest, error) {
	t, err := s.Template(j.ProjectID, j.TemplateID)
	if err != nil {
		return t, nil, err
	}
	if err = validateTemplateRefs(s.DB, j.ProjectID, t, true); err != nil {
		return t, nil, err
	}
	settings, _, err := s.Conductor.Repo.Settings(j.ProjectID)
	if err != nil {
		return t, nil, err
	}
	if err = s.Conductor.Repo.ValidateStages(j.ProjectID, settings); err != nil {
		return t, nil, err
	}
	coder, err := s.AI.ResolveAgent(ctx, j.AgentID)
	if err != nil {
		return t, nil, err
	}
	bodies, err := s.AI.Converter.ToBody(ctx, []string{t.Body})
	if err != nil {
		return t, nil, err
	}
	task := &models.TaskWithProject{ProjectID: j.ProjectID}
	task.Name, task.Body = t.Title, bodies[0]
	settings.PlanningPrompt += "\n\nJob instructions:\n" + t.Prompt
	intent := "ask"
	role := "coder"
	if preset, e := s.AI.Presets.FindOne(j.AgentID); e == nil && preset.BuiltinRole != "" {
		role = preset.BuiltinRole
	}
	if role == "conductor" || role == "chatter" {
		return t, nil, invalid("choose a task agent for this Job")
	}
	if settings.UseRepository && role == "coder" {
		intent = "implement"
	}
	req, err := s.AI.PrepareSnapshot(ctx, task, services.AIRunInput{Intent: intent, Agent: coder, UseRepository: settings.UseRepository, Instruction: settings.WorkingPrompt + "\n\nJob instructions:\n" + t.Prompt})
	if err != nil {
		return t, nil, err
	}
	settings.WorkingPrompt += "\n\nJob instructions:\n" + t.Prompt
	req.Chat = &models.ChatTurn{ClientKey: req.Key}
	req.TaskSession = &models.TaskSession{Role: role, Mode: "work", Settings: &settings, ExpectedStage: settings.WorkingStage}
	req.Conductor = &models.ConductorRun{Independent: true, Phase: "working", Settings: settings, SourceBody: bodies[0], TaskAgent: coder}
	return t, req, nil
}

const activeJobRun = `EXISTS(SELECT 1 FROM scheduled_job_run jr WHERE jr.job=? AND (EXISTS(SELECT 1 FROM conductor_task ct WHERE ct.task=jr.task AND ct.state IN ('waiting','planning','queued','working')) OR EXISTS(SELECT 1 FROM agent_job aj WHERE aj.task=jr.task AND aj.status IN ('pending','claimed','running','canceling'))))`

func (s *Service) DeleteJob(project, id int) error {
	if _, err := s.Job(project, id); err != nil {
		return err
	}
	res, err := s.DB.Exec("DELETE FROM scheduled_job WHERE id=? AND project=? AND NOT "+activeJobRun, id, project, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}

// Fire prepares outside the write transaction, then atomically checks revisions,
// claims the occurrence and inserts both the task and its Conductor queue item.
func (s *Service) Fire(ctx context.Context, project, id int, trigger, key string) (int, error) {
	if trigger != "manual" && trigger != "schedule" {
		return 0, invalid("invalid trigger")
	}
	if key == "" || len(key) > 128 {
		return 0, invalid("provide an idempotency key")
	}
	var previous int
	if err := s.DB.QueryRow("SELECT COALESCE(task,0) FROM scheduled_job_run WHERE job=? AND trigger=? AND occurrence=?", id, trigger, key).Scan(&previous); err == nil {
		if _, err = s.Job(project, id); err != nil {
			return 0, err
		}
		if previous == 0 {
			return 0, invalid("the task from this request was deleted")
		}
		return previous, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	j, err := s.Job(project, id)
	if err != nil {
		return 0, err
	}
	t, req, err := s.prepare(ctx, j)
	if err != nil {
		return 0, err
	}
	now := s.now()
	next := j.NextRun
	if trigger == "schedule" {
		if !j.Enabled || j.NextRun > now.Unix() || key != strconv.FormatInt(j.NextRun, 10) {
			return 0, ErrConflict
		}
		parsed, err := schedule(j.Schedule, j.Timezone)
		if err != nil {
			return 0, err
		}
		next = parsed.Next(now).Unix()
		if next <= 0 {
			return 0, invalid("schedule has no next occurrence")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE scheduled_job SET revision=revision WHERE id=? AND project=?", id, project)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return 0, ErrConflict
	}
	if err = tx.QueryRow("SELECT COALESCE(task,0) FROM scheduled_job_run WHERE job=? AND trigger=? AND occurrence=?", id, trigger, key).Scan(&previous); err == nil {
		if previous == 0 {
			return 0, invalid("the task from this request was deleted")
		}
		return previous, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var currentRevision int
	if err = tx.QueryRow("SELECT revision FROM scheduled_job WHERE id=?", id).Scan(&currentRevision); err != nil {
		return 0, err
	}
	if currentRevision != j.Revision {
		return 0, ErrConflict
	}
	var active bool
	if err = tx.QueryRow("SELECT "+activeJobRun, id).Scan(&active); err != nil {
		return 0, err
	}
	if active {
		if trigger == "schedule" {
			_, err = tx.Exec("UPDATE scheduled_job SET nextRun=?,revision=revision+1,lastError='Skipped overlapping run' WHERE id=?", next, id)
			if err != nil {
				return 0, err
			}
			return 0, tx.Commit()
		}
		return 0, ErrConflict
	}
	var revision int
	if err = tx.QueryRow("SELECT revision FROM task_template WHERE id=?", t.ID).Scan(&revision); err != nil {
		return 0, err
	}
	if revision != t.Revision {
		return 0, ErrConflict
	}
	if err = validateTemplateRefs(tx, project, t, true); err != nil {
		return 0, err
	}
	contract := req.Conductor.Settings
	var validStages int
	if err = tx.QueryRow("SELECT COUNT(*) FROM stage s JOIN project p ON p.workflow=s.workflow WHERE p.id=? AND s.id IN (?,?) AND s.type<>'done'", project, contract.WorkingStage, contract.CompletionStage).Scan(&validStages); err != nil {
		return 0, err
	}
	if validStages != 2 {
		return 0, invalid("Conductor stages changed before the job could start")
	}
	t.StageID = contract.WorkingStage
	t.Assignee = "AI · " + req.PresetName
	taskID, err := insertTask(tx, t, req.Conductor.SourceBody)
	if err != nil {
		return 0, err
	}
	// Settings can be configured while the board toggle is off. Ensure the durable
	// controller row exists without changing that toggle.
	settings, _ := json.Marshal(req.Conductor.Settings)
	if _, err = tx.Exec("INSERT INTO conductor_project(project,enabled,settings) VALUES(?,0,?) ON CONFLICT(project) DO NOTHING", project, string(settings)); err != nil {
		return 0, err
	}
	raw, _ := json.Marshal(req)
	var queueID int
	if err = tx.QueryRow("INSERT INTO agent_job(task,persona,status,requestJson) VALUES(?,(SELECT id FROM persona WHERE id=?),'pending',?) RETURNING id", taskID, req.AgentID, string(raw)).Scan(&queueID); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("INSERT INTO conductor_task(task,project,state,job,expectedStage,message) VALUES(?,?,'queued',?,?,'Task session queued')", taskID, project, queueID, t.StageID); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("INSERT INTO scheduled_job_run(job,task,trigger,occurrence) VALUES(?,?,?,?)", id, taskID, trigger, key); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("UPDATE scheduled_job SET nextRun=?,revision=revision+1,lastError='' WHERE id=?", next, id); err != nil {
		return 0, err
	}
	return taskID, tx.Commit()
}
func (s *Service) Runs(project, id int) ([]Run, error) {
	if _, err := s.Job(project, id); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT jr.id,COALESCE(jr.job,0),COALESCE(jr.task,0),jr.trigger,jr.createdAt,COALESCE(ct.state,CASE WHEN jr.task IS NULL THEN 'deleted' ELSE 'released' END),COALESCE(ct.message,'') FROM scheduled_job_run jr LEFT JOIN conductor_task ct ON ct.task=jr.task WHERE jr.job=? ORDER BY jr.id DESC LIMIT 50`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Run{}
	for rows.Next() {
		var r Run
		if err = rows.Scan(&r.ID, &r.JobID, &r.TaskID, &r.Trigger, &r.CreatedAt, &r.State, &r.Message); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *Service) Tick(ctx context.Context) error {
	now := s.now()
	rows, err := s.DB.QueryContext(ctx, "SELECT project,id,nextRun,revision FROM scheduled_job WHERE enabled=1 AND nextRun>0 AND nextRun<=? ORDER BY nextRun,id LIMIT 20", now.Unix())
	if err != nil {
		return err
	}
	type due struct {
		project, id int
		revision    int
		next        int64
	}
	var list []due
	for rows.Next() {
		var d due
		if err = rows.Scan(&d.project, &d.id, &d.next, &d.revision); err != nil {
			rows.Close()
			return err
		}
		list = append(list, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, d := range list {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err = s.Fire(ctx, d.project, d.id, "schedule", strconv.FormatInt(d.next, 10))
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && !errors.Is(err, ErrConflict) {
			// A failed setup consumes this due occurrence and remains visible. Avoid a
			// tight retry loop or a backlog storm after downtime; the next cadence retries.
			j, getErr := s.Job(d.project, d.id)
			if getErr != nil {
				continue
			}
			failedAt := s.now()
			next := failedAt.Add(time.Minute).Unix()
			if parsed, parseErr := schedule(j.Schedule, j.Timezone); parseErr == nil {
				next = parsed.Next(failedAt).Unix()
			}
			if _, saveErr := s.DB.Exec("UPDATE scheduled_job SET lastError=?,nextRun=?,revision=revision+1 WHERE id=? AND nextRun=? AND revision=?", err.Error(), next, d.id, d.next, d.revision); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}

// Run keeps the scheduler independent of the single harness worker: long model
// turns must not prevent due occurrences being claimed or overlaps being skipped.
// The caller holds the application's database process lock for this loop's life.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Printf("job scheduler: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
