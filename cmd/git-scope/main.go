package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/echaouchna/git-scope/internal/app"
	"github.com/echaouchna/git-scope/internal/browser"
	"github.com/echaouchna/git-scope/internal/config"
	"github.com/echaouchna/git-scope/internal/model"
	"github.com/echaouchna/git-scope/internal/scan"
	"github.com/echaouchna/git-scope/internal/tui"
)

type options struct {
	ConfigPath  string
	ShowVersion bool
	ShowHelp    bool
}

type standupOptions struct {
	Since       string
	AllBranches bool
}

func usage() {
	fmt.Fprintf(os.Stderr, `git-scope v%s — A fast TUI to see the status of all git repositories

Usage:
  git-scope [command] [args...]

Commands:
  (default)   Launch TUI dashboard
  scan        Scan and print repos (JSON)
  scan-all    Full system scan from home directory (with stats)
  standup     Multi-repo standup summary (default period: 24h, all branches)
  pull-rebase Run 'git pull --rebase' in all discovered repos
  switch      Run 'git switch <branch>' in all discovered repos
  create-branch Run 'git switch -c <branch>' in all discovered repos
  merge-no-ff Run 'git merge --no-ff <branch>' in all discovered repos
  init        Create config file interactively
  issue       Open git-scope GitHub issues page in browser
  help        Show this help

Examples:
  git-scope                    # Scan configured dirs or current dir
  git-scope ~/code ~/work      # Scan specific directories
  git-scope scan .             # Scan current directory (JSON)
  git-scope scan-all           # Find ALL repos on your system
  git-scope standup            # Print multi-repo standup summary (24h, all branches)
  git-scope standup 3d         # Last 3 days
  git-scope standup 3d --current-branch # Limit commits to current branch only
  git-scope standup 12h ~/code # Last 12 hours for specific roots
  git-scope pull-rebase        # Pull --rebase across repos
  git-scope switch main        # Switch branch across repos
  git-scope create-branch feat/abc ~/code
  git-scope merge-no-ff release/2026.03
  git-scope init               # Setup config interactively
  git-scope issue              # Open GitHub issues page

Flags:
`, app.Version)
	flag.PrintDefaults()
}

func printVersion() {
	fmt.Printf("git-scope v%s\n", app.Version)
}

func main() {
	flag.Usage = usage

	opts := parseFlags()
	if opts.ShowVersion {
		printVersion()
		return
	}
	if opts.ShowHelp {
		usage()
		return
	}

	cmd, args := parseCommand(flag.Args())
	// Handle help subcommand (e.g. `git-scope help`)
	if cmd == "help" {
		usage()
		return
	}

	if err := run(cmd, args, opts.ConfigPath); err != nil {
		log.Fatal(err)
	}
}

// parseFlags defines and parses all supported CLI flags and returns
// the resolved options. It is responsible only for flag handling and
// does not perform any command execution. So if a user runs
// `git-scope -foo bar`, parseFlags only parses the `-foo` flag, if it
// is supported by git-scope.
func parseFlags() options {
	configPath := flag.String("config", config.DefaultConfigPath(), "Path to config file")

	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "Show version")
	flag.BoolVar(&showVersion, "version", false, "Show version")

	var showHelp bool
	flag.BoolVar(&showHelp, "h", false, "Help")
	flag.BoolVar(&showHelp, "help", false, "Help")

	flag.Parse()

	return options{
		ConfigPath:  *configPath,
		ShowVersion: showVersion,
		ShowHelp:    showHelp,
	}
}

// parseCommand determines the command and command args from positional
// arguments.
func parseCommand(args []string) (cmd string, cmdArgs []string) {
	if len(args) == 0 {
		return "", nil
	}

	switch args[0] {
	case "scan", "tui", "help", "init", "scan-all", "issue", "standup", "pull-rebase", "switch", "create-branch", "merge-no-ff":
		return args[0], args[1:]
	default:
		return "tui", args // assume it's a directory
	}
}

// run executes the requested command using the provided configuration path
// and command args.
func run(cmd string, args []string, configPath string) error {
	if handled, err := runNoConfigCommand(cmd); handled {
		return err
	}

	cfg, branchArg, standupOpts, err := buildRunConfig(cmd, args, configPath)
	if err != nil {
		return err
	}
	return runWithConfig(cmd, cfg, branchArg, standupOpts)
}

func runNoConfigCommand(cmd string) (bool, error) {
	switch cmd {
	case "init":
		runInit()
		return true, nil
	case "issue":
		runIssue()
		return true, nil
	case "scan-all":
		runScanAll()
		return true, nil
	default:
		return false, nil
	}
}

