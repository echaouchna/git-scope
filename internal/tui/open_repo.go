package tui

import (
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	openRepoActionNeovim  = "nvim"
	openRepoActionGitUI   = "gitui"
	openRepoActionTig     = "tig"
	openRepoActionTigAll  = "tig_all"
	openRepoActionVSCode  = "code"
	openRepoActionDismiss = "dismiss"
)

type openRepoOption struct {
	label  string
	action string
}

func (m Model) openRepoOptions() []openRepoOption {
	opts := make([]openRepoOption, 0, 6)
	if m.openRepoHasNeovim {
		opts = append(opts, openRepoOption{label: "Neovim", action: openRepoActionNeovim})
	}
	if m.openRepoHasGitUI {
		opts = append(opts, openRepoOption{label: "GitUI", action: openRepoActionGitUI})
	}
	if m.openRepoHasTig {
		opts = append(opts, openRepoOption{label: "Tig", action: openRepoActionTig})
		opts = append(opts, openRepoOption{label: "Tig (--all)", action: openRepoActionTigAll})
	}
	opts = append(opts, openRepoOption{label: "VS Code", action: openRepoActionVSCode})
	opts = append(opts, openRepoOption{label: "Dismiss", action: openRepoActionDismiss})
	return opts
}

func (m Model) filteredOpenRepoOptions() []openRepoOption {
	options := m.openRepoOptions()
	query := strings.ToLower(strings.TrimSpace(m.openRepoInput.Value()))
	if query == "" {
		return options
	}

	filtered := make([]openRepoOption, 0, len(options))
	for _, opt := range options {
		if strings.Contains(strings.ToLower(opt.label), query) || strings.Contains(strings.ToLower(opt.action), query) {
			filtered = append(filtered, opt)
		}
	}
	return filtered
}

func (m Model) openRepoVisibleRows() int {
	visibleRows := 8
	if m.height > 0 {
		if v := m.height - 18; v > 4 {
			visibleRows = v
		}
	}
	return visibleRows
}

func (m *Model) ensureOpenRepoChoiceVisible(itemsLen int) {
	visible := m.openRepoVisibleRows()
	if m.openRepoChoice < m.openRepoOffset {
		m.openRepoOffset = m.openRepoChoice
	}
	if m.openRepoChoice >= m.openRepoOffset+visible {
		m.openRepoOffset = m.openRepoChoice - visible + 1
	}
	if m.openRepoOffset < 0 {
		m.openRepoOffset = 0
	}
	maxOffset := 0
	if itemsLen > visible {
		maxOffset = itemsLen - visible
	}
	if m.openRepoOffset > maxOffset {
		m.openRepoOffset = maxOffset
	}
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

	gituiBin, err := exec.LookPath("gitui")
	m.openRepoHasGitUI = err == nil
	if err == nil {
		m.openRepoGitUIBin = gituiBin
	} else {
		m.openRepoGitUIBin = ""
	}

	tigBin, err := exec.LookPath("tig")
	m.openRepoHasTig = err == nil
	if err == nil {
		m.openRepoTigBin = tigBin
	} else {
		m.openRepoTigBin = ""
	}

	m.openRepoChoice = 0
	m.openRepoOffset = 0
	m.openRepoInput.SetValue("")
	m.openRepoInput.Focus()
}

func (m *Model) exitOpenRepoMode() {
	m.state = StateReady
	m.openRepoName = ""
	m.openRepoPath = ""
	m.openRepoChoice = 0
	m.openRepoOffset = 0
	m.openRepoInput.Blur()
	m.openRepoInput.SetValue("")
	m.openRepoHasNeovim = false
	m.openRepoNeovimBin = ""
	m.openRepoHasGitUI = false
	m.openRepoGitUIBin = ""
	m.openRepoHasTig = false
	m.openRepoTigBin = ""
}

