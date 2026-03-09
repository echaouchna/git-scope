package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/echaouchna/git-scope/internal/app"
	"github.com/echaouchna/git-scope/internal/workspace"
)

// View renders the TUI
func (m Model) View() string {
	content := m.renderContent()
	return appStyle.Render(content)
}

func (m Model) renderContent() string {
	var b strings.Builder

	switch m.state {
	case StateLoading:
		b.WriteString(m.renderLoading())
	case StateError:
		b.WriteString(m.renderError())
	case StateReady, StateSearching:
		b.WriteString(m.renderDashboard())
	case StateBookmarks, StateBookmarkSearch:
		b.WriteString(m.renderBookmarksScreen())
	case StateWorkspaceSwitch:
		b.WriteString(m.renderWorkspaceModal())
	case StateGitAction:
		b.WriteString(m.renderGitActionModal())
	case StateOpenRepo:
		b.WriteString(m.renderOpenRepoModal())
	case StateShortcuts:
		b.WriteString(m.renderShortcutsModal())
	case StateCommandPalette:
		b.WriteString(m.renderCommandPaletteModal())
	case StateActionLogs:
		b.WriteString(m.renderActionLogsModal())
	}

	return b.String()
}

func (m Model) renderLoading() string {
	var b strings.Builder

	b.WriteString(compactLogo())
	b.WriteString("  ")
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(loadingStyle.Render("Scanning repositories..."))
	b.WriteString("\n\n")

	b.WriteString(subtitleStyle.Render("Searching for git repos in:"))
	b.WriteString("\n")

	// Show workspace path if switching, otherwise show config roots
	if m.activeWorkspace != "" {
		b.WriteString(pathBulletStyle.Render("  → "))
		b.WriteString(pathStyle.Render(m.activeWorkspace))
		b.WriteString("\n")
	} else {
		for _, root := range m.cfg.Roots {
			b.WriteString(pathBulletStyle.Render("  → "))
			b.WriteString(pathStyle.Render(root))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	b.WriteString(helpStyle.Render("Press " + helpKeyStyle.Render("q") + " to quit"))

	return b.String()
}

func (m Model) renderError() string {
	var b strings.Builder

	b.WriteString(compactLogo())
	b.WriteString("  ")
	b.WriteString(errorTitleStyle.Render("✗ Error"))
	b.WriteString("\n\n")

	errContent := ""
	if m.err != nil {
		errContent = m.err.Error()
	} else {
		errContent = "Unknown error occurred"
	}
	b.WriteString(errorBoxStyle.Render(errContent))
	b.WriteString("\n\n")

	// Actionable suggestions
	b.WriteString(subtitleStyle.Render("💡 Suggestions:"))
	b.WriteString("\n")
	b.WriteString(pathBulletStyle.Render("  → "))
	b.WriteString(pathStyle.Render("Check your config at ~/.config/git-scope/config.yml"))
	b.WriteString("\n")
	b.WriteString(pathBulletStyle.Render("  → "))
	b.WriteString(pathStyle.Render("Run 'git-scope init' to reconfigure"))
	b.WriteString("\n")
	b.WriteString(pathBulletStyle.Render("  → "))
	b.WriteString(pathStyle.Render("Make sure git is installed and in PATH"))
	b.WriteString("\n\n")

	b.WriteString(helpItem("r", "retry"))
	b.WriteString("  •  ")
	b.WriteString(helpItem("q", "quit"))

	return b.String()
}

func (m Model) renderDashboard() string {
	var b strings.Builder

	// Header with logo on its own line
	logo := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA")).Render("git-scope")
	version := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(" v" + app.Version)
	b.WriteString(logo + version)
	b.WriteString("\n\n")

	// Stats bar (always show first for consistent layout)
	b.WriteString(m.renderStats())
	b.WriteString("\n")

	// Search bar (show when searching or has active search)
	if m.state == StateSearching {
		b.WriteString(m.renderSearchBar())
		b.WriteString("\n")
	} else if m.searchQuery != "" {
		// Show search badge only if searchQuery is actually set
		b.WriteString(m.renderSearchBadge())
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Main content area
	b.WriteString(m.table.View())
	b.WriteString("\n")
	if pathBar := m.renderSelectedRepoPathBar(); pathBar != "" {
		b.WriteString(pathBar)
		b.WriteString("\n")
	}

	// Status message if any
	if m.statusMsg != "" {
		b.WriteString(statusStyle.Render("→ " + m.statusMsg))
		b.WriteString("\n")
	}

	// Star nudge (if active)
	if m.showStarNudge {
		b.WriteString(m.renderStarNudge())
		b.WriteString("\n")
	}

	// Legend
	b.WriteString(m.renderLegend())
	b.WriteString("\n")

	// Help footer
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderBookmarksScreen() string {
	var b strings.Builder

	logo := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA")).Render("git-scope")
	version := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(" v" + app.Version)
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B")).
		Bold(true).
		Render("Bookmarks")
	b.WriteString(logo + version + "  " + title)
	b.WriteString("\n\n")

	b.WriteString(m.renderStats())
	b.WriteString("\n")
	b.WriteString(m.renderBookmarkSearchSection())
	b.WriteString("\n\n")
	b.WriteString(m.table.View())
	b.WriteString("\n")
	if pathBar := m.renderSelectedRepoPathBar(); pathBar != "" {
		b.WriteString(pathBar)
		b.WriteString("\n")
	}
	if m.statusMsg != "" {
		b.WriteString(statusStyle.Render("→ " + m.statusMsg))
		b.WriteString("\n")
	}
	b.WriteString(m.renderLegend())
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderSearchBar() string {
	searchStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(0, 1)

	// Show active search input
	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true).
		Render("🔍 Search: ")
	return searchStyle.Render(label + m.textInput.View())
}

func (m Model) renderSearchBadge() string {
	// Guard: don't render empty badge
	if m.searchQuery == "" {
		return ""
	}

	// Show current search query as badge
	searchBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Padding(0, 1).
		Render("🔍 " + m.searchQuery)

	clearHint := lipgloss.NewStyle().
		Foreground(mutedColor).
		Render(" (press c to clear)")

	return searchBadge + clearHint
}

func (m Model) renderBookmarkSearchSection() string {
	if m.state == StateBookmarkSearch {
		searchStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#F59E0B")).
			Padding(0, 1)
		label := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Bold(true).
			Render("★ Search bookmarks: ")
		return searchStyle.Render(label + m.bookmarkInput.View())
	}

	if m.bookmarkQuery == "" {
		return lipgloss.NewStyle().
			Foreground(mutedColor).
			Render(fmt.Sprintf("★ %d bookmarked repos  (press / to search)", len(m.sortedRepos)))
	}

	badge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#111827")).
		Background(lipgloss.Color("#FBBF24")).
		Padding(0, 1).
		Bold(true).
		Render("★ " + m.bookmarkQuery)
	clearHint := lipgloss.NewStyle().
		Foreground(mutedColor).
		Render(" (press c to clear)")
	return badge + clearHint
}

func (m Model) renderSelectedRepoPathBar() string {
	repo := m.GetSelectedRepo()
	if repo == nil {
		return ""
	}

	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#111827")).
		Background(lipgloss.Color("#93C5FD")).
		Padding(0, 1).
		Bold(true).
		Render("PATH")

	maxWidth := m.width - 14
	if maxWidth < 20 {
		maxWidth = 20
	}

	pathText := truncatePathFromStart(repo.Path, maxWidth)
	path := lipgloss.NewStyle().
		Foreground(textPrimary).
		Background(bgSurface).
		Padding(0, 1).
		Render(pathText)

	return lipgloss.JoinHorizontal(lipgloss.Left, label, " ", path)
}

func truncatePathFromStart(s string, maxLen int) string {
	runes := []rune(s)
	if maxLen <= 4 || len(runes) <= maxLen {
		return s
	}

	keepRight := maxLen - 4
	if keepRight <= 0 {
		return ".../"
	}
	return ".../" + string(runes[len(runes)-keepRight:])
}

func (m Model) renderStats() string {
	total := len(m.repos)
	shown := len(m.sortedRepos)
	dirty := 0
	clean := 0
	for _, r := range m.repos {
		if r.Status.IsDirty {
			dirty++
		} else {
			clean++
		}
	}

	stats := []string{}

	// Show count with filter info
	if shown == total {
		stats = append(stats, statsBadgeStyle.Render(fmt.Sprintf("📁 %d repos", total)))
	} else {
		stats = append(stats, statsBadgeStyle.Render(fmt.Sprintf("📁 %d/%d repos", shown, total)))
	}

	if dirty > 0 {
		stats = append(stats, dirtyBadgeStyle.Render(fmt.Sprintf("● %d dirty", dirty)))
	}
	if clean > 0 {
		stats = append(stats, cleanBadgeStyle.Render(fmt.Sprintf("○ %d clean", clean)))
	}
	if selected := m.selectedReposCount(); selected > 0 {
		selectedBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#F59E0B")).
			Padding(0, 1).
			Bold(true).
			Render(fmt.Sprintf("☑ %d selected", selected))
		stats = append(stats, selectedBadge)
	}
	if bookmarks := m.bookmarksCount(); bookmarks > 0 {
		bookmarksBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#111827")).
			Background(lipgloss.Color("#FBBF24")).
			Padding(0, 1).
			Bold(true).
			Render(fmt.Sprintf("★ %d bookmarked", bookmarks))
		stats = append(stats, bookmarksBadge)
	}

	// Filter indicator with inline hint
	if m.filterMode != FilterAll {
		filterBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#60A5FA")).
			Padding(0, 1).
			Bold(true).
			Render("⚡ " + m.GetFilterModeName())
		filterHint := hintStyle.Render(" (f)")
		stats = append(stats, filterBadge+filterHint)
	}

	// Sort indicator with inline hint
	sortBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Padding(0, 1).
		Render("⇅ " + m.GetSortModeName())
	sortHint := hintStyle.Render(" (s)")
	stats = append(stats, sortBadge+sortHint)

	// Pagination indicator (only show if more than one page)
	totalPages := m.getTotalPages()
	if totalPages > 1 {
		pageBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#10B981")).
			Padding(0, 1).
			Render(fmt.Sprintf("📄 %d/%d", m.currentPage+1, totalPages))
		pageHint := hintStyle.Render(" ([])")
		stats = append(stats, pageBadge+pageHint)
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, stats...)
}

// renderLegend renders a compact single-line legend (Tuimorphic style)
func (m Model) renderLegend() string {
	dirty := dirtyDotStyle.Render("●") + legendStyle.Render(" dirty")
	clean := cleanDotStyle.Render("○") + legendStyle.Render(" clean")
	editor := legendStyle.Render(fmt.Sprintf("  Editor: %s", m.cfg.Editor))

	return legendStyle.Render(dirty + "  " + clean + editor)
}

// renderHelp renders a Tuimorphic keybindings bar with box-drawing separators
func (m Model) renderHelp() string {
	sep := keyBindingSepStyle.Render(" │ ")
	var items []string

	switch m.state {
	case StateSearching:
		// Search mode help
		items = []string{
			keyBinding("type", "search"),
			keyBinding("enter", "apply"),
			keyBinding("esc", "cancel"),
		}
	case StateBookmarkSearch:
		items = []string{
			keyBinding("type", "search"),
			keyBinding("enter", "apply"),
			keyBinding("esc", "cancel"),
		}
	case StateBookmarks:
		items = []string{
			keyBinding("↑↓", "nav"),
			keyBinding("b", "toggle bookmark"),
			keyBinding("/", "search"),
			keyBinding("enter", "open"),
			keyBinding("esc", "all repos"),
			keyBinding("q", "quit"),
		}
	case StateWorkspaceSwitch:
		// Workspace switch mode help
		items = []string{
			keyBinding("type", "path"),
			keyBinding("tab", "complete"),
			keyBinding("enter", "switch"),
			keyBinding("esc", "cancel"),
		}
	case StateGitAction:
		items = []string{
			keyBinding("↑↓", "action"),
			keyBinding("tab", "autocomplete"),
			keyBinding("enter", "run"),
			keyBinding("l", "logs"),
			keyBinding("esc", "cancel"),
		}
	case StateShortcuts:
		items = []string{
			keyBinding("esc", "close"),
			keyBinding("?", "close"),
		}
	case StateCommandPalette:
		items = []string{
			keyBinding("type", "search"),
			keyBinding("↑↓", "choose"),
			keyBinding("enter", "run"),
			keyBinding("esc", "cancel"),
		}
	case StateActionLogs:
		items = []string{
			keyBinding("type", "filter"),
			keyBinding("↑↓", "scroll"),
			keyBinding("pgup/dn", "page"),
			keyBinding("tab", "autocomplete"),
			keyBinding("esc", "close"),
		}
	case StateOpenRepo:
		items = []string{
			keyBinding("type", "search/cmd"),
			keyBinding("↑↓", "choose"),
			keyBinding("pgup/dn", "page"),
			keyBinding("enter", "confirm/run"),
			keyBinding("esc", "cancel"),
		}
	default:
		// Normal mode help - Tuimorphic style
		items = []string{
			keyBinding("↑↓", "nav"),
			keyBinding("space", "select"),
			keyBinding("b", "bookmark"),
			keyBinding("B", "bookmarks"),
			keyBinding("enter", "open"),
			keyBinding("a", "actions"),
			keyBinding("l", "logs"),
			keyBinding("/", "search"),
			keyBinding("?", "shortcuts"),
			keyBinding("ctrl+p", "commands"),
			keyBinding("q", "quit"),
		}
	}

	return keyBindingsBarStyle.Render(strings.Join(items, sep))
}

func (m Model) renderGitActionModal() string {
	var b strings.Builder

	b.WriteString(compactLogo())
	b.WriteString("\n\n")

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(78)

	content := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("⚙ Git Actions"),
		"",
		"Targets: " + m.gitActionTargetLine(),
		"",
		"Action:",
		strings.Join(m.gitActionActionLines(), "\n"),
	}
	content = m.appendGitActionBranchSection(content)

	finished := m.gitActionFinished()
	content = m.appendGitActionProgressSection(content, finished)

	if m.gitActionError != "" {
		content = append(content, "", lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("❌ "+m.gitActionError))
	}
	content = append(content, "", lipgloss.NewStyle().Foreground(mutedColor).Render(m.gitActionFooterHint(finished)))
	b.WriteString(modalStyle.Render(strings.Join(content, "\n")))

	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

func (m Model) gitActionTargetLine() string {
	targets, source := m.targetReposForAction()
	targetLine := fmt.Sprintf("%d repo(s)", len(targets))
	switch source {
	case "selected":
		targetLine += " from selected set"
	case "highlighted":
		targetLine += " from highlighted row"
	default:
		targetLine += " from filtered list"
	}
	return targetLine
}

func (m Model) gitActionActionLines() []string {
	actions := m.gitActionMenuLabels()
	lines := make([]string, 0, len(actions))
	idleStyle := lipgloss.NewStyle()
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A78BFA")).
		Bold(true)
	if m.gitActionRunning {
		idleStyle = lipgloss.NewStyle().Foreground(mutedColor)
		selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8B82B8")).
			Bold(true)
	}
	for i, label := range actions {
		line := fmt.Sprintf("[%d] %s", i+1, label)
		prefix := "  "
		if i == m.gitActionCursor {
			prefix = "➤ "
			line = selectedStyle.Render(line)
		} else {
			line = idleStyle.Render(line)
		}
		lines = append(lines, prefix+line)
	}
	return lines
}

func (m Model) appendGitActionBranchSection(content []string) []string {
	if !m.gitActionNeedsBranch() {
		return content
	}
	content = append(content, "", "Branch: "+m.gitActionInput.View())
	switch {
	case m.gitActionLoadingBranch:
		content = append(content, hintStyle.Render("Loading common branches..."))
	case len(m.gitActionBranchMatches) > 0:
		suggestions := m.gitActionBranchMatches
		if len(suggestions) > 5 {
			suggestions = suggestions[:5]
		}
		content = append(content, hintStyle.Render("Suggestions: "+strings.Join(suggestions, ", ")))
	case len(m.gitActionBranchOptions) > 0:
		content = append(content, hintStyle.Render("No match for current input"))
	}
	return content
}

func (m Model) gitActionFinished() bool {
	return !m.gitActionRunning && m.gitActionProgressTotal > 0 && m.gitActionProgressIdx >= m.gitActionProgressTotal
}

func (m Model) appendGitActionProgressSection(content []string, finished bool) []string {
	if !m.gitActionRunning && !finished {
		return content
	}
	current := m.gitActionProgressIdx
	if current > m.gitActionProgressTotal {
		current = m.gitActionProgressTotal
	}
	label := "Running"
	if !m.gitActionRunning {
		label = "Last run"
	}
	summaryStyle := m.gitActionSummaryStyle(finished)
	statusLine := fmt.Sprintf("Progress: %d/%d   Success: %d   Failed: %d", current, m.gitActionProgressTotal, m.gitActionSuccess, m.gitActionFailed)
	content = append(content, "",
		fmt.Sprintf("%s: %s", label, m.gitActionCurrentRepo),
		summaryStyle.Render(statusLine),
	)
	if m.gitActionRunning {
		content = append(content, fmt.Sprintf("%s Running action...", m.spinner.View()))
	}
	return content
}

func (m Model) gitActionSummaryStyle(finished bool) lipgloss.Style {
	if !finished {
		return lipgloss.NewStyle().Foreground(mutedColor)
	}
	if m.gitActionFailed > 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true)
}

func (m Model) gitActionFooterHint(finished bool) string {
	if finished {
		return "Completed. Enter run again, 1-4/↑↓ choose action, Esc return, l view logs"
	}
	return "↑/↓ action, type branch, Tab autocomplete, Enter run, Esc cancel"
}

func (m Model) renderOpenRepoModal() string {
	var b strings.Builder
	b.WriteString(compactLogo())
	b.WriteString("\n\n")

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(70)

	options := m.filteredOpenRepoOptions()
	visibleRows := m.openRepoVisibleRows()
	start := m.openRepoOffset
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(options) {
		end = len(options)
	}
	if start > end {
		start = 0
		end = len(options)
		if end > visibleRows {
			end = visibleRows
		}
	}

	listLines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := options[i].label
		if options[i].helper != "" {
			line += lipgloss.NewStyle().Foreground(mutedColor).Render("  - " + options[i].helper)
		}
		prefix := "  "
		if i == m.openRepoChoice {
			prefix = "➤ "
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render(line)
		}
		listLines = append(listLines, prefix+line)
	}
	if len(listLines) == 0 {
		listLines = append(listLines, lipgloss.NewStyle().Foreground(mutedColor).Render("No matching options"))
	}

	content := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Open Project"),
		"",
		"Repository: " + m.openRepoName,
		"Path: " + m.openRepoPath,
		"",
		"Open: " + m.openRepoInput.View(),
		"",
		strings.Join(listLines, "\n"),
		"",
		lipgloss.NewStyle().Foreground(mutedColor).Render("Use ':<command>' (example: ':fish'). Aliases are suggested with short helpers."),
		lipgloss.NewStyle().Foreground(mutedColor).Render(fmt.Sprintf("Item %d/%d • Enter confirm • Esc cancel", m.openRepoChoice+1, maxInt(1, len(options)))),
	}
	b.WriteString(modalStyle.Render(strings.Join(content, "\n")))

	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

func (m Model) renderShortcutsModal() string {
	var b strings.Builder
	b.WriteString(compactLogo())
	b.WriteString("\n\n")

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(76)

	items := m.shortcutsEntries()
	visibleRows := 10
	if m.height > 0 {
		if v := m.height - 16; v > 4 {
			visibleRows = v
		}
	}
	start := m.shortcutsOffset
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(items) {
		end = len(items)
	}
	if start > end {
		start = 0
	}

	listLines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := items[i]
		prefix := "  "
		if i == m.shortcutsCursor {
			prefix = "➤ "
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render(line)
		}
		listLines = append(listLines, prefix+line)
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Keyboard Shortcuts"),
		"",
		strings.Join(listLines, "\n"),
		"",
		lipgloss.NewStyle().Foreground(mutedColor).Render(fmt.Sprintf("Item %d/%d • ↑/↓ scroll • Esc/? close", m.shortcutsCursor+1, len(items))),
	}

	b.WriteString(modalStyle.Render(strings.Join(lines, "\n")))
	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

func (m Model) renderCommandPaletteModal() string {
	var b strings.Builder
	b.WriteString(compactLogo())
	b.WriteString("\n\n")

	modalWidth := 76
	if m.width > 0 {
		if w := m.width - 6; w > 44 {
			modalWidth = w
		}
	}

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(modalWidth)

	items := m.filteredCommandItems()
	visibleRows := m.commandPaletteVisibleRows()
	start := m.commandOffset
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(items) {
		end = len(items)
	}
	if start > end {
		start = 0
		end = len(items)
		if end > visibleRows {
			end = visibleRows
		}
	}
	lines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Command Palette"),
		"",
		"Command: " + m.commandInput.View(),
		"",
	}

	if len(items) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(mutedColor).Render("No matching commands"))
	} else {
		for i := start; i < end; i++ {
			line := items[i].label
			prefix := "  "
			if i == m.commandCursor {
				prefix = "➤ "
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render(line)
			}
			lines = append(lines, prefix+line)
		}
		lines = append(lines, "", lipgloss.NewStyle().Foreground(mutedColor).Render(fmt.Sprintf("Item %d/%d • ↑/↓ scroll • PgUp/PgDn page", m.commandCursor+1, len(items))))
	}

	lines = append(lines, "", lipgloss.NewStyle().Foreground(mutedColor).Render("Type to search, Enter run, Esc cancel"))
	b.WriteString(modalStyle.Render(strings.Join(lines, "\n")))
	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

