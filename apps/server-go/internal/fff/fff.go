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
typedef void (*fff_free_string_t)(char *);
typedef struct FffResult *(*fff_health_check_t)(void *, const char *);

static fff_create_instance_with_t p_create;
static fff_destroy_t p_destroy;
static fff_search_t p_search;
static fff_glob_t p_glob;
static fff_live_grep_t p_grep;
static fff_wait_for_scan_t p_wait;
static fff_free_result_t p_free_result;
static fff_free_search_result_t p_free_search;
static fff_free_grep_result_t p_free_grep;
static fff_free_string_t p_free_string;
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
	p_free_string = (fff_free_string_t)dlsym(dl, "fff_free_string");
	if (!p_free_string) return false;
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
static void go_free_string(char *s) { p_free_string(s); }
static struct FffResult *go_health(void *h) { return p_health(h, NULL); }
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

var errManagerClosed = errors.New("fff manager is closed")

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

// Item is one path-search hit.
type Item struct {
	RelPath string
	Name    string
	Size    int64
	IsDir   bool
}

// GrepMode selects fff's content-search matching strategy.
type GrepMode uint8

const (
	GrepModePlain GrepMode = 0
	GrepModeRegex GrepMode = 1
	GrepModeFuzzy GrepMode = 2
)

// CaseMode selects content-search case behavior. Smart is fff's usual mode:
// lowercase queries are insensitive, while a query containing uppercase text
// is sensitive.
type CaseMode string

const (
	CaseSmart       CaseMode = "smart"
	CaseSensitive   CaseMode = "sensitive"
	CaseInsensitive CaseMode = "insensitive"
)

// ParseCaseMode parses the public case-mode spelling. An empty value defaults
// to Smart so older callers retain the fff default.
func ParseCaseMode(value string) (CaseMode, error) {
	if strings.TrimSpace(value) == "" {
		return CaseSmart, nil
	}
	mode := CaseMode(strings.ToLower(strings.TrimSpace(value)))
	if !mode.valid() {
		return "", fmt.Errorf("invalid case mode %q (want smart, sensitive, or insensitive)", value)
	}
	return mode, nil
}

func (m CaseMode) valid() bool {
	return m == CaseSmart || m == CaseSensitive || m == CaseInsensitive
}

func (m CaseMode) smartCase() bool {
	// fff's legacy flag only distinguishes smart from sensitive. Explicit
	// insensitive mode is represented by the (?i) transform below.
	return m == CaseSmart
}

// MatchRange is a half-open byte range within a returned line. Byte offsets,
// rather than rune indexes, match the fff C ABI and the editor protocol.
type MatchRange struct {
	Start int
	End   int
}

// GrepMatch is one content-search hit.
type GrepMatch struct {
	RelPath             string
	FileName            string
	LineContent         string
	LineNumber          uint64
	Column              int
	EndColumn           int
	ByteOffset          uint64
	MatchRanges         []MatchRange
	ContextBefore       []string
	ContextAfter        []string
	Size                int64
	Modified            uint64
	TotalFrecencyScore  int64
	AccessFrecencyScore int64
	ModFrecencyScore    int64
	FuzzyScore          *uint16
	IsBinary            bool
	IsDefinition        bool
}

// RegexError reports that fff could not compile a regex and fell back to a
// literal search. The result may still contain matches for the literal text.
type RegexError struct{ Message string }

func (e *RegexError) Error() string { return e.Message }

// GrepOptions controls one content-search page.
type GrepOptions struct {
	Mode                GrepMode
	Case                CaseMode
	WholeWord           bool
	ContextLines        int
	MaxMatches          int
	Cursor              uint32
	TimeBudgetMs        uint64
	ClassifyDefinitions bool
}

// GrepResult is a copied, Go-owned view of one native grep result. Copying all
// pointers out before native cleanup makes the value safe to pass to the UI.
type GrepResult struct {
	Matches       []GrepMatch
	TotalMatched  int
	FilesSearched int
	TotalFiles    int
	FilteredFiles int
	NextCursor    uint32
	HasMore       bool
	RegexError    *RegexError
}

