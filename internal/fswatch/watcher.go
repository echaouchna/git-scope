package fswatch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/echaouchna/git-scope/internal/model"
	"github.com/fsnotify/fsnotify"
)

// RepoWatcher watches repository working trees for filesystem changes.
type RepoWatcher struct {
	watcher *fsnotify.Watcher
	events  chan struct{}
	errors  chan error
	done    chan struct{}

	mu   sync.Mutex
	dirs map[string]struct{}
}

// NewRepoWatcher creates a watcher for the given repositories.
func NewRepoWatcher(repos []model.Repo, ignore []string) (*RepoWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	rw := &RepoWatcher{
		watcher: w,
		events:  make(chan struct{}, 1),
		errors:  make(chan error, 1),
		done:    make(chan struct{}),
		dirs:    map[string]struct{}{},
	}

	ignoreSet := make(map[string]struct{}, len(ignore))
	for _, name := range ignore {
		ignoreSet[name] = struct{}{}
	}

	for _, repo := range repos {
		if err := rw.addRepo(repo.Path, ignoreSet); err != nil {
			_ = rw.Close()
			return nil, err
		}
	}

	go rw.loop(ignoreSet)
	return rw, nil
}

// Close stops the watcher and releases resources.
func (rw *RepoWatcher) Close() error {
	select {
	case <-rw.done:
		return nil
	default:
		close(rw.done)
	}
	return rw.watcher.Close()
}

// WaitEvent blocks until a relevant filesystem event or an error occurs.
func (rw *RepoWatcher) WaitEvent() error {
	select {
	case <-rw.events:
		return nil
	case err := <-rw.errors:
		return err
	case <-rw.done:
		return nil
	}
}

func (rw *RepoWatcher) loop(ignoreSet map[string]struct{}) {
	for {
		select {
		case event, ok := <-rw.watcher.Events:
			if !ok {
				return
			}

			// Add watcher for new directories so nested files are tracked.
			if event.Op&fsnotify.Create != 0 {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					_ = rw.addDirTree(event.Name, ignoreSet, true)
				}
			}

			// Any write/create/remove/rename can change git status.
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				select {
				case rw.events <- struct{}{}:
				default:
				}
			}
		case err, ok := <-rw.watcher.Errors:
			if !ok {
				return
			}
			select {
			case rw.errors <- err:
			default:
			}
		case <-rw.done:
			return
		}
	}
}

func (rw *RepoWatcher) addRepo(repoPath string, ignoreSet map[string]struct{}) error {
	// Watch all working-tree directories recursively.
	if err := rw.addDirTree(repoPath, ignoreSet, false); err != nil {
		return err
	}

	// Also watch .git directory itself to catch index/head updates.
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitDir)
	if err == nil && info.IsDir() {
		return rw.addDir(gitDir)
	}
	return nil
}

func (rw *RepoWatcher) addDirTree(root string, ignoreSet map[string]struct{}, skipGit bool) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()
		if skipGit && name == ".git" {
			return filepath.SkipDir
		}
		if _, ignored := ignoreSet[name]; ignored {
			return filepath.SkipDir
		}

		return rw.addDir(path)
	})
}

func (rw *RepoWatcher) addDir(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	rw.mu.Lock()
	if _, ok := rw.dirs[absPath]; ok {
		rw.mu.Unlock()
		return nil
	}
	rw.dirs[absPath] = struct{}{}
	rw.mu.Unlock()

	if err := rw.watcher.Add(absPath); err != nil {
		rw.mu.Lock()
		delete(rw.dirs, absPath)
		rw.mu.Unlock()
		return err
	}
	return nil
}

// IsResourceLimitError reports whether an error likely indicates OS watcher limits.
func IsResourceLimitError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) || errors.Is(err, syscall.ENOSPC) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "too many open files") ||
		strings.Contains(msg, "no space left on device") ||
		strings.Contains(msg, "user limit")
}
