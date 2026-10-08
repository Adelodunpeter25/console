// Recursive filesystem watcher with per-path debouncing, built on fsnotify (which
// needs per-directory watch registration).
package services

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
	"github.com/fsnotify/fsnotify"
)

type FsWatchService struct {
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	watched  map[string]bool
	gitMeta  map[string]string // watched git dir -> project path filter
	debounce map[string]*time.Timer
	subs     map[chan types.FsChangeEvent]string // chan -> project path filter
	closed   bool

	// roots are the project paths callers asked to watch; watched also holds
	// every subdirectory under them, so it must not be used to find a project.
	roots map[string]bool
	// versions bumps per project on every non-ignored change so listing
	// caches drop as soon as anything under that project changes. Entries
	// outlive a release so a re-watched project never reuses an old number.
	versions map[string]uint64
	// release timers drop a project's watches once nobody has used it for
	// idleRelease; every subdirectory watch costs an inotify/kqueue handle.
	release     map[string]*time.Timer
	idleRelease time.Duration
}

const defaultIdleRelease = 2 * time.Minute

// SetIdleRelease changes how long an unsubscribed project keeps its watches.
func (s *FsWatchService) SetIdleRelease(d time.Duration) {
	s.mu.Lock()
	s.idleRelease = d
	s.mu.Unlock()
}

// Version returns a counter that increases whenever a non-ignored path under
// projectPath changes. A cached listing taken at version N is still valid
// while Version(projectPath) == N.
func (s *FsWatchService) Version(projectPath string) uint64 {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		abs = projectPath
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.versions[abs]
}

func NewFsWatchService() (*FsWatchService, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	s := &FsWatchService{
		watcher:  w,
		watched:  make(map[string]bool),
		gitMeta:  make(map[string]string),
		debounce: make(map[string]*time.Timer),
		subs:     make(map[chan types.FsChangeEvent]string),

		roots:       make(map[string]bool),
		versions:    make(map[string]uint64),
		release:     make(map[string]*time.Timer),
		idleRelease: defaultIdleRelease,
	}
	go s.loop()
	return s, nil
}

func (s *FsWatchService) loop() {
	for event := range s.watcher.Events {
		if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
			continue
		}
		// Git metadata (HEAD/index/branch refs) bypasses the ignore rules:
		// .git is ignored as a tree, but these paths record commits,
		// checkouts, and branch switches the status views must reflect.
		if project, ok := s.gitProjectFor(event.Name); ok {
			s.scheduleEmit(project, event.Name)
			continue
		}
		projects := s.projectsFor(event.Name)
		if len(projects) == 0 {
			continue
		}
		// New directories get their own watches for recursion.
		if event.Op&fsnotify.Create != 0 {
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				s.addDirRecursive(event.Name)
			}
		}
		for _, projectPath := range projects {
			// Ignore rules apply to paths relative to the project root, so
			// absolute paths through ignored ancestors (e.g. /tmp) don't get
			// filtered.
			rel := strings.TrimPrefix(event.Name, projectPath+"/")
			if utils.IsPathIgnored(rel) {
				continue
			}
			s.mu.Lock()
			s.versions[projectPath]++
			s.mu.Unlock()
			s.scheduleEmit(projectPath, event.Name)
		}
	}
}

// projectsFor returns every watched project root containing eventPath. The
// old lookup scanned s.watched, which also holds every subdirectory, so a
// nested event could be attributed to a subdirectory "project" nobody had
// subscribed to and was silently dropped. Nested roots all get notified.
func (s *FsWatchService) projectsFor(eventPath string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for root := range s.roots {
		if eventPath == root || strings.HasPrefix(eventPath, root+"/") {
			out = append(out, root)
		}
	}
	return out
}

func (s *FsWatchService) scheduleEmit(projectPath, eventPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timer, ok := s.debounce[projectPath]; ok {
		timer.Reset(300 * time.Millisecond)
		return
	}
	s.debounce[projectPath] = time.AfterFunc(300*time.Millisecond, func() {
		s.mu.Lock()
		delete(s.debounce, projectPath)
		subs := make([]chan types.FsChangeEvent, 0)
		for ch, filter := range s.subs {
			if filter == projectPath {
				subs = append(subs, ch)
			}
		}
		s.mu.Unlock()
		evt := types.FsChangeEvent{Type: "fsChange", ProjectPath: projectPath, EventPath: eventPath}
		for _, ch := range subs {
			select {
			case ch <- evt:
			default: // slow subscriber: drop instead of blocking the watcher
			}
		}
	})
}

func (s *FsWatchService) addDirRecursive(root string) {
	s.mu.Lock()
	w := s.watcher
	s.mu.Unlock()
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if utils.IsPathIgnored(d.Name()) {
			if path != root {
				return filepath.SkipDir
			}
		}
		s.mu.Lock()
		add := !s.watched[path]
		if add {
			s.watched[path] = true
		}
		s.mu.Unlock()
		if add {
			_ = w.Add(path)
		}
		return nil
	})
}