func (m Model) handleOpenRepoMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.exitOpenRepoMode()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	opts := m.filteredOpenRepoOptions()
	lastIndex := len(opts) - 1
	if m.openRepoChoice > lastIndex {
		m.openRepoChoice = maxInt(0, lastIndex)
	}

	switch msg.String() {
	case "up", "k":
		if m.openRepoChoice > 0 {
			m.openRepoChoice--
		}
		m.ensureOpenRepoChoiceVisible(len(opts))
		return m, nil
	case "down", "j":
		if m.openRepoChoice < lastIndex {
			m.openRepoChoice++
		}
		m.ensureOpenRepoChoiceVisible(len(opts))
		return m, nil
	case "pgup":
		m.openRepoChoice -= m.openRepoVisibleRows()
		if m.openRepoChoice < 0 {
			m.openRepoChoice = 0
		}
		m.ensureOpenRepoChoiceVisible(len(opts))
		return m, nil
	case "pgdown":
		m.openRepoChoice += m.openRepoVisibleRows()
		if m.openRepoChoice > lastIndex {
			m.openRepoChoice = lastIndex
			if m.openRepoChoice < 0 {
				m.openRepoChoice = 0
			}
		}
		m.ensureOpenRepoChoiceVisible(len(opts))
		return m, nil
	case "enter":
		if len(opts) == 0 {
			command := strings.TrimSpace(m.openRepoInput.Value())
			if command == "" {
				m.statusMsg = "No matching options and command is empty"
				return m, nil
			}
			repoName := m.openRepoName
			repoPath := m.openRepoPath
			m.exitOpenRepoMode()
			m.statusMsg = "Running command in " + repoName + "..."
			return m, func() tea.Msg {
				return openEditorMsg{
					path:   repoPath,
					cwd:    repoPath,
					binary: "sh",
					args:   []string{"-lc", command},
					label:  "Command",
				}
			}
		}
		if m.openRepoChoice < 0 || m.openRepoChoice > lastIndex {
			m.openRepoChoice = 0
		}
		selected := opts[m.openRepoChoice]
		return m.runOpenRepoAction(selected.action)
	}

	var cmd tea.Cmd
	m.openRepoInput, cmd = m.openRepoInput.Update(msg)
	opts = m.filteredOpenRepoOptions()
	if len(opts) == 0 {
		m.openRepoChoice = 0
		m.openRepoOffset = 0
		return m, cmd
	}
	if m.openRepoChoice >= len(opts) {
		m.openRepoChoice = 0
	}
	m.ensureOpenRepoChoiceVisible(len(opts))
	return m, cmd
}

func (m Model) runOpenRepoAction(action string) (tea.Model, tea.Cmd) {
	repoName := m.openRepoName
	repoPath := m.openRepoPath
	nvimBin := m.openRepoNeovimBin
	gituiBin := m.openRepoGitUIBin
	tigBin := m.openRepoTigBin

	switch action {
	case openRepoActionNeovim:
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
	case openRepoActionGitUI:
		m.exitOpenRepoMode()
		m.statusMsg = "Opening " + repoName + " in GitUI..."
		return m, func() tea.Msg {
			return openEditorMsg{
				path:   repoPath,
				cwd:    repoPath,
				binary: gituiBin,
				args:   []string{},
				label:  "GitUI",
			}
		}
	case openRepoActionTig:
		m.exitOpenRepoMode()
		m.statusMsg = "Opening " + repoName + " in Tig..."
		return m, func() tea.Msg {
			return openEditorMsg{
				path:   repoPath,
				cwd:    repoPath,
				binary: tigBin,
				args:   []string{},
				label:  "Tig",
			}
		}
	case openRepoActionTigAll:
		m.exitOpenRepoMode()
		m.statusMsg = "Opening " + repoName + " in Tig (--all)..."
		return m, func() tea.Msg {
			return openEditorMsg{
				path:   repoPath,
				cwd:    repoPath,
				binary: tigBin,
				args:   []string{"--all"},
				label:  "Tig (--all)",
			}
		}
	case openRepoActionVSCode:
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
