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

typedef struct FffMatchRange {
	uint32_t start;
	uint32_t end;
} FffMatchRange;

typedef struct FffGrepMatch {
	char *relative_path;
	char *file_name;
	char *git_status;
	char *line_content;
	FffMatchRange *match_ranges;
	char **context_before;
	char **context_after;
	uint64_t size;
	uint64_t modified;
	int64_t total_frecency_score;
	int64_t access_frecency_score;
	int64_t modification_frecency_score;
	uint64_t line_number;
	uint64_t byte_offset;
	uint32_t col;
	uint32_t match_ranges_count;
	uint32_t context_before_count;
	uint32_t context_after_count;
	uint16_t fuzzy_score;
	bool has_fuzzy_score;
	bool is_binary;
	bool is_definition;
} FffGrepMatch;

typedef struct FffGrepResult {
	FffGrepMatch *items;
	uint32_t count;
	uint32_t total_matched;
	uint32_t total_files_searched;
	uint32_t total_files;
	uint32_t filtered_file_count;
	uint32_t next_file_offset;
	char *regex_fallback_error;
} FffGrepResult;

typedef struct FffResult *(*fff_create_instance_with_t)(const FffCreateOptions *);
typedef void (*fff_destroy_t)(void *);
typedef struct FffResult *(*fff_search_t)(void *, const char *, const char *,
	uint32_t, uint32_t, uint32_t, int32_t, uint32_t);
typedef struct FffResult *(*fff_glob_t)(void *, const char *, const char *,
	uint32_t, uint32_t, uint32_t);
typedef struct FffResult *(*fff_live_grep_t)(void *, const char *, uint8_t, uint64_t,
	uint32_t, bool, uint32_t, uint32_t, uint64_t, uint32_t, uint32_t, bool);
typedef struct FffResult *(*fff_wait_for_scan_t)(void *, uint64_t);
typedef void (*fff_free_result_t)(struct FffResult *);
typedef void (*fff_free_search_result_t)(struct FffSearchResult *);
typedef void (*fff_free_grep_result_t)(struct FffGrepResult *);

static fff_create_instance_with_t p_create;
static fff_destroy_t p_destroy;
static fff_search_t p_search;
static fff_glob_t p_glob;
static fff_live_grep_t p_grep;
static fff_wait_for_scan_t p_wait;
static fff_free_result_t p_free_result;
static fff_free_search_result_t p_free_search;
static fff_free_grep_result_t p_free_grep;
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
	p_glob = (fff_glob_t)dlsym(dl, "fff_glob");
	if (!p_glob) return false;
	p_grep = (fff_live_grep_t)dlsym(dl, "fff_live_grep");
	if (!p_grep) return false;
	p_wait = (fff_wait_for_scan_t)dlsym(dl, "fff_wait_for_scan");
	if (!p_wait) return false;
	p_free_result = (fff_free_result_t)dlsym(dl, "fff_free_result");
	if (!p_free_result) return false;
	p_free_search = (fff_free_search_result_t)dlsym(dl, "fff_free_search_result");
	if (!p_free_search) return false;
	p_free_grep = (fff_free_grep_result_t)dlsym(dl, "fff_free_grep_result");
	if (!p_free_grep) return false;
	p_health = (fff_health_check_t)dlsym(dl, "fff_health_check");
	return p_health != NULL;
}

