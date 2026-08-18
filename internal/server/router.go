package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/system"
	"github.com/actonos/actonos/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server holds all subsystem references and handles HTTP routing.
type Server struct {
	router       chi.Router
	agentMgr     *agent.AgentManager
	swarmMgr     *agent.SwarmManager
	engine       *agent.Engine
	cronSched    *agent.CronScheduler
	heartbeat    *agent.HeartbeatDaemon
	taskMgr      *agent.TaskManager
	tokenTracker *memory.TokenTracker
	profileMgr   *agent.UserProfileManager
	llmRouter    *llm.ModelCascadeRouter
	toolReg      *tools.ToolRegistry
	mcpHost      *tools.MCPHostEngine
	approvalMgr  *tools.ApprovalManager
	runStore     *agent.RunStore
	hubMgr       *tools.HubManager
	memory       *memory.HybridEngine
	hal          system.HAL
	tailscale    *system.TailscaleManager
	tokenDaemon  *auth.TokenRefreshDaemon
	oauthEngine  *auth.OAuthEngine
	stateStore   *auth.StateStore
	sysAuth      *auth.SystemAuthManager
	bus          *bus.EventBus
	auditLogger  *system.AuditLogger
	vault        *memory.Vault
	pairingMgr   *channels.PairingManager
	channelMgr   *channels.ChannelManager
	tgAdapter    *channels.TelegramAdapter
	waAdapter    *channels.WhatsAppAdapter
	startTime    time.Time
	dataDir      string
	workspaceDir string
	skillsDir    string
	wasmDir      string
	realtime     *realtimeHub
}

