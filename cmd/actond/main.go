package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/plugin"
	"github.com/actonos/actonos/internal/server"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

// Build metadata injected via linker flags from the VERSION file; see LDFLAGS
// in the Makefile. These defaults deliberately do not restate the released
// version: a plain `go build` produces an unstamped binary, and reporting a
// real version number for it would let the API drift from VERSION.
var (
	Version   = "0.0.0-dev"
	GitCommit = "unknown"
	BuildTime = "unspecified"
)

func main() {
	var (
		dataDir      = flag.String("data-dir", "./data", "Directory for persistent storage and databases")
		logLevel     = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		logFormat    = flag.String("log-format", "text", "Log format (text, json)")
		listenAddr   = flag.String("listen-addr", ":8080", "HTTP server listen address")
		embeddingURL = flag.String("embedding-url", "http://127.0.0.1:8091", "Local embeddingd service URL")
		hostname     = flag.String("hostname", "acton-mini", "Appliance network hostname")
		showVer      = flag.Bool("version", false, "Print version information and exit")
	)
	flag.Parse()
	system.WaitForOTAParent()

	// Environment variable overrides
	if envData := os.Getenv("ACTON_DATA_DIR"); envData != "" {
		*dataDir = envData
	}
	if envLog := os.Getenv("ACTON_LOG_LEVEL"); envLog != "" {
		*logLevel = envLog
	}
	if envFormat := os.Getenv("ACTON_LOG_FORMAT"); envFormat != "" {
		*logFormat = envFormat
	}
	if envAddr := os.Getenv("ACTON_LISTEN_ADDR"); envAddr != "" {
		*listenAddr = envAddr
	}
	if envEmbeddingURL := os.Getenv("ACTON_EMBEDDING_URL"); envEmbeddingURL != "" {
		*embeddingURL = envEmbeddingURL
	}

	if *showVer {
		fmt.Printf("ActonOS Daemon (actond) v%s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
		os.Exit(0)
	}

	// Setup Structured Logger
	var level slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if strings.ToLower(*logFormat) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	logger := slog.New(handler)
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
	agentsDir := filepath.Join(*dataDir, "agents")
	pluginsDir := filepath.Join(*dataDir, "plugins")
	skillsDir := filepath.Join(*dataDir, "skills")
	overridesDir := filepath.Join(*dataDir, "overrides")

	for _, dir := range []string{storageDir, vectorDir, workspaceDir, agentsDir, pluginsDir, skillsDir, overridesDir} {
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
	userWorkspace, err := workspacepkg.NewStore(ctx, db.SQLDB(), workspaceDir)
	if err != nil {
		slog.Error("failed to initialize user workspace", "error", err)
		os.Exit(1)
	}

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
	slog.Info("vault initialized (AES-256-GCM + Argon2id)")

	// 4. Initialize Hybrid Memory Engine
	hybridEngine := memory.NewHybridEngine(db, vectorStore, nil)
	embeddingService := memory.NewEmbeddingService(db.SQLDB(), vectorStore, memory.NewHTTPEmbedder(*embeddingURL))
	embeddingService.SetWorkspaceStore(userWorkspace)
	hybridEngine.SetEmbeddingService(embeddingService)
	embeddingService.SetWriteGuard(func() bool { return system.WritesFrozen(*dataDir) })
	embeddingService.Start(ctx)
	defer embeddingService.Stop()
	if watcher, wErr := memory.NewWorkspaceWatcher(workspaceDir, embeddingService); wErr != nil {
		slog.Warn("workspace embedding watcher unavailable", "error", wErr)
	} else if wErr := watcher.Start(ctx); wErr != nil {
		slog.Warn("workspace embedding watcher failed to start", "error", wErr)
	}

	// 5. Initialize Event Bus
	eventBus := bus.NewEventBus()
	defer eventBus.Close()
	slog.Info("event bus initialized")
	inboundQ, inboundErr := channels.NewInboundQueue(db.SQLDB())
	if inboundErr != nil {
		slog.Warn("inbound queue unavailable", "error", inboundErr)
		inboundQ = nil
	} else {
		eventBus.SetPersist(inboundQ.PersistEvent)
	}

	// 6. Initialize LLM Provider Router & Load Configured Keys
	llmRouter := llm.NewModelCascadeRouter()
	configDir := filepath.Join(*dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)

	server.RegisterAllStoredProvidersWithVault(ctx, llmRouter, configDir, vault)

	// Fallback mock only if no real providers are configured
	if llmRouter.Count() == 0 {
		mockLocal := llm.NewMockProvider("local-stub", "ActonOS Core Engine initialized. Please configure your LLM Provider API keys in System > Settings.")
		llmRouter.RegisterProvider("local-stub", mockLocal)
	}

	// 7. Initialize Dynamic Tooling Hub & System Audit Logger
	auditLogger, err := system.NewAuditLogger(*dataDir)
	if err != nil {
		slog.Warn("failed to initialize audit logger", "error", err)
	}
	if auditLogger != nil {
		defer auditLogger.Close()
	}

	toolReg := tools.NewToolRegistry(eventBus)
	if auditLogger != nil {
		toolReg.SetAuditLogger(auditLogger)
	}
	tools.RegisterNativeToolsWithConfig(toolReg, tools.NativeToolsConfig{
		DataDir:       *dataDir,
		AgentsDir:     agentsDir,
		UserWorkspace: userWorkspace,
	})
	toolReg.SetWorkspaceMutationSink(embeddingService)
	mcpHost := tools.NewMCPHostEngine(toolReg)
	mcpHost.SetEventBus(eventBus)
	if err := mcpHost.SetPersistence(db.SQLDB(), vault); err != nil {
		slog.Warn("failed to initialize persistent MCP registry", "error", err)
	}
	mcpHost.RestoreServers(ctx)

	skillWatcher := tools.NewSkillWatcher(toolReg, skillsDir)
	skillWatcher.SetEventBus(eventBus)
	if err := skillWatcher.Start(); err != nil {
		slog.Warn("skill watcher failed to start", "error", err)
	}
	defer skillWatcher.Stop()
	hubMgr := tools.NewHubManager(skillsDir)
	hubMgr.SetEventBus(eventBus)
	hubMgr.SetToolRegistry(toolReg)
	hubMgr.SetSkillWatcher(skillWatcher)
	slog.Info("dynamic tooling hub initialized", "tools_registered", len(toolReg.List()))

	// 8. Initialize Agent Manager, User Profile & Cognitive Memory
	agentMgr, err := agent.NewAgentManager(db, eventBus)
	if err != nil {
		slog.Error("failed to initialize agent manager", "error", err)
		os.Exit(1)
	}
	agentsList, _ := agentMgr.List(ctx)
	slog.Info("agent manager loaded", "agents_registered", len(agentsList))

	approvalMgr := tools.NewApprovalManager(db.SQLDB())
	toolReg.SetApprovalManager(approvalMgr)
	toolReg.SetPolicyResolver(func(ctx context.Context, agentID string) (tools.AgentToolPolicy, error) {
		manifest, resolveErr := agentMgr.Get(ctx, agentID)
		if resolveErr != nil {
			return tools.AgentToolPolicy{}, resolveErr
		}
		return tools.AgentToolPolicy{
			AuthorizedTools:   manifest.AuthorizedTools,
			ApprovalThreshold: string(manifest.DelegationScope.RequireHumanApproval),
			AllowedPaths:      manifest.DelegationScope.AllowedWorkspacePaths,
		}, nil
	})

	profileMgr, err := agent.NewUserProfileManager(db, *dataDir)
	if err != nil {
		slog.Warn("failed to initialize user profile manager", "error", err)
	}

	// 9. Initialize Swarm Manager, ReAct Engine & Proactive Cron Scheduler
	swarmMgr := agent.NewSwarmManager(agentMgr, eventBus, llmRouter, hybridEngine, 8)
	engine := agent.NewEngine(agentMgr, eventBus, llmRouter, hybridEngine)
	engine.SetEmbeddingService(embeddingService)
	contextMgr := agent.NewContextManager(128000)
	contextMgr.SetDB(db.SQLDB())
	engine.SetContextManager(contextMgr)
	engine.SetToolRegistry(toolReg)
	engine.SetDataDir(*dataDir)
	engine.SetWorkspaceDir(workspaceDir)
	runStore := agent.NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)
	if n, err := engine.ReclaimOrphanRuns(ctx); err != nil {
		slog.Warn("failed to reclaim orphan agent runs", "error", err)
	} else if n > 0 {
		slog.Warn("reclaimed orphan agent runs after restart", "count", n)
	}
	engine.SetPlanner(agent.NewPlanner(llmRouter))
	engine.SetSwarmManager(swarmMgr)
	swarmMgr.SetEngine(engine)
	if profileMgr != nil {
		engine.SetProfileManager(profileMgr)
	}
	tokenTracker := memory.NewTokenTracker(db.SQLDB())
	engine.SetTokenTracker(tokenTracker)

	// Initialize Cognitive Reflection Daemon
	reflectionEngine := agent.NewReflectionEngine(profileMgr, hybridEngine, llmRouter, eventBus)
	engine.SetReflectionEngine(reflectionEngine)
	reflectionEngine.Start(ctx)
	defer reflectionEngine.Stop()

	cronSched := agent.NewCronScheduler(engine, eventBus, db.SQLDB())
	tools.AttachCronScheduler(toolReg, cronSched)
	cronSched.Start(ctx)
	defer cronSched.Stop()

	// Initialize Autonomous Task Backlog Manager
	taskMgr, err := agent.NewTaskManager(db.SQLDB(), *dataDir)
	if err != nil {
		slog.Warn("failed to initialize task manager", "error", err)
	}
	if taskMgr != nil {
		engine.SetTaskManager(taskMgr)
		tools.AttachMissionBacklog(toolReg, taskMgr)
	}

	// Multi-Channel Cognitive Session Manager
	sessionMgr := channels.NewChannelSessionManager(db.SQLDB())
	sessionMgr.SetEmbeddingSink(embeddingService)
	engine.SetSessionManager(sessionMgr)

	// Initialize Autonomous Heartbeat Daemon
	heartbeatDaemon := agent.NewHeartbeatDaemon(agentMgr, engine, eventBus, db.SQLDB(), *dataDir, 5*time.Minute)
	if taskMgr != nil {
		heartbeatDaemon.SetTaskManager(taskMgr)
	}
	if approvalMgr != nil {
		heartbeatDaemon.SetApprovalManager(approvalMgr)
	}
	heartbeatDaemon.SetSessionManager(sessionMgr)
	heartbeatDaemon.SetCronScheduler(cronSched)
	heartbeatDaemon.Start(ctx)
	defer heartbeatDaemon.Stop()

	// 10. Initialize Zero-Trust Channel Pairing & Multi-Account Channel Manager
	pairingMgr, err := channels.NewPairingManager(db.SQLDB())
	if err != nil {
		slog.Warn("failed to initialize pairing manager", "error", err)
	}

	channelMgr := channels.NewChannelManager(eventBus, pairingMgr)
	toolReg.SetChannelSender(plugin.ChannelToolSender(channelMgr))

	// 10b. Initialize Unified WasmLoader Plugin System
	pluginKV, err := plugin.NewSQLiteKVStore(db.SQLDB())
	if err != nil {
		slog.Warn("failed to initialize plugin kv store", "error", err)
	}
	wasmLoader, err := plugin.NewWasmLoader(ctx)
	if err != nil {
		slog.Warn("failed to initialize wasm loader", "error", err)
	}
	var pluginMgr *plugin.Manager
	if wasmLoader != nil {
		pluginMgr = plugin.NewManager(wasmLoader, toolReg, channelMgr, eventBus, pluginKV, vault, pluginsDir)
		if userWorkspace != nil {
			pluginMgr.SetWorkspaceStore(userWorkspace)
		}
		if err := pluginMgr.ScanAndLoadAll(ctx); err != nil {
			slog.Warn("failed to load wasm plugins", "error", err)
		}
		defer pluginMgr.Close(ctx)
	}

	// Load initial channel accounts from every *_accounts.json (plugin channel ids).
	var initialAccounts []channels.ChannelAccount
	if matches, err := filepath.Glob(filepath.Join(configDir, "*_accounts.json")); err == nil {
		for _, path := range matches {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				continue
			}
			var accs []channels.ChannelAccount
			if json.Unmarshal(data, &accs) != nil {
				continue
			}
			initialAccounts = append(initialAccounts, accs...)
		}
	}
	if pluginMgr != nil {
		initialAccounts = plugin.MergeChannelAccounts(plugin.AccountsFromPlugins(pluginMgr.ListPlugins()), initialAccounts)
	}

	_ = channelMgr.SyncAccounts(ctx, initialAccounts)
	defer channelMgr.Stop()

	// Set Default Recipient Resolver for Proactive Schedulers
	cronSched.SetDefaultRecipientGetter(func(channel string) string {
		if pairingMgr != nil {
			paired := pairingMgr.ListAuthorized(channel)
			if len(paired) > 0 {
				return paired[0].SenderID
			}
		}
		return ""
	})

	// 10. MessageRouter: Coordinates multi-account messaging dispatch & proactive notifications
	msgRouter := channels.NewMessageRouter(channelMgr, agentMgr, sessionMgr, engine, eventBus)
	msgRouter.SetPairingManager(pairingMgr)
	msgRouter.SetInboundQueue(inboundQ)
	msgRouter.Start(ctx)
	defer msgRouter.Stop()

	// 11. Initialize System Auth, OAuth 2.1 PKCE Engine & Token Refresh Daemon
	sysAuth := auth.NewSystemAuthManager(db.SQLDB())
	stateStore := auth.NewStateStore(10 * time.Minute)
	oauthEngine := auth.NewOAuthEngine(stateStore)
	tokenDaemon := auth.NewTokenRefreshDaemon(oauthEngine, vault, db, eventBus)
	tokenDaemon.Start(ctx)
	defer tokenDaemon.Stop()
	slog.Info("token refresh daemon started (auto-renew 5min before expiry)")

	// 12. Initialize Hardware Abstraction Layer (HAL)
	hal := system.AutoDetectHAL(*dataDir)
	otaEngine := system.NewOTAEngine(*dataDir)
	otaEngine.SetVersionMeta(Version, GitCommit, BuildTime)
	otaEngine.SetRestarter(system.HALRestarter{HAL: hal, Engine: otaEngine})
	slog.Info("hardware abstraction layer loaded", "runtime_mode", hal.RuntimeMode())

	// 13. Initialize Embedded Tailscale Node
	tailscaleMgr := system.NewTailscaleManager(*dataDir, *hostname, "")
	if err := tailscaleMgr.Start(ctx); err != nil {
		slog.Warn("tailscale initialization warning", "error", err)
	}
	defer func() { _ = tailscaleMgr.Close() }()
	// 13b. Initialize Notification Manager
	notifMgr, err := system.NewNotificationManager(db.SQLDB(), eventBus)
	if err != nil {
		slog.Warn("notification manager initialization warning", "error", err)
	} else if notifMgr != nil {
		notifMgr.StartBackgroundListener(ctx)
		defer notifMgr.Stop()
	}

	// 14. Initialize HTTP REST API & Web UI Server
	srvConfig := server.Config{
		AgentManager:        agentMgr,
		SwarmManager:        swarmMgr,
		Engine:              engine,
		CronScheduler:       cronSched,
		HeartbeatDaemon:     heartbeatDaemon,
		TaskManager:         taskMgr,
		TokenTracker:        tokenTracker,
		ProfileManager:      profileMgr,
		LLMRouter:           llmRouter,
		ToolRegistry:        toolReg,
		SkillWatcher:        skillWatcher,
		MCPHost:             mcpHost,
		ApprovalManager:     approvalMgr,
		RunStore:            runStore,
		HubManager:          hubMgr,
		Memory:              hybridEngine,
		Embedding:           embeddingService,
		HAL:                 hal,
		Tailscale:           tailscaleMgr,
		TokenRefreshDaemon:  tokenDaemon,
		OAuthEngine:         oauthEngine,
		StateStore:          stateStore,
		SystemAuth:          sysAuth,
		NotificationManager: notifMgr,
		EventBus:            eventBus,
		AuditLogger:         auditLogger,
		Vault:               vault,
		PairingManager:      pairingMgr,
		ChannelManager:      channelMgr,
		PluginManager:       pluginMgr,
		WorkspaceDir:        workspaceDir,
		WorkspaceStore:      userWorkspace,
		SkillsDir:           skillsDir,
		WASMDir:             pluginsDir,
		PluginsDir:          pluginsDir,
		DataDir:             *dataDir,
		Version:             Version,
		GitCommit:           GitCommit,
		BuildTime:           BuildTime,
		OTAEngine:           otaEngine,
	}

	apiServer := server.NewServer(srvConfig)
	apiServer.RegisterStaticRoutes(overridesDir)
	go runOTACheckTicker(ctx, otaEngine, Version, embeddingService, notifMgr)

	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           apiServer.Router(),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("ActonOS Web UI & REST API listening", "address", *listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	// Serve Web UI & REST API on embedded Tailscale mesh node if active
	if tailscaleMgr != nil {
		for _, port := range []string{":80", ":8080"} {
			port := port
			go func() {
				ln, err := tailscaleMgr.Listen("tcp", port)
				if err != nil {
					slog.Debug("tailscale mesh listener not started", "port", port, "error", err)
					return
				}
				slog.Info("ActonOS Web UI & REST API listening on Tailscale mesh", "port", port)
				tsSrv := &http.Server{
					Handler:           apiServer.Router(),
					ReadHeaderTimeout: 30 * time.Second,
					IdleTimeout:       120 * time.Second,
				}
				go func() {
					<-ctx.Done()
					_ = tsSrv.Close()
				}()
				if err := tsSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
					slog.Debug("tailscale mesh http server closed", "port", port, "error", err)
				}
			}()
		}
	}

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

	// Checkpoint SQLite WAL mode to ensure 100% clean persistent disk state
	_, _ = db.SQLDB().Exec("PRAGMA wal_checkpoint(TRUNCATE);")

	eventBus.Publish(bus.NewEvent(bus.EventSystemShutdown, "kernel", nil))
	slog.Info("ActonOS daemon stopped cleanly")
}

func runOTACheckTicker(ctx context.Context, engine *system.OTAEngine, currentVersion string, embedding *memory.EmbeddingService, notifier *system.NotificationManager) {
	if engine == nil {
		return
	}
	fire := func() {
		in := system.EmbeddingRequiredInput{EnvForce: os.Getenv("ACTONOS_OTA_EMBEDDINGD")}
		if engine != nil {
			active, _ := engine.EmbeddingState()
			in.PriorEmbeddingActive = active
		}
		if embedding != nil {
			if st, err := embedding.Status(ctx); err == nil {
				in.ServiceReady = st.ServiceReady
			}
		}
		res := engine.Check(ctx, currentVersion, false, system.EmbeddingdRequired(in))
		if notifier != nil {
			_ = engine.MaybeNotify(ctx, res, notifier)
		}
	}
	timer := time.NewTimer(15 * time.Minute)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		fire()
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fire()
		}
	}
}
