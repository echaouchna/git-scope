package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/echaouchna/git-scope/internal/browser"
	"github.com/echaouchna/git-scope/internal/fswatch"
	"github.com/echaouchna/git-scope/internal/model"
	"github.com/echaouchna/git-scope/internal/nudge"
	"github.com/echaouchna/git-scope/internal/scan"
	"github.com/echaouchna/git-scope/internal/workspace"
	"mvdan.cc/sh/v3/shell"
)

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if updated, cmd, ok := m.handleWindowAndSpinnerMsgs(msg); ok {
		m = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if updated, cmd, handled := m.handleNonKeyMsgs(msg); handled {
		return updated, batchCmds(cmds, cmd)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if updated, cmd, handled := m.handleStateModeKeys(keyMsg); handled {
			return updated, batchCmds(cmds, cmd)
		}
		if updated, cmd, handled := m.handleReadyKeys(keyMsg); handled {
			return updated, batchCmds(cmds, cmd)
		}

		// Dismiss star nudge on any key (if not already handled)
		if m.showStarNudge {
			m.showStarNudge = false
			nudge.MarkDismissed()
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func batchCmds(cmds []tea.Cmd, cmd tea.Cmd) tea.Cmd {
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m Model) handleWindowAndSpinnerMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeTable()
		return m, nil, true
	case spinner.TickMsg:
		if m.state != StateLoading && (m.state != StateGitAction || !m.gitActionRunning) {
			return m, nil, true
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) handleNonKeyMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	if updated, cmd, handled := m.handleScanMsgs(msg); handled {
		return updated, cmd, true
	}
	if updated, cmd, handled := m.handleWorkspaceScanMsgs(msg); handled {
		return updated, cmd, true
	}
	if updated, cmd, handled := m.handleWatcherMsgs(msg); handled {
		return updated, cmd, true
	}
	if updated, cmd, handled := m.handleEditorMsgs(msg); handled {
		return updated, cmd, true
	}
	if updated, cmd, handled := m.handlePanelDataMsgs(msg); handled {
		return updated, cmd, true
	}
	return m.handleGitActionProgressMsgs(msg)
}

func (m Model) handleScanMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case scanCompleteMsg:
		m.stopRepoWatcher()
		m.repos = msg.repos
		m.syncSelectionsWithRepos()
		m.state = StateReady
		m.resetPage()
		m.updateTable()
		switch {
		case len(msg.repos) == 0:
			m.statusMsg = "⚠️  No git repos found in configured directories. Press 'r' to rescan or run 'git-scope init' to configure."
		case msg.fromCache:
			m.statusMsg = fmt.Sprintf("✓ Loaded %d repos from cache", len(msg.repos))
		default:
			m.statusMsg = fmt.Sprintf("✓ Found %d repos", len(msg.repos))
		}
		if msg.warning != "" {
			m.statusMsg = "⚠ " + msg.warning
			logNonFatal("scan", msg.warning)
		}
		if len(msg.repos) == 0 {
			return m, nil, true
		}
		return m, startRepoWatcherCmd(msg.repos, m.cfg.Ignore), true
	case scanErrorMsg:
		m.state = StateError
		m.err = msg.err
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleWorkspaceScanMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case workspaceScanCompleteMsg:
		m.stopRepoWatcher()
		m.repos = msg.repos
		m.syncSelectionsWithRepos()
		m.state = StateReady
		m.resetPage()
		m.updateTable()
		if len(msg.repos) == 0 {
			m.statusMsg = fmt.Sprintf("⚠️  No git repos found in %s", msg.workspacePath)
			return m, nil, true
		}
		m.statusMsg = fmt.Sprintf("✓ Switched to %s (%d repos)", msg.workspacePath, len(msg.repos))
		if nudge.ShouldShowNudge() && !m.nudgeShownThisSession {
			m.showStarNudge = true
			m.nudgeShownThisSession = true
			nudge.MarkShown()
		}
		return m, startRepoWatcherCmd(msg.repos, m.cfg.Ignore), true
	case workspaceScanErrorMsg:
		m.state = StateError
		m.err = msg.err
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleWatcherMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case repoWatcherStartedMsg:
		m.stopRepoWatcher()
		m.repoWatcher = msg.watcher
		m.watchPolling = false
		m.watchRefreshPending = false
		m.watchRefreshRunning = false
		if m.repoWatcher == nil {
			return m, nil, true
		}
		return m, waitRepoWatchEventCmd(m.repoWatcher), true
	case repoWatchFallbackMsg:
		m.stopRepoWatcher()
		m.watchPolling = true
		m.watchRefreshPending = false
		m.watchRefreshRunning = false
		m.statusMsg = watcherFallbackMessage(msg.err)
		return m, startRepoPollTickCmd(), true
	case repoPollTickMsg:
		if !m.watchPolling || len(m.repos) == 0 {
			return m, nil, true
		}
		cmds := []tea.Cmd{startRepoPollTickCmd()}
		if !m.watchRefreshRunning {
			m.watchRefreshRunning = true
			cmds = append(cmds, refreshRepoStatusesCmd(cloneRepos(m.repos)))
		} else {
			m.watchRefreshPending = true
		}
		return m, tea.Batch(cmds...), true
	case repoWatchEventMsg:
		if m.repoWatcher == nil {
			return m, nil, true
		}
		cmds := []tea.Cmd{waitRepoWatchEventCmd(m.repoWatcher)}
		if !m.watchRefreshRunning {
			m.watchRefreshRunning = true
			cmds = append(cmds, refreshRepoStatusesCmd(cloneRepos(m.repos)))
		} else {
			m.watchRefreshPending = true
		}
		return m, tea.Batch(cmds...), true
	case repoStatusRefreshMsg:
		m.watchRefreshRunning = false
		m.repos = msg.repos
		m.syncSelectionsWithRepos()
		m.updateTable()
		if m.watchRefreshPending {
			m.watchRefreshPending = false
			m.watchRefreshRunning = true
			return m, refreshRepoStatusesCmd(cloneRepos(m.repos)), true
		}
		return m, nil, true
	case repoWatchErrorMsg:
		if fswatch.IsResourceLimitError(msg.err) {
			m.stopRepoWatcher()
			m.watchPolling = true
			m.watchRefreshPending = false
			m.watchRefreshRunning = false
			m.statusMsg = watcherFallbackMessage(msg.err)
			return m, startRepoPollTickCmd(), true
		}
		m.stopRepoWatcher()
		m.statusMsg = "⚠ background watcher stopped: " + msg.err.Error()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleEditorMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case openEditorMsg:
		next, cmd := m.handleOpenEditorMsg(msg)
		return next, cmd, true
	case editorClosedMsg:
		if msg.err != nil {
			m.statusMsg = "Error: " + msg.err.Error()
		} else {
			m.statusMsg = ""
		}
		return m, scanReposCmd(m.cfg, true), true
	default:
		return m, nil, false
	}
}

