package tui

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGitActionWorkerCountBounds(t *testing.T) {
	t.Parallel()

	expectedLarge := runtime.GOMAXPROCS(0) * 2
	if expectedLarge < 4 {
		expectedLarge = 4
	}
	if expectedLarge > 16 {
		expectedLarge = 16
	}

	tests := []struct {
		repos int
		want  int
	}{
		{repos: 0, want: 0},
		{repos: 1, want: 1},
		{repos: 3, want: 3},
		{repos: 1000, want: expectedLarge},
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

	cmd := newGitActionCommand(".", []string{"status"})
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

func TestHandleGitActionHeartbeatDoesNotCancelBatch(t *testing.T) {
	t.Parallel()

	now := time.Now()
	m := Model{
		gitActionRunning:        true,
		gitActionType:           GitActionPullRebase,
		gitActionProgressTotal:  5,
		gitActionProgressIdx:    1,
		gitActionSuccess:        1,
		gitActionFailed:         0,
		gitActionCurrentRepo:    "application",
		gitActionStartedAt:      now.Add(-2 * time.Minute),
		gitActionLastProgressAt: now.Add(-90 * time.Second),
		gitActionRepoTimeout:    60 * time.Second,
	}

	m.handleGitActionHeartbeat(now)

	if m.gitActionCancelPending {
		t.Fatalf("heartbeat should not cancel batch action")
	}
	for _, line := range m.gitActionLogLines {
		if strings.Contains(line, "watchdog") || strings.Contains(line, "cancelling batch") {
			t.Fatalf("heartbeat should not append watchdog cancellation logs, got %q", line)
		}
	}
	if !strings.Contains(m.statusMsg, "idle:") {
		t.Fatalf("status message should still include idle time, got %q", m.statusMsg)
	}
}

func TestEscDuringGitActionReleasesUIImmediately(t *testing.T) {
	t.Parallel()

	m := Model{
		gitActionRunning:   true,
		gitActionType:      GitActionPullRebase,
		gitActionScopeName: "selected",
	}

	out, _, handled := m.handleGitActionRunningState("esc")
	if !handled {
		t.Fatalf("esc should be handled while action is running")
	}

	updated := out.(Model)
	if updated.gitActionRunning {
		t.Fatalf("git action should stop running immediately after cancel")
	}
	if updated.gitActionCancelPending {
		t.Fatalf("cancel pending should be cleared after immediate cancellation")
	}
	if !strings.Contains(updated.statusMsg, "cancelled") {
		t.Fatalf("expected cancelled status, got %q", updated.statusMsg)
	}
}

func TestGitActionRunnerStartedIgnoredWhenRunNotActive(t *testing.T) {
	t.Parallel()

	cancelled := false
	runner := &gitActionRunner{
		results: make(chan gitActionRepoDoneMsg),
		cancel: func() {
			cancelled = true
		},
		id: 7,
	}

	m := Model{
		gitActionRunning: false,
		gitActionRunID:   7,
	}

	updated, cmd, handled := m.handleGitActionProgressMsgs(gitActionRunnerStartedMsg{runner: runner})
	if !handled {
		t.Fatalf("runner started message should be handled")
	}
	if cmd == nil {
		t.Fatalf("expected drain wait command for inactive runner")
	}
	if !cancelled {
		t.Fatalf("inactive run should cancel runner immediately")
	}
	if updated.gitActionRunner != nil {
		t.Fatalf("inactive run should not attach runner to model")
	}
}