// TransformQuery applies the adapter's public matching transformations and
// returns the query/mode that can be passed to fff. Whole-word searches use a
// regexp boundary wrapper; this also makes plain searches exact when combined
// with whole-word mode. Explicit insensitive mode uses Rust/fff's inline
// (?i) flag because the legacy smart_case bit has no third state.
func TransformQuery(query string, mode GrepMode, caseMode CaseMode, wholeWord bool) (string, GrepMode, error) {
	if query == "" {
		return "", mode, errors.New("query is required")
	}
	if mode > GrepModeFuzzy {
		return "", mode, fmt.Errorf("invalid grep mode %d", mode)
	}
	if caseMode == "" {
		caseMode = CaseSmart
	}
	if !caseMode.valid() {
		return "", mode, fmt.Errorf("invalid case mode %q", caseMode)
	}

	pattern := query
	if mode == GrepModePlain {
		pattern = regexp.QuoteMeta(pattern)
	}
	if caseMode == CaseInsensitive {
		pattern = "(?i)" + pattern
		mode = GrepModeRegex
	}
	if wholeWord {
		pattern = `\b(?:` + pattern + `)\b`
		mode = GrepModeRegex
	}
	return pattern, mode, nil
}

// Instance is one indexed directory. Calls hold an RW lock so the manager can
// safely defer native destruction until every in-flight caller has released it.
type Instance struct {
	mu     sync.RWMutex
	handle unsafe.Pointer
}

// Lease keeps an Instance alive for one user operation. A lease must be
// released exactly when the caller is done; it is safe to call Close more than
// once. The manager removes an instance from its LRU map before destroying it,
// so an existing lease remains valid until Close.
type Lease struct {
	mu       sync.Mutex
	instance *Instance
	closed   bool
}

func newLease(instance *Instance) *Lease { return &Lease{instance: instance} }

func (l *Lease) check() error {
	if l == nil || l.instance == nil {
		return errors.New("fff lease is nil")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return errors.New("fff lease released")
	}
	return nil
}

