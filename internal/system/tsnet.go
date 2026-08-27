package system

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

// TailscaleStatus represents live connection details of the embedded node.
type TailscaleStatus struct {
	Connected   bool     `json:"connected"`
	IP          string   `json:"ip,omitempty"`
	FQDN        string   `json:"fqdn,omitempty"`
	Hostname    string   `json:"hostname"`
	PeersCount  int      `json:"peers_count"`
	AuthKeySet  bool     `json:"auth_key_set"`
}

// TailscaleManager manages the embedded tsnet.Server.
type TailscaleManager struct {
	mu       sync.RWMutex
	server   *tsnet.Server
	hostname string
	authKey  string
	stateDir string
	enabled  bool
	started  bool
}

// NewTailscaleManager creates a new TailscaleManager.
func NewTailscaleManager(dataDir, hostname, authKey string) *TailscaleManager {
	if hostname == "" {
		hostname = "acton-mini"
	}
	if authKey == "" {
		authKey = os.Getenv("TAILSCALE_AUTHKEY")
		if authKey == "" {
			authKey = os.Getenv("TAILSCALE_AUTH_KEY")
		}
	}

	stateDir := filepath.Join(dataDir, "config", "tsnet")
	_ = os.MkdirAll(stateDir, 0700)

	enabled := os.Getenv("DISABLE_TAILSCALE") != "true"

	return &TailscaleManager{
		hostname: hostname,
		authKey:  authKey,
		stateDir: stateDir,
		enabled:  enabled,
	}
}

// Start initializes and starts the embedded tsnet server if enabled.
func (m *TailscaleManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		slog.Info("tailscale remote access is disabled via configuration")
		return nil
	}

	srv := &tsnet.Server{
		Hostname: m.hostname,
		AuthKey:  m.authKey,
		Dir:      m.stateDir,
		Logf: func(format string, args ...any) {
			slog.Debug(fmt.Sprintf("[tsnet] "+format, args...))
		},
	}

	m.server = srv
	m.started = true

	slog.Info("embedded tailscale tsnet server initialized",
		"hostname", m.hostname,
		"state_dir", m.stateDir,
	)

	return nil
}

// Listen creates a network listener on the Tailscale mesh network.
func (m *TailscaleManager) Listen(network, addr string) (net.Listener, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.enabled || m.server == nil {
		return nil, fmt.Errorf("tailscale is not enabled or running")
	}

	return m.server.Listen(network, addr)
}

// GetStatus returns the current connection state.
func (m *TailscaleManager) GetStatus(ctx context.Context) (*TailscaleStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &TailscaleStatus{
		Connected:  false,
		Hostname:   m.hostname,
		AuthKeySet: m.authKey != "",
	}

	if !m.enabled || m.server == nil {
		return status, nil
	}

	lc, err := m.server.LocalClient()
	if err != nil {
		return status, nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	st, err := lc.Status(ctxTimeout)
	if err != nil {
		return status, nil
	}

	if st.Self != nil && len(st.TailscaleIPs) > 0 {
		status.Connected = true
		status.IP = st.TailscaleIPs[0].String()
		status.FQDN = st.Self.DNSName
		status.PeersCount = len(st.Peer)
	}

	return status, nil
}

// Close gracefully stops the tsnet server.
func (m *TailscaleManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil && m.started {
		defer func() {
			if r := recover(); r != nil {
				slog.Debug("recovered from tsnet close panic", "recover", r)
			}
		}()
		err := m.server.Close()
		m.server = nil
		m.started = false
		return err
	}
	return nil
}