func (m Model) handleOpenEditorMsg(msg openEditorMsg) (Model, tea.Cmd) {
	binary := msg.binary
	args := msg.args
	label := msg.label

	if binary == "" {
		fields, err := shell.Fields(m.cfg.Editor, nil)
		if err != nil || len(fields) == 0 {
			m.statusMsg = fmt.Sprintf("❌ Invalid editor command: '%s'", m.cfg.Editor)
			return m, nil
		}
		binary = fields[0]
		args = append(append([]string{}, fields[1:]...), msg.path)
		label = m.cfg.Editor
	}

	if _, err := exec.LookPath(binary); err != nil {
		m.statusMsg = fmt.Sprintf("❌ %s (%s) not found in PATH.", label, binary)
		return m, nil
	}

	c := exec.Command(binary, args...)
	if msg.cwd != "" {
		c.Dir = msg.cwd
	} else {
		c.Dir = safeEditorCwd()
	}
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return editorClosedMsg{err: err}
		}
		return editorClosedMsg{}
	})
}

func (m Model) handlePanelDataMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case browserOpenResultMsg:
		if msg.err != nil {
			m.statusMsg = "⚠ failed to open browser: " + msg.err.Error()
			logNonFatal("browser", msg.err.Error())
		}
		return m, nil, true
	case commonBranchesLoadedMsg:
		m.gitActionLoadingBranch = false
		if msg.err != nil {
			m.gitActionBranchOptions = nil
			m.gitActionBranchMatches = nil
			m.gitActionError = msg.err.Error()
			return m, nil, true
		}
		m.gitActionBranchOptions = msg.branches
		m.refreshBranchMatches()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleGitActionProgressMsgs(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case gitActionResultMsg:
		m.gitActionRunning = false
		if msg.failed == 0 {
			m.statusMsg = fmt.Sprintf("✓ %s completed on %d repo(s) [%s]", msg.actionName, msg.success, msg.scopeName)
		} else {
			m.statusMsg = fmt.Sprintf("⚠ %s finished: %d ok, %d failed [%s]", msg.actionName, msg.success, msg.failed, msg.scopeName)
			if msg.firstError != "" {
				m.statusMsg += " — " + msg.firstError
			}
		}
		m.exitGitActionMode()
		return m, scanReposCmd(m.cfg, true), true
	case gitActionRunnerStartedMsg:
		if msg.runner == nil {
			return m, nil, true
		}
		return m, waitGitActionProgressCmd(msg.runner), true
	case gitActionRepoProgressMsg:
		m.applyGitActionRepoResult(msg.result)
		if msg.runner == nil {
			return m, nil, true
		}
		return m, waitGitActionProgressCmd(msg.runner), true
	case gitActionRunnerDoneMsg:
		m.finishGitActionRun()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m *Model) applyGitActionRepoResult(result gitActionRepoDoneMsg) {
	m.gitActionProgressIdx++
	m.gitActionCurrentRepo = result.repoName
	if result.err != nil {
		m.gitActionFailed++
		if m.gitActionFirstError == "" {
			m.gitActionFirstError = fmt.Sprintf("%s: %v (%s)", result.repoName, result.err, result.output)
		}
		m.gitActionLogLines = append(m.gitActionLogLines, fmt.Sprintf("[%s] ERROR", result.repoName))
	} else {
		m.gitActionSuccess++
		m.gitActionLogLines = append(m.gitActionLogLines, fmt.Sprintf("[%s] OK", result.repoName))
	}
	if result.output != "" {
		for _, line := range strings.Split(result.output, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				m.gitActionLogLines = append(m.gitActionLogLines, "  "+line)
			}
		}
	}
}

