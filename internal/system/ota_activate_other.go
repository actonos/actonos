//go:build !linux && !windows

package system

import "fmt"

func (o *OTAEngine) activateActond(src string) error {
	return fmt.Errorf("ota apply is not supported on this OS")
}

func (o *OTAEngine) activateEmbeddingd(src string) error {
	return fmt.Errorf("ota apply is not supported on this OS")
}
