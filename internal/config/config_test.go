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