// Config holds configuration parameters for the HTTP server.
type Config struct {
	AgentManager       *agent.AgentManager
	SwarmManager       *agent.SwarmManager
	Engine             *agent.Engine
	CronScheduler      *agent.CronScheduler
	HeartbeatDaemon    *agent.HeartbeatDaemon
	TaskManager        *agent.TaskManager
	TokenTracker       *memory.TokenTracker
	ProfileManager     *agent.UserProfileManager
	LLMRouter          *llm.ModelCascadeRouter
	ToolRegistry       *tools.ToolRegistry
	MCPHost            *tools.MCPHostEngine
	ApprovalManager    *tools.ApprovalManager
	RunStore           *agent.RunStore
	HubManager         *tools.HubManager
	Memory             *memory.HybridEngine
	HAL                system.HAL
	Tailscale          *system.TailscaleManager
	TokenRefreshDaemon *auth.TokenRefreshDaemon
	OAuthEngine        *auth.OAuthEngine
	StateStore         *auth.StateStore
	SystemAuth         *auth.SystemAuthManager
	EventBus           *bus.EventBus
	AuditLogger        *system.AuditLogger
	Vault              *memory.Vault
	PairingManager     *channels.PairingManager
	ChannelManager     *channels.ChannelManager
	TelegramAdapter    *channels.TelegramAdapter
	WhatsAppAdapter    *channels.WhatsAppAdapter
	WorkspaceDir       string
	SkillsDir          string
	WASMDir            string
	DataDir            string
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
	s := &Server{
		agentMgr:     cfg.AgentManager,
		swarmMgr:     cfg.SwarmManager,
		engine:       cfg.Engine,
		cronSched:    cfg.CronScheduler,
		heartbeat:    cfg.HeartbeatDaemon,
		taskMgr:      cfg.TaskManager,
		tokenTracker: cfg.TokenTracker,
		profileMgr:   cfg.ProfileManager,
		llmRouter:    cfg.LLMRouter,
		toolReg:      cfg.ToolRegistry,
		mcpHost:      cfg.MCPHost,
		approvalMgr:  cfg.ApprovalManager,
		runStore:     cfg.RunStore,
		hubMgr:       cfg.HubManager,
		memory:       cfg.Memory,
		hal:          cfg.HAL,
		tailscale:    cfg.Tailscale,
		tokenDaemon:  cfg.TokenRefreshDaemon,
		oauthEngine:  cfg.OAuthEngine,
		stateStore:   cfg.StateStore,
		sysAuth:      cfg.SystemAuth,
		bus:          cfg.EventBus,
		auditLogger:  cfg.AuditLogger,
		vault:        cfg.Vault,
		pairingMgr:   cfg.PairingManager,
		channelMgr:   cfg.ChannelManager,
		tgAdapter:    cfg.TelegramAdapter,
		waAdapter:    cfg.WhatsAppAdapter,
		startTime:    time.Now(),
		dataDir:      dataDir,
		workspaceDir: workspaceDir,
		skillsDir:    skillsDir,
		wasmDir:      wasmDir,
	}
	s.realtime = newRealtimeHub(s)

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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data: blob: https:; connect-src 'self' ws: wss: https:; frame-src 'self' https: http://localhost:* http://127.0.0.1:*; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.securityHeadersMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

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
		r.Get("/models", s.handleGetModelsCatalog)

		// Authentication Endpoints
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", s.handleGetAuthStatus)
			r.Post("/setup", s.handleSetupAuth)
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.With(s.RequireAuthMiddleware).Put("/password", s.handleChangePassword)
		})

		// OAuth Callbacks
		r.Get("/auth/callback", s.handleOAuthCallback)

		// Webhooks (WhatsApp, Generic)
		r.Route("/webhooks", func(r chi.Router) {
			r.Get("/whatsapp", s.handleWhatsAppVerifyWebhook)
			r.Post("/whatsapp", s.handleWhatsAppInboundWebhook)
		})

		// Protected Subsystems (Require valid token when initialized)
		r.Group(func(r chi.Router) {
			r.Use(s.RequireAuthMiddleware)

			r.Get("/dashboard/summary", s.handleDashboardSummary)
			r.Get("/realtime", s.handleRealtimeStream)

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

				r.Route("/{agentID}", func(r chi.Router) {
					r.Get("/", s.handleGetAgent)
					r.Put("/", s.handleUpdateAgent)
					r.Delete("/", s.handleDeleteAgent)
					r.Get("/soul", s.handleGetSoul)
					r.Put("/soul", s.handleSaveSoul)
					r.Get("/memory-md", s.handleGetMemoryMD)
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
				r.Post("/wasm", s.handleUploadWASM)
				r.Get("/hub/catalog", s.handleListHubCatalog)
				r.Post("/hub/install", s.handleInstallHubSkill)
				r.Post("/hub/uninstall", s.handleUninstallHubSkill)
			})

			r.Route("/approvals", func(r chi.Router) {
				r.Get("/", s.handleListApprovals)
				r.Post("/{id}/approve", s.handleApproveAction)
				r.Post("/{id}/reject", s.handleRejectAction)
			})

			r.Route("/runs", func(r chi.Router) {
				r.Get("/", s.handleListAgentRuns)
				r.Get("/{id}/events", s.handleListRunEvents)
			})

			// Onboarding & Setup
			r.Route("/setup", func(r chi.Router) {
				r.Get("/status", s.handleGetSetupStatus)
				r.Post("/wizard", s.handleSetupWizard)
			})

			// SaaS Integrations & Connectors & Channel Adapters & Pairing
			r.Route("/integrations", func(r chi.Router) {
				r.Get("/", s.handleListIntegrations)
				r.Get("/oauth/callback", s.handleOAuthCallback)
				r.Post("/{provider}/auth-url", s.handleGetAuthURL)
				r.Post("/{provider}/token", s.handleSaveDirectToken)
				r.Post("/{provider}/config", s.handleSaveProviderConfig)
				r.Post("/{provider}/test", s.handleTestIntegration)
				r.Post("/{provider}/disconnect", s.handleDisconnectIntegration)
				r.Post("/{provider}/toggle", s.handleToggleIntegration)
				r.Get("/channels", s.handleGetChannels)
				r.Get("/channels/accounts", s.handleListAllChannelAccounts)
				r.Post("/channels", s.handleSaveChannels)
				r.Post("/pairing/code", s.handleGeneratePairingCode)
				r.Post("/pairing/verify", s.handleVerifyPairingCode)
				r.Get("/authorizations", s.handleListAuthorizations)
				r.Delete("/authorizations", s.handleRevokeAuthorization)
			})

			// Workspace File Manager
			r.Route("/workspace", func(r chi.Router) {
				r.Get("/files", s.handleListWorkspaceFiles)
				r.Get("/file", s.handleGetWorkspaceFile)
				r.Post("/file", s.handleSaveWorkspaceFile)
				r.Delete("/file", s.handleDeleteWorkspaceFile)
				r.Post("/mkdir", s.handleMkdirWorkspace)
				r.Post("/upload", s.handleUploadWorkspaceFile)
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
				r.Get("/backup", s.handleGetBackup)
				r.Post("/ota/check", s.handleCheckOTA)
				r.Get("/tailscale", s.handleGetTailscale)
				r.Get("/wifi/scan", s.handleWifiScan)
				r.Post("/wifi/connect", s.handleWifiConnect)
				r.Post("/restart", s.handleRestart)
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	runtimeMode := "docker"
	if s.hal != nil {
		runtimeMode = s.hal.RuntimeMode()
	}

	activeAgents := 0
	if s.agentMgr != nil {
		agents, _ := s.agentMgr.List(r.Context())
		for _, a := range agents {
			if a.Status == agent.StatusActive {
				activeAgents++
			}
		}
	}

	tailscaleConnected := false
	tailscaleIP := ""
	if s.tailscale != nil {
		st, _ := s.tailscale.GetStatus(r.Context())
		if st != nil {
			tailscaleConnected = st.Connected
			tailscaleIP = st.IP
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":              "healthy",
		"version":             "0.1.0",
		"uptime_seconds":      uint64(time.Since(s.startTime).Seconds()),
		"runtime_mode":        runtimeMode,
		"agents_active":       activeAgents,
		"tailscale_connected": tailscaleConnected,
		"tailscale_ip":        tailscaleIP,
	})
}
