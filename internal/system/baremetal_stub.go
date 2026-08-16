//go:build !linux

package system

func getLinuxBaremetalHAL(dataDir string) HAL {
	return NewDockerHAL(dataDir)
}
