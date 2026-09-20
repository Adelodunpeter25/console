// CGo bindings to the fff file-search C ABI (crates/fff-c, see
// github.com/dmtrKovalenko/fff). The shared library is loaded at runtime via
// dlopen from FFF_LIB_PATH or ./third_party/fff/libfff_c.so — the build does
// not need the library, and callers fall back when fff is absent.
package fff

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

#define FFF_CREATE_OPTIONS_VERSION 2

typedef struct FffResult {
	bool success;
	char *error;
	void *handle;
	int64_t int_value;
} FffResult;

typedef struct FffCreateOptions {
	uint32_t version;
	const char *base_path;
	const char *frecency_db_path;
	const char *history_db_path;
	bool enable_mmap_cache;
	bool enable_content_indexing;
	bool watch;
	bool ai_mode;
	const char *log_file_path;
	const char *log_level;
	uint64_t cache_budget_max_files;
	uint64_t cache_budget_max_bytes;
	uint64_t cache_budget_max_file_size;
	bool enable_fs_root_scanning;
	bool enable_home_dir_scanning;
	bool follow_symlinks;
} FffCreateOptions;

typedef struct FffFileItem {
	char *relative_path;
	char *file_name;
	char *git_status;
	uint64_t size;
	uint64_t modified;
	int64_t access_frecency_score;
	int64_t modification_frecency_score;
	int64_t total_frecency_score;
	bool is_binary;
} FffFileItem;

typedef struct FffScore {
	int32_t total;
	int32_t base_score;
	int32_t filename_bonus;
	int32_t special_filename_bonus;
	int32_t frecency_boost;
	int32_t distance_penalty;
	int32_t current_file_penalty;
	int32_t combo_match_boost;
	int32_t path_alignment_bonus;
	bool exact_match;
	char *match_type;
} FffScore;

typedef struct FffLocation {
	uint8_t tag;
	int32_t line;
	int32_t col;
	int32_t end_line;
	int32_t end_col;
} FffLocation;

typedef struct FffSearchResult {
	FffFileItem *items;
	FffScore *scores;
	uint32_t count;
	uint32_t total_matched;
	uint32_t total_files;
	FffLocation location;
} FffSearchResult;

typedef struct FffResult *(*fff_create_instance_with_t)(const FffCreateOptions *);
typedef void (*fff_destroy_t)(void *);
typedef struct FffResult *(*fff_search_t)(void *, const char *, const char *,
	uint32_t, uint32_t, uint32_t, int32_t, uint32_t);
typedef struct FffResult *(*fff_wait_for_scan_t)(void *, uint64_t);
typedef void (*fff_free_result_t)(struct FffResult *);
typedef void (*fff_free_search_result_t)(struct FffSearchResult *);

static fff_create_instance_with_t p_create;
static fff_destroy_t p_destroy;
static fff_search_t p_search;
static fff_wait_for_scan_t p_wait;
static fff_free_result_t p_free_result;
static fff_free_search_result_t p_free_search;
typedef struct FffResult *(*fff_health_check_t)(void *);
static fff_health_check_t p_health;

// bind_all resolves every symbol; returns false on the first miss.
static bool fff_bind_all(void *dl) {
	p_create = (fff_create_instance_with_t)dlsym(dl, "fff_create_instance_with");
	if (!p_create) return false;
	p_destroy = (fff_destroy_t)dlsym(dl, "fff_destroy");
	if (!p_destroy) return false;
	p_search = (fff_search_t)dlsym(dl, "fff_search");
	if (!p_search) return false;
	p_wait = (fff_wait_for_scan_t)dlsym(dl, "fff_wait_for_scan");
	if (!p_wait) return false;
	p_free_result = (fff_free_result_t)dlsym(dl, "fff_free_result");
	if (!p_free_result) return false;
	p_free_search = (fff_free_search_result_t)dlsym(dl, "fff_free_search_result");
	if (!p_free_search) return false;
	p_health = (fff_health_check_t)dlsym(dl, "fff_health_check");
	return p_health != NULL;
}

static struct FffResult *go_create(const FffCreateOptions *opts) { return p_create(opts); }
static void go_destroy(void *h) { p_destroy(h); }
static struct FffResult *go_search(void *h, const char *q, const char *cf,
	uint32_t t, uint32_t pi, uint32_t ps, int32_t cb, uint32_t mc) {
	return p_search(h, q, cf, t, pi, ps, cb, mc);
}
static struct FffResult *go_wait(void *h, uint64_t ms) { return p_wait(h, ms); }
static void go_free_result(struct FffResult *r) { p_free_result(r); }
static void go_free_search(struct FffSearchResult *r) { p_free_search(r); }
static struct FffResult *go_health(void *h) { return p_health(h); }
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

var (
	loadOnce sync.Once
	bound    bool
	loadErr  error
)