func (m *Model) finishGitActionRun() {
	m.gitActionRunning = false
	if m.gitActionFailed == 0 {
		m.statusMsg = fmt.Sprintf("✓ %s completed on %d repo(s) [%s]", m.gitActionName(), m.gitActionSuccess, m.gitActionScopeName)
	} else {
		m.statusMsg = fmt.Sprintf("⚠ %s finished: %d ok, %d failed [%s]", m.gitActionName(), m.gitActionSuccess, m.gitActionFailed, m.gitActionScopeName)
		if m.gitActionFirstError != "" {
			m.statusMsg += " — " + m.gitActionFirstError
		}
	}
	m.lastActionSummary = m.statusMsg
	m.lastActionLogLines = append([]string{}, m.gitActionLogLines...)
}

func (m Model) handleStateModeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch m.state {
	case StateSearching:
		next, cmd := m.handleSearchMode(msg)
		return next, cmd, true
	case StateWorkspaceSwitch:
		next, cmd := m.handleWorkspaceSwitchMode(msg)
		return next, cmd, true
	case StateGitAction:
		next, cmd := m.handleGitActionMode(msg)
		return next, cmd, true
	case StateOpenRepo:
		next, cmd := m.handleOpenRepoMode(msg)
		return next, cmd, true
	case StateShortcuts:
		next, cmd := m.handleShortcutsMode(msg)
		return next, cmd, true
	case StateCommandPalette:
		next, cmd := m.handleCommandPaletteMode(msg)
		return next, cmd, true
	case StateActionLogs:
		next, cmd := m.handleActionLogsMode(msg)
		return next, cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) handleReadyKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.state != StateReady {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		m.stopRepoWatcher()
		return m, tea.Quit, true
	}
	if updated, cmd, handled := m.handleReadyNavigationKeys(msg); handled {
		return updated, cmd, true
	}
	if updated, cmd, handled := m.handleReadyViewKeys(msg); handled {
		return updated, cmd, true
	}
	return m.handleReadyMetaKeys(msg)
}