func buildRunConfig(cmd string, args []string, configPath string) (*config.Config, string, standupOptions, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, "", standupOptions{}, fmt.Errorf("failed to load config: %w", err)
	}

	dirs := args
	branchArg := ""
	standupOpts := standupOptions{Since: "24h", AllBranches: true}
	if needsBranchArg(cmd) {
		if len(args) < 1 {
			return nil, "", standupOptions{}, fmt.Errorf("%s requires a branch argument", cmd)
		}
		branchArg = args[0]
		dirs = args[1:]
	} else if cmd == "standup" {
		dirs = nil
		remaining := make([]string, 0, len(args))
		for _, arg := range args {
			if arg == "--all-branches" {
				standupOpts.AllBranches = true
				continue
			}
			if arg == "--current-branch" {
				standupOpts.AllBranches = false
				continue
			}
			remaining = append(remaining, arg)
		}
		if len(remaining) >= 1 {
			standupOpts.Since = remaining[0]
			dirs = remaining[1:]
		}
	}

	if len(dirs) > 0 {
		cfg.Roots = expandDirs(dirs)
	}

	return cfg, branchArg, standupOpts, nil
}

func needsBranchArg(cmd string) bool {
	return cmd == "switch" || cmd == "create-branch" || cmd == "merge-no-ff"
}

func runWithConfig(cmd string, cfg *config.Config, branchArg string, standupOpts standupOptions) error {
	switch cmd {
	case "scan":
		repos, err := scan.ScanRoots(cfg.Roots, cfg.Ignore)
		if err != nil {
			return fmt.Errorf("scan error: %w", err)
		}
		if err := scan.PrintJSON(os.Stdout, repos); err != nil {
			return fmt.Errorf("print error: %w", err)
		}
		return nil
	case "tui", "":
		if err := tui.Run(cfg); err != nil {
			return fmt.Errorf("tui error: %w", err)
		}
		return nil
	case "pull-rebase":
		return runBatchGitAction(cfg, []string{"pull", "--rebase"})
	case "switch":
		return runBatchGitAction(cfg, []string{"switch", branchArg})
	case "create-branch":
		return runBatchGitAction(cfg, []string{"switch", "-c", branchArg})
	case "merge-no-ff":
		return runBatchGitAction(cfg, []string{"merge", "--no-ff", branchArg})
	case "standup":
		return runStandup(cfg, standupOpts)
	default:
		usage()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func runStandup(cfg *config.Config, opts standupOptions) error {
	repos, err := scan.ScanRoots(cfg.Roots, cfg.Ignore)
	if err != nil {
		return fmt.Errorf("scan error: %w", err)
	}
	if len(repos) == 0 {
		fmt.Println("No repositories found.")
		return nil
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Path < repos[j].Path
	})

	sinceArg := normalizeSinceArg(opts.Since)
	scope := "current branch"
	if opts.AllBranches {
		scope = "all branches"
	}
	fmt.Printf("%s\n\n", standupColorize("1;36", fmt.Sprintf("Standup (%s, %s) — %s", opts.Since, scope, time.Now().Format("2006-01-02"))))

	updated := 0
	results := collectStandupRepoResults(repos, sinceArg, 5, opts.AllBranches)
	for _, result := range results {
		if len(result.commits) == 0 && !result.repo.Status.IsDirty && result.logErr == nil {
			continue
		}
		updated++
		printStandupRepo(result.repo, result.commits, result.logErr)
	}

	if updated == 0 {
		fmt.Println("No changes in discovered repositories for the selected period.")
		return nil
	}

	fmt.Printf("%s\n", standupColorize("1;32", fmt.Sprintf("Summary: %d/%d repos have recent activity", updated, len(repos))))
	return nil
}

type standupRepoResult struct {
	repo    model.Repo
	commits []string
	logErr  error
}

func collectStandupRepoResults(repos []model.Repo, since string, limit int, allBranches bool) []standupRepoResult {
	type task struct {
		idx  int
		repo model.Repo
	}

	results := make([]standupRepoResult, len(repos))
	if len(repos) == 0 {
		return results
	}

	workerCount := runtime.NumCPU()
	if workerCount < 4 {
		workerCount = 4
	}
	if workerCount > 24 {
		workerCount = 24
	}
	if workerCount > len(repos) {
		workerCount = len(repos)
	}

	tasks := make(chan task, workerCount)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for t := range tasks {
				commits, err := repoRecentCommits(t.repo.Path, since, limit, allBranches)
				results[t.idx] = standupRepoResult{
					repo:    t.repo,
					commits: commits,
					logErr:  err,
				}
			}
		}()
	}

	for i, repo := range repos {
		tasks <- task{idx: i, repo: repo}
	}
	close(tasks)
	wg.Wait()
	return results
}

