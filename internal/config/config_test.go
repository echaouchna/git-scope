package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileCreatesAndLoadsConfig(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "git-scope", "config.yml")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config")
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config file to be created, stat err=%v", err)
	}

	if len(cfg.Roots) == 0 {
		t.Fatal("expected loaded config to include roots")
	}

	if cfg.Editor == "" {
		t.Fatal("expected loaded config to include an editor")
	}
}

func TestSavePersistsBookmarks(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "git-scope", "config.yml")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	cfg.Bookmarks = []string{"~/code/repo-a", filepath.Join(tmp, "repo-b")}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	reloaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}
	if len(reloaded.Bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(reloaded.Bookmarks))
	}
	if !filepath.IsAbs(reloaded.Bookmarks[0]) {
		t.Fatalf("expected expanded bookmark path, got %q", reloaded.Bookmarks[0])
	}
	if reloaded.ConfigPath != cfgPath {
		t.Fatalf("expected ConfigPath %q, got %q", cfgPath, reloaded.ConfigPath)
	}
}
