package main

import (
	"context"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/worker"
	_ "modernc.org/sqlite"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
// Falls back to "dev" for local builds without a tag.
var version = "dev"

// resolvePort returns the port to listen on.
//
// Precedence:
//  1. --port <n> CLI flag
//  2. $AYCORN_PORT env var
//  3. Default: 8000
func resolvePort() int {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil && p > 0 {
				return p
			}
		}
	}
	if p, err := strconv.Atoi(os.Getenv("AYCORN_PORT")); err == nil && p > 0 {
		return p
	}
	return 8000
}

// resolveHost returns the address to bind to.
//
// Precedence:
//  1. --host <addr> CLI flag
//  2. $AYCORN_HOST env var
//  3. Default: 127.0.0.1 (loopback only — not reachable from other devices)
//
// Aycorn is a localhost-only app by design; only override this if you know
// you want it reachable from other devices (e.g. binding to a Tailscale
// interface address, or "0.0.0.0" for your whole LAN).
func resolveHost() string {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--host" && i+1 < len(args) {
			return args[i+1]
		}
	}
	if h := os.Getenv("AYCORN_HOST"); h != "" {
		return h
	}
	return "127.0.0.1"
}

// findAvailablePort tries to bind to startPort, then startPort+1, …, up to 10
// attempts. Returns the bound listener and the port it landed on.
func findAvailablePort(host string, startPort int) (net.Listener, int, error) {
	for port := startPort; port < startPort+10; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			return ln, port, nil
		}
	}
	return nil, 0, fmt.Errorf("no available port found in range %d–%d; use --port or $AYCORN_PORT to choose a different one", startPort, startPort+9)
}

