// CGo bindings to the Cua Driver C ABI (trycua/cua, libs/cua-driver, MIT).
// The shared library is loaded at runtime via dlopen from CUA_DRIVER_LIB_PATH or
// next to the server binary — the build does not need the library, and callers
// fall back when it is absent.
//
// Only the host/protocol boundary lives here. Cua implements every tool.
package cua

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

// ABI 1.1 (cua_driver_abi.h). Kept as C mirrors of the header's types so the
// struct layouts match the Rust #[repr(C)] exports exactly.
#define CUA_ABI_MAJOR 1
#define CUA_ABI_MINOR 1

typedef int32_t CuaDriverStatus;

typedef struct {
	uint8_t *data;
	size_t len;
	size_t capacity;
} CuaDriverBuffer;

typedef struct {
	uint32_t struct_size;
	uint16_t major;
	uint16_t minor;
	uint16_t patch;
	uint16_t reserved;
} CuaDriverAbiVersion;

typedef void (*CuaDriverCompletionV1)(void *context, CuaDriverStatus status,
                                      CuaDriverBuffer result, CuaDriverBuffer error);

typedef CuaDriverStatus (*cua_abi_version_v1_t)(CuaDriverAbiVersion *out_version);
typedef bool (*cua_abi_compatible_v1_t)(uint16_t major, uint16_t minor);
typedef void (*cua_buffer_free_v1_t)(CuaDriverBuffer *buffer);
typedef CuaDriverStatus (*cua_create_v1_t)(const uint8_t *options_json, size_t options_len,
                                            void **out_handle, CuaDriverBuffer *out_error);
typedef void (*cua_destroy_v1_t)(void **handle);
typedef CuaDriverStatus (*cua_is_available_v1_t)(void *handle, bool *out_available,
                                                 CuaDriverBuffer *out_error);
typedef CuaDriverStatus (*cua_metadata_json_v1_t)(void *handle, CuaDriverBuffer *out_json,
                                                  CuaDriverBuffer *out_error);
typedef CuaDriverStatus (*cua_list_tools_json_v1_t)(void *handle, CuaDriverBuffer *out_json,
                                                    CuaDriverBuffer *out_error);
typedef CuaDriverStatus (*cua_invoke_v1_t)(void *handle, const uint8_t *name, size_t name_len,
                                           const uint8_t *arguments_json, size_t arguments_len,
                                           CuaDriverCompletionV1 callback, void *context,
                                           void **out_operation, CuaDriverBuffer *out_error);
typedef CuaDriverStatus (*cua_shutdown_v1_t)(void *handle, CuaDriverCompletionV1 callback,
                                             void *context, void **out_operation,
                                             CuaDriverBuffer *out_error);
typedef void (*cua_operation_cancel_v1_t)(void *operation);
typedef void (*cua_operation_release_v1_t)(void **operation);

static cua_abi_version_v1_t p_abi_version;
static cua_abi_compatible_v1_t p_abi_compatible;
static cua_buffer_free_v1_t p_buffer_free;
static cua_create_v1_t p_create;
static cua_destroy_v1_t p_destroy;
static cua_is_available_v1_t p_is_available;
static cua_metadata_json_v1_t p_metadata_json;
static cua_list_tools_json_v1_t p_list_tools_json;
static cua_invoke_v1_t p_invoke;
static cua_shutdown_v1_t p_shutdown;
static cua_operation_cancel_v1_t p_operation_cancel;
static cua_operation_release_v1_t p_operation_release;

