package tui

import (
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

// openEditorMsg is sent to trigger opening an editor
type openEditorMsg struct {
	path   string
	cwd    string
	binary string
	args   []string
	label  string
}
