package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSinceArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "24 hours ago"},
		{in: "24h", want: "24 hours ago"},
		{in: "3d", want: "3 days ago"},
		{in: "2w", want: "2 weeks ago"},
		{in: "yesterday", want: "yesterday"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := normalizeSinceArg(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeSinceArg(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildRunConfigStandupDefaultsToAllBranches(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	cfg, branchArg, opts, err := buildRunConfig("standup", nil, cfgPath)
	if err != nil {
		t.Fatalf("buildRunConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("buildRunConfig returned nil config")
	}
	if branchArg != "" {
		t.Fatalf("expected empty branchArg, got %q", branchArg)
	}
	if opts.Since != "24h" {
		t.Fatalf("expected default since=24h, got %q", opts.Since)
	}
	if !opts.AllBranches {
		t.Fatal("expected standup default AllBranches=true")
	}
}

func TestBuildRunConfigStandupCurrentBranchOverride(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yml")
	root := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, _, opts, err := buildRunConfig("standup", []string{"3d", "--current-branch", root}, cfgPath)
	if err != nil {
		t.Fatalf("buildRunConfig returned error: %v", err)
	}
	if opts.Since != "3d" {
		t.Fatalf("expected since=3d, got %q", opts.Since)
	}
	if opts.AllBranches {
		t.Fatal("expected AllBranches=false with --current-branch")
	}
}

func TestBuildRunConfigStandupAuthorFilter(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	_, _, opts, err := buildRunConfig("standup", []string{"3d", "--author", "Jane Doe"}, cfgPath)
	if err != nil {
		t.Fatalf("buildRunConfig returned error: %v", err)
	}
	if opts.Author != "Jane Doe" {
		t.Fatalf("expected author filter to be set, got %q", opts.Author)
	}
}

func TestRepoRecentCommitsAllBranchesFlag(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")

	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "main commit")

	currentBranch := strings.TrimSpace(runGitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	runGit(t, repo, "checkout", "-b", "feature/test-standup")
	writeFile(t, filepath.Join(repo, "feature.txt"), "feature\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "feature branch commit")
	runGit(t, repo, "checkout", currentBranch)

	currentOnly, err := repoRecentCommits(repo, "7 days ago", 20, false, "")
	if err != nil {
		t.Fatalf("repoRecentCommits current branch error: %v", err)
	}
	if containsLine(currentOnly, "feature branch commit") {
		t.Fatalf("did not expect feature commit in current-branch log: %v", currentOnly)
	}

	allBranches, err := repoRecentCommits(repo, "7 days ago", 20, true, "")
	if err != nil {
		t.Fatalf("repoRecentCommits all branches error: %v", err)
	}
	if !containsLine(allBranches, "feature branch commit") {
		t.Fatalf("expected feature commit in all-branches log: %v", allBranches)
	}
}

func TestRepoRecentCommitsAuthorFilter(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Main User")
	runGit(t, repo, "config", "user.email", "main@example.com")

	writeFile(t, filepath.Join(repo, "a.txt"), "a\n")
	runGit(t, repo, "add", ".")
	runGitWithEnv(t, repo, []string{
		"GIT_AUTHOR_NAME=Main User",
		"GIT_AUTHOR_EMAIL=main@example.com",
		"GIT_COMMITTER_NAME=Main User",
		"GIT_COMMITTER_EMAIL=main@example.com",
	}, "commit", "-m", "main user commit")

	writeFile(t, filepath.Join(repo, "b.txt"), "b\n")
	runGit(t, repo, "add", ".")
	runGitWithEnv(t, repo, []string{
		"GIT_AUTHOR_NAME=Other User",
		"GIT_AUTHOR_EMAIL=other@example.com",
		"GIT_COMMITTER_NAME=Other User",
		"GIT_COMMITTER_EMAIL=other@example.com",
	}, "commit", "-m", "other user commit")

	filtered, err := repoRecentCommits(repo, "7 days ago", 20, true, "Main User")
	if err != nil {
		t.Fatalf("repoRecentCommits author filter error: %v", err)
	}
	if !containsLine(filtered, "main user commit") {
		t.Fatalf("expected main user commit in filtered log: %v", filtered)
	}
	if containsLine(filtered, "other user commit") {
		t.Fatalf("did not expect other user commit in filtered log: %v", filtered)
	}
}

func containsLine(lines []string, fragment string) bool {
	for _, l := range lines {
		if strings.Contains(l, fragment) {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

func runGitWithEnv(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

func runGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}
