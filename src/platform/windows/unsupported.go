//go:build windows

package windows

import (
	"errors"

	"github.com/soksak-app/sidecar-files/src/platform"
)

type implementation struct{}

func init() { platform.Register(implementation{}) }

func (implementation) Watch(string, func(), func(error)) (func() error, error) {
	return nil, errors.New("watching directories is not implemented on windows")
}

func (implementation) MonotonicNanos() (uint64, error) {
	return 0, errors.New("the monotonic clock is not implemented on windows")
}
