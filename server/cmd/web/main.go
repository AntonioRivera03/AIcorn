package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/accounts"
	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/environments"
	"github.com/waseem-polus/aycorn/server/internal/jobs"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
	"github.com/waseem-polus/aycorn/server/internal/models/services"
	"github.com/waseem-polus/aycorn/server/internal/projectchat"
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
// Loopback is the safe default. To serve other people, bind to a Tailscale
// interface address (or "0.0.0.0" for your whole LAN) — accounts and
// sessions gate every workspace API, but the server still speaks plain HTTP.
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

// app is one workspace's API: the handlers plus the repositories and services
// built on that workspace's database (see workspace_runtime.go).
type app struct {
	projectChatService   *projectchat.Service
	jobService           *jobs.Service
	environmentService   *environments.Service
	conductorService     *services.ConductorService
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

	dataDir, err := appdb.ResolveDataDir()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Using data directory %s", dataDir)

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()

	accountsDB, err := accounts.Open(serverCtx, dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer accountsDB.Close()

	mcpName := "aycorn-mcp"
	if runtime.GOOS == "windows" {
		mcpName += ".exe"
	}
	mcpPath := os.Getenv("AYCORN_MCP_EXECUTABLE")
	if mcpPath == "" {
		executable, _ := os.Executable()
		mcpPath = filepath.Join(filepath.Dir(executable), mcpName)
		if _, err := os.Stat(mcpPath); err != nil {
			mcpPath, _ = filepath.Abs(filepath.Join("bin", mcpName))
		}
	}
	mcpPath, err = filepath.Abs(mcpPath)
	if err != nil {
		log.Fatal(err)
	}

	host := resolveHost()
	ln, port, err := findAvailablePort(host, resolvePort())
	if err != nil {
		log.Fatal(err)
	}

	workspaces := newWorkspaceRegistry(serverCtx, dataDir, runtimeConfig{
		mainURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		mcpPath: mcpPath,
	})
	defer workspaces.stopAll()
	accountService := &accounts.Service{
		Store:     accounts.NewStore(accountsDB),
		Mailer:    accounts.MailerFromEnv(),
		Provision: workspaces.provision,
	}

	// Start every workspace up front so scheduled jobs and agent work keep
	// running for workspaces nobody currently has open.
	ids, err := accountService.AllWorkspaceIDs(serverCtx)
	if err != nil {
		log.Fatal(err)
	}
	for _, id := range ids {
		if _, err := workspaces.get(id); err != nil {
			log.Printf("starting %v", err)
		}
	}
	go pruneSessionsLoop(serverCtx, accountService)

	srv := &server{accounts: accountService, workspaces: workspaces, accountsDB: accountsDB, publicURL: os.Getenv("AYCORN_PUBLIC_URL")}
	httpServer := http.Server{Handler: srv.routes()}

	// Start the server in a goroutine so we can listen for shutdown signals.
	go func() {
		log.Printf("Listening on http://%s", net.JoinHostPort(host, strconv.Itoa(port)))
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
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
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatal("Forced shutdown:", err)
	}

	stopServer()
	workspaces.stopAll()
	log.Println("Done")
}

func pruneSessionsLoop(ctx context.Context, service *accounts.Service) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := service.PruneSessions(ctx); err != nil && ctx.Err() == nil {
			log.Printf("pruning sessions: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
