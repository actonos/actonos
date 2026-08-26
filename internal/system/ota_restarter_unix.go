//go:build !windows

package system

import "context"

func (r HALRestarter) RestartDaemon(ctx context.Context) error {
	if r.HAL == nil {
		return nil
	}
	return r.HAL.RestartDaemon(ctx)
}
