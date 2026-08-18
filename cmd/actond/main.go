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
	"strings"
	"syscall"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/server"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
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
		dataDir    = flag.String("data-dir", "./data", "Directory for persistent storage and databases")
		logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		logFormat  = flag.String("log-format", "text", "Log format (text, json)")
		listenAddr = flag.String("listen-addr", ":8080", "HTTP server listen address")
		hostname   = flag.String("hostname", "acton-mini", "Appliance network hostname")
		showVer    = flag.Bool("version", false, "Print version information and exit")
	)
	flag.Parse()

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

	// 6. Initialize LLM Provider Router & Load Configured Keys
	llmRouter := llm.NewModelCascadeRouter()
	configDir := filepath.Join(*dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)

	readKey := func(file string) string {
		data, _ := os.ReadFile(filepath.Join(configDir, file))
		return strings.TrimSpace(string(data))
	}

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
	tools.RegisterNativeTools(toolReg, workspaceDir)
	mcpHost := tools.NewMCPHostEngine(toolReg)
	if err := mcpHost.SetPersistence(db.SQLDB(), vault); err != nil {
		slog.Warn("failed to initialize persistent MCP registry", "error", err)
	}
	mcpHost.RestoreServers(ctx)
	wasmManager := tools.NewWASMPluginManager(toolReg, pluginsDir)
	_ = wasmManager.ScanAndRegisterPlugins(ctx)

	skillWatcher := tools.NewSkillWatcher(toolReg, skillsDir)
	if err := skillWatcher.Start(); err != nil {
		slog.Warn("skill watcher failed to start", "error", err)
	}
	defer skillWatcher.Stop()
	hubMgr := tools.NewHubManager(skillsDir)
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
	contextMgr := agent.NewContextManager(8192)
	contextMgr.SetDB(db.SQLDB())
	engine.SetContextManager(contextMgr)
	engine.SetToolRegistry(toolReg)
	runStore := agent.NewRunStore(db.SQLDB())
	engine.SetRunStore(runStore)
	engine.SetPlanner(agent.NewPlanner(llmRouter))
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
	taskMgr, err := agent.NewTaskManager(db.SQLDB(), workspaceDir)
	if err != nil {
		slog.Warn("failed to initialize task manager", "error", err)
	}

	// Initialize Autonomous Heartbeat Daemon
	heartbeatDaemon := agent.NewHeartbeatDaemon(agentMgr, engine, eventBus, db.SQLDB(), workspaceDir, 5*time.Minute)
	if taskMgr != nil {
		heartbeatDaemon.SetTaskManager(taskMgr)
	}
	heartbeatDaemon.Start(ctx)
	defer heartbeatDaemon.Stop()

	// 10. Initialize Zero-Trust Channel Pairing & Multi-Account Channel Manager
	pairingMgr, err := channels.NewPairingManager(db.SQLDB())
	if err != nil {
		slog.Warn("failed to initialize pairing manager", "error", err)
	}

	channelMgr := channels.NewChannelManager(eventBus, pairingMgr)

	// Load initial channel accounts from disk
	tgToken := readKey("telegram.token")
	waToken := readKey("whatsapp.token")
	waPhone := readKey("whatsapp.phone_id")
	dcToken := readKey("discord.token")

	var initialAccounts []channels.ChannelAccount
	if tgToken != "" {
		initialAccounts = append(initialAccounts, channels.ChannelAccount{
			ID: "tg_default", Name: "Primary Telegram Bot", Channel: "telegram", Token: tgToken, Enabled: true, BoundAgentIDs: []string{"*"},
		})
	}
	if waToken != "" {
		initialAccounts = append(initialAccounts, channels.ChannelAccount{
			ID: "wa_default", Name: "Primary WhatsApp Number", Channel: "whatsapp", Token: waToken, PhoneID: waPhone, Enabled: true, BoundAgentIDs: []string{"*"},
		})
	}
	if dcToken != "" {
		initialAccounts = append(initialAccounts, channels.ChannelAccount{
			ID: "dc_default", Name: "Primary Discord Bot", Channel: "discord", Token: dcToken, Enabled: true, BoundAgentIDs: []string{"*"},
		})
	}
	_ = channelMgr.SyncAccounts(ctx, initialAccounts)
	_ = channelMgr.Start(ctx)
	defer channelMgr.Stop()

	// Multi-Channel Cognitive Session Manager
	sessionMgr := channels.NewChannelSessionManager(db.SQLDB())
	heartbeatDaemon.SetSessionManager(sessionMgr)

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

	// Background listener for channel messages & proactive cron notifications
	channelSub := eventBus.Subscribe(bus.EventAgentActionStarted)
	doneSub := eventBus.Subscribe(bus.EventAgentActionDone)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-channelSub:
				if !ok {
					return
				}
				if ev.AgentID == "telegram" || ev.AgentID == "whatsapp" || ev.AgentID == "discord" {
					if inMsg, ok := ev.Payload.(channels.InboundMessage); ok {
						go func(msg channels.InboundMessage) {
							// Determine bound agent for this specific channel account
							target := channelMgr.FindBoundAgent(msg.ChannelID, msg.AccountID)
							if target == "" {
								target = "agent_system_core"
							}
							senderID := msg.SenderID
							if msg.Metadata != nil && msg.Metadata["chat_id"] != "" {
								senderID = msg.Metadata["chat_id"]
							}

							// 1. Get or create deterministic intelligent session
							convID, err := sessionMgr.GetOrCreateSession(context.Background(), msg.ChannelID, senderID, msg.SenderName, msg.Content, target)
							if err != nil {
								slog.Warn("failed to get/create channel session", "error", err)
							}

							// 2. Load short-term Working Memory (recent dialogue history)
							history := sessionMgr.LoadRecentHistory(context.Background(), convID, 6)

							// 3. Persist incoming user message into SQLite
							_ = sessionMgr.SaveMessage(context.Background(), convID, target, "user", msg.Content, nil)

							// 4. Construct contextual metadata prompt
							chatMeta := ""
							if msg.Metadata != nil && msg.Metadata["chat_id"] != "" {
								chatMeta = fmt.Sprintf("[Channel: %s | Account: %s | User Chat ID: %s | Sender: %s]\n", msg.ChannelID, msg.AccountID, msg.Metadata["chat_id"], msg.SenderName)
							} else if msg.ChannelID != "" {
								chatMeta = fmt.Sprintf("[Channel: %s | Account: %s | Sender ID: %s]\n", msg.ChannelID, msg.AccountID, msg.SenderID)
							}
							promptWithMeta := chatMeta + msg.Content

							// 5. Execute cognitive ReAct loop with multi-layer memory (Working + Episodic + Procedural + User Profile)
							resp, err := engine.ExecuteStepWithHistory(context.Background(), target, promptWithMeta, history)
							if err != nil {
								slog.Error("failed to process channel message", "channel", msg.ChannelID, "error", err)
								return
							}

							// 6. Persist assistant response into SQLite session history
							if resp != nil {
								_ = sessionMgr.SaveMessage(context.Background(), convID, target, "assistant", resp.Content, resp.ToolCalls)
							}

							// 7. Deliver outbound response via ChannelManager
							_ = channelMgr.SendMessage(context.Background(), channels.OutboundMessage{
								ChannelID: msg.ChannelID,
								AccountID: msg.AccountID,
								Recipient: senderID,
								Content:   resp.Content,
							})
						}(inMsg)
					}
				}
			case ev, ok := <-doneSub:
				if !ok {
					return
				}
				if payloadMap, ok := ev.Payload.(map[string]any); ok {
					if pType, ok := payloadMap["type"].(string); ok && pType == "proactive_cron_notification" {
						content, _ := payloadMap["content"].(string)
						jobName, _ := payloadMap["job_name"].(string)
						targetChan, _ := payloadMap["target_channel"].(string)
						targetAcc, _ := payloadMap["target_account_id"].(string)
						targetRec, _ := payloadMap["target_recipient"].(string)

						if targetChan == "" {
							targetChan = "all"
						}
						if targetAcc == "" {
							targetAcc = "all"
						}

						msgText := fmt.Sprintf("⏰ **[%s]**\n\n%s", jobName, content)

						// Route through ChannelManager to target channel and account(s)
						_ = channelMgr.SendMessage(context.Background(), channels.OutboundMessage{
							ChannelID: targetChan,
							AccountID: targetAcc,
							Recipient: targetRec,
							Content:   msgText,
						})
					}
				}
			}
		}
	}()

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
	slog.Info("hardware abstraction layer loaded", "runtime_mode", hal.RuntimeMode())

	// 13. Initialize Embedded Tailscale Node
	tailscaleMgr := system.NewTailscaleManager(*dataDir, *hostname, "")
	if err := tailscaleMgr.Start(ctx); err != nil {
		slog.Warn("tailscale initialization warning", "error", err)
	}
	defer tailscaleMgr.Close()

	// 14. Initialize HTTP REST API & Web UI Server
	srvConfig := server.Config{
		AgentManager:       agentMgr,
		SwarmManager:       swarmMgr,
		Engine:             engine,
		CronScheduler:      cronSched,
		HeartbeatDaemon:    heartbeatDaemon,
		TaskManager:        taskMgr,
		TokenTracker:       tokenTracker,
		ProfileManager:     profileMgr,
		LLMRouter:          llmRouter,
		ToolRegistry:       toolReg,
		MCPHost:            mcpHost,
		ApprovalManager:    approvalMgr,
		RunStore:           runStore,
		HubManager:         hubMgr,
		Memory:             hybridEngine,
		HAL:                hal,
		Tailscale:          tailscaleMgr,
		TokenRefreshDaemon: tokenDaemon,
		OAuthEngine:        oauthEngine,
		StateStore:         stateStore,
		SystemAuth:         sysAuth,
		EventBus:           eventBus,
		AuditLogger:        auditLogger,
		Vault:              vault,
		PairingManager:     pairingMgr,
		ChannelManager:     channelMgr,
		TelegramAdapter:    nil,
		WhatsAppAdapter:    nil,
		WorkspaceDir:       workspaceDir,
		SkillsDir:          skillsDir,
		WASMDir:            pluginsDir,
		DataDir:            *dataDir,
		Version:            Version,
		GitCommit:          GitCommit,
		BuildTime:          BuildTime,
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

	// Checkpoint SQLite WAL mode to ensure 100% clean persistent disk state
	_, _ = db.SQLDB().Exec("PRAGMA wal_checkpoint(TRUNCATE);")

	eventBus.Publish(bus.NewEvent(bus.EventSystemShutdown, "kernel", nil))
	slog.Info("ActonOS daemon stopped cleanly")
}
