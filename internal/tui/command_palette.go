package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type commandItem struct {
	label string
	key   string
}

func (m *Model) enterShortcutsMode() {
	m.state = StateShortcuts
	m.shortcutsCursor = 0
	m.shortcutsOffset = 0
}

func (m *Model) enterCommandPaletteMode() tea.Cmd {
	m.state = StateCommandPalette
	m.commandCursor = 0
	m.commandOffset = 0
	m.commandInput.SetValue("")
	m.commandInput.Focus()
	return nil
}

func (m *Model) enterActionLogsMode() {
	m.actionLogsReturnState = m.state
	m.state = StateActionLogs
	m.gitActionLogOffset = 0
}

func (m *Model) exitCommandPaletteMode() {
	m.state = StateReady
	m.commandInput.Blur()
	m.commandInput.SetValue("")
	m.commandCursor = 0
	m.commandOffset = 0
}

func (m Model) commandPaletteItems() []commandItem {
	return []commandItem{
		{label: "Rescan repositories", key: "rescan"},
		{label: "Open Git actions", key: "actions"},
		{label: "Toggle filter mode", key: "filter"},
		{label: "Cycle sort mode", key: "sort"},
		{label: "Clear search and filters", key: "clear"},
		{label: "Select/deselect all filtered", key: "select_all"},
		{label: "Open selected repo menu", key: "open_repo"},
		{label: "Switch workspace", key: "workspace"},
		{label: "Show shortcuts", key: "shortcuts"},
		{label: "Show last action logs", key: "action_logs"},
		{label: "Quit", key: "quit"},
	}
}

func (m Model) filteredCommandItems() []commandItem {
	items := m.commandPaletteItems()
	query := strings.ToLower(strings.TrimSpace(m.commandInput.Value()))
	if query == "" {
		return items
	}
	result := make([]commandItem, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.label), query) || strings.Contains(strings.ToLower(item.key), query) {
			result = append(result, item)
		}
	}
	return result
}

func (m Model) handleShortcutsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.shortcutsEntries()
	maxIdx := len(items) - 1
	visibleRows := m.shortcutsVisibleRows()

	switch msg.String() {
	case "esc", "?":
		m.state = StateReady
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.shortcutsCursor > 0 {
			m.shortcutsCursor--
			m.ensureShortcutsCursorVisible(visibleRows)
		}
		return m, nil
	case "down", "j":
		if m.shortcutsCursor < maxIdx {
			m.shortcutsCursor++
			m.ensureShortcutsCursorVisible(visibleRows)
		}
		return m, nil
	case "pgup":
		m.shortcutsCursor -= visibleRows
		if m.shortcutsCursor < 0 {
			m.shortcutsCursor = 0
		}
		m.ensureShortcutsCursorVisible(visibleRows)
		return m, nil
	case "pgdown":
		m.shortcutsCursor += visibleRows
		if m.shortcutsCursor > maxIdx {
			m.shortcutsCursor = maxIdx
		}
		m.ensureShortcutsCursorVisible(visibleRows)
		return m, nil
	}
	return m, nil
}

func (m Model) shortcutsVisibleRows() int {
	visibleRows := 10
	if m.height > 0 {
		if v := m.height - 16; v > 4 {
			visibleRows = v
		}
	}
	return visibleRows
}

func (m *Model) ensureShortcutsCursorVisible(visible int) {
	if m.shortcutsCursor < m.shortcutsOffset {
		m.shortcutsOffset = m.shortcutsCursor
	}
	if m.shortcutsCursor >= m.shortcutsOffset+visible {
		m.shortcutsOffset = m.shortcutsCursor - visible + 1
	}
	if m.shortcutsOffset < 0 {
		m.shortcutsOffset = 0
	}
}

func (m Model) shortcutsEntries() []string {
	return []string{
		"Navigation: ↑/↓ move cursor",
		"Navigation: [/] previous/next page",
		"Navigation: Enter open project menu",
		"Selection: Space select/deselect current repo",
		"Selection: Ctrl+A select/deselect all filtered repos",
		"Actions: a open Git actions modal",
		"Actions: l open last action logs",
		"Search: / open search mode",
		"Search: c clear search/filters",
		"Refresh: r rescan repositories",
		"View: f cycle filter",
		"View: s cycle sort",
		"View: 1..4 set sort mode directly",
		"Panels: d disk usage",
		"Panels: t timeline",
		"Workspace: w switch workspace",
		"Meta: Ctrl+P open command palette",
		"Meta: ? open shortcuts",
		"Meta: q quit",
	}
}

