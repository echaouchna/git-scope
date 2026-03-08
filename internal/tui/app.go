package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/echaouchna/git-scope/internal/cache"
	"github.com/echaouchna/git-scope/internal/config"
	"github.com/echaouchna/git-scope/internal/fswatch"
	"github.com/echaouchna/git-scope/internal/model"
	"github.com/echaouchna/git-scope/internal/scan"
)

// Cache max age - use cached data if less than 5 minutes old
const cacheMaxAge = 5 * time.Minute
const repoPollInterval = 5 * time.Second

// Run starts the Bubbletea TUI application
func Run(cfg *config.Config) error {
	m := NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// scanReposCmd is a command that scans for repositories
// If forceRefresh is true, bypass cache and scan fresh
func scanReposCmd(cfg *config.Config, forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		cacheStore := cache.NewFileStore()

		// Try to load from cache first (unless forcing refresh)
		if !forceRefresh {
			cached, err := cacheStore.Load()
			if err == nil && cacheStore.IsValid(cacheMaxAge) && cacheStore.IsSameRoots(cfg.Roots) {
				return scanCompleteMsg{
					repos:     cached.Repos,
					fromCache: true,
				}
			}
		}

		// Scan fresh
		repos, err := scan.ScanRoots(cfg.Roots, cfg.Ignore)
		if err != nil {
			return scanErrorMsg{err: err}
		}

		// Save to cache (non-fatal on failure).
		warning := ""
		if err := cacheStore.Save(repos, cfg.Roots); err != nil {
			warning = "cache save failed: " + err.Error()
		}

		return scanCompleteMsg{
			repos:     repos,
			fromCache: false,
			warning:   warning,
		}
	}
}

// scanCompleteMsg is sent when scanning is complete
type scanCompleteMsg struct {
	repos     []model.Repo
	fromCache bool
	warning   string
}

// scanErrorMsg is sent when scanning fails
type scanErrorMsg struct {
	err error
}

type repoWatcherStartedMsg struct {
	watcher *fswatch.RepoWatcher
}

type repoWatchEventMsg struct{}

type repoWatchErrorMsg struct {
	err error
}

type repoWatchFallbackMsg struct {
	err error
}

type repoPollTickMsg struct{}

type repoStatusRefreshMsg struct {
	repos []model.Repo
}

type standupReportMsg struct {
	since       string
	allBranches bool
	author      string
	summary     string
	lines       []string
	err         error
}

type standupAuthorsLoadedMsg struct {
	authors []string
	err     error
}

func startRepoWatcherCmd(repos []model.Repo, ignore []string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fswatch.NewRepoWatcher(repos, ignore)
		if err != nil {
			if fswatch.IsResourceLimitError(err) {
				return repoWatchFallbackMsg{err: err}
			}
			return repoWatchErrorMsg{err: err}
		}
		return repoWatcherStartedMsg{watcher: watcher}
	}
}

func waitRepoWatchEventCmd(watcher *fswatch.RepoWatcher) tea.Cmd {
	return func() tea.Msg {
		if watcher == nil {
			return nil
		}
		if err := watcher.WaitEvent(); err != nil {
			return repoWatchErrorMsg{err: err}
		}
		return repoWatchEventMsg{}
	}
}

func refreshRepoStatusesCmd(repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		return repoStatusRefreshMsg{repos: scan.RefreshStatuses(repos)}
	}
}

func startRepoPollTickCmd() tea.Cmd {
	return tea.Tick(repoPollInterval, func(time.Time) tea.Msg {
		return repoPollTickMsg{}
	})
}

func runStandupReportCmd(repos []model.Repo, since string, allBranches bool, author string) tea.Cmd {
	return func() tea.Msg {
		summary, lines, err := buildStandupReport(repos, since, allBranches, author)
		return standupReportMsg{
			since:       since,
			allBranches: allBranches,
			author:      author,
			summary:     summary,
			lines:       lines,
			err:         err,
		}
	}
}

func buildStandupReport(repos []model.Repo, since string, allBranches bool, author string) (string, []string, error) {
	if len(repos) == 0 {
		return "No repositories loaded", nil, nil
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Path < repos[j].Path
	})

	sinceArg := normalizeStandupSinceArg(since)
	lines := []string{}
	active := 0

	results := collectStandupReportResults(repos, sinceArg, 5, allBranches, author)
	for _, result := range results {
		if len(result.commits) == 0 && !result.repo.Status.IsDirty && result.logErr == nil {
			continue
		}

		active++
		lines = append(lines, fmt.Sprintf("[%s] %s", result.repo.Name, result.repo.Path))
		if result.repo.Status.Branch != "" {
			lines = append(lines, fmt.Sprintf("  branch: %s (ahead:%d behind:%d)", result.repo.Status.Branch, result.repo.Status.Ahead, result.repo.Status.Behind))
		}
		if result.repo.Status.IsDirty {
			lines = append(lines, fmt.Sprintf("  dirty: staged=%d modified=%d untracked=%d", result.repo.Status.Staged, result.repo.Status.Unstaged, result.repo.Status.Untracked))
		}
		if len(result.commits) > 0 {
			lines = append(lines, "  commits:")
			for _, c := range result.commits {
				lines = append(lines, "  + "+c)
			}
		}
		if result.logErr != nil {
			lines = append(lines, "  warning: git log failed: "+result.logErr.Error())
		}
		lines = append(lines, "")
	}

	scope := "current branch"
	if allBranches {
		scope = "all branches"
	}
	authorLabel := ""
	if strings.TrimSpace(author) != "" {
		authorLabel = ", author: " + author
	}
	if active == 0 {
		return fmt.Sprintf("Standup (%s, %s%s): no activity across %d repos", since, scope, authorLabel, len(repos)), []string{"No recent commits or dirty working trees found."}, nil
	}
	return fmt.Sprintf("Standup (%s, %s%s): %d/%d repos with activity", since, scope, authorLabel, active, len(repos)), lines, nil
}

