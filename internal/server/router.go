package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/plugin"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
	workspacepkg "github.com/actonos/actonos/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server holds all subsystem references and handles HTTP routing.
type Server struct {
	router           chi.Router
	agentMgr         *agent.AgentManager
	swarmMgr         *agent.SwarmManager
	engine           *agent.Engine
	cronSched        *agent.CronScheduler
	heartbeat        *agent.HeartbeatDaemon
	taskMgr          *agent.TaskManager
	tokenTracker     *memory.TokenTracker
	profileMgr       *agent.UserProfileManager
	llmRouter        *llm.ModelCascadeRouter
	toolReg          *tools.ToolRegistry
	mcpHost          *tools.MCPHostEngine
	approvalMgr      *tools.ApprovalManager
	runStore         *agent.RunStore
	hubMgr           *tools.HubManager
	memory           *memory.HybridEngine
	embedding        *memory.EmbeddingService
	hal              system.HAL
	tailscale        *system.TailscaleManager
	tokenDaemon      *auth.TokenRefreshDaemon
	oauthEngine      *auth.OAuthEngine
	stateStore       *auth.StateStore
	sysAuth          *auth.SystemAuthManager
	bus              *bus.EventBus
	auditLogger      *system.AuditLogger
	notifMgr         *system.NotificationManager
	vault            *memory.Vault
	pairingMgr       *channels.PairingManager
	channelMgr       *channels.ChannelManager
	skillWatcher     *tools.SkillWatcher
	pluginMgr        *plugin.Manager
	pluginHubMgr     *plugin.PluginRegistryManager
	startTime        time.Time
	dataDir          string
	workspaceDir     string
	workspaceStore   *workspacepkg.Store
	skillsDir        string
	wasmDir          string
	pluginsDir       string
	version          string
	gitCommit        string
	buildTime        string
	ota              *system.OTAEngine
	realtime         *realtimeHub
	allowMissingAuth bool
}

// Config holds configuration parameters for the HTTP server.
type Config struct {
	AgentManager        *agent.AgentManager
	SwarmManager        *agent.SwarmManager
	Engine              *agent.Engine
	CronScheduler       *agent.CronScheduler
	HeartbeatDaemon     *agent.HeartbeatDaemon
	TaskManager         *agent.TaskManager
	TokenTracker        *memory.TokenTracker
	ProfileManager      *agent.UserProfileManager
	LLMRouter           *llm.ModelCascadeRouter
	ToolRegistry        *tools.ToolRegistry
	SkillWatcher        *tools.SkillWatcher
	MCPHost             *tools.MCPHostEngine
	ApprovalManager     *tools.ApprovalManager
	RunStore            *agent.RunStore
	HubManager          *tools.HubManager
	Memory              *memory.HybridEngine
	Embedding           *memory.EmbeddingService
	HAL                 system.HAL
	Tailscale           *system.TailscaleManager
	TokenRefreshDaemon  *auth.TokenRefreshDaemon
	OAuthEngine         *auth.OAuthEngine
	StateStore          *auth.StateStore
	SystemAuth          *auth.SystemAuthManager
	NotificationManager *system.NotificationManager
	EventBus            *bus.EventBus
	AuditLogger         *system.AuditLogger
	Vault               *memory.Vault
	PairingManager      *channels.PairingManager
	ChannelManager      *channels.ChannelManager
	PluginManager       *plugin.Manager
	PluginHubManager    *plugin.PluginRegistryManager
	WorkspaceDir        string
	WorkspaceStore      *workspacepkg.Store
	SkillsDir           string
	WASMDir             string
	PluginsDir          string
	DataDir             string
	// Build metadata injected via -ldflags; see the Makefile LDFLAGS target.
	Version   string
	GitCommit string
	BuildTime string
	OTAEngine *system.OTAEngine
	// DisableAuthForTest skips RequireAuthMiddleware when SystemAuth is unset.
	// Production must leave this false.
	DisableAuthForTest bool
}

