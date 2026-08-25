//go:build !windows

package system

import "golang.org/x/sys/unix"

func platformFreeSpace(path string) (free, total uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	bs := uint64(stat.Bsize)
	return stat.Bavail * bs, stat.Blocks * bs, nil
}
