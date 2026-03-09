package tui

import (
	"testing"

	"github.com/echaouchna/git-scope/internal/config"
	"github.com/echaouchna/git-scope/internal/model"
)

func TestBookmarkViewFiltersToBookmarkedRepos(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{PageSize: 10}
	m := NewModel(cfg)
	m.repos = []model.Repo{
		{Name: "git-scope", Path: "/tmp/git-scope", Status: model.RepoStatus{Branch: "main"}},
		{Name: "notes", Path: "/tmp/notes", Status: model.RepoStatus{Branch: "main"}},
		{Name: "service-api", Path: "/tmp/service-api", Status: model.RepoStatus{Branch: "feature/bookmarks"}},
	}
	m.bookmarkedPaths["/tmp/git-scope"] = true
	m.bookmarkedPaths["/tmp/service-api"] = true

	m.enterBookmarksMode()

	if len(m.sortedRepos) != 2 {
		t.Fatalf("expected 2 bookmarked repos, got %d", len(m.sortedRepos))
	}

	m.bookmarkQuery = "gs"
	m.updateTable()
	if len(m.sortedRepos) != 1 {
		t.Fatalf("expected fuzzy bookmark query to match 1 repo, got %d", len(m.sortedRepos))
	}
	if got := m.sortedRepos[0].Name; got != "git-scope" {
		t.Fatalf("expected git-scope match, got %q", got)
	}
}

func TestFilteredCommandItemsUsesFuzzyMatch(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{PageSize: 10}
	m := NewModel(cfg)
	m.commandInput.SetValue("tbr")

	items := m.filteredCommandItems()
	if len(items) == 0 {
		t.Fatal("expected fuzzy command match, got none")
	}

	found := false
	for _, item := range items {
		if item.key == "toggle_bookmark" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected toggle_bookmark in filtered items, got %#v", items)
	}
}
