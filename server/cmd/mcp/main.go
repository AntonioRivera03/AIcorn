// Command mcp runs Aycorn's task data as an MCP server over stdio, so an AI
// agent (Claude Desktop, `claude mcp add`, etc.) can read and write tasks
// through the same service/repo layer cmd/web uses — no HTTP hop. See
// Documentation/phase-1-mcp-server.md and Documentation/ai-architecture.md §3-4.
package main

import (
	"context"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
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
	checklistRepo := &repos.ChecklistRepo{DB: db}
	categoryRepo := &repos.TaskTypeCategoryRepo{DB: db}

	toolset := &toolset{
		taskService:      &services.TaskService{TaskRepo: taskRepo, TaskTypeRepo: taskTypeRepo},
		projectService:   &services.ProjectService{ProjectRepo: projectRepo, TaskRepo: taskRepo},
		stageService:     &services.StageService{StageRepo: stageRepo},
		checklistService: &services.ChecklistService{ChecklistRepo: checklistRepo, TaskRepo: taskRepo},
		taskTypeService:  &services.TaskTypeService{TaskTypeRepo: taskTypeRepo, CategoryRepo: categoryRepo},
		converter:        &markdown.Converter{},
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "aycorn-mcp", Version: "0.1.0"}, nil)
	toolset.register(srv)

	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
