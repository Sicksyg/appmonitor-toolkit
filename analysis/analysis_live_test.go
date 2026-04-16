//go:build livefrida
// +build livefrida

package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	liveFridaUDIDEnv     = "APPMONITOR_FRIDA_UDID"
	liveFridaBundleIDEnv = "APPMONITOR_BUNDLE_ID"

	defaultUDID     = "32a5b4d0c84ba01c4f35d7eb3c31c283fadedcec"
	defaultBundleID = "dk.dr.tv"
)

func liveLogger(t *testing.T) func(string, string) {
	t.Helper()
	return func(message, function string) {
		t.Logf("%s: %s", function, message)
	}
}

func liveUDID() string {
	udid := os.Getenv(liveFridaUDIDEnv)
	if udid == "" {
		return defaultUDID
	}
	return udid
}

func liveBundleID() string {
	bundleID := os.Getenv(liveFridaBundleIDEnv)
	if bundleID == "" {
		return defaultBundleID
	}
	return bundleID
}

func chdirToRepoRoot(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file location")
	}

	repoRoot := filepath.Dir(filepath.Dir(currentFile))
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read current working directory: %v", err)
	}

	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to switch to repo root %q: %v", repoRoot, err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})
}

func TestLiveFrida_SimpleRoutine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Frida test in short mode")
	}

	chdirToRepoRoot(t)
	udid := liveUDID()
	bundleID := liveBundleID()

	m := NewManager(liveLogger(t))
	t.Logf("running simple Frida routine with udid=%s bundleId=%s", udid, bundleID)

	if err := m.FridaSetup(udid, bundleID); err != nil {
		t.Fatalf("FridaSetup failed for bundle %q: %v", bundleID, err)
	}
	if m.fridaData == nil {
		t.Fatal("expected fridaData to be initialized after FridaSetup")
	}

	if err := m.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if m.fridaData != nil {
		t.Fatal("expected fridaData to be nil after Cleanup")
	}
}
