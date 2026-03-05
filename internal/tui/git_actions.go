package tui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

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
	err      error
	output   string
}

type gitActionBatchDoneMsg struct {
	results []gitActionRepoDoneMsg
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
	m.gitActionScopeName = ""
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = 0
	m.gitActionCurrentRepo = ""
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
	m.gitActionScopeName = ""
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = 0
	m.gitActionCurrentRepo = ""
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
	m.gitActionScopeName = ""
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = 0
	m.gitActionCurrentRepo = ""
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

func runParallelGitActionCmd(repos []model.Repo, gitArgs []string) tea.Cmd {
	return func() tea.Msg {
		results := make([]gitActionRepoDoneMsg, len(repos))
		if len(repos) == 0 {
			return gitActionBatchDoneMsg{results: results}
		}

		type job struct {
			index int
			repo  model.Repo
		}

		workers := len(repos)
		if workers > 8 {
			workers = 8
		}
		jobs := make(chan job, len(repos))
		var wg sync.WaitGroup

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					cmd := exec.Command("git", gitArgs...)
					cmd.Dir = j.repo.Path
					out, err := cmd.CombinedOutput()
					results[j.index] = gitActionRepoDoneMsg{
						repoName: j.repo.Name,
						err:      err,
						output:   strings.TrimSpace(string(out)),
					}
				}
			}()
		}

		for i, repo := range repos {
			jobs <- job{index: i, repo: repo}
		}
		close(jobs)
		wg.Wait()

		return gitActionBatchDoneMsg{results: results}
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