// Load resolves the fff C library at runtime. Search paths:
// FFF_LIB_PATH env, ./third_party/fff/libfff_c.so, /usr/local/lib/libfff_c.so.
func Load() error {
	loadOnce.Do(func() {
		candidates := []string{}
		if p := os.Getenv("FFF_LIB_PATH"); p != "" {
			candidates = append(candidates, p)
		}
		// Resolve relative to the executable too — the server CWD is not
		// guaranteed to be the module root.
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(exeDir, "third_party/fff/libfff_c.so"),
				filepath.Join(exeDir, "third_party/fff/libfff_c.dylib"),
			)
		}
		candidates = append(candidates,
			"third_party/fff/libfff_c.so",
			"third_party/fff/libfff_c.dylib",
			"/usr/local/lib/libfff_c.so",
			"/usr/local/lib/libfff_c.dylib",
		)
		var handle unsafe.Pointer
		for _, path := range candidates {
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
			loadErr = fmt.Errorf("fff library not found (set FFF_LIB_PATH)")
			return
		}
		if !bool(C.fff_bind_all(handle)) {
			loadErr = fmt.Errorf("fff library missing required symbols")
			return
		}
		bound = true
	})
	return loadErr
}

// Available reports whether the library loaded successfully.
func Available() bool {
	return Load() == nil
}

// Item is one search hit.
type Item struct {
	RelPath string
	Name    string
	Size    int64
	IsDir   bool
}

// Instance is one indexed directory.
type Instance struct {
	handle unsafe.Pointer
}

// Create indexes basePath with the background watcher enabled.
func Create(basePath string) (*Instance, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	cBase := C.CString(basePath)
	defer C.free(unsafe.Pointer(cBase))
	opts := C.FffCreateOptions{
		version:           C.FFF_CREATE_OPTIONS_VERSION,
		base_path:         cBase,
		enable_mmap_cache: true,
		watch:             true,
		ai_mode:           true,
	}
	res := C.go_create(&opts)
	if res == nil {
		return nil, fmt.Errorf("fff_create_instance returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		msg := "unknown error"
		if res.error != nil {
			msg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff_create_instance: %s", msg)
	}
	inst := &Instance{handle: res.handle}
	// Wait briefly for the initial scan so early searches have results.
	C.go_wait(inst.handle, 2000)
	return inst, nil
}

// Search runs a fuzzy path search with a result limit.
func (i *Instance) Search(query string, limit int) ([]Item, error) {
	if i.handle == nil {
		return nil, fmt.Errorf("fff instance destroyed")
	}
	// Empty query must be "" (NULL is rejected by the C ABI).
	cQuery := C.CString(strings.TrimSpace(query))
	defer C.free(unsafe.Pointer(cQuery))
	if limit <= 0 {
		limit = 20
	}
	res := C.go_search(i.handle, cQuery, nil, 0, 0, C.uint32_t(limit), 0, 0)
	if res == nil {
		return nil, fmt.Errorf("fff_search returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		msg := "unknown error"
		if res.error != nil {
			msg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff_search: %s", msg)
	}
	sr := (*C.FffSearchResult)(unsafe.Pointer(res.handle))
	count := int(sr.count)
	if count > limit {
		count = limit
	}
	items := make([]Item, 0, count)
	slice := unsafe.Slice(sr.items, int(sr.count))
	for _, it := range slice[:count] {
		items = append(items, Item{
			RelPath: C.GoString(it.relative_path),
			Name:    C.GoString(it.file_name),
			Size:    int64(it.size),
		})
	}
	return items, nil
}

// Health returns the instance health JSON (indexed counts etc.). The C ABI
// returns the string in the FffResult envelope; it is freed with the result.
func (i *Instance) Health() string {
	if i.handle == nil {
		return "{}"
	}
	res := C.go_health(i.handle)
	if res == nil {
		return "{}"
	}
	defer C.go_free_result(res)
	if !bool(res.success) || res.handle == nil {
		return "{}"
	}
	return C.GoString((*C.char)(res.handle))
}

// Destroy frees the underlying instance.
func (i *Instance) Destroy() {
	if i.handle != nil {
		C.go_destroy(i.handle)
		i.handle = nil
	}
}

// Manager owns one Instance per project root, created lazily.
type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance
	creating  map[string]*sync.Once
	enabled   bool
}

func NewManager() *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		creating:  make(map[string]*sync.Once),
		enabled:   Available(),
	}
}

// Enabled reports whether fff was loaded.
func (m *Manager) Enabled() bool { return m.enabled }

// SearchAsync returns results when the index is warm; on a cold root it
// kicks off the background scan and returns ok=false so the caller can fall
// back to the walk-based search until the index is ready.
func (m *Manager) SearchAsync(root, query string, limit int) ([]Item, bool) {
	if !m.enabled {
		return nil, false
	}
	m.mu.Lock()
	inst, ok := m.instances[root]
	if !ok {
		once, started := m.creating[root]
		if !started {
			once = &sync.Once{}
			m.creating[root] = once
			go once.Do(func() {
				created, err := Create(root)
				m.mu.Lock()
				if err == nil {
					m.instances[root] = created
				}
				delete(m.creating, root)
				m.mu.Unlock()
			})
		}
		m.mu.Unlock()
		return nil, false
	}
	m.mu.Unlock()
	items, err := inst.Search(query, limit)
	if err != nil {
		return nil, false
	}
	return items, true
}

// CloseAll destroys every instance.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for root, inst := range m.instances {
		inst.Destroy()
		delete(m.instances, root)
	}
}
