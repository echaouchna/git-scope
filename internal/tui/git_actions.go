package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/echaouchna/git-scope/internal/model"
)

type gitActionResultMsg struct {
	actionName string
	scopeName  string
	total      int
	success    int
	failed     int
	firstError string
}

func (m *Model) enterGitActionMode() tea.Cmd {
	m.state = StateGitAction
	m.gitActionType = GitActionNone
	m.gitActionNeedsArg = false
	m.gitActionError = ""
	m.gitActionRunning = false
	m.gitActionInput.SetValue("")
	m.gitActionInput.Blur()
	return nil
}

func (m *Model) exitGitActionMode() {
	m.state = StateReady
	m.gitActionType = GitActionNone
	m.gitActionNeedsArg = false
	m.gitActionError = ""
	m.gitActionRunning = false
	m.gitActionInput.SetValue("")
	m.gitActionInput.Blur()
}

func (m Model) selectedScopeRepos() []model.Repo {
	if m.gitActionScope == GitActionScopeSelected {
		repo := m.GetSelectedRepo()
		if repo == nil {
			return nil
		}
		return []model.Repo{*repo}
	}

	if len(m.sortedRepos) == 0 {
		return nil
	}

	repos := make([]model.Repo, len(m.sortedRepos))
	copy(repos, m.sortedRepos)
	return repos
}

func (m Model) gitActionScopeName() string {
	if m.gitActionScope == GitActionScopeSelected {
		return "selected"
	}
	return "batch(filtered)"
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

func runGitActionCmd(repos []model.Repo, gitArgs []string, actionName, scopeName string) tea.Cmd {
	return func() tea.Msg {
		res := gitActionResultMsg{
			actionName: actionName,
			scopeName:  scopeName,
			total:      len(repos),
		}

		for _, repo := range repos {
			cmd := exec.Command("git", gitArgs...)
			cmd.Dir = repo.Path
			out, err := cmd.CombinedOutput()
			if err != nil {
				res.failed++
				if res.firstError == "" {
					res.firstError = fmt.Sprintf("%s: %v (%s)", repo.Name, err, strings.TrimSpace(string(out)))
				}
				continue
			}
			res.success++
		}

		return res
	}
}