// Watch ensures a recursive watcher exists for the project path.
func (s *FsWatchService) Watch(projectPath string) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return
	}
	s.mu.Lock()
	already := s.roots[abs]
	s.roots[abs] = true
	s.touchLocked(abs)
	s.mu.Unlock()
	if !already {
		s.addDirRecursive(abs)
	}
	s.watchGitMeta(abs)
}

// touchLocked restarts the idle-release countdown for root when nobody is
// subscribed to it. Callers hold s.mu.
func (s *FsWatchService) touchLocked(root string) {
	if t, ok := s.release[root]; ok {
		t.Stop()
		delete(s.release, root)
	}
	if s.closed || s.hasSubscriberLocked(root) {
		return
	}
	s.release[root] = time.AfterFunc(s.idleRelease, func() { s.releaseIfIdle(root) })
}

func (s *FsWatchService) hasSubscriberLocked(root string) bool {
	for _, filter := range s.subs {
		if filter == root {
			return true
		}
	}
	return false
}

// releaseIfIdle drops root's watches unless it has subscribers again, and
// keeps any directory another watched root still covers.
func (s *FsWatchService) releaseIfIdle(root string) {
	s.mu.Lock()
	delete(s.release, root)
	if s.closed || !s.roots[root] || s.hasSubscriberLocked(root) {
		s.mu.Unlock()
		return
	}
	delete(s.roots, root)
	s.versions[root]++
	var drop []string
	for dir := range s.watched {
		if dir != root && !strings.HasPrefix(dir, root+"/") {
			continue
		}
		covered := false
		for other := range s.roots {
			if dir == other || strings.HasPrefix(dir, other+"/") {
				covered = true
				break
			}
		}
		if !covered {
			drop = append(drop, dir)
			delete(s.watched, dir)
		}
	}
	for dir, project := range s.gitMeta {
		if project == root {
			drop = append(drop, dir)
			delete(s.gitMeta, dir)
		}
	}
	w := s.watcher
	s.mu.Unlock()
	for _, dir := range drop {
		_ = w.Remove(dir)
	}
}

// WatchedDirCount reports how many directories currently hold a watch.
func (s *FsWatchService) WatchedDirCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.watched) + len(s.gitMeta)
}

// gitProjectFor maps a git-metadata event back to its project, so worktree
// commits (recorded outside the worktree dir) still notify the right view.
func (s *FsWatchService) gitProjectFor(eventPath string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for dir, project := range s.gitMeta {
		if eventPath == dir || strings.HasPrefix(eventPath, dir+"/") {
			return project, true
		}
	}
	return "", false
}

// watchGitMeta registers non-recursive watches on a repo's git metadata
// dirs: the git dir itself (HEAD/index rewrites) and refs/heads (branch
// updates). Resolves worktree .git pointer files to the real git dir.
func (s *FsWatchService) watchGitMeta(projectAbs string) {
	gitDir := filepath.Join(projectAbs, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		// Worktree (or submodule): .git is a pointer file.
		data, err := os.ReadFile(gitDir)
		if err != nil {
			return
		}
		line := strings.TrimSpace(string(data))
		const prefix = "gitdir: "
		if !strings.HasPrefix(line, prefix) {
			return
		}
		target := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if !filepath.IsAbs(target) {
			target = filepath.Join(projectAbs, target)
		}
		gitDir = target
	}
	s.mu.Lock()
	w := s.watcher
	s.mu.Unlock()
	for _, dir := range []string{gitDir, filepath.Join(gitDir, "refs", "heads")} {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		s.mu.Lock()
		_, seen := s.gitMeta[dir]
		if !seen {
			s.gitMeta[dir] = projectAbs
		}
		s.mu.Unlock()
		if !seen {
			_ = w.Add(dir)
		}
	}
}

// Subscribe returns a channel receiving change events for projectPath.
func (s *FsWatchService) Subscribe(projectPath string) chan types.FsChangeEvent {
	ch := make(chan types.FsChangeEvent, 64)
	if abs, err := filepath.Abs(projectPath); err == nil {
		projectPath = abs
	}
	s.mu.Lock()
	s.subs[ch] = projectPath
	if t, ok := s.release[projectPath]; ok {
		t.Stop()
		delete(s.release, projectPath)
	}
	s.mu.Unlock()
	return ch
}

func (s *FsWatchService) Unsubscribe(ch chan types.FsChangeEvent) {
	s.mu.Lock()
	root := s.subs[ch]
	delete(s.subs, ch)
	if s.roots[root] {
		s.touchLocked(root)
	}
	s.mu.Unlock()
}

func (s *FsWatchService) Close() {
	s.mu.Lock()
	s.closed = true
	for _, t := range s.release {
		t.Stop()
	}
	s.mu.Unlock()
	s.watcher.Close()
}