// NewServer initializes the HTTP API Server with all endpoints and middlewares.
func NewServer(cfg Config) *Server {
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	workspaceDir := cfg.WorkspaceDir
	if workspaceDir == "" {
		workspaceDir = "./data/workspace"
	}
	skillsDir := cfg.SkillsDir
	if skillsDir == "" {
		skillsDir = "./data/skills"
	}
	wasmDir := cfg.WASMDir
	if wasmDir == "" {
		wasmDir = "./data/tools/wasm"
	}
	pluginsDir := cfg.PluginsDir
	if pluginsDir == "" {
		pluginsDir = "./data/plugins"
	}
	version := cfg.Version
	if version == "" {
		version = "0.0.0-dev"
	}
	gitCommit := cfg.GitCommit
	if gitCommit == "" {
		gitCommit = "unknown"
	}
	buildTime := cfg.BuildTime
	if buildTime == "" {
		buildTime = "unspecified"
	}
	s := &Server{
		agentMgr:         cfg.AgentManager,
		swarmMgr:         cfg.SwarmManager,
		engine:           cfg.Engine,
		cronSched:        cfg.CronScheduler,
		heartbeat:        cfg.HeartbeatDaemon,
		taskMgr:          cfg.TaskManager,
		tokenTracker:     cfg.TokenTracker,
		profileMgr:       cfg.ProfileManager,
		llmRouter:        cfg.LLMRouter,
		toolReg:          cfg.ToolRegistry,
		skillWatcher:     cfg.SkillWatcher,
		mcpHost:          cfg.MCPHost,
		approvalMgr:      cfg.ApprovalManager,
		runStore:         cfg.RunStore,
		hubMgr:           cfg.HubManager,
		memory:           cfg.Memory,
		embedding:        cfg.Embedding,
		hal:              cfg.HAL,
		tailscale:        cfg.Tailscale,
		tokenDaemon:      cfg.TokenRefreshDaemon,
		oauthEngine:      cfg.OAuthEngine,
		stateStore:       cfg.StateStore,
		sysAuth:          cfg.SystemAuth,
		bus:              cfg.EventBus,
		auditLogger:      cfg.AuditLogger,
		notifMgr:         cfg.NotificationManager,
		vault:            cfg.Vault,
		pairingMgr:       cfg.PairingManager,
		channelMgr:       cfg.ChannelManager,
		pluginMgr:        cfg.PluginManager,
		pluginHubMgr:     cfg.PluginHubManager,
		startTime:        time.Now(),
		dataDir:          dataDir,
		workspaceDir:     workspaceDir,
		workspaceStore:   cfg.WorkspaceStore,
		skillsDir:        skillsDir,
		wasmDir:          wasmDir,
		pluginsDir:       pluginsDir,
		version:          version,
		gitCommit:        gitCommit,
		buildTime:        buildTime,
		ota:              cfg.OTAEngine,
		allowMissingAuth: cfg.DisableAuthForTest,
	}
	if s.ota == nil {
		s.ota = system.NewOTAEngine(dataDir)
	}
	s.ota.SetVersionMeta(version, gitCommit, buildTime)
	s.ota.SetSkipRestart(cfg.DisableAuthForTest)
	if cfg.HAL != nil {
		s.ota.SetRestarter(system.HALRestarter{HAL: cfg.HAL, Engine: s.ota})
	}
	s.realtime = newRealtimeHub(s)
	if s.engine != nil && s.taskMgr != nil {
		s.engine.SetTaskManager(s.taskMgr)
	}
	if s.toolReg != nil && s.taskMgr != nil {
		tools.AttachMissionBacklog(s.toolReg, s.taskMgr)
	}
	if s.heartbeat != nil && s.approvalMgr != nil {
		s.heartbeat.SetApprovalManager(s.approvalMgr)
	}
	if s.toolReg != nil && s.channelMgr != nil {
		s.toolReg.SetChannelSender(plugin.ChannelToolSender(s.channelMgr))
	}
	if s.pluginHubMgr == nil && s.pluginsDir != "" {
		s.pluginHubMgr = plugin.NewPluginRegistryManager(s.pluginsDir, s.pluginMgr, s.bus)
	}

	s.setupRoutes()
	return s
}

// Router returns the underlying Chi router.
func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data: blob: https:; connect-src 'self' ws: wss: https:; frame-src 'self' data: blob: https: http://localhost:* http://127.0.0.1:*; object-src 'self' data: blob:; base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		next.ServeHTTP(w, r)
	})
}