func (m Model) renderActionLogsModal() string {
	var b strings.Builder
	b.WriteString(compactLogo())
	b.WriteString("\n\n")

	const modalWidth = 90
	const logLineWidth = 82

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(modalWidth)

	visible := m.actionLogsVisibleRows()
	filteredLines := m.filteredActionLogLines()
	start := m.gitActionLogOffset
	if start < 0 {
		start = 0
	}
	maxOffset := 0
	if len(filteredLines) > visible {
		maxOffset = len(filteredLines) - visible
	}
	if start > maxOffset {
		start = maxOffset
	}
	end := start + visible
	if end > len(filteredLines) {
		end = len(filteredLines)
	}
	if start > end {
		start = 0
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render(m.actionLogsTitle()),
		"",
		truncateString(m.actionLogsSummary(), logLineWidth),
		"Filter: " + m.actionLogsInput.View(),
		"",
	}
	if len(filteredLines) == 0 {
		if m.actionLogsLive && m.gitActionRunning {
			lines = append(lines, lipgloss.NewStyle().Foreground(mutedColor).Render("(waiting for live logs...)"))
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(mutedColor).Render("(no logs)"))
		}
	} else {
		for _, raw := range filteredLines[start:end] {
			lines = append(lines, m.renderLogLine(raw, logLineWidth))
		}
		lines = append(lines, "", hintStyle.Render(fmt.Sprintf("Log lines %d-%d/%d", start+1, end, len(filteredLines))))
	}
	autohint := ""
	if cands := m.actionLogsAutocompleteCandidates(); len(cands) > 0 {
		autohint = " | Tab autocomplete author"
	}
	followHint := ""
	if m.actionLogsLive {
		if m.actionLogsAutoFollow {
			followHint = " | live autoscroll: on"
		} else {
			followHint = " | live autoscroll: off"
		}
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(mutedColor).Render("↑/↓ scroll, PgUp/PgDn page, type filter"+autohint+followHint+", Esc close"))
	b.WriteString(modalStyle.Render(strings.Join(lines, "\n")))
	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