func (m Model) handleReadyNavigationKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "enter":
		repo := m.GetSelectedRepo()
		if repo == nil {
			return m, nil, true
		}
		m.enterOpenRepoMode(repo.Name, repo.Path)
		return m, nil, true
	case " ":
		if !m.toggleCurrentRepoSelection() {
			return m, nil, true
		}
		m.updateTable()
		m.statusMsg = fmt.Sprintf("Selected repos: %d", m.selectedReposCount())
		return m, nil, true
	case "ctrl+a":
		count, deselected := m.toggleSelectAllFiltered()
		m.updateTable()
		if deselected {
			m.statusMsg = fmt.Sprintf("Deselected %d repos", count)
		} else {
			m.statusMsg = fmt.Sprintf("Selected %d repos", count)
		}
		return m, nil, true
	case "[":
		if !m.canGoPrev() {
			return m, nil, true
		}
		m.currentPage--
		m.updateTable()
		m.statusMsg = fmt.Sprintf("Page %d of %d", m.currentPage+1, m.getTotalPages())
		return m, nil, true
	case "]":
		if !m.canGoNext() {
			return m, nil, true
		}
		m.currentPage++
		m.updateTable()
		m.statusMsg = fmt.Sprintf("Page %d of %d", m.currentPage+1, m.getTotalPages())
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleReadyViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "r":
		m.stopRepoWatcher()
		m.state = StateLoading
		m.statusMsg = "Rescanning..."
		return m, scanReposCmd(m.cfg, true), true
	case "f":
		m.filterMode = (m.filterMode + 1) % 3
		m.resetPage()
		m.updateTable()
		m.statusMsg = "Filter: " + m.GetFilterModeName()
		return m, nil, true
	case "s":
		m.sortMode = (m.sortMode + 1) % 4
		m.resetPage()
		m.updateTable()
		m.statusMsg = "Sorted by: " + m.GetSortModeName()
		return m, nil, true
	case "1", "2", "3", "4":
		return m.handleReadyDirectSortKey(msg.String())
	case "c":
		m.searchQuery = ""
		m.textInput.SetValue("")
		m.filterMode = FilterAll
		m.resetPage()
		m.resizeTable()
		m.updateTable()
		m.statusMsg = "Filters cleared"
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) handleReadyDirectSortKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "1":
		m.sortMode = SortByDirty
		m.statusMsg = "Sorted by: Dirty First"
	case "2":
		m.sortMode = SortByName
		m.statusMsg = "Sorted by: Name"
	case "3":
		m.sortMode = SortByBranch
		m.statusMsg = "Sorted by: Branch"
	case "4":
		m.sortMode = SortByLastCommit
		m.statusMsg = "Sorted by: Recent"
	default:
		return m, nil, false
	}
	m.resetPage()
	m.updateTable()
	return m, nil, true
}

func (m Model) handleReadyMetaKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	if modelOut, cmd, handled := m.handleReadyMetaNudgeKey(key); handled {
		return modelOut, cmd, true
	}
	if modelOut, cmd, handled := m.handleReadyMetaGeneralKey(key); handled {
		return modelOut, cmd, true
	}
	return m, nil, false
}

