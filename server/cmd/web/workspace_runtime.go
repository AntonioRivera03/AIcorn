package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/environments"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/jobs"
	"github.com/waseem-polus/aycorn/server/internal/markdown"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/projectchat"
	"github.com/waseem-polus/aycorn/server/internal/worker"
)

// Each workspace (a user's personal space or an organization) has its own
// SQLite database in its own directory:
//
//	<data dir>/workspaces/<id>/app.db
//
// Everything the single-user app used to build around its one database —
// repositories, services, the agent worker, the job scheduler, project chat,
// environments, backups — is built once per workspace from that file. This is
// the database-per-tenant (silo) pattern: the existing handlers and queries
// need no workspace filtering, because a request can only ever reach its own
// workspace's database. Anything derived from the database's directory
// (backups, worktrees, chat workspaces, environments) is per-workspace too.

// workspaceRuntime is one workspace's database plus everything built on it.
type workspaceRuntime struct {
	app     *app
	handler http.Handler
	stop    func()
}

type runtimeConfig struct {
	// mainURL is where this server is reachable, for environments that call
	// back into it.
	mainURL string
	mcpPath string
}

// startWorkspaceRuntime opens (creating and migrating if needed) the workspace
// database at dbPath and starts its background work under ctx.
func startWorkspaceRuntime(ctx context.Context, cfg runtimeConfig, dbPath string) (*workspaceRuntime, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := appdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	var cleanups []func()
	fail := func(err error) (*workspaceRuntime, error) {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
		db.Close()
		return nil, err
	}

	releaseWorkerLock, err := worker.AcquireDatabaseLock(dbPath)
	if err != nil {
		return fail(err)
	}
	cleanups = append(cleanups, releaseWorkerLock)

	if err := appdb.Migrate(db, dbPath); err != nil {
		return fail(err)
	}
	if previewMode() {
		if err := seedPreview(db); err != nil {
			return fail(err)
		}
	}

	workerCtx, stopWorker := context.WithCancel(ctx)
	cleanups = append(cleanups, stopWorker)

	backupLoopDone := make(chan struct{})
	go func() {
		defer close(backupLoopDone)
		appdb.RunBackupLoop(workerCtx, db, dbPath, appdb.BackupInterval())
	}()
	cleanups = append(cleanups, func() { stopWorker(); <-backupLoopDone })

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

	// Single-worker ticker reconciles Conductor and claims one Codex job.
	// In-flight work is interrupted on restart and requires an explicit recheck.
	// WAL + busy_timeout already handles concurrent DB access.
	aiService := &services.AIService{Jobs: agentJobRepo, Tasks: taskRepo, Projects: projectRepo, Presets: personaRepo, Converter: &markdown.Converter{}, MCPExecutable: cfg.mcpPath}
	conductorService := &services.ConductorService{Repo: &repos.ConductorRepo{DB: db}, AI: aiService, Runs: agentRunRepo}
	engine := &harness.Registry{Codex: &harness.Codex{MCPExecutable: cfg.mcpPath, DBPath: dbPath}}
	w := worker.New(agentJobService, engine)
	w.Conductor = conductorService
	if !previewMode() {
		if err := w.Start(workerCtx, 2*time.Second); err != nil {
			return fail(fmt.Errorf("worker start: %w", err))
		}
	}
	cleanups = append(cleanups, func() { stopWorker(); w.Stop() })

	jobService := &jobs.Service{DB: db, AI: aiService, Conductor: conductorService}
	schedulerDone := make(chan struct{})
	if previewMode() {
		close(schedulerDone)
	} else {
		go func() { defer close(schedulerDone); jobService.Run(workerCtx, 15*time.Second) }()
	}
	cleanups = append(cleanups, func() { stopWorker(); <-schedulerDone })

	projectChatService := &projectchat.Service{Store: projectchat.Store{DB: db}, AI: aiService, Engine: engine, WorkspaceRoot: filepath.Join(filepath.Dir(dbPath), "project-chat-workspaces")}
	chatDone := make(chan struct{})
	if previewMode() {
		close(chatDone)
	} else {
		if err := projectChatService.Start(workerCtx); err != nil {
			return fail(err)
		}
		go func() { defer close(chatDone); projectChatService.Run(workerCtx) }()
	}
	cleanups = append(cleanups, func() { stopWorker(); <-chatDone })

	app := &app{
		projectChatService:   projectChatService,
		jobService:           jobService,
		conductorService:     conductorService,
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

	if !previewMode() {
		store := &environments.Store{DB: db}
		token, err := store.Installation()
		if err != nil {
			return fail(err)
		}
		runtime := &environments.Kubernetes{Token: token, MainURL: cfg.mainURL}
		app.environmentService = &environments.Service{Store: store, Runtime: runtime, Root: filepath.Join(filepath.Dir(dbPath), "environments-"+token)}
		if err := app.environmentService.Start(workerCtx); err != nil {
			return fail(err)
		}
		cleanups = append(cleanups, func() { stopWorker(); app.environmentService.Wait() })
	}

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
			if err := appdb.BackupOnShutdown(db, dbPath); err != nil {
				log.Printf("shutdown backup (%s): %v", dbPath, err)
			}
			db.Close()
		})
	}
	return &workspaceRuntime{app: app, handler: app.routes(), stop: stop}, nil
}

