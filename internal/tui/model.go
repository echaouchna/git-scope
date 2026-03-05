package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/echaouchna/git-scope/internal/config"
	"github.com/echaouchna/git-scope/internal/model"
	"github.com/echaouchna/git-scope/internal/stats"
)

// State represents the current UI state
type State int

const (
	StateLoading State = iota
	StateReady
	StateError
	StateSearching
	StateWorkspaceSwitch
	StateGitAction
	StateOpenRepo
	StateShortcuts
	StateCommandPalette
)

type GitActionType int

const (
	GitActionNone GitActionType = iota
	GitActionPullRebase
	GitActionSwitch
	GitActionCreateBranch
	GitActionMergeNoFF
)

// SortMode represents different sorting options
type SortMode int

const (
	SortByDirty SortMode = iota
	SortByName
	SortByBranch
	SortByLastCommit
)

// FilterMode represents different filter options
type FilterMode int

const (
	FilterAll FilterMode = iota
	FilterDirty
	FilterClean
)

// Model is the Bubbletea model for the TUI
type Model struct {
	cfg           *config.Config
	table         table.Model
	textInput     textinput.Model
	spinner       spinner.Model
	repos         []model.Repo
	filteredRepos []model.Repo // After filter applied
	sortedRepos   []model.Repo // After sort applied
	state         State
	err           error
	statusMsg     string
	width         int
	height        int
	sortMode      SortMode
	filterMode    FilterMode
	searchQuery   string
	// Panel state
	activePanel  PanelType
	diskData     *stats.DiskUsageData
	timelineData *stats.TimelineData
	// Workspace switch state
	workspaceInput  textinput.Model
	workspaceError  string
	activeWorkspace string
	// Command palette state
	commandInput  textinput.Model
	commandCursor int
	// Shortcuts overlay state
	shortcutsCursor int
	shortcutsOffset int
	// Git action modal state
	gitActionInput         textinput.Model
	gitActionType          GitActionType
	gitActionCursor        int
	gitActionError         string
	gitActionRunning       bool
	gitActionLoadingBranch bool
	gitActionBranchOptions []string
	gitActionBranchMatches []string
	gitActionBranchIndex   int
	gitActionQueue         []model.Repo
	gitActionExecArgs      []string
	gitActionScopeName     string
	gitActionProgressIdx   int
	gitActionProgressTotal int
	gitActionCurrentRepo   string
	gitActionSuccess       int
	gitActionFailed        int
	gitActionFirstError    string
	// Open repo modal state
	openRepoName      string
	openRepoPath      string
	openRepoChoice    int
	openRepoHasNeovim bool
	openRepoNeovimBin string
	// Star nudge state
	showStarNudge         bool
	nudgeShownThisSession bool
	// Pagination state
	currentPage       int
	pageSize          int
	selectedRepoPaths map[string]bool
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config) Model {
	columns := []table.Column{
		{Title: "Sel", Width: 3},
		{Title: "Status", Width: 8},
		{Title: "Repository", Width: 18},
		{Title: "Branch", Width: 14},
		{Title: "Staged", Width: 6},
		{Title: "Modified", Width: 8},
		{Title: "Untracked", Width: 9},
		{Title: "Last Commit", Width: 14},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	// Apply modern table styles with strong highlighting
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Padding(0, 1)

	// Strong row highlighting
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#A78BFA")).
		Bold(true)

	s.Cell = s.Cell.
		Padding(0, 1)

	t.SetStyles(s)

	// Create text input for search
	ti := textinput.New()
	ti.Placeholder = "Search repos..."
	ti.CharLimit = 50
	ti.Width = 30

	// Create text input for workspace switch
	wi := textinput.New()
	wi.Placeholder = "~/projects or /path/to/dir"
	wi.CharLimit = 200
	wi.Width = 40

	ai := textinput.New()
	ai.Placeholder = "branch name (e.g. main or feature/my-work)"
	ai.CharLimit = 100
	ai.Width = 40

	ci := textinput.New()
	ci.Placeholder = "Search commands..."
	ci.CharLimit = 80
	ci.Width = 42

	// Create spinner with Braille pattern
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	return Model{
		cfg:               cfg,
		table:             t,
		textInput:         ti,
		workspaceInput:    wi,
		gitActionInput:    ai,
		commandInput:      ci,
		spinner:           sp,
		state:             StateLoading,
		sortMode:          SortByDirty,
		filterMode:        FilterAll,
		currentPage:       0,
		pageSize:          cfg.PageSize,
		selectedRepoPaths: map[string]bool{},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, scanReposCmd(m.cfg, false))
}

// GetSelectedRepo returns the currently selected repo
func (m Model) GetSelectedRepo() *model.Repo {
	if m.state != StateReady || len(m.sortedRepos) == 0 {
		return nil
	}

	// Get the cursor position within the current page
	cursor := m.table.Cursor()
	// Calculate the actual index in sortedRepos
	actualIndex := m.currentPage*m.pageSize + cursor

	if actualIndex >= 0 && actualIndex < len(m.sortedRepos) {
		return &m.sortedRepos[actualIndex]
	}
	return nil
}

// applyFilter filters repos based on current filter mode and search query
func (m *Model) applyFilter() {
	m.filteredRepos = make([]model.Repo, 0, len(m.repos))

	for _, r := range m.repos {
		// Apply filter mode
		switch m.filterMode {
		case FilterDirty:
			if !r.Status.IsDirty {
				continue
			}
		case FilterClean:
			if r.Status.IsDirty {
				continue
			}
		}

		// Apply search query
		if m.searchQuery != "" {
			query := strings.ToLower(m.searchQuery)
			name := strings.ToLower(r.Name)
			branch := strings.ToLower(r.Status.Branch)

			// Only search Name and Branch to avoid matching parent paths
			if !strings.Contains(name, query) &&
				!strings.Contains(branch, query) {
				continue
			}
		}

		m.filteredRepos = append(m.filteredRepos, r)
	}
}

// sortRepos sorts the filtered repos based on current sort mode
func (m *Model) sortRepos() {
	m.sortedRepos = make([]model.Repo, len(m.filteredRepos))
	copy(m.sortedRepos, m.filteredRepos)

	switch m.sortMode {
	case SortByDirty:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			if m.sortedRepos[i].Status.IsDirty != m.sortedRepos[j].Status.IsDirty {
				return m.sortedRepos[i].Status.IsDirty
			}
			return m.sortedRepos[i].Name < m.sortedRepos[j].Name
		})
	case SortByName:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			return m.sortedRepos[i].Name < m.sortedRepos[j].Name
		})
	case SortByBranch:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			return m.sortedRepos[i].Status.Branch < m.sortedRepos[j].Status.Branch
		})
	case SortByLastCommit:
		sort.Slice(m.sortedRepos, func(i, j int) bool {
			return m.sortedRepos[i].Status.LastCommit.After(m.sortedRepos[j].Status.LastCommit)
		})
	}
}

