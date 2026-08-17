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

	// 6. Initialize LLM Provider Router & Load Configured Keys
	llmRouter := llm.NewModelCascadeRouter()
	configDir := filepath.Join(*dataDir, "config")
	_ = os.MkdirAll(configDir, 0755)

	readKey := func(file string) string {
		data, _ := os.ReadFile(filepath.Join(configDir, file))
		return strings.TrimSpace(string(data))
	}

	server.RegisterAllStoredProviders(llmRouter, configDir)

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

	profileMgr, err := agent.NewUserProfileManager(db, *dataDir)
	if err != nil {
		slog.Warn("failed to initialize user profile manager", "error", err)
	}

	// 9. Initialize Swarm Manager, ReAct Engine & Proactive Cron Scheduler
	swarmMgr := agent.NewSwarmManager(agentMgr, eventBus, llmRouter, hybridEngine, 8)
	engine := agent.NewEngine(agentMgr, eventBus, llmRouter, hybridEngine)
	engine.SetToolRegistry(toolReg)
	if profileMgr != nil {
		engine.SetProfileManager(profileMgr)
	}

	cronSched := agent.NewCronScheduler(engine, eventBus, db.SQLDB())
	tools.AttachCronScheduler(toolReg, cronSched)
	cronSched.Start(ctx)
	defer cronSched.Stop()

	// 10. Initialize Zero-Trust Channel Pairing & Multi-Channel Adapters
	pairingMgr, err := channels.NewPairingManager(db.SQLDB())
	if err != nil {
		slog.Warn("failed to initialize pairing manager", "error", err)
	}

	tgToken := readKey("telegram.token")
	tgAdapter := channels.NewTelegramAdapter(tgToken, eventBus, pairingMgr)
	if err := tgAdapter.Start(ctx); err != nil {
		slog.Warn("failed to start telegram adapter", "error", err)
	}
	defer tgAdapter.Stop()

	waToken := readKey("whatsapp.token")
	waPhone := readKey("whatsapp.phone_id")
	waAdapter := channels.NewWhatsAppAdapter(waToken, waPhone, "acton_verify_token", eventBus, pairingMgr)

	// Multi-Channel Cognitive Session Manager
	sessionMgr := channels.NewChannelSessionManager(db.SQLDB())

	// Set Default Recipient Resolver for Proactive Schedulers
	cronSched.SetDefaultRecipientGetter(func(channel string) string {
		if channel == "telegram" && tgAdapter != nil {
			if lastID := tgAdapter.GetLastChatID(); lastID != "" {
				return lastID
			}
			known := tgAdapter.GetKnownChatIDs()
			if len(known) > 0 {
				return known[0]
			}
			if pairingMgr != nil {
				paired := pairingMgr.ListAuthorized("telegram")
				if len(paired) > 0 {
					return paired[0].SenderID
				}
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
				if ev.AgentID == "telegram" || ev.AgentID == "whatsapp" {
					if inMsg, ok := ev.Payload.(channels.InboundMessage); ok {
						go func(msg channels.InboundMessage) {
							target := "agent_system_core"
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
								chatMeta = fmt.Sprintf("[Channel: %s | User Chat ID: %s | Sender: %s]\n", msg.ChannelID, msg.Metadata["chat_id"], msg.SenderName)
							} else if msg.ChannelID != "" {
								chatMeta = fmt.Sprintf("[Channel: %s | Sender ID: %s]\n", msg.ChannelID, msg.SenderID)
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

							// 7. Deliver outbound response to channel
							if msg.ChannelID == "telegram" && senderID != "" {
								_ = tgAdapter.SendMessage(context.Background(), channels.OutboundMessage{
									ChannelID: "telegram",
									Recipient: senderID,
									Content:   resp.Content,
								})
							} else if msg.ChannelID == "whatsapp" {
								_ = waAdapter.SendMessage(context.Background(), channels.OutboundMessage{
									ChannelID: "whatsapp",
									Recipient: msg.SenderID,
									Content:   resp.Content,
								})
							}
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
						targetRec, _ := payloadMap["target_recipient"].(string)

						if targetChan == "" {
							targetChan = "telegram"
						}

						msgText := fmt.Sprintf("⏰ **[Cron Reminder: %s]**\n\n%s", jobName, content)

						// Route to Telegram
						if targetChan == "telegram" || targetChan == "all" {
							recipients := []string{}
							if targetRec != "" {
								recipients = append(recipients, targetRec)
							} else if lastID := tgAdapter.GetLastChatID(); lastID != "" {
								recipients = append(recipients, lastID)
							} else {
								recipients = tgAdapter.GetKnownChatIDs()
							}

							for _, rec := range recipients {
								_ = tgAdapter.SendMessage(context.Background(), channels.OutboundMessage{
									ChannelID: "telegram",
									Recipient: rec,
									Content:   msgText,
								})
							}
						}

						// Route to WhatsApp
						if (targetChan == "whatsapp" || targetChan == "all") && targetRec != "" {
							_ = waAdapter.SendMessage(context.Background(), channels.OutboundMessage{
								ChannelID: "whatsapp",
								Recipient: targetRec,
								Content:   fmt.Sprintf("⏰ *[Cron Reminder: %s]*\n\n%s", jobName, content),
							})
						}
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
		ProfileManager:     profileMgr,
		LLMRouter:          llmRouter,
		ToolRegistry:       toolReg,
		MCPHost:            mcpHost,
		HubManager:         hubMgr,
		Memory:             hybridEngine,
		HAL:                hal,
		Tailscale:          tailscaleMgr,
		TokenRefreshDaemon: tokenDaemon,
		OAuthEngine:        oauthEngine,
		StateStore:         stateStore,
		SystemAuth:         sysAuth,
		EventBus:           eventBus,
		PairingManager:     pairingMgr,
		TelegramAdapter:    tgAdapter,
		WhatsAppAdapter:    waAdapter,
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
