package scan

import "testing"

func TestShouldIgnoreSupportsExplicitSemantics(t *testing.T) {
	t.Parallel()

	rules := buildIgnoreRules([]string{
		"node_modules",
		"glob:*.cache",
		"path:vendor/github.com",
		"path:third_party/*/gen",
		"regex:^tmp-",
		"/^build-[0-9]+$/",
	})

	tests := []struct {
		name    string
		dir     string
		relPath string
		want    bool
	}{
		{name: "default exact", dir: "node_modules", relPath: "apps/api/node_modules", want: true},
		{name: "default exact does not suffix match", dir: "foo-node_modules", relPath: "apps/api/foo-node_modules", want: false},
		{name: "glob name match", dir: "build.cache", relPath: "apps/api/build.cache", want: true},
		{name: "path subtree exact", dir: "github.com", relPath: "vendor/github.com", want: true},
		{name: "path subtree child", dir: "org", relPath: "vendor/github.com/org", want: true},
		{name: "path glob", dir: "gen", relPath: "third_party/foo/gen", want: true},
		{name: "regex prefix", dir: "tmp-work", relPath: "tmp-work", want: true},
		{name: "slash regex", dir: "build-42", relPath: "build-42", want: true},
		{name: "non match", dir: "src", relPath: "apps/api/src", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldIgnore(tc.dir, tc.relPath, rules)
			if got != tc.want {
				t.Fatalf("shouldIgnore(%q,%q)=%v want %v", tc.dir, tc.relPath, got, tc.want)
			}
		})
	}
}

func TestBuildIgnoreRulesSkipsInvalidRegex(t *testing.T) {
	t.Parallel()

	rules := buildIgnoreRules([]string{
		"regex:[",
		"/[/",
		"node_modules",
	})

	if len(rules) != 1 {
		t.Fatalf("expected only valid rules to be kept, got %d", len(rules))
	}
	if rules[0].kind != ignoreRuleExact || rules[0].pattern != "node_modules" {
		t.Fatalf("unexpected preserved rule: %#v", rules[0])
	}
}