// Release releases the native usage lease. Close is an alias for integrations
// that use the usual resource-closing terminology.
func (l *Lease) Release() {
	if l == nil || l.instance == nil {
		return
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	instance := l.instance
	l.mu.Unlock()
	instance.mu.RUnlock()
}

func (l *Lease) Close() { l.Release() }

// Search runs a fuzzy path search while the lease is held.
func (l *Lease) Search(query string, limit int) ([]Item, error) {
	if err := l.check(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, errors.New("fff lease released")
	}
	return l.instance.search(query, limit)
}

// Glob filters indexed paths while the lease is held.
func (l *Lease) Glob(pattern string, limit int) ([]Item, error) {
	if err := l.check(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, errors.New("fff lease released")
	}
	return l.instance.glob(pattern, limit)
}

// Grep preserves the original adapter call shape for existing internal callers.
func (l *Lease) Grep(query string, mode GrepMode, caseInsensitive bool, contextLines, maxMatches int) ([]GrepMatch, int, error) {
	caseMode := CaseSmart
	if caseInsensitive {
		caseMode = CaseInsensitive
	}
	result, err := l.GrepWithOptions(query, GrepOptions{
		Mode: mode, Case: caseMode, ContextLines: contextLines, MaxMatches: maxMatches,
	})
	return result.Matches, result.FilesSearched, err
}

// GrepWithOptions runs a content search while the lease is held.
func (l *Lease) GrepWithOptions(query string, options GrepOptions) (GrepResult, error) {
	if err := l.check(); err != nil {
		return GrepResult{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return GrepResult{}, errors.New("fff lease released")
	}
	return l.instance.grepWithOptions(query, options)
}

// Health returns the instance health JSON while the lease is held.
func (l *Lease) Health() string {
	if err := l.check(); err != nil {
		return "{}"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return "{}"
	}
	return l.instance.health()
}

// Create indexes basePath with content indexing and the background watcher
// enabled. The returned raw instance is intended for short-lived standalone
// use; Manager users should acquire a Lease instead.
func Create(basePath string) (*Instance, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	cBase := C.CString(basePath)
	defer C.free(unsafe.Pointer(cBase))
	opts := C.FffCreateOptions{
		version:                 C.FFF_CREATE_OPTIONS_VERSION,
		base_path:               cBase,
		enable_mmap_cache:       true,
		enable_content_indexing: true,
		watch:                   true,
		ai_mode:                 true,
	}
	res := C.go_create(&opts)
	if res == nil {
		return nil, fmt.Errorf("fff_create_instance returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		return nil, fmt.Errorf("fff_create_instance: %s", cString(res.error))
	}
	inst := &Instance{handle: res.handle}
	// fff_free_result does not free its handle; the wait result is independent
	// and must still be released even though we only use it as a warmup signal.
	waitRes := C.go_wait(inst.handle, 2000)
	if waitRes != nil {
		C.go_free_result(waitRes)
	}
	return inst, nil
}

func cString(value *C.char) string {
	if value == nil {
		return ""
	}
	return C.GoString(value)
}

func (i *Instance) acquire() (unsafe.Pointer, error) {
	i.mu.RLock()
	if i.handle == nil {
		i.mu.RUnlock()
		return nil, errors.New("fff instance destroyed")
	}
	return i.handle, nil
}

// Search runs a fuzzy path search with a result limit.
func (i *Instance) Search(query string, limit int) ([]Item, error) {
	if _, err := i.acquire(); err != nil {
		return nil, err
	}
	defer i.mu.RUnlock()
	return i.search(query, limit)
}

func (i *Instance) search(query string, limit int) ([]Item, error) {
	if i.handle == nil {
		return nil, errors.New("fff instance destroyed")
	}
	// Empty query must be "" (NULL is rejected by the C ABI).
	cQuery := C.CString(strings.TrimSpace(query))
	defer C.free(unsafe.Pointer(cQuery))
	if limit <= 0 {
		limit = 20
	}
	res := C.go_search(i.handle, cQuery, nil, 0, 0, C.uint32_t(limit), 0, 0)
	if res == nil {
		return nil, errors.New("fff_search returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		return nil, fmt.Errorf("fff_search: %s", cString(res.error))
	}
	if res.handle == nil {
		return []Item{}, nil
	}
	sr := (*C.FffSearchResult)(res.handle)
	if sr == nil {
		return nil, errors.New("fff_search returned a null result payload")
	}
	// fff_free_result only frees the envelope. Search payload ownership is
	// separate and must be released explicitly or every query leaks native
	// strings, score metadata, and the result arrays.
	defer C.go_free_search(sr)
	count := int(sr.count)
	if count > limit {
		count = limit
	}
	items := make([]Item, 0, count)
	if count == 0 || sr.items == nil {
		return items, nil
	}
	slice := unsafe.Slice(sr.items, count)
	for _, it := range slice {
		items = append(items, Item{
			RelPath: cString(it.relative_path),
			Name:    cString(it.file_name),
			Size:    int64(it.size),
		})
	}
	return items, nil
}

// Glob filters indexed files by a glob pattern (native fff glob, no query
// parsing — backs the agent glob tool the same way TS's FileFinder.glob does).
func (i *Instance) Glob(pattern string, limit int) ([]Item, error) {
	if _, err := i.acquire(); err != nil {
		return nil, err
	}
	defer i.mu.RUnlock()
	return i.glob(pattern, limit)
}

func (i *Instance) glob(pattern string, limit int) ([]Item, error) {
	if i.handle == nil {
		return nil, errors.New("fff instance destroyed")
	}
	cPattern := C.CString(pattern)
	defer C.free(unsafe.Pointer(cPattern))
	if limit <= 0 {
		limit = 200
	}
	res := C.go_glob(i.handle, cPattern, nil, 0, 0, C.uint32_t(limit))
	if res == nil {
		return nil, errors.New("fff_glob returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		return nil, fmt.Errorf("fff_glob: %s", cString(res.error))
	}
	if res.handle == nil {
		return []Item{}, nil
	}
	sr := (*C.FffSearchResult)(res.handle)
	if sr == nil {
		return nil, errors.New("fff_glob returned a null result payload")
	}
	defer C.go_free_search(sr)
	count := int(sr.count)
	if count > limit {
		count = limit
	}
	items := make([]Item, 0, count)
	if count == 0 || sr.items == nil {
		return items, nil
	}
	slice := unsafe.Slice(sr.items, count)
	for _, it := range slice {
		items = append(items, Item{
			RelPath: cString(it.relative_path),
			Name:    cString(it.file_name),
			Size:    int64(it.size),
		})
	}
	return items, nil
}

// Grep preserves the original adapter call shape for existing internal callers.
func (i *Instance) Grep(query string, mode GrepMode, caseInsensitive bool, contextLines, maxMatches int) ([]GrepMatch, int, error) {
	caseMode := CaseSmart
	if caseInsensitive {
		caseMode = CaseInsensitive
	}
	result, err := i.GrepWithOptions(query, GrepOptions{
		Mode: mode, Case: caseMode, ContextLines: contextLines, MaxMatches: maxMatches,
	})
	return result.Matches, result.FilesSearched, err
}

// GrepWithOptions runs fff's native content search and copies every useful
// result field into Go-owned memory before freeing the native result.
func (i *Instance) GrepWithOptions(query string, options GrepOptions) (GrepResult, error) {
	if _, err := i.acquire(); err != nil {
		return GrepResult{}, err
	}
	defer i.mu.RUnlock()
	return i.grepWithOptions(query, options)
}

func (i *Instance) grepWithOptions(query string, options GrepOptions) (GrepResult, error) {
	if i.handle == nil {
		return GrepResult{}, errors.New("fff instance destroyed")
	}
	if options.Case == "" {
		options.Case = CaseSmart
	}
	pattern, effectiveMode, err := TransformQuery(query, options.Mode, options.Case, options.WholeWord)
	if err != nil {
		return GrepResult{}, err
	}
	if options.MaxMatches <= 0 {
		options.MaxMatches = 100
	}
	if options.ContextLines < 0 {
		return GrepResult{}, errors.New("context lines must not be negative")
	}

	cQuery := C.CString(pattern)
	defer C.free(unsafe.Pointer(cQuery))
	res := C.go_grep(i.handle, cQuery, C.uint8_t(effectiveMode), 0, 0,
		C.bool(options.Case.smartCase()), C.uint32_t(options.Cursor),
		C.uint32_t(options.MaxMatches), C.uint64_t(options.TimeBudgetMs),
		C.uint32_t(options.ContextLines), C.uint32_t(options.ContextLines),
		C.bool(options.ClassifyDefinitions))
	if res == nil {
		return GrepResult{}, errors.New("fff_live_grep returned null")
	}
	defer C.go_free_result(res)
	if !bool(res.success) {
		return GrepResult{}, fmt.Errorf("fff_live_grep: %s", cString(res.error))
	}
	if res.handle == nil {
		return GrepResult{}, errors.New("fff_live_grep returned a null result payload")
	}
	gr := (*C.FffGrepResult)(res.handle)
	if gr == nil {
		return GrepResult{}, errors.New("fff_live_grep returned a null grep payload")
	}
	defer C.go_free_grep(gr)

	result := GrepResult{
		TotalMatched:  int(gr.total_matched),
		FilesSearched: int(gr.total_files_searched),
		TotalFiles:    int(gr.total_files),
		FilteredFiles: int(gr.filtered_file_count),
		NextCursor:    uint32(gr.next_file_offset),
		HasMore:       gr.next_file_offset != 0,
	}
	if gr.regex_fallback_error != nil {
		result.RegexError = &RegexError{Message: cString(gr.regex_fallback_error)}
	}

	count := int(gr.count)
	if count > options.MaxMatches {
		count = options.MaxMatches
	}
	result.Matches = make([]GrepMatch, 0, count)
	if count == 0 || gr.items == nil {
		return result, nil
	}
	slice := unsafe.Slice(gr.items, count)
	for _, m := range slice {
		match := GrepMatch{
			RelPath:             cString(m.relative_path),
			FileName:            cString(m.file_name),
			LineContent:         cString(m.line_content),
			LineNumber:          uint64(m.line_number),
			Column:              int(m.col),
			ByteOffset:          uint64(m.byte_offset),
			ContextBefore:       copyCStrings(m.context_before, int(m.context_before_count)),
			ContextAfter:        copyCStrings(m.context_after, int(m.context_after_count)),
			Size:                int64(m.size),
			Modified:            uint64(m.modified),
			TotalFrecencyScore:  int64(m.total_frecency_score),
			AccessFrecencyScore: int64(m.access_frecency_score),
			ModFrecencyScore:    int64(m.modification_frecency_score),
			IsBinary:            bool(m.is_binary),
			IsDefinition:        bool(m.is_definition),
		}
		if m.match_ranges_count > 0 && m.match_ranges != nil {
			ranges := unsafe.Slice(m.match_ranges, int(m.match_ranges_count))
			match.MatchRanges = make([]MatchRange, 0, len(ranges))
			for _, r := range ranges {
				match.MatchRanges = append(match.MatchRanges, MatchRange{Start: int(r.start), End: int(r.end)})
			}
			if len(match.MatchRanges) > 0 {
				match.EndColumn = match.MatchRanges[0].End
			}
		} else {
			match.EndColumn = match.Column
		}
		if bool(m.has_fuzzy_score) {
			score := uint16(m.fuzzy_score)
			match.FuzzyScore = &score
		}
		result.Matches = append(result.Matches, match)
	}
	return result, nil
}

func copyCStrings(values **C.char, count int) []string {
	if values == nil || count <= 0 {
		return nil
	}
	slice := unsafe.Slice(values, count)
	out := make([]string, 0, count)
	for _, value := range slice {
		out = append(out, cString(value))
	}
	return out
}

// Health returns the instance health JSON. The C ABI returns a separately
// owned C string in the result handle; copying it and freeing that string is
// required in addition to freeing the envelope.
func (i *Instance) Health() string {
	if _, err := i.acquire(); err != nil {
		return "{}"
	}
	defer i.mu.RUnlock()
	return i.health()
}

func (i *Instance) health() string {
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
	text := cString((*C.char)(res.handle))
	C.go_free_string((*C.char)(res.handle))
	return text
}

// Destroy waits for active leases and then frees the underlying instance.
func (i *Instance) Destroy() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.handle != nil {
		handle := i.handle
		i.handle = nil
		C.go_destroy(handle)
	}
}

// maxInstances caps how many fff instances (each with its own watcher,
// git-status and fsevents threads) stay resident at once. Every project
// root ever opened in a session used to get a permanent instance that was
// never evicted, so a long-running server would accumulate one watcher set
// per project and burn CPU on all of them concurrently. Keeping only the
// most recently used roots warm bounds that to a fixed thread/CPU budget.
const maxInstances = 1

type createState struct {
	done chan struct{}
	err  error
}

// Manager owns one Instance per project root, creates lazily, and evicts
// least-recently-used instances once more than maxInstances are warm.
type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance
	lastUsed  map[string]time.Time
	creating  map[string]*createState
	enabled   bool
	closed    bool
}

func NewManager() *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		lastUsed:  make(map[string]time.Time),
		creating:  make(map[string]*createState),
		enabled:   Available(),
	}
}

// Enabled reports whether fff was loaded.
func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled && !m.closed
}

