package system

import "context"

// HALRestarter adapts HAL to OTARestarter. Windows uses parent-wait spawn
// instead of DockerHAL.RestartDaemon (os.Exit(0) without a new argv0).
type HALRestarter struct {
	HAL    HAL
	Engine *OTAEngine
}

func (r HALRestarter) RestartEmbeddingd(ctx context.Context) error {
	if r.HAL == nil {
		return nil
	}
	return r.HAL.RestartEmbeddingd(ctx)
}
