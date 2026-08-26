package paths

import (
	"os"
	"path/filepath"
	"strings"
)

var deniedExact = []string{"/"}

var deniedPrefixes = []string{
	"/etc", "/root", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys", "/run", "/var",
}

// Allowed reports whether abs is a safe project/browse directory.
// allowRoot, if set, further restricts paths to that directory tree.
func Allowed(abs, allowRoot string) bool {
	abs = filepath.Clean(abs)
	if !filepath.IsAbs(abs) {
		return false
	}
	for _, d := range deniedExact {
		if abs == d {
			return false
		}
	}
	for _, d := range deniedPrefixes {
		if abs == d || strings.HasPrefix(abs, d+string(os.PathSeparator)) {
			return false
		}
	}
	if allowRoot != "" {
		root := filepath.Clean(allowRoot)
		if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			return false
		}
	}
	return true
}

func NormalizeDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