type app struct {
	aiService            *services.AIService
	projectRepo          *repos.ProjectRepo
	checklistRepo        *repos.ChecklistRepo
	workflowRepo         *repos.WorkflowRepo
	stageRepo            *repos.StageRepo
	taskTypeRepo         *repos.TaskTypeRepo
	taskTypeCategoryRepo *repos.TaskTypeCategoryRepo
	taskRelationshipRepo *repos.TaskRelationshipRepo
	personaRepo          *repos.PersonaRepo

	projectService          *services.ProjectService
	checklistService        *services.ChecklistService
	taskService             *services.TaskService
	workflowService         *services.WorkflowService
	stageService            *services.StageService
	taskTypeService         *services.TaskTypeService
	taskTypeCategoryService *services.TaskTypeCategoryService
	taskRelationshipService *services.TaskRelationshipService
	personaService          *services.PersonaService
	agentJobService         *services.AgentJobService
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version":
			fmt.Println(version)
			return
		case "backup":
			if err := runBackup(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		case "restore":
			if err := runRestore(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	dbPath, err := appdb.ResolveDBPath()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Using database at %s", dbPath)

	db, err := appdb.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	releaseWorkerLock, err := worker.AcquireDatabaseLock(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer releaseWorkerLock()

	if err := appdb.Migrate(db, dbPath); err != nil {
		log.Fatal(err)
	}

	backupCtx, stopBackups := context.WithCancel(context.Background())
	defer stopBackups()
	backupLoopDone := make(chan struct{})
	go func() {
		defer close(backupLoopDone)
		appdb.RunBackupLoop(backupCtx, db, dbPath, appdb.BackupInterval())
	}()

	projectRepo := &repos.ProjectRepo{DB: db}
	checklistRepo := &repos.ChecklistRepo{DB: db}
	taskRepo := &repos.TaskRepo{DB: db}
	workflowRepo := &repos.WorkflowRepo{DB: db}
	stageRepo := &repos.StageRepo{DB: db}
	stagePersonaRepo := &repos.StagePersonaRepo{DB: db}
	taskTypeRepo := &repos.TaskTypeRepo{DB: db}
	taskTypeCategoryRepo := &repos.TaskTypeCategoryRepo{DB: db}
	taskRelationshipRepo := &repos.TaskRelationshipRepo{DB: db}
	personaRepo := &repos.PersonaRepo{DB: db}
	agentJobRepo := &repos.AgentJobRepo{DB: db}
	agentRunRepo := &repos.AgentRunRepo{DB: db}

	agentJobService := &services.AgentJobService{
		JobRepo:     agentJobRepo,
		RunRepo:     agentRunRepo,
		PersonaRepo: personaRepo,
		TaskRepo:    taskRepo,
	}

	projectService := &services.ProjectService{
		ProjectRepo:   projectRepo,
		TaskRepo:      taskRepo,
		ChecklistRepo: checklistRepo,
		WorkflowRepo:  workflowRepo,
		StageRepo:     stageRepo,
		TaskTypeRepo:  taskTypeRepo,
	}
	checklistService := &services.ChecklistService{
		ChecklistRepo: checklistRepo,
		TaskRepo:      taskRepo,
	}
	taskService := &services.TaskService{
		TaskRepo:         taskRepo,
		TaskTypeRepo:     taskTypeRepo,
		AgentJobService:  agentJobService,
		StagePersonaRepo: stagePersonaRepo,
		ProjectRepo:      projectRepo,
		PersonaRepo:      personaRepo,
	}
	workflowService := &services.WorkflowService{
		WorkflowRepo: workflowRepo,
		ProjectRepo:  projectRepo,
		StageRepo:    stageRepo,
	}
	stageService := &services.StageService{
		StageRepo:        stageRepo,
		StagePersonaRepo: stagePersonaRepo,
	}
	taskTypeService := &services.TaskTypeService{
		TaskTypeRepo: taskTypeRepo,
		CategoryRepo: taskTypeCategoryRepo,
	}
	taskTypeCategoryService := &services.TaskTypeCategoryService{
		CategoryRepo: taskTypeCategoryRepo,
		TaskTypeRepo: taskTypeRepo,
	}
	taskRelationshipService := &services.TaskRelationshipService{
		TaskRelationshipRepo: taskRelationshipRepo,
	}
	personaService := &services.PersonaService{PersonaRepo: personaRepo}

	// Single-worker ticker: one goroutine claims pending jobs every 10s,
	// recovers stale on startup (claimed > 5m), executes read-only shim.
	// WAL + busy_timeout already handles concurrent DB access.
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	mcpPath := os.Getenv("AYCORN_MCP_EXECUTABLE")
	if mcpPath == "" {
		executable, _ := os.Executable()
		mcpPath = filepath.Join(filepath.Dir(executable), "aycorn-mcp")
		if _, err := os.Stat(mcpPath); err != nil {
			mcpPath, _ = filepath.Abs("bin/aycorn-mcp")
		}
	}
	aiService := &services.AIService{Jobs: agentJobRepo, Tasks: taskRepo, Projects: projectRepo, Presets: personaRepo, Converter: &markdown.Converter{}, MCPExecutable: mcpPath}
	engine := &harness.OpenCode{MCPExecutable: mcpPath, DBPath: dbPath}
	w := worker.New(agentJobService, engine)
	if err := w.Start(workerCtx, 10*time.Second); err != nil {
		log.Fatalf("worker start: %v", err)
	}
	defer w.Stop()

	app := app{
		aiService:            aiService,
		projectRepo:          projectRepo,
		checklistRepo:        checklistRepo,
		workflowRepo:         workflowRepo,
		stageRepo:            stageRepo,
		taskTypeRepo:         taskTypeRepo,
		taskTypeCategoryRepo: taskTypeCategoryRepo,
		taskRelationshipRepo: taskRelationshipRepo,
		personaRepo:          personaRepo,

		projectService:          projectService,
		checklistService:        checklistService,
		taskService:             taskService,
		workflowService:         workflowService,
		stageService:            stageService,
		taskTypeService:         taskTypeService,
		taskTypeCategoryService: taskTypeCategoryService,
		taskRelationshipService: taskRelationshipService,
		personaService:          personaService,
		agentJobService:         agentJobService,
	}

	host := resolveHost()
	ln, port, err := findAvailablePort(host, resolvePort())
	if err != nil {
		log.Fatal(err)
	}

	server := http.Server{Handler: app.routes()}

	// Start the server in a goroutine so we can listen for shutdown signals.
	go func() {
		log.Printf("Listening on http://%s", net.JoinHostPort(host, strconv.Itoa(port)))
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Block until SIGINT (Ctrl-C) or SIGTERM (kill / make upgrade).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down — waiting for in-flight requests to finish...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Forced shutdown:", err)
	}

	stopWorker()
	w.Stop()

	stopBackups()
	<-backupLoopDone

	if err := appdb.BackupOnShutdown(db, dbPath); err != nil {
		log.Printf("shutdown backup: %v", err)
	}
	log.Println("Done")
}
