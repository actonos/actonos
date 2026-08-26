//go:build windows

package system

import (
	"context"
	"os"
)

func (r HALRestarter) RestartDaemon(ctx context.Context) error {
	_ = ctx
	version := ""
	if r.Engine != nil {
		if j := r.Engine.Job(); j != nil {
			version = j.Version
		}
		if err := SpawnOTAChild(r.Engine.dataDir, version); err != nil {
			return err
		}
	}
	os.Exit(0)
	return nil
}