func (m Model) actionLogsTitle() string {
	if m.actionLogsLive && m.gitActionRunning {
		return m.spinner.View() + " Action Logs (Live)"
	}
	return "Last Action Logs"
}

func (m Model) actionLogsSummary() string {
	if m.actionLogsLive && m.gitActionRunning {
		return m.statusMsg
	}
	return m.lastActionSummary
}

func (m Model) renderLogLine(line string, maxWidth int) string {
	trimmed := truncateString(line, maxWidth)
	switch {
	case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "] RUNNING"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true).Render(trimmed)
	case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "] OK"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true).Render(trimmed)
	case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "] ERROR"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true).Render(trimmed)
	case strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "] "):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Bold(true).Render(trimmed)
	case strings.HasPrefix(trimmed, "  branch:"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE")).Render(trimmed)
	case strings.HasPrefix(trimmed, "  dirty:"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render(trimmed)
	case strings.HasPrefix(trimmed, "  warning:"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(trimmed)
	case strings.HasPrefix(trimmed, "  commits:"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true).Render(trimmed)
	case strings.HasPrefix(trimmed, "  +"), strings.HasPrefix(trimmed, "+"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Render(trimmed)
	case strings.HasPrefix(trimmed, "  -"), strings.HasPrefix(trimmed, "-"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(trimmed)
	}
	return lipgloss.NewStyle().Foreground(textPrimary).Render(trimmed)
}

// keyBinding creates a styled key-action pair for the keybindings bar
func keyBinding(key, action string) string {
	return keyBindingKeyStyle.Render(key) + " " + action
}

// renderWorkspaceModal renders the workspace switch modal
func (m Model) renderWorkspaceModal() string {
	var b strings.Builder

	// Header with logo
	b.WriteString(compactLogo())
	b.WriteString("\n\n")

	// Modal box
	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(50)

	// Modal title
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A78BFA")).
		Bold(true).
		Render("📁 Switch Workspace")

	// Path input
	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true).
		Render("Path: ")

	// Error message if any
	errorLine := ""
	if m.workspaceError != "" {
		errorLine = "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Render("❌ "+m.workspaceError)
	}

	// Autocomplete hint (what Tab will apply)
	autocompleteLine := ""
	currentPath := m.workspaceInput.Value()
	if currentPath != "" {
		completedPath := workspace.CompleteDirectoryPath(currentPath)
		if completedPath != currentPath {
			autocompleteLine = "\n" + lipgloss.NewStyle().
				Foreground(mutedColor).
				Render("↳ autocomplete: "+completedPath)
		}
	}

	// Footer hints
	footer := lipgloss.NewStyle().
		Foreground(mutedColor).
		Render("\n\nTab = complete   Enter = scan   Esc = cancel")

	modalContent := title + "\n\n" + label + m.workspaceInput.View() + autocompleteLine + errorLine + footer
	b.WriteString(modalStyle.Render(modalContent))

	// Help bar
	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderStarNudge renders the subtle star nudge message in the footer
func (m Model) renderStarNudge() string {
	nudgeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FCD34D")).
		Italic(true)

	ctaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A78BFA")).
		Bold(true)

	message := nudgeStyle.Render("✨ If git-scope helped you stay in flow, a GitHub star helps others discover it.")
	cta := ctaStyle.Render(" (S) Open GitHub")

	return message + cta
}
