package security

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ErrUnsafeAddress is returned when a host resolves to, or is, a local or private address.
var ErrUnsafeAddress = errors.New("the address is local or private")

// DialPolicy controls SafeDial. The zero value is the production policy: only public
// addresses, the system resolver, the system trust store.
type DialPolicy struct {
	// Resolve overrides the resolver (tests).
	Resolve func(ctx context.Context, host string) ([]net.IP, error)
	// AllowLoopback lets 127.0.0.0/8 and ::1 through. Tests only; never set it in production code.
	AllowLoopback bool
	// TLS is the base TLS configuration (tests inject a root CA). ServerName is always set to the host.
	TLS *tls.Config
	// ConnectTimeout bounds resolution, dialing and the handshake together. Default 10 s.
	ConnectTimeout time.Duration
}

func (p DialPolicy) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if p.Resolve != nil {
		return p.Resolve(ctx, host)
	}
	addrs, err := lookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IP, len(addrs))
	for i, a := range addrs {
		out[i] = a.IP
	}
	return out, nil
}

func (p DialPolicy) unsafe(ip net.IP) bool {
	if p.AllowLoopback && ip.IsLoopback() {
		return false
	}
	return unsafeIP(ip) || ip.IsInterfaceLocalMulticast() || isCarrierGradeNAT(ip) || ip.Equal(net.IPv4bcast)
}

// isCarrierGradeNAT reports 100.64.0.0/10, which net.IP.IsPrivate does not cover.
func isCarrierGradeNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xC0 == 64
}

// SafeAddr resolves host once and returns the single address the caller must dial. Every
// resolved address has to be safe: a name that also points at the LAN is refused outright.
// Dialing the returned IP (not the name) closes the DNS-rebinding window that exists when a
// name is checked and then resolved a second time by the dialer.
func (p DialPolicy) SafeAddr(ctx context.Context, host string) (net.IP, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("%w: %q", ErrUnsafeAddress, host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if p.unsafe(ip) {
			return nil, fmt.Errorf("%w: %s", ErrUnsafeAddress, ip)
		}
		return ip, nil
	}
	ips, err := p.resolve(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolving %q: no addresses", host)
	}
	for _, ip := range ips {
		if p.unsafe(ip) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrUnsafeAddress, host, ip)
		}
	}
	return ips[0], nil
}

func (p DialPolicy) timeout() time.Duration {
	if p.ConnectTimeout > 0 {
		return p.ConnectTimeout
	}
	return 10 * time.Second
}

// DialTCP opens a plain TCP connection to a vetted address. Callers that use it must upgrade
// to TLS before sending anything secret (see UpgradeTLS).
func (p DialPolicy) DialTCP(ctx context.Context, host string, port int) (net.Conn, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	ip, err := p.SafeAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: p.timeout()}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
}

// UpgradeTLS runs the TLS handshake over conn, verifying the certificate against host (not
// against the IP that was dialed). TLS 1.2 is the floor.
func (p DialPolicy) UpgradeTLS(ctx context.Context, conn net.Conn, host string) (*tls.Conn, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if p.TLS != nil {
		cfg = p.TLS.Clone()
		if cfg.MinVersion < tls.VersionTLS12 {
			cfg.MinVersion = tls.VersionTLS12
		}
	}
	cfg.ServerName = strings.TrimSuffix(strings.ToLower(host), ".")
	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	tc := tls.Client(conn, cfg)
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("TLS handshake with %s: %w", host, err)
	}
	return tc, nil
}

// DialTLS connects and completes an implicit-TLS handshake in one step.
func (p DialPolicy) DialTLS(ctx context.Context, host string, port int) (net.Conn, error) {
	conn, err := p.DialTCP(ctx, host, port)
	if err != nil {
		return nil, err
	}
	return p.UpgradeTLS(ctx, conn, host)
}
