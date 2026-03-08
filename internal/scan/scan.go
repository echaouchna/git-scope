package scan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/echaouchna/git-scope/internal/gitstatus"
	"github.com/echaouchna/git-scope/internal/model"
)

// smartIgnorePatterns are always-ignored directories for performance
// These are system/tool directories that should never contain user repos
var smartIgnorePatterns = []string{
	// macOS/Linux system directories
	"Library", ".Trash", ".cache", ".local",
	// Package managers & runtimes
	".npm", ".yarn", ".pnpm", ".bun", ".cargo", ".rustup", ".go",
	".venv", ".pyenv", ".rbenv", ".nvm", ".sdkman",
	// IDE extensions (contain third-party repos, not your code)
	".vscode", ".vscode-server", ".cursor", ".zed", ".idea", ".atom",
	// Shell & tools configs
	".oh-my-zsh", ".tmux", ".vim", ".emacs.d", ".gemini",
	// Docker/Cloud
	".docker", ".kube", ".ssh", ".gnupg",
	// Cloud sync (slow and likely duplicates)
	"Google Drive", "OneDrive", "Dropbox", "iCloud",
}

// ScanRoots recursively scans the given root directories for git repositories
// It skips directories matching the ignore patterns
func ScanRoots(roots, ignore []string) ([]model.Repo, error) {
	// Build ignore rules from user config + smart defaults.
	// User rules support explicit semantics:
	// - exact:<name> (or just <name>) for exact directory-name match
	// - glob:<pattern> for directory-name glob match
	// - path:<pattern> for repo-root-relative path matching
	// - regex:<expr> or /expr/ for regex match (name or relative path)
	ignoreRules := buildIgnoreRules(ignore)
	for _, pattern := range smartIgnorePatterns {
		ignoreRules = append(ignoreRules, ignoreRule{
			kind:    ignoreRuleExact,
			pattern: pattern,
		})
	}

	var mu sync.Mutex
	var repos []model.Repo
	var wg sync.WaitGroup

	for _, root := range roots {
		// Expand ~ and environment variables
		root = expandPath(root)

		// Check if root exists
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			err := filepath.WalkDir(r, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					// Skip directories we can't access
					return nil
				}

				// Skip ignored directories
				if d.IsDir() {
					relPath, err := filepath.Rel(r, path)
					if err != nil {
						relPath = d.Name()
					}
					if shouldIgnore(d.Name(), relPath, ignoreRules) {
						return filepath.SkipDir
					}
				}

				// Found a .git directory
				if d.IsDir() && d.Name() == ".git" {
					repoPath := filepath.Dir(path)

					// Resolve to absolute path to get proper repo name
					// This handles cases where path is "." or relative
					absPath, err := filepath.Abs(repoPath)
					if err == nil {
						repoPath = absPath
					}
					repoName := filepath.Base(repoPath)

					repo := model.Repo{
						Name: repoName,
						Path: repoPath,
					}

					mu.Lock()
					repos = append(repos, repo)
					mu.Unlock()

					// Don't walk into .git directory
					return filepath.SkipDir
				}

				return nil
			})
			if err != nil {
				// Log but don't fail
				fmt.Fprintf(os.Stderr, "warning: scan error in %s: %v\n", r, err)
			}
		}(root)
	}

	wg.Wait()
	return refreshStatusesWithPool(repos, defaultStatusWorkers()), nil
}

type ignoreRuleKind int

const (
	ignoreRuleExact ignoreRuleKind = iota
	ignoreRuleGlob
	ignoreRulePath
	ignoreRuleRegex
)

type ignoreRule struct {
	kind    ignoreRuleKind
	pattern string
	regex   *regexp.Regexp
}