func normalizeSinceArg(input string) string {
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" {
		return "24 hours ago"
	}

	unit := value[len(value)-1]
	n, err := strconv.Atoi(value[:len(value)-1])
	if err == nil && n > 0 {
		switch unit {
		case 'h':
			return fmt.Sprintf("%d hours ago", n)
		case 'd':
			return fmt.Sprintf("%d days ago", n)
		case 'w':
			return fmt.Sprintf("%d weeks ago", n)
		}
	}
	return input
}

func repoRecentCommits(repoPath, since string, limit int, allBranches bool) ([]string, error) {
	args := []string{"log", "--since=" + since}
	if allBranches {
		args = append(args, "--all")
	}
	args = append(args, "--pretty=format:%h %s", fmt.Sprintf("-%d", limit))
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	commits := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}

func printStandupRepo(repo model.Repo, commits []string, logErr error) {
	fmt.Printf("%s\n", standupColorize("1;35", fmt.Sprintf("• %s", repo.Name)))
	fmt.Printf("  %s\n", standupColorize("2;37", repo.Path))
	if repo.Status.Branch != "" {
		fmt.Printf("  %s", standupColorize("36", "branch: "+repo.Status.Branch))
		if repo.Status.Ahead > 0 || repo.Status.Behind > 0 {
			fmt.Printf("  %s", standupColorize("34", fmt.Sprintf("[ahead:%d behind:%d]", repo.Status.Ahead, repo.Status.Behind)))
		}
		fmt.Println()
	}
	if repo.Status.IsDirty {
		fmt.Printf("  %s\n", standupColorize("33", fmt.Sprintf("dirty: staged=%d modified=%d untracked=%d", repo.Status.Staged, repo.Status.Unstaged, repo.Status.Untracked)))
	}
	if len(commits) > 0 {
		fmt.Printf("  %s\n", standupColorize("32", "commits:"))
		for _, c := range commits {
			fmt.Printf("    %s %s\n", standupColorize("32", "+"), c)
		}
	}
	if logErr != nil {
		fmt.Printf("  %s\n", standupColorize("31", fmt.Sprintf("warning: failed to read git log: %v", logErr)))
	}
	fmt.Println()
}

func standupColorize(code, value string) string {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func runBatchGitAction(cfg *config.Config, gitArgs []string) error {
	repos, err := scan.ScanRoots(cfg.Roots, cfg.Ignore)
	if err != nil {
		return fmt.Errorf("scan error: %w", err)
	}
	if len(repos) == 0 {
		fmt.Println("No repositories found.")
		return nil
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Path < repos[j].Path
	})

	failures := 0
	for _, repo := range repos {
		fmt.Printf("[%s] git %s\n", repo.Path, strings.Join(gitArgs, " "))
		cmd := exec.Command("git", gitArgs...)
		cmd.Dir = repo.Path
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			fmt.Print(indent(string(out), "  "))
		}
		if err != nil {
			failures++
			fmt.Printf("  ERROR: %v\n", err)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d repository operation(s) failed", failures)
	}
	return nil
}

func indent(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// expandDirs converts relative paths and ~ to absolute paths
func expandDirs(dirs []string) []string {
	result := make([]string, 0, len(dirs))
	for _, d := range dirs {
		switch {
		case d == ".":
			if cwd, err := os.Getwd(); err == nil {
				result = append(result, cwd)
			}
		case strings.HasPrefix(d, "~/"):
			if home, err := os.UserHomeDir(); err == nil {
				result = append(result, filepath.Join(home, d[2:]))
			}
		case filepath.IsAbs(d):
			result = append(result, d)
		default:
			if abs, err := filepath.Abs(d); err == nil {
				result = append(result, abs)
			}
		}
	}
	return result
}

// getSmartDefaults returns directories that likely contain git repos
func getSmartDefaults() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		cwd, _ := os.Getwd()
		return []string{cwd}
	}

	// Common developer directories to check
	candidates := []string{
		filepath.Join(home, "code"),
		filepath.Join(home, "Code"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "dev"),
		filepath.Join(home, "Dev"),
		filepath.Join(home, "work"),
		filepath.Join(home, "Work"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "Repos"),
		filepath.Join(home, "src"),
		filepath.Join(home, "Developer"),
		filepath.Join(home, "Documents", "GitHub"),
		filepath.Join(home, "Desktop", "projects"),
	}

	found := []string{}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			found = append(found, dir)
		}
	}

	// If no common dirs found, use current directory
	if len(found) == 0 {
		cwd, _ := os.Getwd()
		return []string{cwd}
	}

	return found
}

