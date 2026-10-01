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