// workspaceRegistry starts each workspace's runtime once and hands it out to
// requests. Runtimes live until the server shuts down.
type workspaceRegistry struct {
	ctx     context.Context
	dataDir string
	cfg     runtimeConfig

	// mu guards the map only. Starting a runtime (migrations, goroutines)
	// happens outside it, so a workspace being created never stalls requests
	// to the others.
	mu      sync.Mutex
	entries map[int64]*runtimeEntry
}

// runtimeEntry is one workspace's runtime, possibly still starting. ready is
// closed once rt/err are set; concurrent callers for the same workspace wait
// on it instead of starting a second runtime.
type runtimeEntry struct {
	ready chan struct{}
	rt    *workspaceRuntime
	err   error
}

func newWorkspaceRegistry(ctx context.Context, dataDir string, cfg runtimeConfig) *workspaceRegistry {
	return &workspaceRegistry{ctx: ctx, dataDir: dataDir, cfg: cfg, entries: map[int64]*runtimeEntry{}}
}

func (r *workspaceRegistry) dbPath(id int64) string {
	return appdb.WorkspaceDBPath(r.dataDir, id)
}

// get returns the workspace's runtime, starting it on first use. Background
// work runs under the registry's context, never the caller's request.
func (r *workspaceRegistry) get(id int64) (*workspaceRuntime, error) {
	r.mu.Lock()
	if entry, ok := r.entries[id]; ok {
		r.mu.Unlock()
		<-entry.ready
		return entry.rt, entry.err
	}
	entry := &runtimeEntry{ready: make(chan struct{})}
	r.entries[id] = entry
	r.mu.Unlock()

	rt, err := startWorkspaceRuntime(r.ctx, r.cfg, r.dbPath(id))
	if err != nil {
		err = fmt.Errorf("workspace %d: %w", id, err)
		// Forget the failure so a later request can retry.
		r.mu.Lock()
		delete(r.entries, id)
		r.mu.Unlock()
	}
	entry.rt, entry.err = rt, err
	close(entry.ready)
	return rt, err
}

// provision creates, migrates, and seeds a new workspace's database. It is
// the accounts service's Provision hook. The migrations seed task types,
// relationship types, and agents; a starter workflow is added here so the
// first project can be created straight away.
func (r *workspaceRegistry) provision(_ context.Context, id int64) error {
	rt, err := r.get(id)
	if err != nil {
		return err
	}
	return rt.app.workflowService.EnsureStarterWorkflow()
}

// unprovision undoes provision for a workspace whose creation rolled back:
// it stops the runtime and deletes its directory. SQLite hands a rolled-back
// ID to the next workspace, which must start from nothing.
func (r *workspaceRegistry) unprovision(id int64) {
	r.mu.Lock()
	entry, ok := r.entries[id]
	delete(r.entries, id)
	r.mu.Unlock()
	if ok {
		<-entry.ready
		if entry.rt != nil {
			entry.rt.stop()
		}
	}
	if err := os.RemoveAll(filepath.Dir(r.dbPath(id))); err != nil {
		log.Printf("removing rolled-back workspace %d: %v", id, err)
	}
}

// stopAll stops every runtime's background work and takes a final backup.
func (r *workspaceRegistry) stopAll() {
	r.mu.Lock()
	entries := r.entries
	r.entries = map[int64]*runtimeEntry{}
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, entry := range entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-entry.ready
			if entry.rt != nil {
				entry.rt.stop()
			}
		}()
	}
	wg.Wait()
}
