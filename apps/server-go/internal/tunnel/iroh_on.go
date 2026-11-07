//go:build iroh && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

package tunnel

import (
	"context"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/irohbackend"
)

// IrohAvailable reports whether this binary was built with the iroh backend.
const IrohAvailable = true

// StartIroh serves target over an iroh endpoint and returns its closer.
func StartIroh(dir, target string, onReady func(nodeID string)) (func(), error) {
	m := &irohbackend.Manager{Dir: dir, Target: target, OnReady: onReady}
	errc := make(chan error, 1)
	ready := make(chan struct{})
	m.OnReady = func(id string) {
		close(ready)
		if onReady != nil {
			onReady(id)
		}
	}
	go func() { errc <- m.Start(context.Background()) }()
	select {
	case <-ready:
		return m.Close, nil
	case err := <-errc:
		return func() {}, err
	}
}

func irohInfo(dir string) (Info, bool) {
	info, ok := irohbackend.ReadInfo(dir)
	return Info{NodeID: info.NodeID, Ready: ok}, ok
}
