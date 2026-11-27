//go:build !(linux || darwin)

package computer

import (
	"context"
	"errors"
)

// FrameHandler receives raw wire frames to stream to viewers.
type FrameHandler func(kind uint32, data []byte)

var errUnsupported = errors.New("computer: desktop streaming is not supported on this platform")

// Service is a stub: libvoxl ships for Linux and macOS only.
type Service struct{}

func New() *Service      { return &Service{} }
func Default() *Service  { return &Service{} }
func (s *Service) Subscribe(context.Context, FrameHandler) (func(), error) {
	return nil, errUnsupported
}
func (s *Service) SendMouse(int32, int32, uint32, int32) error { return errUnsupported }
func (s *Service) SendKey(uint32, bool) error                  { return errUnsupported }
func (s *Service) Resize(uint32, uint32) error                 { return errUnsupported }
func (s *Service) Close()                                      {}
