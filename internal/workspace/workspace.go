package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NormalizeWorkspacePath normalizes a workspace path input from the user.
// It expands ~, converts relative paths to absolute, resolves symlinks,
// and validates that the path exists and is a directory.
func NormalizeWorkspacePath(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	path := input

	// Step 1: Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = filepath.Join(home, path[2:])
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = home
	}

	// Step 2: Convert relative paths to absolute
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("cannot resolve path: %w", err)
		}
		path = absPath
	}

	// Step 3: Check if path exists before resolving symlinks
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path does not exist: %s", input)
		}
		return "", fmt.Errorf("cannot access path: %w", err)
	}

	// Step 4: Validate it's a directory
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", input)
	}

	// Step 5: Resolve symlinks
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If symlink resolution fails, use the original path
		// (might happen with broken symlinks)
		return path, nil
	}

	return resolved, nil
}

// expandTilde expands ~ to home directory without validation
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	return path
}

// CompleteDirectoryPath attempts to complete a partial directory path.
// It returns the completed path if a unique match is found, or the longest
// common prefix if multiple matches exist. Returns the original input if
// no matches are found.
func CompleteDirectoryPath(input string) string {
	if input == "" {
		return input
	}

	path, hadTilde, ok := absoluteCompletionPath(input)
	if !ok {
		return input
	}
	if done := completeExistingDir(path, hadTilde); done != "" {
		return done
	}

	dir := filepath.Dir(path)
	prefix := filepath.Base(path)
	matches, ok := matchingDirectories(dir, prefix)
	if !ok || len(matches) == 0 {
		return input
	}
	if len(matches) == 1 {
		return formatCompletionPath(filepath.Join(dir, matches[0]), hadTilde, true)
	}

	commonPrefix := longestCommonPrefix(matches)
	if len(commonPrefix) <= len(prefix) {
		return input
	}
	return formatCompletionPath(filepath.Join(dir, commonPrefix), hadTilde, false)
}

func absoluteCompletionPath(input string) (path string, hadTilde bool, ok bool) {
	hadTilde = strings.HasPrefix(input, "~")
	path = expandTilde(input)
	if filepath.IsAbs(path) {
		return path, hadTilde, true
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", hadTilde, false
	}
	return absPath, hadTilde, true
}

func completeExistingDir(path string, hadTilde bool) string {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return formatCompletionPath(path, hadTilde, true)
}

func matchingDirectories(dir, prefix string) ([]string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}

	matches := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			matches = append(matches, entry.Name())
		}
	}
	return matches, true
}

func longestCommonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, value := range values[1:] {
		for i := 0; i < len(prefix) && i < len(value); i++ {
			if prefix[i] != value[i] {
				prefix = prefix[:i]
				break
			}
		}
		if len(value) < len(prefix) {
			prefix = value
		}
	}
	return prefix
}

func formatCompletionPath(path string, hadTilde, withTrailingSlash bool) string {
	formatted := path
	if hadTilde {
		home, err := os.UserHomeDir()
		if err == nil && strings.HasPrefix(path, home) {
			formatted = "~" + strings.TrimPrefix(path, home)
		}
	}
	if withTrailingSlash && !strings.HasSuffix(formatted, "/") {
		formatted += "/"
	}
	return formatted
}