func (m Model) handleReadyMetaNudgeKey(key string) (tea.Model, tea.Cmd, bool) {
	if key != "S" {
		return m, nil, false
	}
	if !m.showStarNudge {
		return m, nil, true
	}
	m.showStarNudge = false
	nudge.MarkCompleted()
	m.statusMsg = "⭐ Opening GitHub..."
	return m, openBrowserCmd(nudge.GitHubRepoURL), true
}

func (m Model) handleReadyMetaGeneralKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "/":
		m.state = StateSearching
		m.resizeTable()
		m.textInput.Focus()
		m.textInput.SetValue(m.searchQuery)
		return m, textinput.Blink, true
	case "?":
		m.enterShortcutsMode()
		return m, nil, true
	case "ctrl+p":
		return m, m.enterCommandPaletteMode(), true
	case "l":
		if len(m.lastActionLogLines) == 0 {
			m.statusMsg = "No action logs yet"
			return m, nil, true
		}
		m.enterActionLogsMode()
		return m, nil, true
	case "e":
		return m.handleReadyEditorCheck()
	case "w":
		m.state = StateWorkspaceSwitch
		m.workspaceInput.SetValue(m.currentWorkspacePath())
		m.workspaceInput.CursorEnd()
		m.workspaceInput.Focus()
		m.workspaceError = ""
		return m, textinput.Blink, true
	case "a":
		return m, m.enterGitActionMode(), true
	default:
		return m, nil, false
	}
}

func (m Model) handleReadyEditorCheck() (tea.Model, tea.Cmd, bool) {
	fields, err := shell.Fields(m.cfg.Editor, nil)
	if err != nil || len(fields) == 0 {
		m.statusMsg = fmt.Sprintf("❌ Invalid editor command: '%s'", m.cfg.Editor)
		return m, nil, true
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		m.statusMsg = fmt.Sprintf("❌ Editor '%s' not found in PATH. Install it or edit ~/.config/git-scope/config.yml", fields[0])
		return m, nil, true
	}
	m.statusMsg = fmt.Sprintf("✓ Editor: %s (edit config at ~/.config/git-scope/config.yml)", m.cfg.Editor)
	return m, nil, true
}

func safeEditorCwd() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return home
	}
	return "/"
}

// handleSearchMode handles key events when in search mode
func (m Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel search, keep previous query
		m.state = StateReady
		m.resizeTable()
		m.textInput.Blur()
		return m, nil

	case "enter":
		// Apply search
		m.searchQuery = m.textInput.Value()
		m.state = StateReady
		m.resizeTable()
		m.textInput.Blur()
		m.resetPage()
		m.updateTable()
		if m.searchQuery != "" {
			m.statusMsg = "Searching: " + m.searchQuery
		} else {
			m.statusMsg = "Search cleared"
		}
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	// Update text input
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	// Live search as you type
	m.searchQuery = m.textInput.Value()
	m.updateTable()

	return m, cmd
}

// editorClosedMsg is sent when the editor process closes
type editorClosedMsg struct {
	err error
}