// runInit creates a config file interactively
func runInit() {
	configPath := config.DefaultConfigPath()

	fmt.Println("git-scope init — Setup your configuration")
	fmt.Println()

	// Check if config already exists
	if config.ConfigExists(configPath) {
		fmt.Printf("Config file already exists at: %s\n", configPath)
		fmt.Print("Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	reader := bufio.NewReader(os.Stdin)

	// Get directories
	fmt.Println("Enter directories to scan for git repos (one per line, empty line to finish):")
	fmt.Println()
	fmt.Println("💡 Path hints:")
	fmt.Println("   • Use ~/folder for home-relative paths (e.g., ~/code)")
	fmt.Println("   • Use absolute paths like /Users/you/projects")
	fmt.Println("   • Use . for current directory")
	fmt.Println()
	fmt.Println("Examples: ~/code, ~/projects, ~/work")
	fmt.Println()

	dirs := []string{}
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		dirs = append(dirs, line)
	}

	if len(dirs) == 0 {
		// Suggest detected directories
		detected := getSmartDefaults()
		if len(detected) > 0 {
			fmt.Println("\nNo directories entered. Detected these on your system:")
			for _, d := range detected {
				fmt.Printf("  - %s\n", d)
			}
			fmt.Print("\nUse these? [Y/n]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer == "" || answer == "y" || answer == "yes" {
				dirs = detected
			} else {
				fmt.Println("No directories configured. Run 'git-scope init' again to set up.")
				return
			}
		}
	}

	// Get editor
	fmt.Print("\nEditor command (default: code): ")
	editor, _ := reader.ReadString('\n')
	editor = strings.TrimSpace(editor)
	if editor == "" {
		editor = "code"
	}

	// Create config
	if err := config.CreateConfig(configPath, dirs, editor); err != nil {
		log.Fatalf("Failed to create config: %v", err)
	}

	fmt.Printf("\n✅ Config created successfully!\n")
	fmt.Printf("\n📁 Location: %s\n", configPath)
	fmt.Println("\n📝 Configuration:")
	fmt.Println("   Directories to scan:")
	for _, d := range dirs {
		fmt.Printf("     • %s\n", d)
	}
	fmt.Printf("   Editor: %s\n", editor)
	fmt.Println("\n🚀 Run 'git-scope' to launch the dashboard!")
}

// runIssue opens the git-scope GitHub issues page in the default browser
func runIssue() {
	issuesURL := "https://github.com/echaouchna/git-scope/issues"
	if err := browser.Open(issuesURL); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open browser: %v\n", err)
		fmt.Fprintf(os.Stderr, "You can visit the issues page manually at: %s\n", issuesURL)
		os.Exit(1)
	}
}

// runScanAll performs a full system scan starting from home directory
func runScanAll() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}

	fmt.Println("🔍 Full System Scan — Finding all git repositories...")
	fmt.Printf("📁 Scanning from: %s\n\n", home)
	fmt.Println("⏳ This may take a while depending on your disk size...")
	fmt.Println()

	// Ignore common non-user directories (third-party tools, extensions, system)
	ignorePatterns := []string{
		// Build artifacts
		"node_modules", ".next", "dist", "build", "target", ".output",
		// Package managers
		".npm", ".yarn", ".pnpm", ".bun",
		// Language runtimes
		".cargo", ".rustup", ".go", ".venv", "vendor", ".pyenv", ".rbenv", ".nvm",
		// System/OS
		".Trash", "Library", ".cache", ".local",
		// IDE/Editor extensions (third-party repos)
		".vscode", ".vscode-server", ".gemini", ".cursor", ".zed",
		".atom", ".sublime-text", ".idea",
		// Config directories (often contain extension repos)
		".config", ".docker", ".kube", ".ssh", ".gnupg",
		// Other tools
		".oh-my-zsh", ".tmux", ".vim", ".emacs.d",
		// Cloud/sync
		"Google Drive", "OneDrive", "Dropbox", "iCloud",
	}

	repos, err := scan.ScanRoots([]string{home}, ignorePatterns)
	if err != nil {
		log.Fatalf("scan error: %v", err)
	}

	// Calculate stats
	dirty := 0
	clean := 0
	for _, r := range repos {
		if r.Status.IsDirty {
			dirty++
		} else {
			clean++
		}
	}

	// Display summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("                    📊 SCAN COMPLETE")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Printf("   📦 Total repos found:  %d\n", len(repos))
	fmt.Printf("   ● Dirty (needs work):  %d\n", dirty)
	fmt.Printf("   ✓ Clean:               %d\n", clean)
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println()

	// Show dirty repos
	if dirty > 0 {
		fmt.Println("⚠️  Dirty repos that need attention:")
		for _, r := range repos {
			if r.Status.IsDirty {
				fmt.Printf("   • %s (%s) - %s\n", r.Name, r.Status.Branch, r.Path)
			}
		}
		fmt.Println()
	}

	fmt.Println("💡 To add these directories to your config, run: git-scope init")
	fmt.Println("💡 Or run: git-scope ~/path/to/folder to scan specific folders")
}
