package appdb_test

import (
	"github.com/pressly/goose/v3"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/migrations"
	"path/filepath"
	"testing"
)

func TestCodexMigrationPreservesAgentsAndRequiresExplicitRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := appdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "sql", 16); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO workflow(id,name) VALUES(1,'Test')`,
		`INSERT INTO stage(id,workflow,name,color,icon,position,type) VALUES(1,1,'Open','gray','circle',1,'open')`,
		`INSERT INTO project(id,name,workflow) VALUES(1,'Test',1)`,
		`INSERT INTO checklist(id,name,project) VALUES(1,'Test',1)`,
		`INSERT INTO task(id,name,checklist,stage,type,priority) VALUES(1,'Test',1,1,1,'Medium')`,
		`INSERT INTO persona(id,name,harness,model,system_prompt) VALUES(100,'My agent','opencode','anthropic/claude','my prompt'),(101,'OpenAI','opencode','opencode-go/gpt-5.6-luna','another prompt')`,
		`UPDATE ai_settings SET model='openai/gpt-5.5',executable='/old/opencode'`,
		`INSERT INTO agent_job(id,task,persona,status,requestJson) VALUES(7,1,100,'completed','{"model":"old/model"}'),(8,1,100,'pending','{"conductor":{"phase":"planning"}}')`,
		`INSERT INTO agent_run(id,job,output) VALUES(9,7,'Historical answer')`,
		`INSERT INTO conductor_project(project,enabled,settings) VALUES(1,1,'{"enabled":true,"planningStage":1,"planningPrompt":"my plan","plannerModel":"old/model","workerModels":["old/worker"]}')`,
		`INSERT INTO conductor_task(task,project,state,job,expectedStage) VALUES(1,1,'planning',8,1)`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = appdb.Migrate(db, path); err != nil {
		t.Fatal(err)
	}
	var name, engine, model, prompt string
	if err = db.QueryRow(`SELECT name,harness,model,system_prompt FROM persona WHERE id=100`).Scan(&name, &engine, &model, &prompt); err != nil {
		t.Fatal(err)
	}
	if name != "My agent" || engine != "codex" || model != "gpt-5.6-sol" || prompt != "my prompt" {
		t.Fatal(name, engine, model, prompt)
	}
	if err = db.QueryRow(`SELECT model FROM persona WHERE id=101`).Scan(&model); err != nil || model != "gpt-5.6-luna" {
		t.Fatal(model, err)
	}
	if err = db.QueryRow(`SELECT model,executable FROM ai_settings`).Scan(&model, &engine); err != nil || model != "gpt-5.5" || engine != "" {
		t.Fatal(model, engine, err)
	}
	var status, output, request, state string
	if err = db.QueryRow(`SELECT status FROM agent_job WHERE id=8`).Scan(&status); err != nil || status != "interrupted" {
		t.Fatal(status, err)
	}
	if err = db.QueryRow(`SELECT output,requestJson FROM agent_run JOIN agent_job ON agent_job.id=agent_run.job WHERE agent_run.id=9`).Scan(&output, &request); err != nil || output != "Historical answer" || request != `{"model":"old/model"}` {
		t.Fatal(output, request, err)
	}
	var expectedAgent int
	if err = db.QueryRow("SELECT id FROM persona WHERE builtin_role='conductor'").Scan(&expectedAgent); err != nil {
		t.Fatal(err)
	}
	var enabled, agent int
	if err = db.QueryRow(`SELECT enabled,json_extract(settings,'$.conductorAgentId'),json_extract(settings,'$.planningPrompt') FROM conductor_project`).Scan(&enabled, &agent, &prompt); err != nil || enabled != 0 || agent != expectedAgent || prompt != "my plan" {
		t.Fatal(enabled, agent, prompt, err)
	}
	if err = db.QueryRow(`SELECT state FROM conductor_task`).Scan(&state); err != nil || state != "held" {
		t.Fatal(state, err)
	}
}
