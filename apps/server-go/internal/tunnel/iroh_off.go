//go:build !(iroh && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64)))

package tunnel

import "errors"

// IrohAvailable is false: build with -tags iroh on linux, macOS or windows-amd64.
const IrohAvailable = false

// StartIroh always fails in builds without the iroh backend.
func StartIroh(dir, target string, onReady func(nodeID string)) (func(), error) {
	return func() {}, errors.New("this build has no iroh backend (rebuild with -tags iroh)")
}

func irohInfo(dir string) (Info, bool) { return Info{}, false }