static struct FffResult *go_create(const FffCreateOptions *opts) { return p_create(opts); }
static void go_destroy(void *h) { p_destroy(h); }
static struct FffResult *go_search(void *h, const char *q, const char *cf,
	uint32_t t, uint32_t pi, uint32_t ps, int32_t cb, uint32_t mc) {
	return p_search(h, q, cf, t, pi, ps, cb, mc);
}
static struct FffResult *go_glob(void *h, const char *pattern, const char *cf,
	uint32_t t, uint32_t pi, uint32_t ps) {
	return p_glob(h, pattern, cf, t, pi, ps);
}
static struct FffResult *go_grep(void *h, const char *q, uint8_t mode, uint64_t maxFileSize,
	uint32_t maxMatchesPerFile, bool smartCase, uint32_t fileOffset, uint32_t pageLimit,
	uint64_t timeBudgetMs, uint32_t beforeCtx, uint32_t afterCtx, bool classifyDefs) {
	return p_grep(h, q, mode, maxFileSize, maxMatchesPerFile, smartCase, fileOffset,
		pageLimit, timeBudgetMs, beforeCtx, afterCtx, classifyDefs);
}
static struct FffResult *go_wait(void *h, uint64_t ms) { return p_wait(h, ms); }
static void go_free_result(struct FffResult *r) { p_free_result(r); }
static void go_free_search(struct FffSearchResult *r) { p_free_search(r); }
static void go_free_grep(struct FffGrepResult *r) { p_free_grep(r); }
static struct FffResult *go_health(void *h) { return p_health(h); }
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
		// guaranteed to be the module root. `console upgrade` drops the
		// sidecar directly next to the binary (no third_party nesting).
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(exeDir, "libfff_c.so"),
				filepath.Join(exeDir, "libfff_c.dylib"),
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

// GrepMatch is one content-search hit.
type GrepMatch struct {
	RelPath     string
	LineContent string
	LineNumber  uint64
}

// GrepMode selects fff's content-search matching strategy.
type GrepMode uint8

const (
	GrepModePlain GrepMode = 0
	GrepModeRegex GrepMode = 1
	GrepModeFuzzy GrepMode = 2
)

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

// Glob filters indexed files by a glob pattern (native fff glob, no query
// parsing — backs the agent glob tool the same way TS's FileFinder.glob does).
func (i *Instance) Glob(pattern string, limit int) ([]Item, error) {
	if i.handle == nil {
		return nil, fmt.Errorf("fff instance destroyed")
	}
	cPattern := C.CString(pattern)
	defer C.free(unsafe.Pointer(cPattern))
	if limit <= 0 {
		limit = 200
	}
	res := C.go_glob(i.handle, cPattern, nil, 0, 0, C.uint32_t(limit))
	if res == nil {
		return nil, fmt.Errorf("fff_glob returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		msg := "unknown error"
		if res.error != nil {
			msg = C.GoString(res.error)
		}
		return nil, fmt.Errorf("fff_glob: %s", msg)
	}
	sr := (*C.FffSearchResult)(unsafe.Pointer(res.handle))
	defer C.go_free_search(sr)
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

// Grep runs fff's native content search (backs the agent grep tool the same
// way TS's FileFinder.grep does).
func (i *Instance) Grep(query string, mode GrepMode, caseInsensitive bool, contextLines, maxMatches int) ([]GrepMatch, int, error) {
	if i.handle == nil {
		return nil, 0, fmt.Errorf("fff instance destroyed")
	}
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))
	if maxMatches <= 0 {
		maxMatches = 100
	}
	res := C.go_grep(i.handle, cQuery, C.uint8_t(mode), 0, 0, C.bool(!caseInsensitive),
		0, C.uint32_t(maxMatches), 0, C.uint32_t(contextLines), C.uint32_t(contextLines), false)
	if res == nil {
		return nil, 0, fmt.Errorf("fff_live_grep returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		msg := "unknown error"
		if res.error != nil {
			msg = C.GoString(res.error)
		}
		return nil, 0, fmt.Errorf("fff_live_grep: %s", msg)
	}
	gr := (*C.FffGrepResult)(unsafe.Pointer(res.handle))
	defer C.go_free_grep(gr)
	count := int(gr.count)
	if count > maxMatches {
		count = maxMatches
	}
	matches := make([]GrepMatch, 0, count)
	if count > 0 {
		slice := unsafe.Slice(gr.items, int(gr.count))
		for _, m := range slice[:count] {
			matches = append(matches, GrepMatch{
				RelPath:     C.GoString(m.relative_path),
				LineContent: C.GoString(m.line_content),
				LineNumber:  uint64(m.line_number),
			})
		}
	}
	return matches, int(gr.total_files_searched), nil
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

// maxInstances caps how many fff instances (each with its own watcher,
// git-status and fsevents threads) stay resident at once. Every project
// root ever opened in a session used to get a permanent instance that was
// never evicted, so a long-running server would accumulate one watcher set
// per project and burn CPU on all of them concurrently. Keeping only the
// most recently used roots warm bounds that to a fixed thread/CPU budget.
const maxInstances = 3

// Manager owns one Instance per project root, created lazily and evicted
// least-recently-used once more than maxInstances are warm.
type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance
	lastUsed  map[string]time.Time
	creating  map[string]*sync.Once
	enabled   bool
}

func NewManager() *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		lastUsed:  make(map[string]time.Time),
		creating:  make(map[string]*sync.Once),
		enabled:   Available(),
	}
}

