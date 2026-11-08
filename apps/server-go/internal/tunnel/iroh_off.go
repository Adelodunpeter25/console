//go:build !((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

package tunnel

import "errors"

// IrohAvailable is false: iroh-go ships static libraries for linux, macOS
// and windows-amd64 only.
const IrohAvailable = false

// StartIroh always fails on platforms without the iroh libraries.
func StartIroh(dir, target string, onReady func(nodeID string)) (func(), error) {
	return func() {}, errors.New("this platform has no iroh backend")
}

func irohInfo(dir string) (Info, bool) { return Info{}, false }