// updateTable refreshes the table with current filtered and sorted repos
func (m *Model) updateTable() {
	m.applyFilter()
	m.sortRepos()
	m.table.SetRows(m.reposToRows(m.getCurrentPageRepos()))
}

// getTotalPages returns the total number of pages
func (m Model) getTotalPages() int {
	if len(m.sortedRepos) == 0 {
		return 1
	}
	return (len(m.sortedRepos) + m.pageSize - 1) / m.pageSize
}

// getCurrentPageRepos returns repos for the current page
func (m Model) getCurrentPageRepos() []model.Repo {
	if len(m.sortedRepos) == 0 {
		return []model.Repo{}
	}

	start := m.currentPage * m.pageSize
	end := start + m.pageSize

	if start >= len(m.sortedRepos) {
		start = 0
		end = m.pageSize
	}
	if end > len(m.sortedRepos) {
		end = len(m.sortedRepos)
	}

	return m.sortedRepos[start:end]
}

// canGoPrev returns true if there's a previous page
func (m Model) canGoPrev() bool {
	return m.currentPage > 0
}

// canGoNext returns true if there's a next page
func (m Model) canGoNext() bool {
	return m.currentPage < m.getTotalPages()-1
}

// resetPage resets pagination to first page
func (m *Model) resetPage() {
	m.currentPage = 0
}

// GetSortModeName returns the display name of current sort mode
func (m Model) GetSortModeName() string {
	switch m.sortMode {
	case SortByDirty:
		return "Dirty First"
	case SortByName:
		return "Name"
	case SortByBranch:
		return "Branch"
	case SortByLastCommit:
		return "Recent"
	}
	return "Unknown"
}

// GetFilterModeName returns the display name of current filter mode
func (m Model) GetFilterModeName() string {
	switch m.filterMode {
	case FilterAll:
		return "All"
	case FilterDirty:
		return "Dirty Only"
	case FilterClean:
		return "Clean Only"
	}
	return "All"
}

