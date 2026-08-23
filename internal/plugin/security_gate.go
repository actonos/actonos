package plugin

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

var (
	ErrPermissionDenied   = errors.New("plugin permission denied")
	ErrDomainNotWhitelisted = errors.New("outbound network domain not whitelisted in plugin manifest")
	ErrSecretUnauthorized = errors.New("secret access not authorized in plugin manifest")
	ErrStorageDisabled    = errors.New("persistent storage is not enabled for this plugin")
	ErrBusEventUnauthorized = errors.New("bus event emission not authorized for topic")
)

// SecurityGate enforces sandboxing invariants and capability permissions for a plugin.
type SecurityGate struct {
	manifest PluginManifest
	mu       sync.RWMutex
}

// NewSecurityGate creates a SecurityGate bound to a plugin's manifest.
func NewSecurityGate(manifest PluginManifest) *SecurityGate {
	return &SecurityGate{
		manifest: manifest,
	}
}

// CheckOutboundURL verifies if a target URL's host is permitted by the plugin's net_outbound whitelist.
func (g *SecurityGate) CheckOutboundURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid url %q: %v", ErrDomainNotWhitelisted, rawURL, err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("%w: missing host in url %q", ErrDomainNotWhitelisted, rawURL)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, pattern := range g.manifest.Permissions.NetOutbound {
		if matchDomain(pattern, host) {
			return nil
		}
	}

	return fmt.Errorf("%w: host %q is not permitted by manifest net_outbound %v",
		ErrDomainNotWhitelisted, host, g.manifest.Permissions.NetOutbound)
}

// CheckSecretAccess verifies if the plugin is authorized to retrieve the given secret key.
func (g *SecurityGate) CheckSecretAccess(secretKey string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, allowed := range g.manifest.Permissions.Secrets {
		if allowed == "*" || allowed == secretKey {
			return nil
		}
	}

	return fmt.Errorf("%w: secret %q is not authorized in manifest", ErrSecretUnauthorized, secretKey)
}

// CheckStorageAccess verifies if the plugin has permission to use scoped KV storage.
func (g *SecurityGate) CheckStorageAccess() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.manifest.Permissions.Storage {
		return ErrStorageDisabled
	}
	return nil
}

// CheckBusEvent verifies if the plugin is permitted to emit the specified event topic.
func (g *SecurityGate) CheckBusEvent(topic string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, allowed := range g.manifest.Permissions.BusEvents {
		if allowed == "*" || allowed == topic {
			return nil
		}
	}

	return fmt.Errorf("%w: topic %q is not allowed in manifest bus_events", ErrBusEventUnauthorized, topic)
}

// matchDomain checks if a host matches a pattern (supports exact match or wildcard prefix like *.slack.com).
func matchDomain(pattern, host string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	h := strings.ToLower(strings.TrimSpace(host))

	if p == "*" || p == h {
		return true
	}

	if strings.HasPrefix(p, "*.") {
		suffix := p[2:]
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}

	return false
}
