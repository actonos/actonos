package system

import (
	"os"
	"runtime"
)

const (
	ReasonDocker = "docker"
	ReasonDarwin = "darwin"
	ReasonOS     = "unsupported_os"
)

// OTAApplySupport reports whether binary apply is allowed on this process.
// Detection uses RUNTIME_MODE / /.dockerenv / GOOS — never HAL.RuntimeMode(),
// which is hardcoded "docker" on native Windows via DockerHAL.
func OTAApplySupport() (supported bool, reason string) {
	return otaApplySupport(runtime.GOOS, os.Getenv("RUNTIME_MODE"), dockerEnvPresent)
}

func dockerEnvPresent() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func otaApplySupport(goos, runtimeMode string, dockerEnv func() bool) (bool, string) {
	if runtimeMode == "docker" || dockerEnv() {
		return false, ReasonDocker
	}
	switch goos {
	case "linux", "windows":
		return true, ""
	case "darwin":
		return false, ReasonDarwin
	default:
		return false, ReasonOS
	}
}

// EmbeddingRequiredInput is the deploy-graph evidence that embeddingd is in use.
type EmbeddingRequiredInput struct {
	ServiceReady         bool
	SystemdUnitEnabled   bool
	PriorEmbeddingActive string
	EnvForce             string // ACTONOS_OTA_EMBEDDINGD
}

// EmbeddingdRequired is true only when the helper is actually deployed.
// Never use "embedding client pointer != nil" — production always constructs it.
func EmbeddingdRequired(in EmbeddingRequiredInput) bool {
	switch in.EnvForce {
	case "1", "true", "TRUE", "yes":
		return true
	case "0", "false", "FALSE", "no":
		return false
	}
	if in.ServiceReady {
		return true
	}
	if in.SystemdUnitEnabled {
		return true
	}
	if in.PriorEmbeddingActive != "" {
		return true
	}
	return false
}

// AllowUnsignedApply is the operator hatch for missing GitHub digests.
func AllowUnsignedApply() bool {
	v := os.Getenv("ACTONOS_OTA_ALLOW_UNSIGNED")
	return v == "1" || v == "true"
}

// JobIsActive reports whether a job status blocks a new enqueue.
func JobIsActive(status string) bool {
	switch status {
	case JobQueued, JobDownloading, JobVerifying, JobSwapping, JobRestarting:
		return true
	default:
		return false
	}
}

// JobIsTerminal reports a job that may be overwritten by the next enqueue.
func JobIsTerminal(status string) bool {
	switch status {
	case JobSucceeded, JobFailed, JobInterrupted, "":
		return true
	default:
		return false
	}
}