// cua_bind_all resolves every symbol; returns false on the first miss. A
// library missing a single entry point is rejected rather than half-used.
static bool cua_bind_all(void *dl) {
	p_abi_version = (cua_abi_version_v1_t)dlsym(dl, "cua_driver_abi_version_v1");
	if (!p_abi_version) return false;
	p_abi_compatible = (cua_abi_compatible_v1_t)dlsym(dl, "cua_driver_abi_is_compatible_v1");
	if (!p_abi_compatible) return false;
	p_buffer_free = (cua_buffer_free_v1_t)dlsym(dl, "cua_driver_buffer_free_v1");
	if (!p_buffer_free) return false;
	p_create = (cua_create_v1_t)dlsym(dl, "cua_driver_create_v1");
	if (!p_create) return false;
	p_destroy = (cua_destroy_v1_t)dlsym(dl, "cua_driver_destroy_v1");
	if (!p_destroy) return false;
	p_is_available = (cua_is_available_v1_t)dlsym(dl, "cua_driver_is_available_v1");
	if (!p_is_available) return false;
	p_metadata_json = (cua_metadata_json_v1_t)dlsym(dl, "cua_driver_metadata_json_v1");
	if (!p_metadata_json) return false;
	p_list_tools_json = (cua_list_tools_json_v1_t)dlsym(dl, "cua_driver_list_tools_json_v1");
	if (!p_list_tools_json) return false;
	p_invoke = (cua_invoke_v1_t)dlsym(dl, "cua_driver_invoke_v1");
	if (!p_invoke) return false;
	p_shutdown = (cua_shutdown_v1_t)dlsym(dl, "cua_driver_shutdown_v1");
	if (!p_shutdown) return false;
	p_operation_cancel = (cua_operation_cancel_v1_t)dlsym(dl, "cua_driver_operation_cancel_v1");
	if (!p_operation_cancel) return false;
	p_operation_release = (cua_operation_release_v1_t)dlsym(dl, "cua_driver_operation_release_v1");
	if (!p_operation_release) return false;
	return true;
}

static CuaDriverStatus go_abi_version(CuaDriverAbiVersion *v) { return p_abi_version(v); }
static bool go_abi_compatible(uint16_t major, uint16_t minor) { return p_abi_compatible(major, minor); }
static void go_buffer_free(CuaDriverBuffer *b) { p_buffer_free(b); }
static CuaDriverStatus go_create(const uint8_t *o, size_t n, void **h, CuaDriverBuffer *e) {
	return p_create(o, n, h, e);
}
static void go_destroy(void **h) { p_destroy(h); }
static CuaDriverStatus go_is_available(void *h, bool *a, CuaDriverBuffer *e) {
	return p_is_available(h, a, e);
}
static CuaDriverStatus go_metadata_json(void *h, CuaDriverBuffer *j, CuaDriverBuffer *e) {
	return p_metadata_json(h, j, e);
}
static CuaDriverStatus go_list_tools_json(void *h, CuaDriverBuffer *j, CuaDriverBuffer *e) {
	return p_list_tools_json(h, j, e);
}
static CuaDriverStatus go_invoke(void *h, const uint8_t *n, size_t nl, const uint8_t *a,
                                 size_t al, CuaDriverCompletionV1 cb, void *ctx,
                                 void **op, CuaDriverBuffer *e) {
	return p_invoke(h, n, nl, a, al, cb, ctx, op, e);
}
static CuaDriverStatus go_shutdown(void *h, CuaDriverCompletionV1 cb, void *ctx,
                                   void **op, CuaDriverBuffer *e) {
	return p_shutdown(h, cb, ctx, op, e);
}
static void go_operation_cancel(void *op) { p_operation_cancel(op); }
static void go_operation_release(void **op) { p_operation_release(op); }

// The completion lands through a single process-wide slot rather than a Go
// callback. cgo cannot export a Go func with C struct parameters, and the
// driver calls the completion exactly once per admitted operation. Calls are
// already serialized by a mutex in Go, so one slot is sufficient and keeps the
// native-to-Go boundary free of //export.
typedef struct {
	CuaDriverStatus status;
	CuaDriverBuffer result;
	CuaDriverBuffer error;
	int done;
} CuaCompletionSlot;

static CuaCompletionSlot g_slot;
static void cua_on_complete(void *ctx, CuaDriverStatus status,
                             CuaDriverBuffer result, CuaDriverBuffer error) {
	(void)ctx;
	g_slot.status = status;
	g_slot.result = result;
	g_slot.error = error;
	g_slot.done = 1;
}

// cua_slot_reset clears the slot before an operation is admitted, so a stale
// completion from an earlier call can never be mistaken for this one.
static void cua_slot_reset(void) {
	g_slot.status = 0;
	g_slot.result.data = NULL;
	g_slot.result.len = 0;
	g_slot.result.capacity = 0;
	g_slot.error.data = NULL;
	g_slot.error.len = 0;
	g_slot.error.capacity = 0;
	g_slot.done = 0;
}

