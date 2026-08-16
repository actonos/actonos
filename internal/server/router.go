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
	router         chi.Router
	agentMgr       *agent.AgentManager
	swarmMgr       *agent.SwarmManager
	engine         *agent.Engine
	cronSched      *agent.CronScheduler
	profileMgr     *agent.UserProfileManager
	llmRouter      *llm.ModelCascadeRouter
	toolReg        *tools.ToolRegistry
	mcpHost        *tools.MCPHostEngine
	hubMgr         *tools.HubManager
	memory         *memory.HybridEngine
	hal            system.HAL
	tailscale      *system.TailscaleManager
	tokenDaemon    *auth.TokenRefreshDaemon
	bus            *bus.EventBus
	pairingMgr     *channels.PairingManager
	tgAdapter      *channels.TelegramAdapter
	waAdapter      *channels.WhatsAppAdapter
	startTime      time.Time
}

// Config holds configuration parameters for the HTTP server.
type Config struct {
	AgentManager       *agent.AgentManager
	SwarmManager       *agent.SwarmManager
	Engine             *agent.Engine
	CronScheduler      *agent.CronScheduler
	ProfileManager     *agent.UserProfileManager
	LLMRouter          *llm.ModelCascadeRouter
	ToolRegistry       *tools.ToolRegistry
	MCPHost            *tools.MCPHostEngine
	HubManager         *tools.HubManager
	Memory             *memory.HybridEngine
	HAL                system.HAL
	Tailscale          *system.TailscaleManager
	TokenRefreshDaemon *auth.TokenRefreshDaemon
	EventBus           *bus.EventBus
	PairingManager     *channels.PairingManager
	TelegramAdapter    *channels.TelegramAdapter
	WhatsAppAdapter    *channels.WhatsAppAdapter
}

// NewServer initializes the HTTP API Server with all endpoints and middlewares.
func NewServer(cfg Config) *Server {
	s := &Server{
		agentMgr:    cfg.AgentManager,
		swarmMgr:    cfg.SwarmManager,
		engine:      cfg.Engine,
		cronSched:   cfg.CronScheduler,
		profileMgr:  cfg.ProfileManager,
		llmRouter:   cfg.LLMRouter,
		toolReg:     cfg.ToolRegistry,
		mcpHost:     cfg.MCPHost,
		hubMgr:      cfg.HubManager,
		memory:      cfg.Memory,
		hal:         cfg.HAL,
		tailscale:   cfg.Tailscale,
		tokenDaemon: cfg.TokenRefreshDaemon,
		bus:         cfg.EventBus,
		pairingMgr:  cfg.PairingManager,
		tgAdapter:   cfg.TelegramAdapter,
		waAdapter:   cfg.WhatsAppAdapter,
		startTime:   time.Now(),
	}

	s.setupRoutes()
	return s
}

// Router returns the underlying Chi router.
func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS for development and cross-origin Web UI
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API Routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/dashboard/summary", s.handleDashboardSummary)

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
				r.Post("/start", s.handleStartAgent)
				r.Post("/stop", s.handleStopAgent)
				r.Post("/chat", s.handleChat)
			})
		})

		// Standalone Cron Route Alias
		r.Route("/cron", func(r chi.Router) {
			r.Get("/", s.handleListCronJobs)
			r.Post("/", s.handleSaveCronJob)
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
			r.Post("/mcp", s.handleConnectMCP)
			r.Delete("/mcp/{serverID}", s.handleDisconnectMCP)
			r.Post("/execute", s.handleExecuteTool)
			r.Post("/skill", s.handleCreateSkill)
			r.Post("/wasm", s.handleUploadWASM)
			r.Get("/hub/catalog", s.handleListHubCatalog)
			r.Post("/hub/install", s.handleInstallHubSkill)
			r.Post("/hub/uninstall", s.handleUninstallHubSkill)
		})

		// Onboarding & Setup
		r.Route("/setup", func(r chi.Router) {
			r.Get("/status", s.handleGetSetupStatus)
			r.Post("/wizard", s.handleSetupWizard)
		})

		// SaaS Integrations & Channel Adapters & Pairing
		r.Route("/integrations", func(r chi.Router) {
			r.Get("/", s.handleListIntegrations)
			r.Post("/{provider}/auth-url", s.handleGetAuthURL)
			r.Post("/{provider}/toggle", s.handleToggleIntegration)
			r.Get("/channels", s.handleGetChannels)
			r.Post("/channels", s.handleSaveChannels)
			r.Post("/pairing/code", s.handleGeneratePairingCode)
			r.Post("/pairing/verify", s.handleVerifyPairingCode)
			r.Get("/authorizations", s.handleListAuthorizations)
			r.Delete("/authorizations", s.handleRevokeAuthorization)
		})

		// Webhooks (WhatsApp, Generic)
		r.Route("/webhooks", func(r chi.Router) {
			r.Get("/whatsapp", s.handleWhatsAppVerifyWebhook)
			r.Post("/whatsapp", s.handleWhatsAppInboundWebhook)
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

		// System, HAL, Keys, Audit & Tailscale
		r.Route("/system", func(r chi.Router) {
			r.Get("/metrics", s.handleGetMetrics)
			r.Get("/keys", s.handleGetAPIKeys)
			r.Post("/keys", s.handleSaveAPIKeys)
			r.Post("/keys/test", s.handleTestAPIKey)
			r.Get("/audit", s.handleGetAuditLogs)
			r.Get("/storage", s.handleGetStorageInfo)
			r.Get("/backup", s.handleGetBackup)
			r.Post("/ota/check", s.handleCheckOTA)
			r.Get("/tailscale", s.handleGetTailscale)
			r.Get("/wifi/scan", s.handleWifiScan)
			r.Post("/wifi/connect", s.handleWifiConnect)
			r.Post("/restart", s.handleRestart)
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
