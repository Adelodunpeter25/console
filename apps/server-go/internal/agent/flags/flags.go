// Package flags centralizes the harness feature toggles (the Phase 4 A/B
// switches). Each flag ships with today's behavior as its default; a default
// only flips after a bench A/B shows no drop in task success at lower cost.
//
// Flags reach the tools through their constructors or, until then, by reading
// the environment at call sites via FromEnv — one place to rename when the
// console.toml [harness] section lands.
package flags

// HarnessEnvPrefix marks every environment variable that toggles a harness
// feature. The bench records all set variables with this prefix into its
// results, so a run documents which flags were active.
const HarnessEnvPrefix = "CONSOLE_HARNESS_"

// Flags holds the harness feature toggles.
type Flags struct {
	// SparseLineNumbers makes read_file number only line 1, every 10th line
	// and the last line (~11% smaller reads). Default off: the A/B halved
	// read output but doubled editFile failures, for flat cost.
	SparseLineNumbers bool
	// LazyTools holds six rarely used built-in tools (webSearch, webFetch,
	// bashJob, askMany, memory, batchWrite) back from the request and loads
	// them on demand through loadTools. Default off until a bench A/B shows
	// no drop in success at lower cost.
	LazyTools bool
}

// FromEnv builds Flags from CONSOLE_HARNESS_* environment variables. getenv
// is injectable so tests and the bench can drive it directly.
func FromEnv(getenv func(string) string) Flags {
	return Flags{
		SparseLineNumbers: getenv(HarnessEnvPrefix+"SPARSE_LINE_NUMBERS") == "1",
		LazyTools:         getenv(HarnessEnvPrefix+"LAZY_TOOLS") == "1",
	}
}