static int cua_slot_done(void) { return g_slot.done; }
static CuaDriverStatus cua_slot_status(void) { return g_slot.status; }
static CuaDriverBuffer cua_slot_result(void) { return g_slot.result; }
static CuaDriverBuffer cua_slot_error(void) { return g_slot.error; }
static void cua_slot_clear(void) {
	// Take ownership so Go copies the bytes exactly once, then release here.
	p_buffer_free(&g_slot.result);
	p_buffer_free(&g_slot.error);
	g_slot.result.data = NULL;
	g_slot.result.len = 0;
	g_slot.result.capacity = 0;
	g_slot.error.data = NULL;
	g_slot.error.len = 0;
	g_slot.error.capacity = 0;
	g_slot.done = 0;
}

static CuaDriverCompletionV1 cua_completion(void) { return cua_on_complete; }
*/
import "C"

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

// ABI version this binding was written against. cua_driver_abi_is_compatible_v1
// decides whether a loaded library can serve us.
const (
	abiMajor = 1
	abiMinor = 1
)

// Status codes from the header's CuaDriverStatus enum.
const (
	statusOK                 = 0
	statusInvalidArgument    = 1
	statusNullPointer        = 2
	statusRuntimeUnavailable = 3
	statusShutdown           = 4
	statusCancelled          = 5
	statusInternal           = 6
	statusPanic              = 7
	statusRuntimeConflict    = 8
)

// cancelPollInterval matches the reference hosts' completion wait, which
// polls every 50ms so a Stop is noticed promptly rather than after the call
// finishes on its own.
const cancelPollInterval = 50 * time.Millisecond

// shutdownDrainTimeout bounds the wait for the driver's asynchronous
// shutdown. Cua drains admitted work before finalizing; a wedged drain must
// not block the server's own shutdown.
const shutdownDrainTimeout = 10 * time.Second

// ErrStopped reports that a call was cancelled. The action's completion is
// deliberately NOT reported as unknown here; see CancelledError.
var ErrStopped = errors.New("computer use stopped before the action completed")

// CancelledError means the operation was cancelled mid-flight. The underlying
// action may still have taken effect, so callers must inspect fresh state
// rather than retry blindly. This wording is the contract the reference hosts
// use and the driver's own skill repeats: "An interrupted action may have
// completed."
type CancelledError struct {
	// Unknown reports that the operation was already admitted when it was
	// cancelled, so its effect cannot be determined.
	Unknown bool
}

func (e *CancelledError) Error() string {
	if e.Unknown {
		return "computer use stopped; action completion is unknown. Inspect fresh state before retrying."
	}
	return "computer use stopped before the action started."
}

func (e *CancelledError) Unwrap() error { return ErrStopped }

// StatusError is a non-OK status from the ABI.
type StatusError struct {
	Status int32
	Detail string
}

func (e *StatusError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("cua driver failed (status %d): %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("cua driver failed (status %d)", e.Status)
}

func (e *StatusError) Cancelled() bool {
	return e.Status == statusCancelled || e.Status == statusShutdown
}

var (
	loadOnce sync.Once
	bound    bool
	loadErr  error
)

// libraryNames are the per-platform file names, matching what Cua's own
// installer stages next to the driver binary.
func libraryNames() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"libcua_driver_sdk.dylib"}
	case "windows":
		return []string{"cua_driver_sdk.dll"}
	default:
		return []string{"libcua_driver_sdk.so"}
	}
}

// LibraryPathOverride returns the configured CUA_DRIVER_LIB_PATH override,
// rejecting a relative path. A computer-use library resolved against an
// ambient location is an attack surface rather than a convenience, so a
// relative override is refused instead of being resolved against whatever the
// process happens to have as its working directory.
//
// Exported separately from Load so a status endpoint can report the configured
// path without triggering a library load.
func LibraryPathOverride() (string, error) {
	p := strings.TrimSpace(os.Getenv("CUA_DRIVER_LIB_PATH"))
	if p == "" {
		return "", nil
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("CUA_DRIVER_LIB_PATH must be an absolute path, got %q", p)
	}
	return p, nil
}

