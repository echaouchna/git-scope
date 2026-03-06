package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/echaouchna/git-scope/internal/config"
	"github.com/echaouchna/git-scope/internal/fswatch"
	"github.com/echaouchna/git-scope/internal/model"
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
	StateActionLogs
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
	// Workspace switch state
	workspaceInput  textinput.Model
	workspaceError  string
	activeWorkspace string
	// Command palette state
	commandInput  textinput.Model
	commandCursor int
	commandOffset int
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
	gitActionLogLines      []string
	gitActionLogOffset     int
	lastActionLogLines     []string
	lastActionSummary      string
	actionLogsReturnState  State
	// Background watcher state
	repoWatcher         *fswatch.RepoWatcher
	watchRefreshRunning bool
	watchRefreshPending bool
	// Open repo modal state
	openRepoName       string
	openRepoPath       string
	openRepoChoice     int
	openRepoOffset     int
	openRepoInput      textinput.Model
	openRepoHasNeovim  bool
	openRepoNeovimBin  string
	openRepoHasGitUI   bool
	openRepoGitUIBin   string
	openRepoHasTig     bool
	openRepoTigBin     string
	openRepoToolsReady bool
	// Star nudge state
	showStarNudge         bool
	nudgeShownThisSession bool
	// Pagination state
	currentPage       int
	pageSize          int
	selectedRepoPaths map[string]bool
}

type tableLayout struct {
	tableWidth      int
	statusWidth     int
	repoWidth       int
	branchWidth     int
	stagedWidth     int
	modifiedWidth   int
	untrackedWidth  int
	lastCommitWidth int
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

	oi := textinput.New()
	oi.Placeholder = "Search open options..."
	oi.CharLimit = 80
	oi.Width = 42

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
		openRepoInput:     oi,
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
	if len(m.sortedRepos) == 0 {
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
	query := strings.ToLower(strings.TrimSpace(m.searchQuery))

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
		if query != "" {
			fields := []searchField{
				{value: strings.ToLower(r.Name), allowFuzzy: true},
				{value: strings.ToLower(r.Status.Branch), allowFuzzy: true},
				{value: strings.ToLower(filepath.ToSlash(r.Path)), allowFuzzy: false},
			}

			// Space-separated terms must all match at least one field.
			if !matchesAllSearchTerms(query, fields) {
				continue
			}
		}

		m.filteredRepos = append(m.filteredRepos, r)
	}
}

type searchField struct {
	value      string
	allowFuzzy bool
}

func matchesAllSearchTerms(query string, fields []searchField) bool {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return false
	}
	// Keep multi-term queries strict to avoid broad matches on long paths.
	allowFuzzy := len(terms) == 1

	for _, term := range terms {
		matched := false
		for _, field := range fields {
			if searchTermMatchesField(term, field, allowFuzzy) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

func searchTermMatchesField(term string, field searchField, allowFuzzy bool) bool {
	candidate := field.value
	if term == "" || candidate == "" {
		return false
	}
	if strings.Contains(candidate, term) {
		return true
	}
	if !allowFuzzy || !field.allowFuzzy {
		return false
	}
	return fuzzySubsequenceMatch(term, candidate)
}

// fuzzySubsequenceMatch checks whether all query chars appear in order.
func fuzzySubsequenceMatch(query, value string) bool {
	q := []rune(query)
	v := []rune(value)
	if len(q) == 0 {
		return true
	}

	idx := 0
	for _, ch := range v {
		if ch == q[idx] {
			idx++
			if idx == len(q) {
				return true
			}
		}
	}

	return false
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
	m.applyTableLayout()
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

		// Status indicator with text.
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
			r.Name,
			r.Status.Branch,
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
	if m.GetSelectedRepo() != nil {
		usedHeight++
	}

	h := m.height - usedHeight
	if h < 1 {
		h = 1
	}
	m.table.SetHeight(h)
	// Keep pagination aligned with visible table rows so the table fills
	// the available vertical space.
	m.pageSize = h
	m.applyTableLayout()
}

func (m *Model) applyTableLayout() {
	layout := m.currentTableLayout()
	m.table.SetWidth(layout.tableWidth)
	m.table.SetColumns([]table.Column{
		{Title: "Sel", Width: 3},
		{Title: "Status", Width: layout.statusWidth},
		{Title: "Repository", Width: layout.repoWidth},
		{Title: "Branch", Width: layout.branchWidth},
		{Title: "Staged", Width: layout.stagedWidth},
		{Title: "Modified", Width: layout.modifiedWidth},
		{Title: "Untracked", Width: layout.untrackedWidth},
		{Title: "Last Commit", Width: layout.lastCommitWidth},
	})
}

func (m Model) currentTableLayout() tableLayout {
	tableWidth := m.width - 4
	if tableWidth < 36 {
		tableWidth = 36
	}

	layout := tableLayout{tableWidth: tableWidth}

	// Table cell style uses horizontal padding of 1 on each side for every column.
	// Reserve that space so computed content widths align with the rendered table.
	const columnPaddingBudget = 16 // 8 columns * 2 chars
	const selWidth = 3
	contentBudget := tableWidth - columnPaddingBudget - selWidth
	if contentBudget < 14 {
		contentBudget = 14
	}

	// Order: status, repo, branch, staged, modified, untracked, lastCommit.
	mins := []int{5, 7, 6, 1, 1, 1, 5}
	weights := []int{1, 4, 3, 1, 1, 1, 2}

	totalMin := 0
	for _, v := range mins {
		totalMin += v
	}

	widths := make([]int, len(mins))
	copy(widths, mins)

	extra := contentBudget - totalMin
	for extra > 0 {
		for i := range widths {
			if extra == 0 {
				break
			}
			widths[i] += weights[i]
			extra -= weights[i]
			if extra < 0 {
				widths[i] += extra
				extra = 0
			}
		}
	}

	layout.statusWidth = widths[0]
	layout.repoWidth = widths[1]
	layout.branchWidth = widths[2]
	layout.stagedWidth = widths[3]
	layout.modifiedWidth = widths[4]
	layout.untrackedWidth = widths[5]
	layout.lastCommitWidth = widths[6]
	return layout
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

	repo := m.GetSelectedRepo()
	if repo == nil {
		return nil, "highlighted"
	}
	return []model.Repo{*repo}, "highlighted"
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
