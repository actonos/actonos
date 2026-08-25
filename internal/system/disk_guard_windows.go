package system

import (
	"golang.org/x/sys/windows"
)

func platformFreeSpace(path string) (free, total uint64, err error) {
	var freeBytes, totalBytes, avail uint64
	err = windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(path), &avail, &totalBytes, &freeBytes)
	if err != nil {
		return 0, 0, err
	}
	return avail, totalBytes, nil
}
