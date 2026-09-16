// Command mcp runs Aycorn's task data as an MCP server over stdio, so an AI
// agent (Claude Desktop, `claude mcp add`, etc.) can read and write tasks
// through the same service/repo layer cmd/web uses — no HTTP hop. See
// Documentation/phase-1-mcp-server.md and Documentation/ai-architecture.md §3-4.
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	_ "modernc.org/sqlite"
)

func main() {
	// stdout is the JSON-RPC channel for stdio transport — a single stray
	// write there corrupts the whole stream. log defaults to stderr already;
	// this line makes that explicit so it can't regress silently.
	log.SetOutput(os.Stderr)

	dbPath, err := appdb.ResolveDBPath()
	if err != nil {
		log.Fatal(err)
	}
	db, err := appdb.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := appdb.Migrate(db, dbPath); err != nil {
		log.Fatal(err)
	}

	// Best-effort — a missed backup shouldn't stop the MCP server from
	// starting. No ticker here: unlike cmd/web this process's lifetime is
	// controlled by the MCP host, not guaranteed to run long enough to make
	// one worthwhile beyond this startup check.
	if err := appdb.BackupIfStale(db, dbPath, appdb.BackupInterval()); err != nil {
		log.Printf("startup backup: %v", err)
	}

	taskRepo := &repos.TaskRepo{DB: db}
	taskTypeRepo := &repos.TaskTypeRepo{DB: db}
	projectRepo := &repos.ProjectRepo{DB: db}
	stageRepo := &repos.StageRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	checklistRepo := &repos.ChecklistRepo{DB: db}
	categoryRepo := &repos.TaskTypeCategoryRepo{DB: db}
	personaRepo := &repos.PersonaRepo{DB: db}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}
	agentJobService := &services.AgentJobService{
		JobRepo:     agentJobRepo,
		RunRepo:     agentRunRepo,
		PersonaRepo: personaRepo,
		TaskRepo:    taskRepo,
	}

	toolset := &toolset{
		taskService:      &services.TaskService{TaskRepo: taskRepo, TaskTypeRepo: taskTypeRepo, AgentJobService: agentJobService, StagePersonaRepo: stagePersonaRepo},
		projectService:   &services.ProjectService{ProjectRepo: projectRepo, TaskRepo: taskRepo},
		stageService:     &services.StageService{StageRepo: stageRepo},
		checklistService: &services.ChecklistService{ChecklistRepo: checklistRepo, TaskRepo: taskRepo},
		taskTypeService:  &services.TaskTypeService{TaskTypeRepo: taskTypeRepo, CategoryRepo: categoryRepo},
		converter:        &markdown.Converter{},
	}

	if raw, scoped := os.LookupEnv("AYCORN_RUN_TASK"); scoped {
		taskID, err := strconv.Atoi(raw)
		if err != nil || taskID <= 0 {
			log.Fatal("invalid run task scope")
		}
		toolset.runTaskID = taskID
	}
	if raw, scoped := os.LookupEnv("AYCORN_CONDUCTOR_PROJECT"); scoped {
		projectID, err := strconv.Atoi(raw)
		if err != nil || projectID <= 0 || toolset.runTaskID <= 0 {
			log.Fatal("invalid Conductor project scope")
		}
		task, err := taskRepo.FindOneWithProject(toolset.runTaskID)
		if err != nil || task.ProjectID != projectID {
			log.Fatal("Conductor task does not belong to project scope")
		}
		toolset.runProjectID = projectID
	}
	if raw, scoped := os.LookupEnv("AYCORN_RUN_JOB"); scoped {
		jobID, err := strconv.Atoi(raw)
		if err != nil || jobID <= 0 || toolset.runTaskID <= 0 {
			log.Fatal("invalid run job scope")
		}
		job, err := agentJobRepo.FindOne(jobID)
		if err != nil || job.Task != toolset.runTaskID {
			log.Fatal("run job does not belong to task scope")
		}
		toolset.runJobID = jobID
	}
	if raw, scoped := os.LookupEnv("AYCORN_CHAT_TURN"); scoped {
		turn, err := strconv.Atoi(raw)
		if err != nil || turn <= 0 || toolset.runTaskID != 0 {
			log.Fatal("invalid Chatter turn scope")
		}
		project, err := strconv.Atoi(os.Getenv("AYCORN_CHAT_PROJECT"))
		if err != nil || project <= 0 {
			log.Fatal("invalid Chatter project scope")
		}
		if err = taskownership.CheckChat(db, project, turn); err != nil {
			log.Fatal(err)
		}
		toolset.runProjectID = project
		toolset.runChatTurnID = turn
		executable, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		toolset.aiService = &services.AIService{Jobs: agentJobRepo, Tasks: taskRepo, Projects: projectRepo, Presets: personaRepo, Converter: toolset.converter, MCPExecutable: executable}
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "aycorn-mcp", Version: "0.1.0"}, nil)
	toolset.register(srv)

	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
