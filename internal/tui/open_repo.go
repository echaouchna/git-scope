package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	openRepoActionCmd     = "cmd:"
	openRepoActionAlias   = "alias:"
	openRepoCmdPrefix     = ":"
)

type openRepoOption struct {
	label  string
	helper string
	action string
}

type openRepoCommandAlias struct {
	name    string
	command string
	helper  string
}

func openRepoCommandAliases() []openRepoCommandAlias {
	return []openRepoCommandAlias{
		{name: "st", command: "git status --short --branch", helper: "git status (short + branch)"},
		{name: "fetch", command: "git fetch --all --prune", helper: "fetch all remotes + prune"},
		{name: "pull", command: "git pull --rebase", helper: "pull with rebase"},
		{name: "lg", command: "git log --oneline --decorate -20", helper: "show recent commit log"},
		{name: "sh", command: "$SHELL -i", helper: "open an interactive shell"},
	}
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
	rawQuery := strings.TrimSpace(m.openRepoInput.Value())
	if strings.HasPrefix(rawQuery, openRepoCmdPrefix) {
		cmdQuery := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rawQuery, openRepoCmdPrefix)))
		return m.filteredOpenRepoBinaryOptions(cmdQuery)
	}

	options := m.openRepoOptions()
	query := strings.ToLower(rawQuery)
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

func (m Model) filteredOpenRepoBinaryOptions(query string) []openRepoOption {
	aliases := openRepoCommandAliases()
	options := make([]openRepoOption, 0, len(aliases)+len(m.openRepoBinaries))

	for _, alias := range aliases {
		aliasName := strings.ToLower(alias.name)
		if query != "" && !strings.Contains(aliasName, query) {
			continue
		}
		options = append(options, openRepoOption{
			label:  ":" + alias.name,
			helper: alias.helper,
			action: openRepoActionAlias + alias.command,
		})
	}

	for _, bin := range m.openRepoBinaries {
		if query != "" && !strings.Contains(strings.ToLower(bin), query) {
			continue
		}
		options = append(options, openRepoOption{
			label:  ":" + bin,
			helper: "run binary from PATH",
			action: openRepoActionCmd + bin,
		})
	}
	return options
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
	m.ensureOpenRepoToolsReady()

	m.openRepoChoice = 0
	m.openRepoOffset = 0
	m.openRepoInput.SetValue("")
	m.openRepoInput.Focus()
}

func (m *Model) ensureOpenRepoToolsReady() {
	if m.openRepoToolsReady {
		return
	}

	m.openRepoNeovimBin, m.openRepoHasNeovim = lookupBinary("nvim")
	m.openRepoGitUIBin, m.openRepoHasGitUI = lookupBinary("gitui")
	m.openRepoTigBin, m.openRepoHasTig = lookupBinary("tig")
	m.openRepoBinaries = listAvailableBinaries()
	m.openRepoToolsReady = true
}

func lookupBinary(name string) (string, bool) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return bin, true
}

func userShellBinary() string {
	sh := strings.TrimSpace(os.Getenv("SHELL"))
	if sh == "" {
		return "sh"
	}
	if filepath.IsAbs(sh) {
		return sh
	}
	if resolved, err := exec.LookPath(sh); err == nil {
		return resolved
	}
	return "sh"
}

func openRepoShellCommand(path, command string) openEditorMsg {
	return openEditorMsg{
		path:   path,
		cwd:    path,
		binary: userShellBinary(),
		args:   []string{"-c", command},
		label:  "Command",
	}
}

func listAvailableBinaries() []string {
	pathValue := os.Getenv("PATH")
	if pathValue == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var bins []string
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}
			mode := info.Mode()
			if mode.IsDir() || mode&0111 == 0 {
				continue
			}
			seen[name] = struct{}{}
			bins = append(bins, name)
		}
	}

	sort.Strings(bins)
	return bins
}

