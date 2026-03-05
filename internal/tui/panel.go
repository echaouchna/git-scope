package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/echaouchna/git-scope/internal/stats"
)

// PanelType represents which panel is currently active
type PanelType int

const (
	PanelNone PanelType = iota
	PanelDisk
	PanelTimeline
)

var (
	panelBorderActiveStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7C3AED")).
				Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#f0f6fc")).
			MarginBottom(1)

	panelMutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6e7681"))
)

// renderSplitPane renders a split-pane layout with table on left and panel on right
func renderSplitPane(leftContent, rightContent string, totalWidth int) string {
	// 60% for table, 40% for panel
	leftWidth := int(float64(totalWidth) * 0.58)
	rightWidth := totalWidth - leftWidth - 3 // Account for borders/gaps

	if rightWidth < 20 {
		rightWidth = 20
		leftWidth = totalWidth - rightWidth - 3
	}

	leftPane := lipgloss.NewStyle().
		Width(leftWidth).
		Render(leftContent)

	// Use active border style for panel (Tuimorphic)
	rightPane := panelBorderActiveStyle.
		Width(rightWidth).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
}

var (
	diskNameStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	diskSizeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
	diskNodeSizeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316")).Bold(true) // Orange for node_modules value

	// Separate colors for git and node_modules
	diskBarGit  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B5CF6")) // Purple for .git
	diskBarNode = lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316")) // Orange for node_modules
)

// renderDiskPanel renders the disk usage panel with bar chart
func renderDiskPanel(data *stats.DiskUsageData, width, height int) string {
	if data == nil {
		return panelMutedStyle.Render("Loading disk usage data...")
	}

	var b strings.Builder

	// Title - update based on what we're showing
	if data.HasNodeModules {
		b.WriteString(panelTitleStyle.Render("💾 Disk Usage (.git + node_modules)"))
	} else {
		b.WriteString(panelTitleStyle.Render("💾 Disk Usage (.git folders)"))
	}
	b.WriteString("\n\n")

	// Summary with breakdown
	b.WriteString(diskSizeStyle.Render(stats.FormatBytes(data.TotalSize)))
	b.WriteString(panelMutedStyle.Render(" total"))
	b.WriteString("\n")

	// Show breakdown if we have node_modules
	if data.HasNodeModules {
		b.WriteString(diskBarGit.Render("█"))
		b.WriteString(panelMutedStyle.Render(" .git: "))
		b.WriteString(diskSizeStyle.Render(stats.FormatBytes(data.TotalGitSize)))
		b.WriteString("  ")
		b.WriteString(diskBarNode.Render("█"))
		b.WriteString(panelMutedStyle.Render(" node_modules: "))
		b.WriteString(diskNodeSizeStyle.Render(stats.FormatBytes(data.TotalNodeSize)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	maxRows := diskMaxRows(height)
	barWidth := diskBarWidth(width)

	for i, repo := range data.Repos {
		if i >= maxRows {
			remaining := len(data.Repos) - maxRows
			if remaining > 0 {
				b.WriteString(panelMutedStyle.Render(fmt.Sprintf("  ... and %d more\n", remaining)))
			}
			break
		}

		// Truncate name
		name := repo.Name
		if len(name) > 10 {
			name = name[:9] + "…"
		}
		name = fmt.Sprintf("%-10s", name)

		// Calculate bar lengths
		gitBarLen := diskBarLen(repo.GitSize, data.MaxSize, barWidth)
		nodeBarLen := diskBarLen(repo.NodeModulesSize, data.MaxSize, barWidth)

		// Create stacked bar (git + node_modules)
		gitBar := strings.Repeat("█", gitBarLen)
		nodeBar := strings.Repeat("█", nodeBarLen)

		b.WriteString(diskNameStyle.Render(name))
		b.WriteString(" ")
		b.WriteString(diskBarGit.Render(gitBar))
		b.WriteString(diskBarNode.Render(nodeBar))
		b.WriteString(" ")
		b.WriteString(panelMutedStyle.Render(stats.FormatBytes(repo.TotalSize)))
		b.WriteString("\n")
	}

	// Legend
	b.WriteString("\n")
	b.WriteString(diskBarGit.Render("█"))
	b.WriteString(panelMutedStyle.Render(" .git "))
	if data.HasNodeModules {
		b.WriteString(diskBarNode.Render("█"))
		b.WriteString(panelMutedStyle.Render(" node_modules"))
	}

	return b.String()
}

// diskMaxRows computes how many repository rows can be shown in the disk panel.
// It subtracts space used by headers and legends, then clamps the result
// between 3 and 12 rows.
func diskMaxRows(height int) int {
	maxRows := height - 10
	if maxRows < 3 {
		return 3
	}
	if maxRows > 12 {
		return 12
	}
	return maxRows
}

// diskBarWidth computes the horizontal width of each disk-usage bar.
// It reserves space for the repository name and size, then clamps the
// remaining width between 8 and 25 characters.
func diskBarWidth(width int) int {
	barWidth := width - 35
	if barWidth < 8 {
		return 8
	}
	if barWidth > 25 {
		return 25
	}
	return barWidth
}

// diskBarLen converts a repo size into a bar length relative to maxSize.
// If size > 0, it returns at least 1 so non-zero repos are always visible.
func diskBarLen(size, maxSize int64, barWidth int) int {
	if maxSize <= 0 {
		return 0
	}

	n := int(float64(size) / float64(maxSize) * float64(barWidth))
	if n < 1 && size > 0 {
		return 1
	}

	return n
}

// Timeline styling
var (
	timelineTodayStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true) // Green
	timelineYesterdayStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308")).Bold(true) // Yellow
	timelineOlderStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))            // Gray
	timelineRepoStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	timelineBranchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))
	timelineMessageStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Italic(true)
	timelineTimeStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
)

