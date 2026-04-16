package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func noopLogger(string, string) {}

func withTempWorkingDir(t *testing.T, fn func()) {
	t.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	fn()
}

func writePermissionsFixture(t *testing.T, signatures map[string]ApplePermissionSignature) {
	t.Helper()

	if err := os.MkdirAll("tmp", 0o755); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}

	payload := map[string]map[string]ApplePermissionSignature{
		"permissions": signatures,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}

	if err := os.WriteFile(filepath.Join("tmp", "ios_permissions.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
}

func TestParsePermissionsResults(t *testing.T) {
	m := NewManager(noopLogger)

	valid := `{"payload":{"NSCameraUsageDescription":"Need camera","NSLocationWhenInUseUsageDescription":"Need location"}}`
	got := m.parsePermissionsResults(valid)

	if len(got) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(got))
	}
	if got["NSCameraUsageDescription"] != "Need camera" {
		t.Fatalf("unexpected camera description: %q", got["NSCameraUsageDescription"])
	}
	if got["NSLocationWhenInUseUsageDescription"] != "Need location" {
		t.Fatalf("unexpected location description: %q", got["NSLocationWhenInUseUsageDescription"])
	}

	if res := m.parsePermissionsResults("not-json"); res != nil {
		t.Fatalf("expected nil for invalid JSON, got %#v", res)
	}

	wrongPayloadType := `{"payload":["not-a-map"]}`
	if res := m.parsePermissionsResults(wrongPayloadType); res != nil {
		t.Fatalf("expected nil for wrong payload type, got %#v", res)
	}
}

func TestParseStaticResults(t *testing.T) {
	m := NewManager(noopLogger)

	valid := `{"payload":["ClassA","ClassB"]}`
	got := m.parseStaticResults(valid)

	if len(got) != 2 {
		t.Fatalf("expected 2 class names, got %d", len(got))
	}
	if got[0] != "ClassA" || got[1] != "ClassB" {
		t.Fatalf("unexpected class list: %#v", got)
	}

	if res := m.parseStaticResults("not-json"); res != nil {
		t.Fatalf("expected nil for invalid JSON, got %#v", res)
	}

	wrongPayloadType := `{"payload":{"ClassA":true}}`
	if res := m.parseStaticResults(wrongPayloadType); res != nil {
		t.Fatalf("expected nil for wrong payload type, got %#v", res)
	}
}

func TestAnalyseDetectPermissions_EnrichesAndFallsBack(t *testing.T) {
	withTempWorkingDir(t, func() {
		writePermissionsFixture(t, map[string]ApplePermissionSignature{
			"camera": {
				PlistKey:       "NSCameraUsageDescription",
				CommonName:     "Camera",
				IosDescription: "Required to use the camera",
				Category:       "Privacy",
			},
		})

		m := NewManager(noopLogger)
		appPermissions := map[string]string{
			"NSCameraUsageDescription":     "Needed for profile photo",
			"NSMicrophoneUsageDescription": "Needed for voice notes",
		}

		enriched, err := m.AnalyseDetectPermissions(appPermissions)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		camera := enriched["NSCameraUsageDescription"]
		if camera.CommonName != "Camera" {
			t.Fatalf("expected Camera common name, got %q", camera.CommonName)
		}
		if camera.AppleDescription != "Required to use the camera" {
			t.Fatalf("unexpected camera apple description: %q", camera.AppleDescription)
		}
		if camera.Category != "Privacy" {
			t.Fatalf("unexpected camera category: %q", camera.Category)
		}
		if camera.PlistKey != "NSCameraUsageDescription" {
			t.Fatalf("unexpected camera plist key: %q", camera.PlistKey)
		}
		if camera.DeveloperDescription != "Needed for profile photo" {
			t.Fatalf("unexpected camera developer description: %q", camera.DeveloperDescription)
		}

		microphone := enriched["NSMicrophoneUsageDescription"]
		if microphone.CommonName != "NSMicrophoneUsageDescription" {
			t.Fatalf("expected fallback common name, got %q", microphone.CommonName)
		}
		if microphone.AppleDescription != "Unknown permission" {
			t.Fatalf("expected fallback apple description, got %q", microphone.AppleDescription)
		}
		if microphone.DeveloperDescription != "Needed for voice notes" {
			t.Fatalf("unexpected microphone developer description: %q", microphone.DeveloperDescription)
		}
	})
}