// candidates lists the library locations tried, in order. Only fixed
// locations are considered, never PATH and never an ambient search: an
// explicit absolute override first, then beside our own binary (installed
// layout), then the repo layouts for dev servers and test binaries, then the
// system library dir. A relative env override is refused outright rather
// than resolved against whatever the process has as its working directory.
func candidates() ([]string, error) {
	override, err := LibraryPathOverride()
	if err != nil {
		return nil, err
	}
	var out []string
	if override != "" {
		out = append(out, override)
	}
	var exeDir string
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()
	seen := map[string]bool{}
	for _, path := range candidatePaths(exeDir, cwd, libraryNames()) {
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	return out, nil
}

// candidatePaths lists locations for one executable directory and one working
// directory, in priority order. The working-directory forms exist because a
// dev server (`go run`) executes from a temp dir: without them the vendored
// library beside the repo is invisible, which is exactly how fff's
// third_party fallback earns its keep. Ancestors are walked so test binaries
// (whose working directory is their package dir) resolve the same tree.
func candidatePaths(exeDir, cwd string, names []string) []string {
	var out []string
	if exeDir != "" {
		for _, n := range names {
			out = append(out,
				filepath.Join(exeDir, n),
				filepath.Join(exeDir, "third_party", "cua", n),
			)
		}
	}
	dir := cwd
	for i := 0; i < 8 && dir != "" && dir != "."; i++ {
		for _, n := range names {
			out = append(out,
				filepath.Join(dir, "third_party", "cua", n),
				filepath.Join(dir, "apps", "server-go", "third_party", "cua", n),
			)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, n := range names {
		out = append(out, filepath.Join("/usr/local/lib", n))
	}
	return out
}

// CandidatePathsForTest exposes the location order so it is pinned without
// touching the filesystem: beside the binary, the repo layouts for dev
// servers and test binaries, then the system dir.
func CandidatePathsForTest(exeDir, cwd string, names ...string) []string {
	if len(names) == 0 {
		names = libraryNames()
	}
	return candidatePaths(exeDir, cwd, names)
}

// Load resolves the Cua Driver library at runtime. It is safe to call
// repeatedly; the attempt happens once.
func Load() error {
	loadOnce.Do(func() {
		// An explicit off switch beats every candidate: operators can force
		// computer use off without uninstalling the library, and tests can
		// force absence without depending on what happens to be on disk.
		if disabled, _ := strconv.ParseBool(os.Getenv("CUA_DRIVER_DISABLED")); disabled {
			loadErr = errors.New("cua driver disabled by CUA_DRIVER_DISABLED")
			return
		}
		paths, err := candidates()
		if err != nil {
			loadErr = err
			return
		}
		var handle unsafe.Pointer
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				continue
			}
			cpath := C.CString(path)
			h := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
			C.free(unsafe.Pointer(cpath))
			if h != nil {
				handle = h
				break
			}
		}
		if handle == nil {
			loadErr = fmt.Errorf("cua driver library not found (set CUA_DRIVER_LIB_PATH)")
			return
		}
		if !bool(C.cua_bind_all(handle)) {
			loadErr = fmt.Errorf("cua driver library missing required ABI symbols")
			return
		}
		if err := checkABI(); err != nil {
			loadErr = err
			return
		}
		bound = true
	})
	return loadErr
}

// checkABI verifies the loaded library can serve callers built for our
// major.minor. A newer patch is fine; a different minor is not.
func checkABI() error {
	version := C.CuaDriverAbiVersion{
		struct_size: C.uint32_t(unsafe.Sizeof(C.CuaDriverAbiVersion{})),
	}
	if status := C.go_abi_version(&version); status != statusOK {
		return fmt.Errorf("cua driver abi version call failed (status %d)", int32(status))
	}
	if !bool(C.go_abi_compatible(C.uint16_t(abiMajor), C.uint16_t(abiMinor))) {
		return fmt.Errorf("cua driver ABI %d.%d is required, found %d.%d",
			abiMajor, abiMinor, uint16(version.major), uint16(version.minor))
	}
	return nil
}

// Available reports whether the library loaded and its ABI matches.
func Available() bool { return Load() == nil }

// ABIVersion is the loaded library's reported ABI.
func ABIVersion() (major, minor, patch int, err error) {
	if err := Load(); err != nil {
		return 0, 0, 0, err
	}
	version := C.CuaDriverAbiVersion{
		struct_size: C.uint32_t(unsafe.Sizeof(C.CuaDriverAbiVersion{})),
	}
	if status := C.go_abi_version(&version); status != statusOK {
		return 0, 0, 0, &StatusError{Status: int32(status)}
	}
	return int(version.major), int(version.minor), int(version.patch), nil
}

// ContentPart is one item of a tool result's content array. Image parts carry
// raw bytes; Text carries the string. Unknown types are ignored by the
// converters, so they are kept as raw JSON rather than dropped.
type ContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     []byte          `json:"-"`
	MIMEType string          `json:"mimeType,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Name     string          `json:"name,omitempty"`
	Extra    json.RawMessage `json:"-"`
}

// ToolResult is one Cua tool result, mirroring the driver's envelope.
type ToolResult struct {
	Content           []ContentPart   `json:"content,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
	ErrorCode         string          `json:"error_code,omitempty"`
}