// renderTimelinePanel renders the activity timeline panel
func renderTimelinePanel(data *stats.TimelineData, width, height int) string {
	if data == nil {
		return panelMutedStyle.Render("Loading timeline...")
	}

	var b strings.Builder

	// Title
	b.WriteString(panelTitleStyle.Render("⏰ Recent Activity"))
	b.WriteString("\n\n")

	if len(data.Entries) == 0 {
		b.WriteString(panelMutedStyle.Render("No recent commits found."))
		return b.String()
	}

	// Show entries grouped by day
	maxRows := height - 6
	if maxRows < 5 {
		maxRows = 5
	}

	currentDayLabel := ""
	rowCount := 0

	for _, entry := range data.Entries {
		if rowCount >= maxRows {
			remaining := len(data.Entries) - rowCount
			if remaining > 0 {
				b.WriteString(panelMutedStyle.Render(fmt.Sprintf("\n  ... and %d more\n", remaining)))
			}
			break
		}

		// Day header
		if entry.DayLabel != currentDayLabel {
			if currentDayLabel != "" {
				b.WriteString("\n")
			}

			b.WriteString(timelineDayStyle(entry.DayLabel).Render("● " + entry.DayLabel))
			b.WriteString("\n")
			currentDayLabel = entry.DayLabel
			rowCount++
		}

		// Entry
		name := entry.Name
		if len(name) > 15 {
			name = name[:14] + "…"
		}

		b.WriteString("  ")
		b.WriteString(timelineRepoStyle.Render(name))
		b.WriteString(" ")

		branch := entry.Branch
		if len(branch) > 10 {
			branch = branch[:9] + "…"
		}
		b.WriteString(timelineBranchStyle.Render("(" + branch + ")"))
		b.WriteString("\n")

		// Commit message
		if entry.Message != "" {
			msg := entry.Message
			maxMsgLen := width - 8
			if maxMsgLen < 20 {
				maxMsgLen = 20
			}
			if len(msg) > maxMsgLen {
				msg = msg[:maxMsgLen-3] + "..."
			}
			b.WriteString("    ")
			b.WriteString(timelineMessageStyle.Render("\"" + msg + "\""))
			b.WriteString("\n")
			rowCount++
		}

		// Time ago
		b.WriteString("    ")
		b.WriteString(timelineTimeStyle.Render(entry.TimeAgo))
		b.WriteString("\n")

		rowCount += 2
	}

	return b.String()
}

// timelineDayStyle returns the style used for a given day label in the timeline.
// "Today" and "Yesterday" are highlighted; everything else uses the muted style.
func timelineDayStyle(dayLabel string) lipgloss.Style {
	switch dayLabel {
	case "Today":
		return timelineTodayStyle
	case "Yesterday":
		return timelineYesterdayStyle
	default:
		return timelineOlderStyle
	}
}
