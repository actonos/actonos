package mailcal

import (
	"context"
	"time"
)

// SetSleepForTest replaces the back-off sleeper. Compiled only into tests of this package
// tree (it lives in a _test.go file of package mailcal, so external test packages see it).
func (s *Service) SetSleepForTest(fn func(ctx context.Context, d time.Duration) error) { s.sleep = fn }

// SetURLCheckForTest replaces the SSRF check on token endpoints (tests use a local TLS server).
func (f *OAuthFlow) SetURLCheckForTest(fn func(ctx context.Context, rawURL string) error) {
	f.checkURL = fn
}