type standupReportResult struct {
	repo    model.Repo
	commits []string
	logErr  error
}

func collectStandupReportResults(repos []model.Repo, since string, limit int, allBranches bool, author string) []standupReportResult {
	type task struct {
		idx  int
		repo model.Repo
	}

	results := make([]standupReportResult, len(repos))
	if len(repos) == 0 {
		return results
	}

	workerCount := runtime.NumCPU()
	if workerCount < 4 {
		workerCount = 4
	}
	if workerCount > 24 {
		workerCount = 24
	}
	if workerCount > len(repos) {
		workerCount = len(repos)
	}

	tasks := make(chan task, workerCount)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for t := range tasks {
				commits, err := repoRecentCommitsForStandup(t.repo.Path, since, limit, allBranches, author)
				results[t.idx] = standupReportResult{
					repo:    t.repo,
					commits: commits,
					logErr:  err,
				}
			}
		}()
	}

	for i, repo := range repos {
		tasks <- task{idx: i, repo: repo}
	}
	close(tasks)
	wg.Wait()
	return results
}

func repoRecentCommitsForStandup(repoPath, since string, limit int, allBranches bool, author string) ([]string, error) {
	args := []string{"log", "--since=" + since}
	if allBranches {
		args = append(args, "--all")
	}
	if strings.TrimSpace(author) != "" {
		args = append(args, "--author="+author)
	}
	args = append(args, "--pretty=format:%h %an | %s", fmt.Sprintf("-%d", limit))
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "\n")
	commits := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			commits = append(commits, p)
		}
	}
	return commits, nil
}

func normalizeStandupSinceArg(input string) string {
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" || value == "24h" {
		return "24 hours ago"
	}
	if strings.HasSuffix(value, "h") || strings.HasSuffix(value, "d") || strings.HasSuffix(value, "w") {
		n := strings.TrimSpace(value[:len(value)-1])
		if n != "" {
			switch value[len(value)-1] {
			case 'h':
				return n + " hours ago"
			case 'd':
				return n + " days ago"
			case 'w':
				return n + " weeks ago"
			}
		}
	}
	return input
}

func loadStandupAuthorsCmd(repos []model.Repo, since string, allBranches bool) tea.Cmd {
	return func() tea.Msg {
		authors, err := collectStandupAuthors(repos, normalizeStandupSinceArg(since), allBranches)
		return standupAuthorsLoadedMsg{authors: authors, err: err}
	}
}

func collectStandupAuthors(repos []model.Repo, since string, allBranches bool) ([]string, error) {
	type task struct {
		idx  int
		repo model.Repo
	}

	if len(repos) == 0 {
		return nil, nil
	}

	workerCount := runtime.NumCPU()
	if workerCount < 4 {
		workerCount = 4
	}
	if workerCount > 24 {
		workerCount = 24
	}
	if workerCount > len(repos) {
		workerCount = len(repos)
	}

	tasks := make(chan task, workerCount)
	out := make(chan []string, len(repos))
	errs := make(chan error, len(repos))
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for t := range tasks {
				authors, err := repoActiveAuthors(t.repo.Path, since, allBranches)
				if err != nil {
					errs <- err
					continue
				}
				out <- authors
			}
		}()
	}

	for i, repo := range repos {
		tasks <- task{idx: i, repo: repo}
	}
	close(tasks)
	wg.Wait()
	close(out)
	close(errs)

	seen := map[string]struct{}{}
	for group := range out {
		for _, a := range group {
			if a == "" {
				continue
			}
			seen[a] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for a := range seen {
		result = append(result, a)
	}
	sort.Strings(result)

	for err := range errs {
		if err != nil {
			// Non-fatal: keep partial author list.
			return result, err
		}
	}
	return result, nil
}

func repoActiveAuthors(repoPath, since string, allBranches bool) ([]string, error) {
	args := []string{"log", "--since=" + since}
	if allBranches {
		args = append(args, "--all")
	}
	args = append(args, "--format=%an")
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log authors: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	lines := strings.Split(raw, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return lines, nil
}

// openEditorMsg is sent to trigger opening an editor
type openEditorMsg struct {
	path   string
	cwd    string
	binary string
	args   []string
	label  string
}
