package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/echaouchna/git-scope/internal/model"
)

type gitActionResultMsg struct {
	actionName string
	scopeName  string
	success    int
	failed     int
	firstError string
}

type gitActionRepoDoneMsg struct {
	repoName string
	started  bool
	err      error
	output   string
}

type gitActionRunner struct {
	results chan gitActionRepoDoneMsg
	cancel  context.CancelFunc
}

type gitActionRunnerStartedMsg struct {
	runner *gitActionRunner
}

type gitActionRepoProgressMsg struct {
	runner *gitActionRunner
	result gitActionRepoDoneMsg
}

type gitActionRunnerDoneMsg struct {
	runner *gitActionRunner
}

type gitActionRunnerHeartbeatMsg struct {
	runner *gitActionRunner
	at     time.Time
}

type commonBranchesLoadedMsg struct {
	branches []string
	err      error
}

func (m *Model) enterGitActionMode() tea.Cmd {
	m.state = StateGitAction
	m.gitActionCursor = 0
	m.gitActionType = GitActionPullRebase
	m.gitActionError = ""
	m.gitActionRunning = false
	m.gitActionLoadingBranch = false
	m.gitActionInput.SetValue("")
	m.gitActionInput.Blur()
	m.gitActionBranchOptions = nil
	m.gitActionBranchMatches = nil
	m.gitActionBranchIndex = 0
	m.gitActionQueue = nil
	m.gitActionExecArgs = nil
	m.gitActionRunner = nil
	m.gitActionCancelPending = false
	m.gitActionScopeName = ""
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = 0
	m.gitActionCurrentRepo = ""
	m.gitActionStartedAt = time.Time{}
	m.gitActionLastProgressAt = time.Time{}
	m.gitActionRepoTimeout = 0
	m.gitActionSuccess = 0
	m.gitActionFailed = 0
	m.gitActionFirstError = ""
	m.gitActionLogLines = nil
	m.gitActionLogOffset = 0
	return nil
}

func (m *Model) exitGitActionMode() {
	m.state = StateReady
	m.gitActionCursor = 0
	m.gitActionType = GitActionNone
	m.gitActionError = ""
	m.gitActionRunning = false
	m.gitActionLoadingBranch = false
	m.gitActionInput.SetValue("")
	m.gitActionInput.Blur()
	m.gitActionBranchOptions = nil
	m.gitActionBranchMatches = nil
	m.gitActionBranchIndex = 0
	m.gitActionQueue = nil
	m.gitActionExecArgs = nil
	m.gitActionRunner = nil
	m.gitActionCancelPending = false
	m.gitActionScopeName = ""
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = 0
	m.gitActionCurrentRepo = ""
	m.gitActionStartedAt = time.Time{}
	m.gitActionLastProgressAt = time.Time{}
	m.gitActionRepoTimeout = 0
	m.gitActionSuccess = 0
	m.gitActionFailed = 0
	m.gitActionFirstError = ""
	m.gitActionLogLines = nil
	m.gitActionLogOffset = 0
}

func (m *Model) resetGitActionRunState() {
	m.gitActionRunning = false
	m.gitActionQueue = nil
	m.gitActionExecArgs = nil
	m.gitActionRunner = nil
	m.gitActionCancelPending = false
	m.gitActionScopeName = ""
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = 0
	m.gitActionCurrentRepo = ""
	m.gitActionStartedAt = time.Time{}
	m.gitActionLastProgressAt = time.Time{}
	m.gitActionRepoTimeout = 0
	m.gitActionSuccess = 0
	m.gitActionFailed = 0
	m.gitActionFirstError = ""
	m.gitActionLogLines = nil
	m.gitActionLogOffset = 0
	m.gitActionError = ""
}

func (m *Model) setGitActionFromCursor() {
	switch m.gitActionCursor {
	case 0:
		m.gitActionType = GitActionPullRebase
	case 1:
		m.gitActionType = GitActionSwitch
	case 2:
		m.gitActionType = GitActionCreateBranch
	case 3:
		m.gitActionType = GitActionMergeNoFF
	default:
		m.gitActionType = GitActionPullRebase
	}
}

func (m Model) gitActionMenuLabels() []string {
	return []string{
		"pull --rebase",
		"switch branch",
		"create branch",
		"merge --no-ff",
	}
}

func (m Model) gitActionName() string {
	switch m.gitActionType {
	case GitActionPullRebase:
		return "pull --rebase"
	case GitActionSwitch:
		return "switch"
	case GitActionCreateBranch:
		return "create-branch"
	case GitActionMergeNoFF:
		return "merge --no-ff"
	default:
		return ""
	}
}

func (m *Model) refreshBranchMatches() {
	m.gitActionBranchMatches = nil
	m.gitActionBranchIndex = 0
	if len(m.gitActionBranchOptions) == 0 {
		return
	}

	query := strings.ToLower(strings.TrimSpace(m.gitActionInput.Value()))
	for _, branch := range m.gitActionBranchOptions {
		if query == "" || strings.HasPrefix(strings.ToLower(branch), query) {
			m.gitActionBranchMatches = append(m.gitActionBranchMatches, branch)
		}
	}
}

func (m *Model) applyNextBranchAutocomplete() {
	if len(m.gitActionBranchMatches) == 0 {
		return
	}
	branch := m.gitActionBranchMatches[m.gitActionBranchIndex]
	m.gitActionInput.SetValue(branch)
	m.gitActionInput.CursorEnd()
	m.gitActionBranchIndex = (m.gitActionBranchIndex + 1) % len(m.gitActionBranchMatches)
}

