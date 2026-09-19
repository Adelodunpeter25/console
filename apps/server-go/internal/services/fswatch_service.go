// Recursive filesystem watcher with per-path debouncing. Port of
// apps/server/api/src/services/fswatch.service.ts using fsnotify (which
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
	debounce map[string]*time.Timer
	subs     map[chan types.FsChangeEvent]string // chan -> project path filter
	closed   bool
}

func NewFsWatchService() (*FsWatchService, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	s := &FsWatchService{
		watcher:  w,
		watched:  make(map[string]bool),
		debounce: make(map[string]*time.Timer),
		subs:     make(map[chan types.FsChangeEvent]string),
	}
	go s.loop()
	return s, nil
}

func (s *FsWatchService) loop() {
	for event := range s.watcher.Events {
		if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
			continue
		}
		projectPath := s.projectFor(event.Name)
		if projectPath == "" {
			continue
		}
		// The TS watcher applies ignore rules to paths relative to the
		// project root, so absolute paths through ignored ancestors
		// (e.g. /tmp) don't get filtered.
		rel := strings.TrimPrefix(event.Name, projectPath+"/")
		if utils.IsPathIgnored(rel) {
			continue
		}
		// New directories get their own watches for recursion.
		if event.Op&fsnotify.Create != 0 {
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				s.addDirRecursive(event.Name)
			}
		}
		s.scheduleEmit(projectPath, event.Name)
	}
}

func (s *FsWatchService) projectFor(eventPath string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for project := range s.watched {
		if len(eventPath) >= len(project) && eventPath[:len(project)] == project {
			return project
		}
	}
	return ""
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
	already := s.watched[abs]
	s.mu.Unlock()
	if !already {
		s.addDirRecursive(abs)
	}
}

// Subscribe returns a channel receiving change events for projectPath.
func (s *FsWatchService) Subscribe(projectPath string) chan types.FsChangeEvent {
	ch := make(chan types.FsChangeEvent, 64)
	s.mu.Lock()
	s.subs[ch] = projectPath
	s.mu.Unlock()
	return ch
}

func (s *FsWatchService) Unsubscribe(ch chan types.FsChangeEvent) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

func (s *FsWatchService) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.watcher.Close()
}
