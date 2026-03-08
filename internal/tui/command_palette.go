package tui

import (
	"fmt"
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
	m.commandStandupAuthors = nil
	m.commandInput.SetValue("")
	m.commandInput.Focus()
	if len(m.repos) == 0 {
		return nil
	}
	return loadStandupAuthorsCmd(cloneRepos(m.repos), "24h", true)
}

func (m *Model) enterActionLogsMode() {
	live := m.state == StateGitAction && m.gitActionRunning
	m.actionLogsReturnState = m.state
	m.state = StateActionLogs
	m.actionLogsInput.SetValue("")
	m.actionLogsInput.Focus()
	m.actionLogsAutocomplete = 0
	m.actionLogsLastQuery = ""
	m.actionLogsLive = live
	m.actionLogsAutoFollow = live
	if live {
		m.setActionLogsOffsetToBottom()
		return
	}
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
	items := []commandItem{
		{label: "Rescan repositories", key: "rescan"},
		{label: "Open Git actions", key: "actions"},
		{label: "Toggle filter mode", key: "filter"},
		{label: "Cycle sort mode", key: "sort"},
		{label: "Clear search and filters", key: "clear"},
		{label: "Select/deselect all filtered", key: "select_all"},
		{label: "Open selected repo menu", key: "open_repo"},
		{label: "Switch workspace", key: "workspace"},
		{label: "Standup (24h, all branches)", key: "standup_24h"},
		{label: "Standup (3d, all branches)", key: "standup_3d"},
		{label: "Standup (7d, all branches)", key: "standup_7d"},
		{label: "Standup (24h, current branch)", key: "standup_24h_current"},
		{label: "Standup (3d, current branch)", key: "standup_3d_current"},
		{label: "Standup (7d, current branch)", key: "standup_7d_current"},
		{label: "Show shortcuts", key: "shortcuts"},
		{label: "Show last action logs", key: "action_logs"},
		{label: "Quit", key: "quit"},
	}
	for _, author := range m.commandStandupAuthors {
		items = append(items, commandItem{
			label: fmt.Sprintf("Standup (24h, all branches) by %s", author),
			key:   "standup_author:" + author,
		})
	}
	return items
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
	case "standup_24h":
		return m.runStandupFromPalette("24h", true)
	case "standup_3d":
		return m.runStandupFromPalette("3d", true)
	case "standup_7d":
		return m.runStandupFromPalette("7d", true)
	case "standup_24h_current":
		return m.runStandupFromPalette("24h", false)
	case "standup_3d_current":
		return m.runStandupFromPalette("3d", false)
	case "standup_7d_current":
		return m.runStandupFromPalette("7d", false)
	default:
		if strings.HasPrefix(item.key, "standup_author:") {
			author := strings.TrimPrefix(item.key, "standup_author:")
			return m.runStandupFromPalette("24h", true, author)
		}
	}
	switch item.key {
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

func (m Model) runStandupFromPalette(since string, allBranches bool, author ...string) (tea.Model, tea.Cmd) {
	if len(m.repos) == 0 {
		m.statusMsg = "No repositories loaded"
		return m, nil
	}
	authorFilter := ""
	if len(author) > 0 {
		authorFilter = strings.TrimSpace(author[0])
	}
	if allBranches {
		m.statusMsg = "Generating standup (" + since + ", all branches)..."
	} else {
		m.statusMsg = "Generating standup (" + since + ")..."
	}
	if authorFilter != "" {
		m.statusMsg = "Generating standup (" + since + ", author: " + authorFilter + ")..."
	}
	return m, runStandupReportCmd(cloneRepos(m.repos), since, allBranches, authorFilter)
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
	lines := m.filteredActionLogLines()
	visible := m.actionLogsVisibleRows()
	maxOffset := 0
	if len(lines) > visible {
		maxOffset = len(lines) - visible
	}

	switch msg.String() {
	case "esc":
		if m.actionLogsReturnState == StateGitAction {
			m.state = StateGitAction
		} else {
			m.state = StateReady
		}
		m.actionLogsReturnState = StateReady
		m.actionLogsInput.Blur()
		m.actionLogsLive = false
		m.actionLogsAutoFollow = false
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		candidates := m.actionLogsAutocompleteCandidates()
		if len(candidates) == 0 {
			return m, nil
		}
		query := strings.ToLower(strings.TrimSpace(m.actionLogsInput.Value()))
		if query != m.actionLogsLastQuery {
			m.actionLogsAutocomplete = 0
			m.actionLogsLastQuery = query
		} else {
			m.actionLogsAutocomplete = (m.actionLogsAutocomplete + 1) % len(candidates)
		}
		m.actionLogsInput.SetValue(candidates[m.actionLogsAutocomplete])
		m.actionLogsInput.CursorEnd()
		if m.actionLogsLive {
			m.actionLogsAutoFollow = true
			m.setActionLogsOffsetToBottom()
		} else {
			m.gitActionLogOffset = 0
		}
		return m, nil
	case "up", "k":
		if m.actionLogsLive {
			m.actionLogsAutoFollow = false
		}
		if m.gitActionLogOffset > 0 {
			m.gitActionLogOffset--
		}
		return m, nil
	case "down", "j":
		if m.gitActionLogOffset < maxOffset {
			m.gitActionLogOffset++
		}
		if m.actionLogsLive {
			m.actionLogsAutoFollow = m.gitActionLogOffset >= maxOffset
		}
		return m, nil
	case "pgup":
		if m.actionLogsLive {
			m.actionLogsAutoFollow = false
		}
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
		if m.actionLogsLive {
			m.actionLogsAutoFollow = m.gitActionLogOffset >= maxOffset
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.actionLogsInput, cmd = m.actionLogsInput.Update(msg)
	m.actionLogsAutocomplete = 0
	m.actionLogsLastQuery = strings.ToLower(strings.TrimSpace(m.actionLogsInput.Value()))
	if m.actionLogsLive {
		m.actionLogsAutoFollow = true
		m.setActionLogsOffsetToBottom()
	} else {
		m.gitActionLogOffset = 0
	}
	return m, cmd
}

func (m Model) actionLogsVisibleRows() int {
	visible := 12
	if m.height > 0 {
		if v := m.height - 18; v > 4 {
			visible = v
		}
	}
	return visible
}

func (m Model) filteredActionLogLines() []string {
	sourceLines := m.actionLogSourceLines()
	query := strings.ToLower(strings.TrimSpace(m.actionLogsInput.Value()))
	if query == "" {
		return sourceLines
	}

	// Filter by logical blocks (separated by empty lines) so repo context
	// remains visible when matching commit/body lines.
	blocks := make([][]string, 0)
	current := make([]string, 0)
	for _, line := range sourceLines {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = make([]string, 0)
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	out := make([]string, 0, len(sourceLines))
	for _, block := range blocks {
		if len(block) == 0 {
			continue
		}
		header := block[0]
		headerMatch := strings.Contains(strings.ToLower(header), query)

		// If the header itself matches, keep the whole block for full context.
		if headerMatch {
			out = append(out, block...)
			out = append(out, "")
			continue
		}

		keep := make([]string, 0, len(block))
		matchedCommitLine := false
		for _, line := range block {
			if strings.Contains(strings.ToLower(line), query) {
				if strings.HasPrefix(strings.TrimLeft(line, " "), "+") {
					matchedCommitLine = true
				}
				keep = append(keep, line)
			}
		}
		if len(keep) == 0 {
			continue
		}

		// Keep repo header to preserve readability of filtered matches.
		if header != "" {
			out = append(out, header)
		}
		// If commit lines matched, keep the "commits:" marker when present.
		if matchedCommitLine {
			for _, line := range block {
				if strings.TrimSpace(strings.ToLower(line)) == "commits:" || strings.TrimSpace(strings.ToLower(line)) == "commits" {
					out = append(out, line)
					break
				}
			}
		}
		out = append(out, keep...)
		out = append(out, "")
	}

	// Trim trailing separator for cleaner pagination math.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func (m Model) actionLogsAutocompleteCandidates() []string {
	query := strings.ToLower(strings.TrimSpace(m.actionLogsInput.Value()))
	seen := map[string]struct{}{}
	candidates := make([]string, 0)

	for _, line := range m.actionLogSourceLines() {
		if strings.HasPrefix(line, "  + ") {
			trimmed := strings.TrimPrefix(line, "  + ")
			parts := strings.SplitN(trimmed, " | ", 2)
			if len(parts) > 0 {
				left := strings.TrimSpace(parts[0]) // "<hash> <author>"
				sp := strings.Index(left, " ")
				if sp >= 0 && sp+1 < len(left) {
					author := strings.TrimSpace(left[sp+1:])
					if author != "" {
						lower := strings.ToLower(author)
						if query == "" || strings.Contains(lower, query) {
							if _, ok := seen[author]; !ok {
								seen[author] = struct{}{}
								candidates = append(candidates, author)
							}
						}
					}
				}
			}
		}
	}
	return candidates
}

func (m Model) actionLogSourceLines() []string {
	if m.actionLogsLive && m.gitActionRunning {
		return m.gitActionLogLines
	}
	return m.lastActionLogLines
}

func (m *Model) setActionLogsOffsetToBottom() {
	lines := m.filteredActionLogLines()
	visible := m.actionLogsVisibleRows()
	maxOffset := 0
	if len(lines) > visible {
		maxOffset = len(lines) - visible
	}
	m.gitActionLogOffset = maxOffset
}
