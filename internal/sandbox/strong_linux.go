//go:build linux

package sandbox

import "os/exec"

func strongSandboxAvailable() bool {
	if isContainerEnvironment() {
		return false
	}
	_, err := exec.LookPath("bwrap")
	return err == nil
}

func newStrongSandbox() Sandbox {
	return NewBubblewrapSandbox()
}

