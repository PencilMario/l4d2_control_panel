//go:build !linux

package traffic

import (
	"context"
	"errors"
)

func StartCapture(context.Context, Observer) (<-chan error, error) {
	return nil, errors.New("traffic capture is unsupported on this platform")
}
