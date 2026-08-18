//go:build !linux

package sandbox

func strongSandboxAvailable() bool {
	return false
}

func newStrongSandbox() Sandbox {
	return &unavailableSandbox{}
}