func (m Model) gitActionArgs() ([]string, error) {
	branch := strings.TrimSpace(m.gitActionInput.Value())
	switch m.gitActionType {
	case GitActionPullRebase:
		return []string{"pull", "--rebase"}, nil
	case GitActionSwitch:
		if branch == "" {
			return nil, fmt.Errorf("branch is required for switch")
		}
		return []string{"switch", branch}, nil
	case GitActionCreateBranch:
		if branch == "" {
			return nil, fmt.Errorf("branch is required for create branch")
		}
		return []string{"switch", "-c", branch}, nil
	case GitActionMergeNoFF:
		if branch == "" {
			return nil, fmt.Errorf("branch is required for merge --no-ff")
		}
		return []string{"merge", "--no-ff", branch}, nil
	default:
		return nil, fmt.Errorf("choose an action first")
	}
}

func startParallelGitActionCmd(repos []model.Repo, gitArgs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		runner := &gitActionRunner{
			results: make(chan gitActionRepoDoneMsg, len(repos)),
			cancel:  cancel,
		}
		if len(repos) == 0 {
			close(runner.results)
			return gitActionRunnerStartedMsg{runner: runner}
		}

		jobs := make(chan model.Repo)
		var wg sync.WaitGroup

		workers := gitActionWorkerCount(len(repos))
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for repo := range jobs {
					select {
					case runner.results <- gitActionRepoDoneMsg{
						repoName: repo.Name,
						started:  true,
					}:
					case <-ctx.Done():
						return
					}

					result := runGitActionRepo(ctx, repo, gitArgs)
					select {
					case runner.results <- result:
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		go func() {
			defer close(jobs)
			for _, repo := range repos {
				select {
				case <-ctx.Done():
					return
				case jobs <- repo:
				}
			}
		}()

		go func() {
			wg.Wait()
			cancel()
			close(runner.results)
		}()

		return gitActionRunnerStartedMsg{runner: runner}
	}
}

func gitActionWorkerCount(repoCount int) int {
	if repoCount <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0) * 2
	if workers < 4 {
		workers = 4
	}
	if workers > 16 {
		workers = 16
	}
	if workers > repoCount {
		workers = repoCount
	}
	return workers
}

func gitActionRepoTimeout(gitArgs []string) time.Duration {
	if len(gitArgs) > 0 && gitArgs[0] == "pull" {
		return 60 * time.Second
	}
	return 45 * time.Second
}

func newGitActionCommand(ctx context.Context, repoPath string, gitArgs []string) *exec.Cmd {
	args := gitActionCommandArgs(gitArgs)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(
		os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
	)
	return cmd
}

func gitActionCommandArgs(gitArgs []string) []string {
	// Keep a dedicated hook for future command-specific git options.
	if len(gitArgs) > 0 && gitArgs[0] == "pull" {
		return append([]string{}, gitArgs...)
	}
	return append([]string{}, gitArgs...)
}

func runGitActionRepo(parentCtx context.Context, repo model.Repo, gitArgs []string) gitActionRepoDoneMsg {
	timeout := gitActionRepoTimeout(gitArgs)
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	cmd := newGitActionCommand(ctx, repo.Path, gitArgs)
	out, err := cmd.CombinedOutput()
	result := gitActionRepoDoneMsg{
		repoName: repo.Name,
		err:      err,
		output:   strings.TrimSpace(string(out)),
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.err = fmt.Errorf("timed out after %s", timeout)
		timeoutMsg := "git command exceeded timeout; verify network/credentials and rerun."
		if result.output == "" {
			result.output = timeoutMsg
		} else {
			result.output = result.output + "\n" + timeoutMsg
		}
	case errors.Is(err, context.Canceled):
		result.err = fmt.Errorf("cancelled")
		if result.output == "" {
			result.output = "action cancelled"
		}
	}
	return result
}

func (m *Model) cancelGitActionRun() bool {
	if m.gitActionRunner != nil && m.gitActionRunner.cancel != nil {
		m.gitActionRunner.cancel()
	}
	if !m.gitActionRunning {
		return false
	}
	m.gitActionCancelPending = true
	m.gitActionError = "cancel requested; stopping running git commands..."
	m.statusMsg = "⚠ cancelling running action..."
	return true
}

func waitGitActionProgressCmd(runner *gitActionRunner) tea.Cmd {
	return func() tea.Msg {
		select {
		case result, ok := <-runner.results:
			if !ok {
				return gitActionRunnerDoneMsg{runner: runner}
			}
			return gitActionRepoProgressMsg{
				runner: runner,
				result: result,
			}
		case <-time.After(1 * time.Second):
			return gitActionRunnerHeartbeatMsg{runner: runner, at: time.Now()}
		}
	}
}

func loadCommonBranchesCmd(repos []model.Repo) tea.Cmd {
	return func() tea.Msg {
		if len(repos) == 0 {
			return commonBranchesLoadedMsg{branches: []string{}}
		}

		common := map[string]bool{}
		for i, repo := range repos {
			branches, err := listLocalBranches(repo.Path)
			if err != nil {
				return commonBranchesLoadedMsg{err: err}
			}

			current := map[string]bool{}
			for _, b := range branches {
				current[b] = true
			}

			if i == 0 {
				for b := range current {
					common[b] = true
				}
				continue
			}

			for b := range common {
				if !current[b] {
					delete(common, b)
				}
			}
		}

		result := make([]string, 0, len(common))
		for b := range common {
			result = append(result, b)
		}
		sort.Strings(result)
		return commonBranchesLoadedMsg{branches: result}
	}
}

func listLocalBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list branches in %s: %w", repoPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	branches := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}