// handleWorkspaceSwitchMode handles key events when in workspace switch mode
func (m Model) handleWorkspaceSwitchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel workspace switch
		m.state = StateReady
		m.workspaceInput.Blur()
		m.workspaceError = ""
		return m, nil

	case "enter":
		// Validate and switch workspace
		inputPath := m.workspaceInput.Value()
		if inputPath == "" {
			m.workspaceError = "Please enter a path"
			return m, nil
		}

		// Normalize the path (expand ~, resolve symlinks, validate)
		normalizedPath, err := workspace.NormalizeWorkspacePath(inputPath)
		if err != nil {
			m.workspaceError = err.Error()
			return m, nil
		}

		// Switch to loading state and scan the new workspace
		m.stopRepoWatcher()
		m.state = StateLoading
		m.workspaceInput.Blur()
		m.workspaceError = ""
		m.activeWorkspace = normalizedPath
		m.statusMsg = "🔄 Switching to " + normalizedPath + "..."

		return m, scanWorkspaceCmd(normalizedPath, m.cfg.Ignore)

	case "tab":
		// Tab completion for directory paths
		currentPath := m.workspaceInput.Value()
		if currentPath != "" {
			completedPath := workspace.CompleteDirectoryPath(currentPath)
			if completedPath != currentPath {
				m.workspaceInput.SetValue(completedPath)
				// Move cursor to end
				m.workspaceInput.CursorEnd()
			}
		}
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}

	// Update text input
	var cmd tea.Cmd
	m.workspaceInput, cmd = m.workspaceInput.Update(msg)

	// Clear error when typing
	if m.workspaceError != "" {
		m.workspaceError = ""
	}

	return m, cmd
}

func (m Model) handleGitActionMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	finished := !m.gitActionRunning && m.gitActionProgressTotal > 0 && m.gitActionProgressIdx >= m.gitActionProgressTotal

	key := msg.String()
	if modelOut, cmd, handled := m.handleGitActionRunningState(key); handled {
		return modelOut, cmd
	}

	switch key {
	case "esc":
		m.exitGitActionMode()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		return m.moveGitActionCursor(-1)
	case "down", "j":
		return m.moveGitActionCursor(1)
	case "1", "2", "3", "4":
		return m.selectGitActionByNumber(key)
	case "tab":
		if m.gitActionNeedsBranch() {
			m.applyNextBranchAutocomplete()
			m.refreshBranchMatches()
		}
		return m, nil
	case "l":
		if len(m.lastActionLogLines) == 0 {
			m.gitActionError = "no action logs yet"
			return m, nil
		}
		m.enterActionLogsMode()
		return m, nil
	case "enter":
		return m.startGitActionRun(finished)
	}

	return m.handleGitActionBranchInput(msg)
}

func (m Model) handleGitActionRunningState(key string) (tea.Model, tea.Cmd, bool) {
	if !m.gitActionRunning {
		return m, nil, false
	}
	if key == "ctrl+c" || key == "q" {
		return m, tea.Quit, true
	}
	return m, nil, true
}

func (m Model) handleGitActionBranchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.gitActionNeedsBranch() {
		var cmd tea.Cmd
		m.gitActionInput, cmd = m.gitActionInput.Update(msg)
		m.refreshBranchMatches()
		return m, cmd
	}
	return m, nil
}

func (m Model) moveGitActionCursor(delta int) (tea.Model, tea.Cmd) {
	next := m.gitActionCursor + delta
	if next < 0 || next >= len(m.gitActionMenuLabels()) {
		return m, nil
	}
	m.gitActionCursor = next
	m.setGitActionFromCursor()
	return m.syncGitActionBranchInput()
}

func (m Model) selectGitActionByNumber(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "1":
		m.gitActionCursor = 0
	case "2":
		m.gitActionCursor = 1
	case "3":
		m.gitActionCursor = 2
	case "4":
		m.gitActionCursor = 3
	default:
		return m, nil
	}
	m.setGitActionFromCursor()
	return m.syncGitActionBranchInput()
}

func (m Model) syncGitActionBranchInput() (tea.Model, tea.Cmd) {
	if m.gitActionNeedsBranch() {
		m.gitActionInput.Focus()
		m.gitActionInput.SetValue("")
		return m.loadGitActionBranches()
	}
	m.gitActionInput.Blur()
	return m.loadGitActionBranches()
}

func (m Model) loadGitActionBranches() (tea.Model, tea.Cmd) {
	if m.gitActionType == GitActionSwitch || m.gitActionType == GitActionMergeNoFF {
		repos, _ := m.targetReposForAction()
		m.gitActionLoadingBranch = true
		m.gitActionError = ""
		return m, loadCommonBranchesCmd(repos)
	}
	m.gitActionLoadingBranch = false
	m.gitActionBranchOptions = nil
	m.gitActionBranchMatches = nil
	return m, nil
}