// Risk is Cua's own assessment of one tool. The class is per tool and per
// operation: the inventory advertises the strongest shipped enforcement for any
// typed operation, and the exact call is narrowed at dispatch. Cua's classes do
// not follow name prefixes — kill_app has no exec-ish prefix but is r3 — so this
// is what Console's own tiering must read rather than guessing from the name.
type Risk struct {
	Class              string `json:"class"`
	Enforcement        string `json:"enforcement"`
	OperationSensitive bool   `json:"operation_sensitive"`
	Version            string `json:"version,omitempty"`
}

// Annotations are the standard MCP tool hints the driver advertises.
type Annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ToolDef is one entry of the driver's advertised inventory.
type ToolDef struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	Risk         Risk            `json:"risk"`
	Annotations  Annotations     `json:"annotations"`
	Capabilities []string        `json:"capabilities,omitempty"`
}

// RiskClass returns the advertised risk class, or "" when the driver did not
// classify the tool.
func (t ToolDef) RiskClass() string { return strings.ToLower(strings.TrimSpace(t.Risk.Class)) }

// ReadOnly reports the driver's readOnlyHint, which is authoritative for
// observation tools and is why get_window_state is a read rather than a write.
func (t ToolDef) ReadOnly() bool {
	if t.Annotations.ReadOnlyHint == nil {
		return false
	}
	return *t.Annotations.ReadOnlyHint
}

// ToolInventory is the parsed `tools/list` payload.
type ToolInventory struct {
	Tools []ToolDef `json:"tools"`
}

func takeBuffer(buf C.CuaDriverBuffer) []byte {
	if buf.data == nil || buf.len == 0 {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(buf.data), C.int(buf.len))
}

// freeBuffer releases a caller-owned buffer. Freeing an empty buffer is
// harmless per the header, so this is safe to defer unconditionally.
func freeBuffer(buf *C.CuaDriverBuffer) { C.go_buffer_free(buf) }

func statusError(status int32, detail []byte) error {
	if status == statusOK {
		return nil
	}
	return &StatusError{Status: status, Detail: strings.TrimSpace(string(detail))}
}

// Driver is one in-process Cua Driver runtime. Cua owns process-global UI and
// executor threads, so a driver is expected to be long-lived: create one and
// keep it.
type Driver struct {
	mu        sync.Mutex
	handle    unsafe.Pointer
	operation unsafe.Pointer
	cancelled bool
	closed    bool
	shutdown  bool
}

// Options are the runtime creation options. Only fields Cua documents are
// exposed; it fails closed on unknown keys.
type Options struct {
	// ClaudeCodeCompatibility reshapes `screenshot` to the Claude Code
	// computer-use shape. Off by default.
	ClaudeCodeCompatibility bool
}

