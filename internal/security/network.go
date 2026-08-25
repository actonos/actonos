package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrUnsafeURL indicates an outbound URL that can reach a local or private network.
var ErrUnsafeURL = errors.New("outbound URL targets a local or private network")

var lookupIP = net.DefaultResolver.LookupIPAddr

// ValidateOutboundURL rejects non-HTTP schemes, embedded credentials, localhost,
// private/link-local addresses, and hostnames resolving to those addresses.
func ValidateOutboundURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parsing URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return fmt.Errorf("%w: unsupported scheme %q", ErrUnsafeURL, parsed.Scheme)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: embedded credentials are not allowed", ErrUnsafeURL)
	}

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrUnsafeURL
	}
	if ip := net.ParseIP(host); ip != nil {
		if unsafeIP(ip) {
			return ErrUnsafeURL
		}
		return nil
	}

	addresses, err := lookupIP(ctx, host)
	if err != nil {
		return fmt.Errorf("resolving outbound host: %w", err)
	}
	for _, address := range addresses {
		if unsafeIP(address.IP) {
			return fmt.Errorf("%w: %s", ErrUnsafeURL, address.IP.String())
		}
	}
	return nil
}

func unsafeIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}
