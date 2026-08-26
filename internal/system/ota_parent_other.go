//go:build !windows

package system

// WaitForOTAParent is a no-op off Windows.
func WaitForOTAParent() {}
