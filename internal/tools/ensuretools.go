package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"AppMonitor/assets"
)

const bundledToolsVersion = "v1" // bump this whenever bundled binaries change

// EnsureBundledTools extracts embedded helper binaries for the current macOS arch into ~/Library/Application Support/AppMonitor/bin and returns that bin dir.
func EnsureBundledTools(appName string) (string, error) {
	// Only support macOS because the bundled helper binaries are platform-specific.
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("EnsureBundledTools only supports darwin, got %s", runtime.GOOS)
	}

	// Only arm64 and amd64 builds are expected to have bundled helpers.
	arch := runtime.GOARCH // arm64 or amd64
	if arch != "arm64" && arch != "amd64" {
		return "", fmt.Errorf("unsupported darwin arch: %s", arch)
	}

	// Resolve the user configuration directory, which maps to Application Support on macOS.
	configDir, err := os.UserConfigDir() // macOS => ~/Library/Application Support
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}

	// Create the destination directory for extracted helper binaries.
	toolsDir := filepath.Join(configDir, appName, "bin")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("create bin dir: %w", err)
	}

	// Skip extraction if current version already installed.
	versionFilePath := filepath.Join(toolsDir, ".tools-version")
	if b, err := os.ReadFile(versionFilePath); err == nil && string(b) == bundledToolsVersion {
		// The installed tools match this version, so reuse them.
		return toolsDir, nil
	}

	// List all bundled binaries for the current architecture.
	binaryEntries, err := assets.ListBundledBinaries(arch)
	if err != nil {
		return "", fmt.Errorf("list embedded binaries for arch %s: %w", arch, err)
	}

	// Extract each embedded binary into the bin directory.
	for _, entry := range binaryEntries {
		if entry.IsDir() {
			// Ignore embedded directories.
			continue
		}

		binaryName := entry.Name()
		// Read the binary bytes from the embedded assets package.
		binaryData, err := assets.ReadBundledBinary(arch, binaryName)
		if err != nil {
			return "", fmt.Errorf("read embedded binary %s: %w", binaryName, err)
		}

		// Write the binary to disk with executable permissions.
		destinationPath := filepath.Join(toolsDir, binaryName)
		if err := os.WriteFile(destinationPath, binaryData, 0o755); err != nil {
			return "", fmt.Errorf("write binary %s: %w", binaryName, err)
		}
		// Ensure the file remains executable after extraction.
		if err := os.Chmod(destinationPath, 0o755); err != nil {
			return "", fmt.Errorf("chmod binary %s: %w", binaryName, err)
		}
	}

	// Persist the version marker for future runs.
	if err := os.WriteFile(versionFilePath, []byte(bundledToolsVersion), 0o644); err != nil {
		return "", fmt.Errorf("write version file: %w", err)
	}

	// Return the directory containing the extracted helper binaries.
	return toolsDir, nil
}

func EnsureBundledLibs(appName string) (string, error) {
	// Only support macOS because the bundled helper binaries are platform-specific.
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("EnsureBundledLibs only supports darwin, got %s", runtime.GOOS)
	}

	// Only arm64 and amd64 builds are expected to have bundled helpers.
	arch := runtime.GOARCH // arm64 or amd64
	if arch != "arm64" && arch != "amd64" {
		return "", fmt.Errorf("unsupported darwin arch: %s", arch)
	}

	// Resolve the user configuration directory, which maps to Application Support on macOS.
	configDir, err := os.UserConfigDir() // macOS => ~/Library/Application Support
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}

	// Create the destination directory for extracted helper binaries.
	libDir := filepath.Join(configDir, appName, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		return "", fmt.Errorf("create lib dir: %w", err)
	}

	// List all bundled libraries for the current architecture.
	libEntries, err := assets.ListBundledLibs(arch)
	if err != nil {
		return "", fmt.Errorf("list embedded libs for arch %s: %w", arch, err)
	}

	// Extract each embedded library into the lib directory.
	for _, entry := range libEntries {
		if entry.IsDir() {
			// Ignore embedded directories.
			continue
		}

		libName := entry.Name()
		// Read the library bytes from the embedded assets package.
		libData, err := assets.ReadBundledLib(arch, libName)
		if err != nil {
			return "", fmt.Errorf("read embedded lib %s: %w", libName, err)
		}

		// Create the architecture-specific subdirectory.
		if err := os.MkdirAll(filepath.Join(libDir, "darwin-"+arch), 0o755); err != nil {
			return "", fmt.Errorf("create arch dir: %w", err)
		}

		// Write the library to disk with readable permissions.
		destinationPath := filepath.Join(libDir, "darwin-"+arch, libName)
		if err := os.WriteFile(destinationPath, libData, 0o644); err != nil {
			return "", fmt.Errorf("write lib %s: %w", libName, err)
		}

		// Ensure the file remains executable after extraction.
		if err := os.Chmod(destinationPath, 0o755); err != nil {
			return "", fmt.Errorf("chmod lib %s: %w", libName, err)
		}
	}

	// Return the directory containing the extracted helper libraries.
	return libDir, nil
}