func (m *Model) exitOpenRepoMode() {
	m.state = StateReady
	m.openRepoName = ""
	m.openRepoPath = ""
	m.openRepoChoice = 0
	m.openRepoOffset = 0
	m.openRepoInput.Blur()
	m.openRepoInput.SetValue("")
}

func (m Model) handleOpenRepoMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
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

	if m.handleOpenRepoNavKey(key, len(opts), lastIndex) {
		return m, nil
	}
	if key == "enter" {
		return m.handleOpenRepoEnter(opts, lastIndex)
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

func (m *Model) handleOpenRepoNavKey(key string, optsLen, lastIndex int) bool {
	switch key {
	case "up", "k":
		if m.openRepoChoice > 0 {
			m.openRepoChoice--
		}
	case "down", "j":
		if m.openRepoChoice < lastIndex {
			m.openRepoChoice++
		}
	case "pgup":
		m.openRepoChoice -= m.openRepoVisibleRows()
		if m.openRepoChoice < 0 {
			m.openRepoChoice = 0
		}
	case "pgdown":
		m.openRepoChoice += m.openRepoVisibleRows()
		if m.openRepoChoice > lastIndex {
			m.openRepoChoice = maxInt(0, lastIndex)
		}
	default:
		return false
	}
	m.ensureOpenRepoChoiceVisible(optsLen)
	return true
}

func (m Model) handleOpenRepoEnter(opts []openRepoOption, lastIndex int) (tea.Model, tea.Cmd) {
	if len(opts) == 0 {
		raw := strings.TrimSpace(m.openRepoInput.Value())
		if raw == "" {
			m.statusMsg = "No matching options"
			return m, nil
		}
		if !strings.HasPrefix(raw, openRepoCmdPrefix) {
			m.statusMsg = "No matching options. Use ':<command>' to run a shell command."
			return m, nil
		}
		command := strings.TrimSpace(strings.TrimPrefix(raw, openRepoCmdPrefix))
		if command == "" {
			m.statusMsg = "Empty command. Use ':<command>'."
			return m, nil
		}
		repoName := m.openRepoName
		repoPath := m.openRepoPath
		m.exitOpenRepoMode()
		m.statusMsg = "Running command in " + repoName + "..."
		return m, func() tea.Msg {
			return openRepoShellCommand(repoPath, command)
		}
	}
	if m.openRepoChoice < 0 || m.openRepoChoice > lastIndex {
		m.openRepoChoice = 0
	}
	selected := opts[m.openRepoChoice]
	return m.runOpenRepoAction(selected.action)
}

func (m Model) runOpenRepoAction(action string) (tea.Model, tea.Cmd) {
	repoName := m.openRepoName
	repoPath := m.openRepoPath
	nvimBin := m.openRepoNeovimBin
	gituiBin := m.openRepoGitUIBin
	tigBin := m.openRepoTigBin

	switch action {
	case openRepoActionDismiss:
		m.exitOpenRepoMode()
		return m, nil
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
		if strings.HasPrefix(action, openRepoActionAlias) {
			command := strings.TrimSpace(strings.TrimPrefix(action, openRepoActionAlias))
			if command == "" {
				m.statusMsg = "Empty alias command."
				return m, nil
			}
			m.exitOpenRepoMode()
			m.statusMsg = "Running command in " + repoName + "..."
			return m, func() tea.Msg {
				return openRepoShellCommand(repoPath, command)
			}
		}
		if strings.HasPrefix(action, openRepoActionCmd) {
			command := strings.TrimSpace(strings.TrimPrefix(action, openRepoActionCmd))
			if command == "" {
				m.statusMsg = "Empty command. Use ':<command>'."
				return m, nil
			}
			m.exitOpenRepoMode()
			m.statusMsg = "Running command in " + repoName + "..."
			return m, func() tea.Msg {
				return openRepoShellCommand(repoPath, command)
			}
		}
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