// Enabled reports whether fff was loaded.
func (m *Manager) Enabled() bool { return m.enabled }

// evictLRULocked destroys the least-recently-used instance(s) until the
// live set is back at or under maxInstances. Caller must hold m.mu.
func (m *Manager) evictLRULocked() {
	for len(m.instances) > maxInstances {
		var oldestRoot string
		var oldestTime time.Time
		first := true
		for root := range m.instances {
			t := m.lastUsed[root]
			if first || t.Before(oldestTime) {
				oldestRoot, oldestTime, first = root, t, false
			}
		}
		if first {
			return
		}
		if inst, ok := m.instances[oldestRoot]; ok {
			inst.Destroy()
		}
		delete(m.instances, oldestRoot)
		delete(m.lastUsed, oldestRoot)
	}
}

// ensureAsync starts the background index scan for root unless one is
// already running or complete.
func (m *Manager) ensureAsync(root string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.instances[root]; ok {
		m.lastUsed[root] = time.Now()
		return
	}
	if _, started := m.creating[root]; started {
		return
	}
	once := &sync.Once{}
	m.creating[root] = once
	go once.Do(func() {
		created, err := Create(root)
		m.mu.Lock()
		if err == nil {
			m.instances[root] = created
			m.lastUsed[root] = time.Now()
			m.evictLRULocked()
		}
		delete(m.creating, root)
		m.mu.Unlock()
	})
}

// Prewarm kicks off the background index scan for root without blocking.
// Call when a project/session opens so the first search hits a warm index
// instead of paying for the initial scan. No-op when fff is unavailable.
func (m *Manager) Prewarm(root string) {
	if !m.enabled {
		return
	}
	m.ensureAsync(root)
}

// SearchAsync returns results when the index is warm; on a cold root it
// kicks off the background scan and returns ok=false so the caller can fall
// back to the walk-based search until the index is ready.
func (m *Manager) SearchAsync(root, query string, limit int) ([]Item, bool) {
	if !m.enabled {
		return nil, false
	}
	m.ensureAsync(root)
	m.mu.Lock()
	inst, ok := m.instances[root]
	if ok {
		m.lastUsed[root] = time.Now()
	}
	m.mu.Unlock()
	if !ok {
		return nil, false
	}
	items, err := inst.Search(query, limit)
	if err != nil {
		return nil, false
	}
	return items, true
}

// GetOrCreate returns the instance for root, creating and indexing it
// synchronously if needed. Unlike SearchAsync (interactive, non-blocking),
// tool calls (glob/grep) can afford to wait for the initial scan.
func (m *Manager) GetOrCreate(root string) (*Instance, error) {
	if !m.enabled {
		return nil, fmt.Errorf("fff is not available")
	}
	m.mu.Lock()
	if inst, ok := m.instances[root]; ok {
		m.lastUsed[root] = time.Now()
		m.mu.Unlock()
		return inst, nil
	}
	m.mu.Unlock()
	inst, err := Create(root)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if existing, ok := m.instances[root]; ok {
		m.lastUsed[root] = time.Now()
		m.mu.Unlock()
		inst.Destroy()
		return existing, nil
	}
	m.instances[root] = inst
	m.lastUsed[root] = time.Now()
	m.evictLRULocked()
	m.mu.Unlock()
	return inst, nil
}

// CloseAll destroys every instance.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for root, inst := range m.instances {
		inst.Destroy()
		delete(m.instances, root)
		delete(m.lastUsed, root)
	}
}