// evictLRULocked removes the least-recently-used instance(s) from the map until
// the live set is back at or under maxInstances. The returned instances are
// destroyed by the caller after releasing m.mu, because native teardown can
// block while a watcher is finishing a scan. Existing leases keep their native
// instance alive until they are released.
func (m *Manager) evictLRULocked() []*Instance {
	var victims []*Instance
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
			return victims
		}
		if inst, ok := m.instances[oldestRoot]; ok {
			victims = append(victims, inst)
		}
		delete(m.instances, oldestRoot)
		delete(m.lastUsed, oldestRoot)
	}
	return victims
}

func (m *Manager) create(root string, state *createState) {
	created, err := Create(root)
	m.mu.Lock()
	if m.closed {
		state.err = errManagerClosed
		if err == nil {
			state.err = nil
		}
		delete(m.creating, root)
		close(state.done)
		m.mu.Unlock()
		if created != nil {
			created.Destroy()
		}
		return
	}
	if err != nil {
		state.err = err
	} else {
		m.instances[root] = created
		m.lastUsed[root] = time.Now()
	}
	victims := m.evictLRULocked()
	delete(m.creating, root)
	close(state.done)
	m.mu.Unlock()

	if err == nil {
		for _, victim := range victims {
			victim.Destroy()
		}
	} else if created != nil {
		created.Destroy()
	}
}