func (m Model) executeCommandItem(item commandItem) (tea.Model, tea.Cmd) {
	switch item.key {
	case "rescan":
		m.state = StateLoading
		m.statusMsg = "Rescanning..."
		return m, scanReposCmd(m.cfg, true)
	case "actions":
		return m, m.enterGitActionMode()
	case "filter":
		m.filterMode = (m.filterMode + 1) % 3
		m.resetPage()
		m.updateTable()
		m.statusMsg = "Filter: " + m.GetFilterModeName()
		return m, nil
	case "sort":
		m.sortMode = (m.sortMode + 1) % 4
		m.resetPage()
		m.updateTable()
		m.statusMsg = "Sorted by: " + m.GetSortModeName()
		return m, nil
	case "clear":
		m.searchQuery = ""
		m.textInput.SetValue("")
		m.filterMode = FilterAll
		m.resetPage()
		m.resizeTable()
		m.updateTable()
		m.statusMsg = "Filters cleared"
		return m, nil
	case "select_all":
		count, deselected := m.toggleSelectAllFiltered()
		m.updateTable()
		if deselected {
			m.statusMsg = "Deselected " + itoa(count) + " repos"
		} else {
			m.statusMsg = "Selected " + itoa(count) + " repos"
		}
		return m, nil
	case "open_repo":
		repo := m.GetSelectedRepo()
		if repo == nil {
			m.statusMsg = "No repo selected"
			return m, nil
		}
		m.enterOpenRepoMode(repo.Name, repo.Path)
		return m, nil
	case "workspace":
		m.state = StateWorkspaceSwitch
		m.workspaceInput.SetValue("")
		m.workspaceInput.Focus()
		m.workspaceError = ""
		return m, textinput.Blink
	case "shortcuts":
		m.enterShortcutsMode()
		return m, nil
	case "quit":
		return m, tea.Quit
	case "action_logs":
		if len(m.lastActionLogLines) == 0 {
			m.statusMsg = "No action logs yet"
			return m, nil
		}
		m.enterActionLogsMode()
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleCommandPaletteMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.filteredCommandItems()
	visibleRows := m.commandPaletteVisibleRows()

	switch msg.String() {
	case "esc":
		m.exitCommandPaletteMode()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.commandCursor > 0 {
			m.commandCursor--
			m.ensureCommandCursorVisible(len(items), visibleRows)
		}
		return m, nil
	case "down", "j":
		if len(items) > 0 && m.commandCursor < len(items)-1 {
			m.commandCursor++
			m.ensureCommandCursorVisible(len(items), visibleRows)
		}
		return m, nil
	case "pgup":
		m.commandCursor -= visibleRows
		if m.commandCursor < 0 {
			m.commandCursor = 0
		}
		m.ensureCommandCursorVisible(len(items), visibleRows)
		return m, nil
	case "pgdown":
		m.commandCursor += visibleRows
		if len(items) == 0 || m.commandCursor > len(items)-1 {
			m.commandCursor = len(items) - 1
			if m.commandCursor < 0 {
				m.commandCursor = 0
			}
		}
		m.ensureCommandCursorVisible(len(items), visibleRows)
		return m, nil
	case "enter":
		if len(items) == 0 {
			return m, nil
		}
		if m.commandCursor < 0 || m.commandCursor >= len(items) {
			m.commandCursor = 0
		}
		selected := items[m.commandCursor]
		m.exitCommandPaletteMode()
		return m.executeCommandItem(selected)
	}

	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	items = m.filteredCommandItems()
	if m.commandCursor >= len(items) {
		m.commandCursor = 0
	}
	m.ensureCommandCursorVisible(len(items), visibleRows)
	return m, cmd
}

func (m *Model) ensureCommandCursorVisible(itemsLen, visibleRows int) {
	if m.commandCursor < m.commandOffset {
		m.commandOffset = m.commandCursor
	}
	if m.commandCursor >= m.commandOffset+visibleRows {
		m.commandOffset = m.commandCursor - visibleRows + 1
	}
	if m.commandOffset < 0 {
		m.commandOffset = 0
	}
	maxOffset := 0
	if itemsLen > visibleRows {
		maxOffset = itemsLen - visibleRows
	}
	if m.commandOffset > maxOffset {
		m.commandOffset = maxOffset
	}
}

func (m Model) commandPaletteVisibleRows() int {
	visibleRows := 8
	if m.height > 0 {
		if v := m.height - 16; v > 4 {
			visibleRows = v
		}
	}
	return visibleRows
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		buf = append([]byte{byte('0' + v%10)}, buf...)
		v /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func (m Model) handleActionLogsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := 10
	if m.height > 0 {
		if v := m.height - 16; v > 4 {
			visible = v
		}
	}
	maxOffset := 0
	if len(m.lastActionLogLines) > visible {
		maxOffset = len(m.lastActionLogLines) - visible
	}

	switch msg.String() {
	case "esc", "l":
		if m.actionLogsReturnState == StateGitAction {
			m.state = StateGitAction
		} else {
			m.state = StateReady
		}
		m.actionLogsReturnState = StateReady
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.gitActionLogOffset > 0 {
			m.gitActionLogOffset--
		}
		return m, nil
	case "down", "j":
		if m.gitActionLogOffset < maxOffset {
			m.gitActionLogOffset++
		}
		return m, nil
	case "pgup":
		m.gitActionLogOffset -= visible
		if m.gitActionLogOffset < 0 {
			m.gitActionLogOffset = 0
		}
		return m, nil
	case "pgdown":
		m.gitActionLogOffset += visible
		if m.gitActionLogOffset > maxOffset {
			m.gitActionLogOffset = maxOffset
		}
		return m, nil
	}
	return m, nil
}
