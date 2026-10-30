package helpers

import (
	"path/filepath"
	"testing"
)

func TestHasInstallerError(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{output: "Install: Complete", want: false},
		{output: "ERROR: Installation failed", want: true},
		{output: "  ERROR: Installation failed", want: true},
		{output: "status: ERROR: not an installer error line", want: false},
	}

	for _, test := range tests {
		if got := hasInstallerError(test.output); got != test.want {
			t.Errorf("hasInstallerError(%q) = %t, want %t", test.output, got, test.want)
		}
	}
}

func TestNewToolPathsUsesConfiguredLDID(t *testing.T) {
	t.Setenv("APPMONITOR_LDID_PATH", "/custom/bin/ldid")
	paths := NewToolPaths(filepath.Join(t.TempDir(), "tools"))
	if paths.LDID != "/custom/bin/ldid" {
		t.Fatalf("LDID path = %q, want configured override", paths.LDID)
	}
}

func TestIsInstallCoordinatorConflict(t *testing.T) {
	out := `ERROR: Install failed. Got error "APIInternalError" with code 0x00000000: Error Domain=IXErrorDomain Code=46 "A coordinated app install already exists for [dk.digst.AltID/P]"`
	if !isInstallCoordinatorConflict(out) {
		t.Fatal("expected coordinator conflict to be detected")
	}
	if isInstallCoordinatorConflict("ERROR: Install failed. Got error \"MismatchedApplicationIdentifierEntitlement\"") {
		t.Fatal("unrelated install error must not be classified as a coordinator conflict")
	}
}

func TestSummarizeInstallerOutput(t *testing.T) {
	out := "Copying...DONE.\nInstall: VerifyingApplication (40%)ERROR: Install failed. Got error \"X\"\n"
	if got := summarizeInstallerOutput(out); got != `ERROR: Install failed. Got error "X"` {
		t.Errorf("unexpected summary %q", got)
	}
	if got := summarizeInstallerOutput(" plain "); got != "plain" {
		t.Errorf("unexpected summary %q", got)
	}
}
