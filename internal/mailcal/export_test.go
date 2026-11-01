package mailcal

import (
	"context"
	"time"
)

// SetSleepForTest replaces the back-off sleeper. Compiled only into tests of this package
// tree (it lives in a _test.go file of package mailcal, so external test packages see it).
func (s *Service) SetSleepForTest(fn func(ctx context.Context, d time.Duration) error) { s.sleep = fn }