func buildIgnoreRules(patterns []string) []ignoreRule {
	rules := make([]ignoreRule, 0, len(patterns))
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}

		switch {
		case strings.HasPrefix(p, "exact:"):
			v := strings.TrimSpace(strings.TrimPrefix(p, "exact:"))
			if v != "" {
				rules = append(rules, ignoreRule{kind: ignoreRuleExact, pattern: v})
			}
		case strings.HasPrefix(p, "glob:"):
			v := strings.TrimSpace(strings.TrimPrefix(p, "glob:"))
			if v != "" {
				rules = append(rules, ignoreRule{kind: ignoreRuleGlob, pattern: v})
			}
		case strings.HasPrefix(p, "path:"):
			v := cleanIgnorePath(strings.TrimSpace(strings.TrimPrefix(p, "path:")))
			if v != "" {
				rules = append(rules, ignoreRule{kind: ignoreRulePath, pattern: v})
			}
		case strings.HasPrefix(p, "regex:"):
			v := strings.TrimSpace(strings.TrimPrefix(p, "regex:"))
			if v == "" {
				continue
			}
			re, err := regexp.Compile(v)
			if err != nil {
				continue
			}
			rules = append(rules, ignoreRule{kind: ignoreRuleRegex, pattern: v, regex: re})
		case strings.HasPrefix(p, "/") && strings.HasSuffix(p, "/") && len(p) > 2:
			v := strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
			re, err := regexp.Compile(v)
			if err != nil {
				continue
			}
			rules = append(rules, ignoreRule{kind: ignoreRuleRegex, pattern: v, regex: re})
		default:
			// Default semantics are exact directory-name matching.
			rules = append(rules, ignoreRule{kind: ignoreRuleExact, pattern: p})
		}
	}
	return rules
}

func cleanIgnorePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.Trim(p, "/")
	if p == "." {
		return ""
	}
	return p
}

// shouldIgnore checks if a directory name/relative path matches explicit ignore rules.
// Default semantics for user-provided patterns are exact directory-name matching.
func shouldIgnore(name, relPath string, rules []ignoreRule) bool {
	rel := cleanIgnorePath(relPath)
	for _, rule := range rules {
		switch rule.kind {
		case ignoreRuleExact:
			if name == rule.pattern {
				return true
			}
		case ignoreRuleGlob:
			if ok, _ := path.Match(rule.pattern, name); ok {
				return true
			}
		case ignoreRulePath:
			if rel == rule.pattern {
				return true
			}
			prefix := rule.pattern + "/"
			if strings.HasPrefix(rel, prefix) {
				return true
			}
			if ok, _ := path.Match(rule.pattern, rel); ok {
				return true
			}
		case ignoreRuleRegex:
			if rule.regex != nil && (rule.regex.MatchString(name) || rule.regex.MatchString(rel)) {
				return true
			}
		}
	}
	return false
}

// expandPath expands ~ and environment variables in a path
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		path = filepath.Join(home, path[2:])
	}
	return os.ExpandEnv(path)
}

// PrintJSON outputs the repos as formatted JSON
func PrintJSON(w io.Writer, repos []model.Repo) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(repos); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

// RefreshStatuses updates git status for an already-known repository list.
// It preserves repo identity fields (name/path) and only refreshes status.
func RefreshStatuses(repos []model.Repo) []model.Repo {
	return refreshStatusesWithPool(repos, defaultStatusWorkers())
}

func defaultStatusWorkers() int {
	workers := runtime.NumCPU() * 2
	if workers < 4 {
		workers = 4
	}
	if workers > 32 {
		workers = 32
	}
	return workers
}

func refreshStatusesWithPool(repos []model.Repo, maxWorkers int) []model.Repo {
	out := make([]model.Repo, len(repos))
	if len(repos) == 0 {
		return out
	}
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	if maxWorkers > len(repos) {
		maxWorkers = len(repos)
	}

	type task struct {
		idx int
	}

	tasks := make(chan task, maxWorkers)
	var wg sync.WaitGroup
	wg.Add(maxWorkers)
	for range maxWorkers {
		go func() {
			defer wg.Done()
			for t := range tasks {
				r := repos[t.idx]
				status, err := gitstatus.Status(r.Path)
				r.Status = status
				if err != nil {
					r.Status.ScanError = err.Error()
				}
				out[t.idx] = r
			}
		}()
	}

	for i := range repos {
		tasks <- task{idx: i}
	}
	close(tasks)
	wg.Wait()
	return out
}
