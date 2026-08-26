//go:build linux

package system

import (
	"os"
	"path/filepath"
)

func (o *OTAEngine) activateActond(src string) error {
	return atomicSymlink(src, filepath.Join(o.binDir, "actond"))
}

func (o *OTAEngine) activateEmbeddingd(src string) error {
	return atomicSymlink(src, filepath.Join(o.binDir, "embeddingd"))
}

func atomicSymlink(target, link string) error {
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}
