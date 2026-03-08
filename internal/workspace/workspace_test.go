package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeWorkspacePathRejectsMissingPath(t *testing.T) {
	t.Parallel()

	_, err := NormalizeWorkspacePath("/definitely/missing/path/for/git-scope")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestCompleteDirectoryPathUniqueMatch(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	target := filepath.Join(tmp, "project-alpha")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	input := filepath.Join(tmp, "project-a")
	got := CompleteDirectoryPath(input)
	want := target + "/"
	if got != want {
		t.Fatalf("completion mismatch: got %q want %q", got, want)
	}
}
