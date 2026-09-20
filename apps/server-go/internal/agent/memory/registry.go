// Memory store registry. Port of agent/src/memory/registry.ts: resolves
// (scope, projectId) to a cached store instance with LRU eviction.
package memory

import (
	"container/list"
	"path/filepath"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// MaxCachedProjectStores mirrors the TS LRU cap.
const MaxCachedProjectStores = 50

// Registry opens memory databases on demand:
// <storage>/projects/<projectId>/memory.db (project scope) and
// <storage>/memory-global.db (global scope).
type Registry struct {
	mu         sync.Mutex
	storageDir string
	global     *Store
	projects   map[string]*list.Element
	lru        *list.List
}

type projectEntry struct {
	id    string
	store *Store
}

// NewRegistry creates a registry rooted at dir ("" resolves the console
// storage dir, mirroring getConsoleStorageDir).
func NewRegistry(dir string) *Registry {
	if dir == "" {
		dir = utils.ConsoleStorageDir()
	}
	return &Registry{storageDir: dir, projects: map[string]*list.Element{}, lru: list.New()}
}

// ProjectMemoryDBPath returns the project scope database path.
func ProjectMemoryDBPath(storageDir, projectID string) string {
	return filepath.Join(storageDir, "projects", projectID, "memory.db")
}

// GlobalMemoryDBPath returns the shared global database path.
func GlobalMemoryDBPath(storageDir string) string {
	return filepath.Join(storageDir, "memory-global.db")
}

// StorageDir returns the registry root (tests).
func (r *Registry) StorageDir() string { return r.storageDir }

// ForProject returns the cached (or newly opened) project store.
func (r *Registry) ForProject(projectID string) (*Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if el, ok := r.projects[projectID]; ok {
		r.lru.MoveToFront(el)
		return el.Value.(*projectEntry).store, nil
	}
	store, err := OpenStore(ProjectMemoryDBPath(r.storageDir, projectID), ScopeProject)
	if err != nil {
		return nil, err
	}
	r.projects[projectID] = r.lru.PushFront(&projectEntry{id: projectID, store: store})
	for r.lru.Len() > MaxCachedProjectStores {
		back := r.lru.Back()
		if back == nil {
			break
		}
		victim := back.Value.(*projectEntry)
		if victim.id == projectID {
			break
		}
		victim.store.Close()
		delete(r.projects, victim.id)
		r.lru.Remove(back)
	}
	return store, nil
}

// ForGlobal returns the shared global store.
func (r *Registry) ForGlobal() (*Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.global != nil {
		return r.global, nil
	}
	store, err := OpenStore(GlobalMemoryDBPath(r.storageDir), ScopeGlobal)
	if err != nil {
		return nil, err
	}
	r.global = store
	return store, nil
}

// Resolve returns the store for a scope, requiring a project id for the
// project scope (mirrors the TS resolve error).
func (r *Registry) Resolve(scope Scope, projectID string) (*Store, error) {
	if scope == ScopeGlobal {
		return r.ForGlobal()
	}
	if projectID == "" {
		return nil, errNoProject()
	}
	return r.ForProject(projectID)
}

type noProjectError struct{}

func (e *noProjectError) Error() string {
	return "Memory scope 'project' requires a projectId; this session has no project."
}

func errNoProject() error { return &noProjectError{} }