func isLongLivedHTTPPath(path string) bool {
	switch {
	case strings.Contains(path, "/chat"):
		return true
	case strings.HasSuffix(path, "/realtime"):
		return true
	case strings.Contains(path, "/terminal/"):
		return true
	default:
		return false
	}
}

func timeoutExceptLongLived(timeout time.Duration) func(http.Handler) http.Handler {
	timed := middleware.Timeout(timeout)
	return func(next http.Handler) http.Handler {
		limited := timed(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isLongLivedHTTPPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.securityHeadersMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(timeoutExceptLongLived(10 * time.Minute))

	// CORS for development and cross-origin Web UI
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:*", "http://127.0.0.1:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API Routes
	r.Route("/api", func(r chi.Router) {
		// Public Endpoints
		r.Get("/health", s.handleHealth)
		r.Get("/ready", s.handleReady)
		r.Get("/models", s.handleGetModelsCatalog)
		r.Get("/notifications/push/vapid-key", s.handleGetVAPIDPublicKey)

		// Authentication Endpoints
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", s.handleGetAuthStatus)
			r.Post("/setup", s.handleSetupAuth)
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.With(s.RequireAuthMiddleware).Put("/password", s.handleChangePassword)
		})

		// Protected Subsystems (Require valid token when initialized)
		r.Group(func(r chi.Router) {
			r.Use(s.RequireAuthMiddleware)

			r.Get("/dashboard/summary", s.handleDashboardSummary)
			r.Get("/realtime", s.handleRealtimeStream)
			r.Get("/models", s.handleGetModelsCatalog)

			// Agent Management, Soul & Cron
			r.Route("/agents", func(r chi.Router) {
				r.Get("/", s.handleListAgents)
				r.Post("/", s.handleCreateAgent)
				r.Get("/cron", s.handleListCronJobs)
				r.Post("/cron", s.handleSaveCronJob)
				r.Post("/cron/{id}/run", s.handleRunCronJob)
				r.Delete("/cron/{id}", s.handleDeleteCronJob)
				r.Get("/soul", s.handleGetSoul)
				r.Put("/soul", s.handleSaveSoul)
				r.Get("/memory-md", s.handleGetMemoryMD)
				r.Delete("/memory-md", s.handleClearMemoryMD)

				r.Route("/{agentID}", func(r chi.Router) {
					r.Get("/", s.handleGetAgent)
					r.Put("/", s.handleUpdateAgent)
					r.Delete("/", s.handleDeleteAgent)
					r.Get("/soul", s.handleGetSoul)
					r.Put("/soul", s.handleSaveSoul)
					r.Get("/memory-md", s.handleGetMemoryMD)
					r.Delete("/memory-md", s.handleClearMemoryMD)
					r.Post("/start", s.handleStartAgent)
					r.Post("/stop", s.handleStopAgent)
					r.Post("/chat", s.handleChat)
					r.Post("/chat/stream", s.handleChatStream)
				})
			})

			// Standalone Cron Route Alias
			r.Route("/cron", func(r chi.Router) {
				r.Get("/", s.handleListCronJobs)
				r.Post("/", s.handleSaveCronJob)
				r.Get("/history", s.handleListAllCronHistory)
				r.Get("/{id}/history", s.handleGetCronJobHistory)
				r.Post("/{id}/run", s.handleRunCronJob)
				r.Delete("/{id}", s.handleDeleteCronJob)
			})

			// Conversations & History
			r.Route("/conversations", func(r chi.Router) {
				r.Get("/", s.handleListConversations)
				r.Post("/", s.handleCreateConversation)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", s.handleGetConversation)
					r.Put("/", s.handleUpdateConversation)
					r.Put("/pin", s.handleTogglePinConversation)
					r.Delete("/", s.handleDeleteConversation)
				})
			})

			// Tool Hub & Skills Marketplace
			r.Route("/tools", func(r chi.Router) {
				r.Get("/", s.handleListTools)
				r.Get("/mcp", s.handleListMCPServers)
				r.Post("/mcp", s.handleConnectMCP)
				r.Delete("/mcp/{serverID}", s.handleDisconnectMCP)
				r.Put("/mcp/{serverID}", s.handleToggleMCPServer)
				r.Post("/execute", s.handleExecuteTool)
				r.Post("/skill", s.handleCreateSkill)
				r.Put("/skills/{name}/toggle", s.handleToggleSkill)
				r.Post("/wasm", s.handleUploadWASM)
				r.Get("/hub/catalog", s.handleListHubCatalog)
				r.Post("/hub/install", s.handleInstallHubSkill)
				r.Post("/hub/uninstall", s.handleUninstallHubSkill)
			})

			// WasmLoader Plugin System
			r.Route("/plugins", func(r chi.Router) {
				r.Get("/", s.handleListPlugins)
				r.Get("/available", s.handleListAvailablePlugins)
				r.Post("/install", s.handleInstallAvailablePlugin)
				r.Post("/upload", s.handleUploadPlugin)
				r.Post("/{id}/enable", s.handleEnablePlugin)
				r.Post("/{id}/disable", s.handleDisablePlugin)
				r.Delete("/{id}", s.handleDeletePlugin)
				r.Get("/{id}/logs", s.handleGetPluginLogs)
				r.Post("/{id}/config", s.handleUpdatePluginConfig)
				r.Put("/{id}/config", s.handleUpdatePluginConfig)
			})

			// Hardware-Bound Vault Secrets Management
			r.Route("/vault", func(r chi.Router) {
				r.Get("/secrets", s.handleListVaultSecrets)
				r.Post("/secrets", s.handleSetVaultSecret)
				r.Get("/secrets/{name}", s.handleGetVaultSecret)
				r.Put("/secrets/{name}", s.handleSetVaultSecret)
				r.Delete("/secrets/{name}", s.handleDeleteVaultSecret)
			})

			r.Route("/approvals", func(r chi.Router) {
				r.Get("/", s.handleListApprovals)
				r.Post("/{id}/approve", s.handleApproveAction)
				r.Post("/{id}/reject", s.handleRejectAction)
			})

			r.Route("/runs", func(r chi.Router) {
				r.Get("/", s.handleListAgentRuns)
				r.Get("/{id}", s.handleGetAgentRun)
				r.Get("/{id}/events", s.handleListRunEvents)
				r.Post("/{id}/cancel", s.handleCancelAgentRun)
			})

			// Onboarding & Setup
			r.Route("/setup", func(r chi.Router) {
				r.Get("/status", s.handleGetSetupStatus)
				r.Post("/wizard", s.handleSetupWizard)
			})

			// Channel Accounts & Device Pairing
			r.Route("/integrations", func(r chi.Router) {
				r.Get("/channels", s.handleGetChannels)
				r.Get("/channels/accounts", s.handleListAllChannelAccounts)
				r.Post("/channels", s.handleSaveChannels)
				r.Post("/pairing/code", s.handleGeneratePairingCode)
				r.Post("/pairing/verify", s.handleVerifyPairingCode)
				r.Get("/pairing/codes", s.handleListPairingCodes)
				r.Get("/pairing/pending", s.handleListPendingPairing)
				r.Get("/pairing/policy", s.handleGetPairingPolicies)
				r.Post("/pairing/policy", s.handleSetPairingPolicy)
				r.Post("/pairing/allow", s.handleAllowPairingSender)
				r.Get("/authorizations", s.handleListAuthorizations)
				r.Delete("/authorizations", s.handleRevokeAuthorization)
			})

			// Workspace File Manager
			r.Route("/workspace", func(r chi.Router) {
				r.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						next.ServeHTTP(w, r.WithContext(tools.WithBypassApproval(r.Context())))
					})
				})
				r.Get("/files", s.handleDBListWorkspaceFiles)
				r.Get("/file", s.handleDBGetWorkspaceFile)
				r.Post("/file", s.handleDBSaveWorkspaceFile)
				r.Delete("/file", s.handleDBDeleteWorkspaceFile)
				r.Post("/mkdir", s.handleDBMkdirWorkspace)
				r.Post("/upload", s.handleDBUploadWorkspaceFile)
				r.Get("/raw", s.handleDBRawWorkspaceFile)
				r.Post("/rename", s.handleDBRenameWorkspaceFile)
				r.Post("/duplicate", s.handleDBDuplicateWorkspaceFile)
				r.Get("/stats", s.handleDBGetWorkspaceStats)
				r.Get("/zip", s.handleDBDownloadWorkspaceZip)
				r.Post("/reindex", s.handleDBReindexWorkspaceFile)
				r.Get("/chunks", s.handleDBGetFileEmbeddingChunks)
			})

			// Autonomous Tasks & Operations Backlog
			r.Route("/tasks", func(r chi.Router) {
				r.Get("/", s.handleListTasks)
				r.Post("/", s.handleCreateTask)
				r.Get("/{id}", s.handleGetTask)
				r.Put("/{id}", s.handleUpdateTask)
				r.Delete("/{id}", s.handleDeleteTask)
			})

			// Autonomous Heartbeat Coordinator
			r.Route("/heartbeat", func(r chi.Router) {
				r.Get("/config", s.handleGetHeartbeatConfig)
				r.Put("/config", s.handleSaveHeartbeatConfig)
				r.Post("/trigger", s.handleTriggerHeartbeatPulse)
				r.Get("/runs", s.handleListHeartbeatRuns)
			})

			// Notification Center
			r.Route("/notifications", func(r chi.Router) {
				r.Get("/", s.handleListNotifications)
				r.Get("/unread-count", s.handleGetUnreadNotificationsCount)
				r.Post("/mark-read", s.handleMarkNotificationRead)
				r.Delete("/", s.handleDeleteNotifications)
				r.Get("/push/vapid-key", s.handleGetVAPIDPublicKey)
				r.Post("/push/subscribe", s.handleSubscribePush)
				r.Post("/push/unsubscribe", s.handleUnsubscribePush)
				r.Post("/push/test", s.handleTestPushNotification)
			})

			// System, HAL, Keys, Identity, Audit & Tailscale
			r.Route("/system", func(r chi.Router) {
				r.Get("/metrics", s.handleGetMetrics)
				r.Get("/metrics/prometheus", s.handlePrometheusMetrics)
				r.Get("/models", s.handleGetModelsCatalog)
				r.Get("/token-usage", s.handleGetTokenUsage)
				r.Get("/token-usage/history", s.handleGetTokenHistory)
				r.Get("/heartbeat/history", s.handleGetHeartbeatHistory)
				r.Get("/identity", s.handleGetIdentity)
				r.Put("/identity", s.handleSaveIdentity)
				r.Get("/profile", s.handleGetIdentity)
				r.Put("/profile", s.handleSaveIdentity)
				r.Get("/keys", s.handleGetAPIKeys)
				r.Post("/keys", s.handleSaveAPIKeys)
				r.Delete("/keys/{provider}", s.handleDeleteAPIKey)
				r.Post("/keys/test", s.handleTestAPIKey)
				r.Get("/audit", s.handleGetAuditLogs)
				r.Get("/audit/verify", s.handleVerifyAuditChain)
				r.Get("/storage", s.handleGetStorageInfo)
				r.Get("/embedding", s.handleGetEmbeddingStatus)
				r.Get("/backup", s.handleGetBackup)
				r.Post("/ota/check", s.handleCheckOTA)
				r.Get("/ota/status", s.handleOTAStatus)
				r.Post("/ota/apply", s.handleApplyOTA)
				r.Post("/ota/rollback", s.handleRollbackOTA)
				r.Get("/tailscale", s.handleGetTailscale)
				r.Get("/wifi/scan", s.handleWifiScan)
				r.Post("/wifi/connect", s.handleWifiConnect)
				r.Post("/restart", s.handleRestart)
			})

			// Interactive Web Terminal
			r.Route("/terminal", func(r chi.Router) {
				r.Get("/info", s.handleTerminalInfo)
				r.Get("/ws", s.handleTerminalWebSocket)
			})
		})
	})

	s.router = r
}

// Helper: JSON Success Response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

// Helper: JSON Error Response
func (s *Server) respondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// Helper: Decode JSON request body
func (s *Server) decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB limit
	return json.NewDecoder(r.Body).Decode(v)
}
