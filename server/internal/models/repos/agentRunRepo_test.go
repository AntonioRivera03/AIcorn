package repos

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/waseem-polus/aycorn/server/internal/models"
	_ "modernc.org/sqlite"
)

func TestAgentRunRepo_CreateAndList_ValidRowAndNulls(t *testing.T) {
	db := setupAgentJobTestDB(t)
	jobRepo := &AgentJobRepo{DB: db}
	runRepo := &AgentRunRepo{DB: db}

	job, err := jobRepo.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}

	exit := 0
	usage := `{"prompt_tokens":10,"completion_tokens":20}`
	run := &models.AgentRun{
		Job:       job.ID,
		Output:    "# hello",
		Summary:   "did stuff",
		ExitCode:  &exit,
		UsageJson: usage,
	}
	created, err := runRepo.CreateRun(run)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected run ID non-zero")
	}
	if created.Output != "# hello" {
		t.Fatalf("Output = %q; want # hello", created.Output)
	}
	if created.Summary != "did stuff" {
		t.Fatalf("Summary = %q", created.Summary)
	}
	if created.ExitCode == nil || *created.ExitCode != 0 {
		t.Fatalf("ExitCode = %v; want 0", created.ExitCode)
	}
	// Validate usageJson was accepted as valid JSON and returned verbatim
	var js map[string]int
	if err := json.Unmarshal([]byte(created.UsageJson), &js); err != nil {
		t.Fatalf("UsageJson not valid JSON: %v", err)
	}
	if js["prompt_tokens"] != 10 {
		t.Fatalf("prompt_tokens = %d; want 10", js["prompt_tokens"])
	}
	if created.CreatedAt == nil {
		t.Fatal("CreatedAt nil")
	}

	// Null handling: insert run with NULL nullable cols via repo (nil ExitCode, empty strings)
	run2 := &models.AgentRun{Job: job.ID, Output: "", Summary: "", ExitCode: nil, UsageJson: ""}
	created2, err := runRepo.CreateRun(run2)
	if err != nil {
		t.Fatalf("CreateRun with nulls: %v", err)
	}
	if created2.Output != "" || created2.Summary != "" {
		t.Fatalf("expected empty output/summary for nulls, got %q / %q", created2.Output, created2.Summary)
	}
	if created2.ExitCode != nil {
		t.Fatalf("ExitCode = %v; want nil", created2.ExitCode)
	}
	if created2.UsageJson != "" {
		t.Fatalf("UsageJson = %q; want empty for null/empty", created2.UsageJson)
	}

	// Raw NULL insertion: verify COALESCE handling on read path
	if _, err := db.Exec(`INSERT INTO agent_run (job, output, summary, exitCode, usageJson) VALUES (?, NULL, NULL, NULL, NULL);`, job.ID); err != nil {
		t.Fatalf("raw insert null run: %v", err)
	}
	runs, err := runRepo.ListByJob(job.ID)
	if err != nil {
		t.Fatalf("ListByJob after raw NULL: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("ListByJob count = %d; want 3", len(runs))
	}
	last := runs[2]
	if last.Output != "" || last.Summary != "" || last.ExitCode != nil || last.UsageJson != "" {
		t.Fatalf("COALESCE null handling failed: %+v", last)
	}

	// Via AgentJobRepo.CreateRun convenience
	run3 := &models.AgentRun{Job: job.ID, Output: "via job repo", UsageJson: `{"x":1}`}
	if _, err := jobRepo.CreateRun(run3); err != nil {
		t.Fatalf("AgentJobRepo.CreateRun: %v", err)
	}
	runs2, _ := runRepo.ListByJob(job.ID)
	if len(runs2) != 4 {
		t.Fatalf("after AgentJobRepo.CreateRun count = %d; want 4", len(runs2))
	}
}

func TestAgentRunRepo_InvalidUsageJson_ReturnsError(t *testing.T) {
	db := setupAgentJobTestDB(t)
	jobRepo := &AgentJobRepo{DB: db}
	runRepo := &AgentRunRepo{DB: db}

	job, err := jobRepo.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}
	// Insert invalid JSON directly bypassing repo validation
	if _, err := db.Exec(`INSERT INTO agent_run (job, usageJson) VALUES (?, 'not-json');`, job.ID); err != nil {
		t.Fatalf("raw insert invalid json: %v", err)
	}
	// List path must surface parse error via scanAgentRun
	if _, err := runRepo.ListByJob(job.ID); err == nil {
		t.Fatal("expected error for invalid usageJson, got nil")
	}
	// Direct scan via Find path: insert another invalid and query single
	if _, err := db.Exec(`INSERT INTO agent_run (job, usageJson) VALUES (?, '{bad}');`, job.ID); err != nil {
		t.Fatalf("second raw insert: %v", err)
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM agent_run WHERE usageJson = '{bad}';`).Scan(&id); err != nil {
		t.Fatalf("select bad id: %v", err)
	}
	// Querying that specific row via raw scan should also error
	rows, err := db.Query(`SELECT `+agentRunColumns+` FROM agent_run WHERE id = ?;`, id)
	if err != nil {
		t.Fatalf("query bad row: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		var r models.AgentRun
		if err := scanAgentRun(rows, &r); err == nil {
			t.Fatal("expected scanAgentRun to fail on invalid usageJson")
		}
	}
}

func TestAgentRunRepo_ListByJob_ValidJSONVariants(t *testing.T) {
	db := setupAgentJobTestDB(t)
	jobRepo := &AgentJobRepo{DB: db}
	runRepo := &AgentRunRepo{DB: db}

	job, _ := jobRepo.Create(&models.AgentJob{Task: 1, Persona: 1, Status: models.AgentJobStatusPending})
	// JSON array variant
	if _, err := runRepo.CreateRun(&models.AgentRun{Job: job.ID, UsageJson: `[1,2,3]`}); err != nil {
		t.Fatalf("create with array json: %v", err)
	}
	// JSON object variant already covered; null string handled as empty
	runs, err := runRepo.ListByJob(job.ID)
	if err != nil {
		t.Fatalf("ListByJob: %v", err)
	}
	if len(runs) != 1 || runs[0].UsageJson != `[1,2,3]` {
		t.Fatalf("unexpected usageJson: %+v", runs)
	}
	// Empty/blank usageJson inserted as empty should stay empty (not "null")
	if _, err := db.Exec(`INSERT INTO agent_run (job, usageJson) VALUES (?, '');`, job.ID); err != nil {
		t.Fatalf("insert empty: %v", err)
	}
	runs, _ = runRepo.ListByJob(job.ID)
	// last should have empty UsageJson
	if runs[1].UsageJson != "" {
		t.Fatalf("empty usageJson = %q; want empty", runs[1].UsageJson)
	}
	// Ensure plain string non-JSON still errors
	if _, err := db.Exec(`INSERT INTO agent_run (job, usageJson) VALUES (?, 'plain text');`, job.ID); err != nil {
		t.Fatalf("insert plain: %v", err)
	}
	if _, err := runRepo.ListByJob(job.ID); err == nil {
		t.Fatal("expected error for plain text usageJson")
	}
	_ = sql.ErrNoRows // silence unused import check
}