func (m Model) startGitActionRun(finished bool) (tea.Model, tea.Cmd) {
	if finished {
		m.resetGitActionRunState()
	}
	if m.gitActionType == GitActionNone {
		m.gitActionError = "choose an action first"
		return m, nil
	}
	repos, source := m.targetReposForAction()
	if len(repos) == 0 {
		m.gitActionError = "no repositories to run against"
		return m, nil
	}
	if (m.gitActionType == GitActionSwitch || m.gitActionType == GitActionMergeNoFF) &&
		strings.TrimSpace(m.gitActionInput.Value()) == "" &&
		len(m.gitActionBranchMatches) > 0 {
		m.gitActionInput.SetValue(m.gitActionBranchMatches[0])
	}
	gitArgs, err := m.gitActionArgs()
	if err != nil {
		m.gitActionError = err.Error()
		return m, nil
	}

	m.gitActionRunning = true
	m.gitActionError = ""
	m.gitActionQueue = repos
	m.gitActionExecArgs = gitArgs
	m.gitActionScopeName = source
	m.gitActionProgressIdx = 0
	m.gitActionProgressTotal = len(repos)
	m.gitActionSuccess = 0
	m.gitActionFailed = 0
	m.gitActionFirstError = ""
	m.gitActionLogLines = nil
	m.gitActionLogOffset = 0
	m.gitActionCurrentRepo = fmt.Sprintf("%d repos in parallel", len(repos))
	m.statusMsg = fmt.Sprintf(
		"Running %s on %d repo(s) in parallel",
		m.gitActionName(),
		m.gitActionProgressTotal,
	)
	return m, tea.Batch(startParallelGitActionCmd(repos, gitArgs), m.spinner.Tick)
}

// workspaceScanCompleteMsg is sent when workspace scanning is complete
type workspaceScanCompleteMsg struct {
	repos         []model.Repo
	workspacePath string
}

// workspaceScanErrorMsg is sent when workspace scanning fails
type workspaceScanErrorMsg struct {
	err error
}

// scanWorkspaceCmd scans a single workspace path for repositories
func scanWorkspaceCmd(workspacePath string, ignore []string) tea.Cmd {
	return func() tea.Msg {
		repos, err := scan.ScanRoots([]string{workspacePath}, ignore)
		if err != nil {
			return workspaceScanErrorMsg{err: err}
		}

		return workspaceScanCompleteMsg{
			repos:         repos,
			workspacePath: workspacePath,
		}
	}
}

// openBrowserCmd opens a URL in the default browser
func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return browserOpenResultMsg{
			url: url,
			err: browser.Open(url),
		}
	}
}

type browserOpenResultMsg struct {
	url string
	err error
}

func cloneRepos(repos []model.Repo) []model.Repo {
	out := make([]model.Repo, len(repos))
	copy(out, repos)
	return out
}

func (m *Model) stopRepoWatcher() {
	if m.repoWatcher == nil {
		m.watchPolling = false
		m.watchRefreshRunning = false
		m.watchRefreshPending = false
		return
	}
	_ = m.repoWatcher.Close()
	m.repoWatcher = nil
	m.watchPolling = false
	m.watchRefreshRunning = false
	m.watchRefreshPending = false
}

func watcherFallbackMessage(err error) string {
	return "⚠ file watcher hit OS limits; switched to polling every 5s. " +
		"Tip: increase inotify/FD limits (fs.inotify.max_user_watches, ulimit -n) or narrow roots/ignore dirs. Cause: " +
		err.Error()
}

func logNonFatal(component, detail string) {
	_, _ = fmt.Fprintf(os.Stderr, "git-scope non-fatal [%s]: %s\n", component, detail)
}
