package tui

import (
	"os/exec"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	openRepoActionNeovim  = "nvim"
	openRepoActionVSCode  = "code"
	openRepoActionDismiss = "dismiss"
)

type openRepoOption struct {
	label  string
	action string
}

func (m Model) openRepoOptions() []openRepoOption {
	opts := make([]openRepoOption, 0, 3)
	if m.openRepoHasNeovim {
		opts = append(opts, openRepoOption{label: "Neovim", action: openRepoActionNeovim})
	}
	opts = append(opts, openRepoOption{label: "VS Code", action: openRepoActionVSCode})
	opts = append(opts, openRepoOption{label: "Dismiss", action: openRepoActionDismiss})
	return opts
}

func (m *Model) enterOpenRepoMode(name, path string) {
	m.state = StateOpenRepo
	m.openRepoName = name
	m.openRepoPath = path
	nvimBin, err := exec.LookPath("nvim")
	m.openRepoHasNeovim = err == nil
	if err == nil {
		m.openRepoNeovimBin = nvimBin
	} else {
		m.openRepoNeovimBin = ""
	}
	m.openRepoChoice = 0
}

func (m *Model) exitOpenRepoMode() {
	m.state = StateReady
	m.openRepoName = ""
	m.openRepoPath = ""
	m.openRepoChoice = 0
	m.openRepoHasNeovim = false
	m.openRepoNeovimBin = ""
}

func (m Model) handleOpenRepoMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.openRepoOptions()
	lastIndex := len(opts) - 1

	switch msg.String() {
	case "esc":
		m.exitOpenRepoMode()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.openRepoChoice > 0 {
			m.openRepoChoice--
		}
		return m, nil
	case "down", "j":
		if m.openRepoChoice < lastIndex {
			m.openRepoChoice++
		}
		return m, nil
	case "1", "2", "3":
		n, _ := strconv.Atoi(msg.String())
		idx := n - 1
		if idx >= 0 && idx <= lastIndex {
			m.openRepoChoice = idx
		}
		return m, nil
	case "enter":
		if m.openRepoChoice < 0 || m.openRepoChoice > lastIndex {
			m.openRepoChoice = 0
		}
		selected := opts[m.openRepoChoice]
		switch selected.action {
		case openRepoActionNeovim:
			repoName := m.openRepoName
			repoPath := m.openRepoPath
			nvimBin := m.openRepoNeovimBin
			m.exitOpenRepoMode()
			m.statusMsg = "Opening " + repoName + " in Neovim..."
			return m, func() tea.Msg {
				return openEditorMsg{
					path:   repoPath,
					cwd:    repoPath,
					binary: nvimBin,
					args:   []string{},
					label:  "Neovim",
				}
			}
		case openRepoActionVSCode:
			repoName := m.openRepoName
			repoPath := m.openRepoPath
			m.exitOpenRepoMode()
			m.statusMsg = "Opening " + repoName + " in VS Code..."
			return m, func() tea.Msg {
				return openEditorMsg{
					path:   repoPath,
					binary: "code",
					args:   []string{repoPath},
					label:  "VS Code",
				}
			}
		default:
			m.exitOpenRepoMode()
			return m, nil
		}
	}

	return m, nil
}
