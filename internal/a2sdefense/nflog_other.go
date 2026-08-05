//go:build !linux

package a2sdefense

import (
	"context"
	"errors"
	"time"
)

type NFLogSource struct{}

func NewNFLogSource(*EventRing, func() time.Time) *NFLogSource { return &NFLogSource{} }

func (*NFLogSource) Run(context.Context) error { return errors.New("NFLOG requires Linux") }