// Open loads the library and creates a driver handle.
func Open(opts Options) (*Driver, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if opts.ClaudeCodeCompatibility {
		payload["claude_code_compatibility"] = true
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var handle unsafe.Pointer
	var errBuf C.CuaDriverBuffer
	defer freeBuffer(&errBuf)
	status := C.go_create(
		(*C.uint8_t)(unsafe.Pointer(&encoded[0])), C.size_t(len(encoded)),
		&handle, &errBuf,
	)
	if err := statusError(int32(status), takeBuffer(errBuf)); err != nil {
		return nil, err
	}
	if handle == nil {
		return nil, errors.New("cua driver create returned a null handle")
	}
	return &Driver{handle: handle}, nil
}

// IsAvailable reports whether the driver still accepts operations.
func (d *Driver) IsAvailable() (bool, error) {
	if d == nil {
		return false, errors.New("cua driver is not open")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return false, errors.New("cua driver is closed")
	}
	var available C.bool
	var errBuf C.CuaDriverBuffer
	defer freeBuffer(&errBuf)
	status := C.go_is_available(d.handle, &available, &errBuf)
	if err := statusError(int32(status), takeBuffer(errBuf)); err != nil {
		return false, err
	}
	return bool(available), nil
}

// Metadata returns the driver's metadata JSON.
func (d *Driver) Metadata() (json.RawMessage, error) {
	return d.callJSON(func(handle unsafe.Pointer, out, errBuf *C.CuaDriverBuffer) C.CuaDriverStatus {
		return C.go_metadata_json(handle, out, errBuf)
	})
}

// rawListTools returns the undecoded tools/list payload.
func (d *Driver) rawListTools() (json.RawMessage, error) {
	return d.callJSON(func(handle unsafe.Pointer, out, errBuf *C.CuaDriverBuffer) C.CuaDriverStatus {
		return C.go_list_tools_json(handle, out, errBuf)
	})
}

// ListTools returns the canonical advertised tool inventory.
func (d *Driver) ListTools() ([]ToolDef, error) {
	raw, err := d.callJSON(func(handle unsafe.Pointer, out, errBuf *C.CuaDriverBuffer) C.CuaDriverStatus {
		return C.go_list_tools_json(handle, out, errBuf)
	})
	if err != nil {
		return nil, err
	}
	// The inventory is either a bare array or an object with a tools array,
	// depending on the driver version, so accept both shapes.
	var inv ToolInventory
	if err := json.Unmarshal(raw, &inv); err == nil && len(inv.Tools) > 0 {
		return inv.Tools, nil
	}
	var list []ToolDef
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("could not parse the tool inventory: %w", err)
	}
	return list, nil
}

func (d *Driver) callJSON(call func(unsafe.Pointer, *C.CuaDriverBuffer, *C.CuaDriverBuffer) C.CuaDriverStatus) (json.RawMessage, error) {
	if d == nil {
		return nil, errors.New("cua driver is not open")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return nil, errors.New("cua driver is closed")
	}
	var out, errBuf C.CuaDriverBuffer
	defer freeBuffer(&out)
	defer freeBuffer(&errBuf)
	status := call(d.handle, &out, &errBuf)
	raw := takeBuffer(out)
	if err := statusError(int32(status), takeBuffer(errBuf)); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// Call invokes one tool by name. cancel is polled while the call is in flight
// so a Stop interrupts admitted work rather than waiting for it to finish.
func (d *Driver) Call(ctx context.Context, name string, args map[string]any, cancel func() bool) (*ToolResult, error) {
	if d == nil {
		return nil, errors.New("cua driver is not open")
	}
	if err := Load(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	if len(encoded) == 0 {
		encoded = []byte("{}")
	}

	d.mu.Lock()
	if d.handle == nil || d.closed {
		d.mu.Unlock()
		return nil, errors.New("cua driver is closed")
	}
	if d.cancelled {
		// Never start work after a stop: an action whose start we refused has
		// no unknown outcome, so this is not a CancelledError.
		d.mu.Unlock()
		return nil, &CancelledError{}
	}

	cName := C.CBytes([]byte(name))
	defer C.free(cName)
	cArgs := C.CBytes(encoded)
	defer C.free(cArgs)

	var operation unsafe.Pointer
	var errBuf C.CuaDriverBuffer
	C.cua_slot_reset()
	status := C.go_invoke(
		d.handle,
		(*C.uint8_t)(cName), C.size_t(len(name)),
		(*C.uint8_t)(cArgs), C.size_t(len(encoded)),
		C.cua_completion(), nil,
		&operation, &errBuf,
	)
	// The operation token must be tracked before we release the lock so a
	// concurrent Stop can cancel work that is already admitted.
	d.operation = operation
	admitted := status == statusOK
	detail := takeBuffer(errBuf)
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		if d.operation == operation {
			d.operation = nil
		}
		d.mu.Unlock()
		var op unsafe.Pointer = operation
		C.go_operation_release(&op)
	}()

	if !admitted {
		// A rejected admission never invokes the completion callback.
		return nil, statusError(int32(status), detail)
	}

	stopped := false
	for {
		if C.cua_slot_done() != 0 {
			status := int32(C.cua_slot_status())
			raw := takeBuffer(C.cua_slot_result())
			detail := takeBuffer(C.cua_slot_error())
			// Clear the slot so the buffers are released exactly once and the
			// next admission starts from a clean state.
			C.cua_slot_clear()
			if err := statusError(status, detail); err != nil {
				var se *StatusError
				if errors.As(err, &se) && se.Cancelled() {
					return nil, &CancelledError{Unknown: true}
				}
				return nil, err
			}
			result, err := parseResult(raw)
			if err != nil {
				return nil, err
			}
			if stopped {
				return nil, &CancelledError{Unknown: true}
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			if stopped {
				continue
			}
			stopped = true
			// Request cancellation once; the driver drains rather than
			// aborting, and the completion may still arrive normally.
			d.cancelOperation(operation)
		case <-time.After(cancelPollInterval):
			if cancel != nil && cancel() && !stopped && !d.stopped() {
				stopped = true
				d.cancelOperation(operation)
			}
		}
	}
}

// cancelOperation requests cancellation of an admitted operation, if it is
// still the one in flight.
func (d *Driver) cancelOperation(operation unsafe.Pointer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.operation == operation && operation != nil {
		C.go_operation_cancel(operation)
	}
}

func (d *Driver) stopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancelled
}

// Stop refuses new work and cancels anything admitted. An action already in
// flight may still complete; its outcome is reported as unknown.
func (d *Driver) Stop() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cancelled = true
	if d.operation != nil {
		C.go_operation_cancel(d.operation)
	}
}

