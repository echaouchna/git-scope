package scan

import "testing"

func TestShouldIgnoreWithExactAndSuffixRules(t *testing.T) {
	t.Parallel()

	ignoreSet := make(map[string]struct{})
	for _, pattern := range []string{
		"node_modules",
		".cache",
	} {
		ignoreSet[pattern] = struct{}{}
	}

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{name: "exact name", dir: "node_modules", want: true},
		{name: "suffix match", dir: "build.cache", want: true},
		{name: "non match", dir: "src", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldIgnore(tc.dir, ignoreSet)
			if got != tc.want {
				t.Fatalf("shouldIgnore(%q)=%v want %v", tc.dir, got, tc.want)
			}
		})
	}
}
