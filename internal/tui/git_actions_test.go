package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGitActionWorkerCountBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		repos int
		want  int
	}{
		{repos: 0, want: 0},
		{repos: 1, want: 1},
		{repos: 3, want: 3},
		{repos: 1000, want: 16},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("repos-%d", tt.repos), func(t *testing.T) {
			got := gitActionWorkerCount(tt.repos)
			if got != tt.want {
				t.Fatalf("gitActionWorkerCount(%d) = %d, want %d", tt.repos, got, tt.want)
			}
		})
	}
}

func TestGitActionRepoTimeout(t *testing.T) {
	t.Parallel()

	if got := gitActionRepoTimeout([]string{"pull", "--rebase"}); got != 60*time.Second {
		t.Fatalf("pull timeout = %s, want %s", got, 60*time.Second)
	}
	if got := gitActionRepoTimeout([]string{"switch", "main"}); got != 45*time.Second {
		t.Fatalf("switch timeout = %s, want %s", got, 45*time.Second)
	}
}

func TestNewGitActionCommandSetsNonInteractiveEnv(t *testing.T) {
	t.Parallel()

	cmd := newGitActionCommand(context.Background(), ".", []string{"status"})
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("command env missing GIT_TERMINAL_PROMPT=0")
	}
	if strings.Contains(joined, "GIT_SSH_COMMAND=") {
		t.Fatalf("command env should not override GIT_SSH_COMMAND")
	}
}

func TestGitActionCommandArgsForPull(t *testing.T) {
	t.Parallel()

	args := gitActionCommandArgs([]string{"pull", "--rebase"})
	if len(args) != 2 || args[0] != "pull" || args[1] != "--rebase" {
		t.Fatalf("unexpected pull args: %#v", args)
	}
}