// Resume clears a previous stop so the driver accepts work again.
func (d *Driver) Resume() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cancelled = false
}

// Shutdown stops admission, drains admitted calls, and finalizes the runtime.
// It must not race an in-flight Call: callers serialize through the manager.
func (d *Driver) Shutdown() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if d.handle == nil || d.shutdown {
		d.mu.Unlock()
		return nil
	}
	d.shutdown = true
	handle := d.handle
	var operation unsafe.Pointer
	var errBuf C.CuaDriverBuffer
	C.cua_slot_reset()
	status := C.go_shutdown(handle,
		C.cua_completion(), nil,
		&operation, &errBuf)
	detail := takeBuffer(errBuf)
	d.mu.Unlock()

	if status == statusOK {
		// Shutdown is asynchronous; drain it so the handle is finalized
		// before destroy. Bound the wait: a stuck drain must not wedge the
		// server's shutdown path.
		deadline := time.Now().Add(shutdownDrainTimeout)
		for C.cua_slot_done() == 0 && time.Now().Before(deadline) {
			time.Sleep(cancelPollInterval)
		}
	}
	C.cua_slot_clear()
	var op unsafe.Pointer = operation
	C.go_operation_release(&op)

	d.mu.Lock()
	d.handle = nil
	d.closed = true
	d.mu.Unlock()

	return statusError(int32(status), detail)
}

// parseResult decodes the driver's JSON envelope. Image data arrives base64
// encoded and is decoded here so the converter receives raw bytes.
func parseResult(raw []byte) (*ToolResult, error) {
	if len(raw) == 0 {
		return &ToolResult{}, nil
	}
	// Decode into a loose shape first: content items carry different fields
	// per type and unknown keys must survive untouched.
	var envelope struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MIMEType string `json:"mimeType"`
			URI      string `json:"uri"`
			Name     string `json:"name"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
		ErrorCode         string          `json:"error_code"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("could not parse the tool result: %w", err)
	}
	result := &ToolResult{
		StructuredContent: envelope.StructuredContent,
		IsError:           envelope.IsError,
		ErrorCode:         envelope.ErrorCode,
	}
	for _, item := range envelope.Content {
		part := ContentPart{
			Type:     item.Type,
			Text:     item.Text,
			MIMEType: item.MIMEType,
			URI:      item.URI,
			Name:     item.Name,
		}
		if item.Data != "" {
			decoded, err := base64Decode(item.Data)
			if err != nil {
				return nil, fmt.Errorf("image data in %s result is not valid base64: %w", item.Type, err)
			}
			part.Data = decoded
		}
		result.Content = append(result.Content, part)
	}
	return result, nil
}
