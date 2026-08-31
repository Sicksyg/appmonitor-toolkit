package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"AppMonitor/assets"
)

const fridaVersion = "v1" // bump when files in assets/frida change

// EnsureFridaProject extracts embedded assets/frida to the user's config dir and returns the absolute project root path.
// Result path: <UserConfigDir>/<appName>/frida
func EnsureFridaProject(appName string) (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}

	targetRoot := filepath.Join(cfgDir, appName, "frida")
	versionFile := filepath.Join(targetRoot, ".frida-version")

	// Fast path: already extracted at current version
	if b, err := os.ReadFile(versionFile); err == nil && strings.TrimSpace(string(b)) == fridaVersion {
		return targetRoot, nil
	}

	// Recreate target to avoid stale files
	if err := os.RemoveAll(targetRoot); err != nil {
		return "", fmt.Errorf("clear frida dir: %w", err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return "", fmt.Errorf("create frida dir: %w", err)
	}

	// Walk embedded "frida" tree and copy to disk preserving structure
	err = fs.WalkDir(assets.FridaFS, "frida", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel("frida", path)
		if err != nil {
			return fmt.Errorf("rel path for %s: %w", path, err)
		}
		if rel == "." {
			return nil
		}

		dst := filepath.Join(targetRoot, rel)

		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		data, err := assets.FridaFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded file %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", dst, err)
		}

		mode := os.FileMode(0o644)
		if strings.HasSuffix(d.Name(), ".sh") {
			mode = 0o755
		}

		if err := os.WriteFile(dst, data, mode); err != nil {
			return fmt.Errorf("write file %s: %w", dst, err)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("extract frida project: %w", err)
	}

	if err := os.WriteFile(versionFile, []byte(fridaVersion), 0o644); err != nil {
		return "", fmt.Errorf("write frida version file: %w", err)
	}

	return targetRoot, nil
}