// reposToRows converts repos to table rows with status indicators
func (m Model) reposToRows(repos []model.Repo) []table.Row {
	rows := make([]table.Row, 0, len(repos))
	for _, r := range repos {
		lastCommit := "N/A"
		if !r.Status.LastCommit.IsZero() {
			lastCommit = r.Status.LastCommit.Format("Jan 02 15:04")
		}

		// Status indicator with text
		status := "✓ Clean"
		if r.Status.IsDirty {
			status = "● Dirty"
		}

		selected := " "
		if m.selectedRepoPaths[r.Path] {
			selected = "✓"
		}

		rows = append(rows, table.Row{
			selected,
			status,
			truncateString(r.Name, 18),
			truncateString(r.Status.Branch, 14),
			formatNumber(r.Status.Staged),
			formatNumber(r.Status.Unstaged),
			formatNumber(r.Status.Untracked),
			lastCommit,
		})
	}
	return rows
}

// truncateString shortens a string with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// formatNumber formats a number for display
func formatNumber(n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", n)
}

// resizeTable calculates and sets the correct table height based on UI state
func (m *Model) resizeTable() {
	// Keep only the strictly necessary overhead so the table expands to fill
	// most of the viewport height.
	usedHeight := 9 // header/stats/legend/help/app padding/newlines
	if m.state == StateSearching || m.searchQuery != "" {
		usedHeight += 2 // search row + spacing
	}
	if m.statusMsg != "" {
		usedHeight++
	}
	if m.showStarNudge {
		usedHeight++
	}

	h := m.height - usedHeight
	if h < 1 {
		h = 1
	}
	m.table.SetHeight(h)
}

func (m *Model) syncSelectionsWithRepos() {
	known := map[string]bool{}
	for _, repo := range m.repos {
		known[repo.Path] = true
	}
	for path := range m.selectedRepoPaths {
		if !known[path] {
			delete(m.selectedRepoPaths, path)
		}
	}
}

func (m *Model) toggleCurrentRepoSelection() bool {
	repo := m.GetSelectedRepo()
	if repo == nil {
		return false
	}
	if m.selectedRepoPaths[repo.Path] {
		delete(m.selectedRepoPaths, repo.Path)
	} else {
		m.selectedRepoPaths[repo.Path] = true
	}
	return true
}

func (m *Model) toggleSelectAllFiltered() (selected int, deselected bool) {
	if len(m.sortedRepos) == 0 {
		return 0, false
	}

	allSelected := true
	for _, repo := range m.sortedRepos {
		if !m.selectedRepoPaths[repo.Path] {
			allSelected = false
			break
		}
	}

	if allSelected {
		for _, repo := range m.sortedRepos {
			delete(m.selectedRepoPaths, repo.Path)
		}
		return len(m.sortedRepos), true
	}

	for _, repo := range m.sortedRepos {
		m.selectedRepoPaths[repo.Path] = true
	}
	return len(m.sortedRepos), false
}

func (m Model) selectedReposCount() int {
	return len(m.selectedRepoPaths)
}

func (m Model) targetReposForAction() ([]model.Repo, string) {
	if len(m.selectedRepoPaths) > 0 {
		targets := make([]model.Repo, 0, len(m.selectedRepoPaths))
		for _, repo := range m.sortedRepos {
			if m.selectedRepoPaths[repo.Path] {
				targets = append(targets, repo)
			}
		}
		// Include selected repos not currently visible due to filters/search.
		if len(targets) < len(m.selectedRepoPaths) {
			for _, repo := range m.repos {
				if m.selectedRepoPaths[repo.Path] {
					found := false
					for _, t := range targets {
						if t.Path == repo.Path {
							found = true
							break
						}
					}
					if !found {
						targets = append(targets, repo)
					}
				}
			}
		}
		return targets, "selected"
	}

	targets := make([]model.Repo, len(m.sortedRepos))
	copy(targets, m.sortedRepos)
	return targets, "filtered"
}

func (m Model) gitActionNeedsBranch() bool {
	return m.gitActionType == GitActionSwitch || m.gitActionType == GitActionCreateBranch || m.gitActionType == GitActionMergeNoFF
}

func (m Model) currentWorkspacePath() string {
	if m.activeWorkspace != "" {
		return m.activeWorkspace
	}
	if len(m.cfg.Roots) > 0 && m.cfg.Roots[0] != "" {
		return m.cfg.Roots[0]
	}
	wd, err := os.Getwd()
	if err == nil {
		return wd
	}
	return ""
}