// ensureAsync starts the background index scan for root unless one is already
// running or complete.
func (m *Manager) ensureAsync(root string) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if _, ok := m.instances[root]; ok {
		m.lastUsed[root] = time.Now()
		m.mu.Unlock()
		return
	}
	if _, started := m.creating[root]; started {
		m.mu.Unlock()
		return
	}
	state := &createState{done: make(chan struct{})}
	m.creating[root] = state
	m.mu.Unlock()
	go m.create(root, state)
}

// Prewarm kicks off the background index scan for root without blocking.
// Call when a project/session opens so the first search hits a warm index
// instead of paying for the initial scan. No-op when fff is unavailable.
func (m *Manager) Prewarm(root string) {
	if !m.Enabled() {
		return
	}
	m.ensureAsync(root)
}

// SearchAsync returns results when the index is warm; on a cold root it kicks
// off the background scan and returns ok=false so the caller can fall back.
func (m *Manager) SearchAsync(root, query string, limit int) ([]Item, bool) {
	if !m.Enabled() {
		return nil, false
	}
	m.ensureAsync(root)
	lease, err := m.tryLease(root)
	if err != nil || lease == nil {
		return nil, false
	}
	defer lease.Release()
	items, err := lease.Search(query, limit)
	if err != nil {
		return nil, false
	}
	return items, true
}

