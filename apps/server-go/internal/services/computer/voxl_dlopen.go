//go:build linux || darwin

// CGo dlopen bindings to libvoxl (github.com/Adelodunpeter25/voxl-go).
// Loaded at runtime from VOXL_LIB_PATH or next to the server binary so the
// server starts cleanly even when libvoxl is absent.
package computer

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

#define VOXL_OK 0
#define VOXL_ERR_ARG (-1)
#define VOXL_ERR_STATE (-2)
#define VOXL_ERR_INIT (-3)

typedef struct VoxlSession VoxlSession;

typedef struct VoxlConfig {
	const char *display;
	const char *preset;
	uint32_t fps;
	uint32_t max_kbps;
	uint32_t min_kbps;
	uint32_t threads;
	float crf;
	uint8_t enable_resize;
} VoxlConfig;

typedef void (*VoxlFrameCallback)(const uint8_t *data, size_t len, uint32_t kind, void *user_data);

typedef VoxlSession *(*voxl_session_create_t)(const VoxlConfig *config);
typedef int32_t (*voxl_session_start_t)(VoxlSession *s, VoxlFrameCallback cb, void *user_data);
typedef int32_t (*voxl_session_stop_t)(VoxlSession *s);
typedef int32_t (*voxl_session_send_mouse_t)(VoxlSession *s, int32_t x, int32_t y, uint32_t button_mask, int32_t wheel);
typedef int32_t (*voxl_session_send_key_t)(VoxlSession *s, uint32_t keysym, int32_t down);
typedef int32_t (*voxl_session_resize_t)(VoxlSession *s, uint32_t width, uint32_t height);
typedef void (*voxl_session_destroy_t)(VoxlSession *s);

static voxl_session_create_t p_voxl_create;
static voxl_session_start_t p_voxl_start;
static voxl_session_stop_t p_voxl_stop;
static voxl_session_send_mouse_t p_voxl_send_mouse;
static voxl_session_send_key_t p_voxl_send_key;
static voxl_session_resize_t p_voxl_resize;
static voxl_session_destroy_t p_voxl_destroy;

static void *voxl_open_library(const char *path) { return dlopen(path, RTLD_NOW|RTLD_LOCAL); }
static void *voxl_symbol(void *dl, const char *name) { return dlsym(dl, name); }

static bool voxl_bind_all(void *dl) {
	p_voxl_create = (voxl_session_create_t)voxl_symbol(dl, "voxl_session_create");
	if (!p_voxl_create) return false;
	p_voxl_start = (voxl_session_start_t)voxl_symbol(dl, "voxl_session_start");
	if (!p_voxl_start) return false;
	p_voxl_stop = (voxl_session_stop_t)voxl_symbol(dl, "voxl_session_stop");
	if (!p_voxl_stop) return false;
	p_voxl_send_mouse = (voxl_session_send_mouse_t)voxl_symbol(dl, "voxl_session_send_mouse");
	if (!p_voxl_send_mouse) return false;
	p_voxl_send_key = (voxl_session_send_key_t)voxl_symbol(dl, "voxl_session_send_key");
	if (!p_voxl_send_key) return false;
	p_voxl_resize = (voxl_session_resize_t)voxl_symbol(dl, "voxl_session_resize");
	if (!p_voxl_resize) return false;
	p_voxl_destroy = (voxl_session_destroy_t)voxl_symbol(dl, "voxl_session_destroy");
	return p_voxl_destroy != NULL;
}

extern void goVoxlFrame(uint8_t *data, size_t len, uint32_t kind, void *user);
static void voxl_frame_trampoline(const uint8_t *d, size_t n, uint32_t k, void *u) {
	goVoxlFrame((uint8_t *)d, n, k, u);
}

static VoxlSession *go_voxl_create(const VoxlConfig *c) { return p_voxl_create(c); }
static int32_t go_voxl_start(VoxlSession *s, uintptr_t h) {
	return p_voxl_start(s, voxl_frame_trampoline, (void *)h);
}
static int32_t go_voxl_stop(VoxlSession *s) { return p_voxl_stop(s); }
static int32_t go_voxl_send_mouse(VoxlSession *s, int32_t x, int32_t y, uint32_t mask, int32_t wheel) {
	return p_voxl_send_mouse(s, x, y, mask, wheel);
}
static int32_t go_voxl_send_key(VoxlSession *s, uint32_t keysym, int32_t down) {
	return p_voxl_send_key(s, keysym, down);
}
static int32_t go_voxl_resize(VoxlSession *s, uint32_t w, uint32_t h) {
	return p_voxl_resize(s, w, h);
}
static void go_voxl_destroy(VoxlSession *s) { p_voxl_destroy(s); }
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/cgo"
	"sync"
	"unsafe"
)

var (
	voxlLoadOnce sync.Once
	voxlLoadErr  error
)

