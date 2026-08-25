package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/actonos/actonos/internal/security"
)

var (
	ErrPermissionDenied     = errors.New("plugin permission denied")
	ErrDomainNotWhitelisted = errors.New("outbound network domain not whitelisted in plugin manifest")
	ErrSecretUnauthorized   = errors.New("secret access not authorized in plugin manifest")
	ErrStorageDisabled      = errors.New("persistent storage is not enabled for this plugin")
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

// CheckOutboundURL verifies whitelist membership and rejects SSRF targets.
func (g *SecurityGate) CheckOutboundURL(rawURL string) error {
	return g.CheckOutboundURLContext(context.Background(), rawURL)
}

// CheckOutboundURLContext is CheckOutboundURL with a resolver context.
func (g *SecurityGate) CheckOutboundURLContext(ctx context.Context, rawURL string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid url %q: %v", ErrDomainNotWhitelisted, rawURL, err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("%w: missing host in url %q", ErrDomainNotWhitelisted, rawURL)
	}

	g.mu.RLock()
	patterns := append([]string(nil), g.manifest.Permissions.NetOutbound...)
	g.mu.RUnlock()

	allowed := false
	for _, pattern := range patterns {
		if matchDomain(pattern, host) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("%w: host %q is not permitted by manifest net_outbound %v",
			ErrDomainNotWhitelisted, host, patterns)
	}

	if err := security.ValidateOutboundURL(ctx, rawURL); err != nil {
		return fmt.Errorf("%w: %v", ErrDomainNotWhitelisted, err)
	}
	return nil
}

// CheckSecretAccess verifies if the plugin is authorized to retrieve the given secret key.
func (g *SecurityGate) CheckSecretAccess(secretKey string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, allowed := range g.manifest.Permissions.Secrets {
		if allowed == "*" || allowed == secretKey {
			return nil
		}
		if strings.HasSuffix(allowed, ".*") {
			prefix := strings.TrimSuffix(allowed, ".*")
			if strings.HasPrefix(secretKey, prefix+".") || secretKey == prefix {
				return nil
			}
		}
		if strings.HasSuffix(allowed, "*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(secretKey, prefix) {
				return nil
			}
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
		if strings.HasSuffix(allowed, ".*") {
			prefix := strings.TrimSuffix(allowed, ".*")
			if strings.HasPrefix(topic, prefix+".") || topic == prefix {
				return nil
			}
		}
		if strings.HasSuffix(allowed, "*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(topic, prefix) {
				return nil
			}
		}
	}

	return fmt.Errorf("%w: topic %q is not allowed in manifest bus_events", ErrBusEventUnauthorized, topic)
}

// matchDomain checks if a host matches a pattern (exact or *.example.com).
// A bare "*" or a one-label wildcard (*.com) is never a match.
func matchDomain(pattern, host string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	h := strings.ToLower(strings.TrimSpace(host))
	if p == "" || p == "*" || h == "" {
		return false
	}
	if strings.Contains(p, "://") {
		if u, err := url.Parse(p); err == nil {
			if parsedHost := strings.ToLower(u.Hostname()); parsedHost != "" {
				p = parsedHost
			}
		}
	}
	if p == h {
		return true
	}
	if strings.HasPrefix(p, "*.") {
		suffix := p[2:]
		if suffix == "" || !strings.Contains(suffix, ".") {
			return false
		}
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}
	return false
}