func (m *Manager) tryLease(root string) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errManagerClosed
	}
	inst, ok := m.instances[root]
	if !ok {
		return nil, nil
	}
	// Acquire the read lease while holding the manager lock. Eviction also
	// holds this lock before removing the pointer, so a newly acquired lease
	// cannot race with Destroy.
	inst.mu.RLock()
	if inst.handle == nil {
		inst.mu.RUnlock()
		return nil, errors.New("fff instance destroyed")
	}
	m.lastUsed[root] = time.Now()
	return newLease(inst), nil
}

// GetOrCreate returns a usage lease for root, creating and indexing it
// synchronously if needed. Callers must release the returned lease.
func (m *Manager) GetOrCreate(root string) (*Lease, error) {
	if !m.Enabled() {
		return nil, errors.New("fff is not available")
	}
	m.ensureAsync(root)
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, errManagerClosed
		}
		if inst, ok := m.instances[root]; ok {
			inst.mu.RLock()
			if inst.handle == nil {
				inst.mu.RUnlock()
				m.mu.Unlock()
				return nil, errors.New("fff instance destroyed")
			}
			m.lastUsed[root] = time.Now()
			lease := newLease(inst)
			m.mu.Unlock()
			return lease, nil
		}
		state := m.creating[root]
		m.mu.Unlock()
		if state == nil {
			// The creation may have completed and been evicted between the
			// checks. Start another attempt; the manager's closed check keeps
			// this from becoming an infinite loop after shutdown.
			m.ensureAsync(root)
			continue
		}
		<-state.done
		if state.err != nil {
			return nil, state.err
		}
	}
}

// CloseAll stops accepting new work, removes all manager-owned instances, and
// waits for in-flight leases before native teardown. It is safe to call more
// than once.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	victims := make([]*Instance, 0, len(m.instances))
	states := make([]*createState, 0, len(m.creating))
	for root, inst := range m.instances {
		victims = append(victims, inst)
		delete(m.instances, root)
		delete(m.lastUsed, root)
	}
	for _, state := range m.creating {
		states = append(states, state)
	}
	m.mu.Unlock()

	for _, victim := range victims {
		victim.Destroy()
	}
	for _, state := range states {
		<-state.done
	}
}