func loadVoxl() error {
	voxlLoadOnce.Do(func() {
		libName := "libvoxl.so"
		if runtime.GOOS == "darwin" {
			libName = "libvoxl.dylib"
		}

		var candidates []string
		if p := os.Getenv("VOXL_LIB_PATH"); p != "" {
			candidates = append(candidates, p)
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), libName))
		}
		candidates = append(candidates, filepath.Join("/usr/local/lib", libName))

		var handle unsafe.Pointer
		for _, path := range candidates {
			if _, err := os.Stat(path); err != nil {
				continue
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			cPath := C.CString(abs)
			h := C.voxl_open_library(cPath)
			C.free(unsafe.Pointer(cPath))
			if h != nil {
				handle = h
				break
			}
		}

		// Fall back to the system dynamic linker search path (e.g. LD_LIBRARY_PATH).
		if handle == nil {
			cName := C.CString(libName)
			handle = C.voxl_open_library(cName)
			C.free(unsafe.Pointer(cName))
		}

		if handle == nil {
			voxlLoadErr = fmt.Errorf("libvoxl not found (%s); set VOXL_LIB_PATH or place %s next to the console binary", libName, libName)
			return
		}
		if !C.voxl_bind_all(handle) {
			voxlLoadErr = fmt.Errorf("libvoxl is missing required symbols")
			return
		}
	})
	return voxlLoadErr
}

type voxlConfig struct {
	Display      string
	Preset       string
	FPS          uint32
	MaxKbps      uint32
	MinKbps      uint32
	Threads      uint32
	CRF          float32
	EnableResize bool
}

type voxlSession struct {
	mu     sync.Mutex
	p      *C.VoxlSession
	handle cgo.Handle
}

func voxlRC(code C.int32_t) error {
	switch code {
	case C.VOXL_OK:
		return nil
	case C.VOXL_ERR_ARG:
		return errors.New("voxl: invalid argument")
	case C.VOXL_ERR_STATE:
		return errors.New("voxl: invalid state")
	case C.VOXL_ERR_INIT:
		return errors.New("voxl: initialisation failed")
	default:
		return errors.New("voxl: unknown error")
	}
}

func newVoxlSession(cfg voxlConfig) (*voxlSession, error) {
	if err := loadVoxl(); err != nil {
		return nil, err
	}
	var c C.VoxlConfig
	if cfg.Display != "" {
		c.display = C.CString(cfg.Display)
		defer C.free(unsafe.Pointer(c.display))
	}
	if cfg.Preset != "" {
		c.preset = C.CString(cfg.Preset)
		defer C.free(unsafe.Pointer(c.preset))
	}
	c.fps = C.uint32_t(cfg.FPS)
	c.max_kbps = C.uint32_t(cfg.MaxKbps)
	c.min_kbps = C.uint32_t(cfg.MinKbps)
	c.threads = C.uint32_t(cfg.Threads)
	c.crf = C.float(cfg.CRF)
	if cfg.EnableResize {
		c.enable_resize = 1
	}
	p := C.go_voxl_create(&c)
	if p == nil {
		return nil, errors.New("voxl: create failed")
	}
	return &voxlSession{p: p}, nil
}

//export goVoxlFrame
func goVoxlFrame(data *C.uint8_t, n C.size_t, kind C.uint32_t, user unsafe.Pointer) {
	h := cgo.Handle(uintptr(user))
	fn := h.Value().(FrameHandler)
	fn(uint32(kind), unsafe.Slice((*byte)(unsafe.Pointer(data)), int(n)))
}

func (s *voxlSession) Start(fn FrameHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return errors.New("voxl: session destroyed")
	}
	s.handle = cgo.NewHandle(fn)
	if err := voxlRC(C.go_voxl_start(s.p, C.uintptr_t(s.handle))); err != nil {
		s.handle.Delete()
		s.handle = 0
		return err
	}
	return nil
}

func (s *voxlSession) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return nil
	}
	return voxlRC(C.go_voxl_stop(s.p))
}

func (s *voxlSession) SendMouse(x, y int32, buttonMask uint32, wheel int32) error {
	return voxlRC(C.go_voxl_send_mouse(s.p, C.int32_t(x), C.int32_t(y), C.uint32_t(buttonMask), C.int32_t(wheel)))
}

func (s *voxlSession) SendKey(keysym uint32, down bool) error {
	d := C.int32_t(0)
	if down {
		d = 1
	}
	return voxlRC(C.go_voxl_send_key(s.p, C.uint32_t(keysym), d))
}

func (s *voxlSession) Resize(w, h uint32) error {
	return voxlRC(C.go_voxl_resize(s.p, C.uint32_t(w), C.uint32_t(h)))
}

func (s *voxlSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil {
		return
	}
	C.go_voxl_destroy(s.p)
	s.p = nil
	if s.handle != 0 {
		s.handle.Delete()
		s.handle = 0
	}
}
