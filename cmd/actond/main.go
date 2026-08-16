package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/server"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
)

var (
	// Version metadata set via linker flags (-ldflags).
	Version   = "0.1.0"
	GitCommit = "dev"
	BuildTime = "unspecified"
)

func main() {
	var (
		dataDir    = flag.String("data-dir", "./data", "Directory for persistent storage and databases")
		logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		listenAddr = flag.String("listen-addr", ":8080", "HTTP server listen address")
		hostname   = flag.String("hostname", "acton-mini", "Appliance network hostname")
		showVer    = flag.Bool("version", false, "Print version information and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("ActonOS Daemon (actond) v%s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
		os.Exit(0)
	}

	// Setup Structured Logger
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	slog.Info("starting ActonOS daemon...",
		"version", Version,
		"commit", GitCommit,
		"data_dir", *dataDir,
		"listen_addr", *listenAddr,
	)

	// Ensure runtime directories
	storageDir := filepath.Join(*dataDir, "storage")
	vectorDir := filepath.Join(*dataDir, "vectors")
	workspaceDir := filepath.Join(*dataDir, "workspace")
	pluginsDir := filepath.Join(*dataDir, "plugins")
	skillsDir := filepath.Join(*dataDir, "skills")
	overridesDir := filepath.Join(*dataDir, "overrides")

	for _, dir := range []string{storageDir, vectorDir, workspaceDir, pluginsDir, skillsDir, overridesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("failed to create directory", "path", dir, "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize SQLite Database
	dbPath := filepath.Join(storageDir, "acton.db")
	db, err := memory.Open(dbPath)
	if err != nil {
		slog.Error("failed to open database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database initialized (SQLite WAL mode)", "path", dbPath)

	// 2. Initialize Vector Store (Chromem-go)
	vectorStore, err := memory.NewVectorStore(vectorDir)
	if err != nil {
		slog.Error("failed to initialize vector store", "path", vectorDir, "error", err)
		os.Exit(1)
	}
	slog.Info("vector database initialized (Chromem-go)", "path", vectorDir)

	// 3. Initialize Hardware-Bound Vault
	vault, err := memory.NewVault(db, "", nil)
	if err != nil {
		slog.Error("failed to initialize vault", "error", err)
		os.Exit(1)
	}
	slog.Info("hardware-bound vault initialized (AES-256-GCM + Argon2id)")

	// 4. Initialize Hybrid Memory Engine
	hybridEngine := memory.NewHybridEngine(db, vectorStore, nil)

	// 5. Initialize Event Bus
	eventBus := bus.NewEventBus()
	defer eventBus.Close()
	slog.Info("event bus initialized")

	// 6. Initialize LLM Provider Router
	llmRouter := llm.NewModelCascadeRouter()
	mockLocal := llm.NewMockProvider("local-stub", "ActonOS Core Engine initialized and operating normally.")
	llmRouter.RegisterProvider("local-stub", mockLocal)

	// 7. Initialize Dynamic Tooling Hub
	toolReg := tools.NewToolRegistry(eventBus)
	tools.RegisterNativeTools(toolReg, workspaceDir)
	mcpHost := tools.NewMCPHostEngine(toolReg)
	wasmManager := tools.NewWASMPluginManager(toolReg, pluginsDir)
	_ = wasmManager.ScanAndRegisterPlugins(ctx)

	skillWatcher := tools.NewSkillWatcher(toolReg, skillsDir)
	if err := skillWatcher.Start(); err != nil {
		slog.Warn("skill watcher failed to start", "error", err)
	}
	defer skillWatcher.Stop()
	slog.Info("dynamic tooling hub initialized", "tools_registered", len(toolReg.List()))

	// 8. Initialize Agent Manager
	agentMgr, err := agent.NewAgentManager(db, eventBus)
	if err != nil {
		slog.Error("failed to initialize agent manager", "error", err)
		os.Exit(1)
	}
	agentsList, _ := agentMgr.List(ctx)
	slog.Info("agent manager loaded", "agents_registered", len(agentsList))

	// 9. Initialize Swarm Manager & ReAct Engine
	swarmMgr := agent.NewSwarmManager(agentMgr, eventBus, llmRouter, hybridEngine, 8)
	engine := agent.NewEngine(agentMgr, eventBus, llmRouter, hybridEngine)

	// 10. Initialize OAuth 2.1 PKCE Engine & Token Refresh Daemon
	stateStore := auth.NewStateStore(10 * time.Minute)
	oauthEngine := auth.NewOAuthEngine(stateStore)
	tokenDaemon := auth.NewTokenRefreshDaemon(oauthEngine, vault, db, eventBus)
	tokenDaemon.Start(ctx)
	defer tokenDaemon.Stop()
	slog.Info("token refresh daemon started (auto-renew 5min before expiry)")

	// 11. Initialize Hardware Abstraction Layer (HAL)
	hal := system.AutoDetectHAL(*dataDir)
	slog.Info("hardware abstraction layer loaded", "runtime_mode", hal.RuntimeMode())

	// 12. Initialize Embedded Tailscale Node
	tailscaleMgr := system.NewTailscaleManager(*dataDir, *hostname, "")
	if err := tailscaleMgr.Start(ctx); err != nil {
		slog.Warn("tailscale initialization warning", "error", err)
	}
	defer tailscaleMgr.Close()

	// 13. Initialize HTTP REST API & Web UI Server
	srvConfig := server.Config{
		AgentManager:       agentMgr,
		SwarmManager:       swarmMgr,
		Engine:             engine,
		LLMRouter:          llmRouter,
		ToolRegistry:       toolReg,
		MCPHost:            mcpHost,
		Memory:             hybridEngine,
		HAL:                hal,
		Tailscale:          tailscaleMgr,
		TokenRefreshDaemon: tokenDaemon,
		EventBus:           eventBus,
	}

	apiServer := server.NewServer(srvConfig)
	apiServer.RegisterStaticRoutes(overridesDir)

	httpServer := &http.Server{
		Addr:         *listenAddr,
		Handler:      apiServer.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("ActonOS Web UI & REST API listening", "address", *listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "error", err)
		}
	}()

	// Publish System Boot Event
	eventBus.Publish(bus.NewEvent(bus.EventSystemBoot, "kernel", map[string]any{
		"version": Version,
		"time":    time.Now().UTC(),
	}))

	slog.Info("ActonOS daemon is running and ready for instructions")

	// Handle Graceful Shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	sig := <-sigCh
	slog.Info("shutdown signal received", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}

	eventBus.Publish(bus.NewEvent(bus.EventSystemShutdown, "kernel", nil))
	slog.Info("ActonOS daemon stopped cleanly")
}
