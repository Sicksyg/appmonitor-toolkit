package ios

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"AppMonitor/helpers"
)

func TestMinimumOSVersionPolicy(t *testing.T) {
	tests := []struct {
		version     string
		needsPatch  bool
		unsupported bool
		wantErr     bool
	}{
		{version: "15.7"},
		{version: "16.0", needsPatch: true},
		{version: "17.5.1", needsPatch: true},
		{version: "18.9", needsPatch: true},
		{version: "19.0", unsupported: true},
		{version: "eighteen", wantErr: true},
	}
	for _, test := range tests {
		needsPatch, unsupported, err := MinimumOSVersionPolicy(test.version)
		if (err != nil) != test.wantErr {
			t.Errorf("MinimumOSVersionPolicy(%q) error = %v, wantErr %v", test.version, err, test.wantErr)
			continue
		}
		if needsPatch != test.needsPatch || unsupported != test.unsupported {
			t.Errorf("MinimumOSVersionPolicy(%q) = (%v, %v), want (%v, %v)",
				test.version, needsPatch, unsupported, test.needsPatch, test.unsupported)
		}
	}
}

func TestMinimumOSVersionFromStoreResponse(t *testing.T) {
	version, err := MinimumOSVersionFromStoreResponse(`{"results":[{"minimumOsVersion":"17.0"}]}`)
	if err != nil || version != "17.0" {
		t.Fatalf("MinimumOSVersionFromStoreResponse() = (%q, %v), want (17.0, nil)", version, err)
	}
}

func TestDecodeTrackerScanDump(t *testing.T) {
	input := `{
		"bundleID":"com.example.app",
		"version":"1.2",
		"classes":[{"name":"ExampleTracker","source":"static"}],
		"frameworkNames":["ExampleKit"],
		"plistTokens":["NSUserTrackingUsageDescription"],
		"trackingDomains":["track.example.com"],
		"permissions":{"NSCameraUsageDescription":"Camera"},
		"bundleInfo":{"shortVersion":"1.2"},
		"privacyManifests":1,
		"privacyTracking":true
	}`
	dump, err := DecodeTrackerScanDump([]byte(input))
	if err != nil {
		t.Fatalf("DecodeTrackerScanDump() error = %v", err)
	}
	if dump.BundleID != "com.example.app" || len(dump.Classes) != 1 ||
		dump.Classes[0].Name != "ExampleTracker" ||
		dump.Permissions["NSCameraUsageDescription"] != "Camera" ||
		!dump.PrivacyTracking {
		t.Fatalf("unexpected decoded dump: %+v", dump)
	}
}

func TestDecodeTrackerScanDumpRejectsInvalidOrMissingFields(t *testing.T) {
	for _, input := range []string{
		`not json`,
		`{"bundleID":"com.example.app"}`,
		`{"classes":[{"name":"Example"}]}`,
	} {
		if _, err := DecodeTrackerScanDump([]byte(input)); err == nil {
			t.Errorf("DecodeTrackerScanDump(%q) unexpectedly succeeded", input)
		}
	}
}

func TestDecodeTrackerScanDumpAcceptsRuntimeErrorWithEvidence(t *testing.T) {
	input := `{"bundleID":"com.example.app","runtimeError":"posix_spawn failed","classes":[{"name":"StaticSDK","source":"static"}],"frameworkNames":[],"plistTokens":[]}`
	dump, err := DecodeTrackerScanDump([]byte(input))
	if err != nil {
		t.Fatalf("DecodeTrackerScanDump() error = %v", err)
	}

	if !strings.Contains(dump.RuntimeError, "posix_spawn") {
		t.Fatalf("runtime error was not retained: %q", dump.RuntimeError)
	}
}

func TestTrackerScanRemoteCommandAddsRootlessPaths(t *testing.T) {
	command := trackerScanRemoteCommand("trackerscan", "com.example.app")
	for _, expected := range []string{
		`/var/jb/usr/local/bin`,
		`/var/jb/usr/bin`,
		`exec 'trackerscan' --dump 'com.example.app'`,
	} {
		if !strings.Contains(command, expected) {
			t.Errorf("trackerScanRemoteCommand() = %q, missing %q", command, expected)
		}
	}
}

func TestTrackerScanListRemoteCommand(t *testing.T) {
	command := trackerScanListRemoteCommand("trackerscan")
	for _, expected := range []string{
		`/var/jb/usr/local/bin`,
		`/var/jb/usr/bin`,
		`exec 'trackerscan' --list --json`,
	} {
		if !strings.Contains(command, expected) {
			t.Errorf("trackerScanListRemoteCommand() = %q, missing %q", command, expected)
		}
	}
	if strings.Contains(command, "--dump") {
		t.Errorf("trackerScanListRemoteCommand() unexpectedly requests a dump: %q", command)
	}
}

func TestOpenURLUsesSSHAndQuotesURL(t *testing.T) {
	capturePath := filepath.Join(t.TempDir(), "remote-command.txt")
	sshScript := filepath.Join(t.TempDir(), "fake-ios-ssh.sh")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > '" + capturePath + "'\n"
	if err := os.WriteFile(sshScript, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	close(ready)
	manager := NewManager(func(string, string) {}, Paths{SSHScriptPath: sshScript})
	manager.sshReady = ready
	manager.sshConnected = true

	targetURL := "https://apps.example.test/app?id=1&name=two"
	if err := manager.OpenURL(context.Background(), targetURL); err != nil {
		t.Fatalf("OpenURL() error = %v", err)
	}
	command, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(command), "exec uiopen 'https://apps.example.test/app?id=1&name=two'") {
		t.Fatalf("SSH command did not safely quote URL: %q", command)
	}
}

func TestDecodeTrackerScanAppList(t *testing.T) {
	iconData := testPNG(t)
	input, err := json.Marshal(map[string]interface{}{
		"apps": []map[string]string{
			{
				"bundleID":   "com.example.icon",
				"name":       "Example",
				"version":    "1.2",
				"iconData":   base64.StdEncoding.EncodeToString(iconData),
				"iconFormat": "png",
			},
			{
				"bundleID": "com.example.noicon",
				"name":     "No Icon",
				"version":  "2.0",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	apps, err := DecodeTrackerScanAppList(input)
	if err != nil {
		t.Fatalf("DecodeTrackerScanAppList() error = %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}
	if apps[0].CFBundleIdentifier != "com.example.icon" ||
		apps[0].CFBundleDisplayName != "Example" ||
		apps[0].Version != "1.2" ||
		!strings.HasPrefix(apps[0].IconDataURI, "data:image/png;base64,") {
		t.Fatalf("unexpected decoded app: %+v", apps[0])
	}
	if apps[1].IconDataURI != "" {
		t.Fatalf("app without icon unexpectedly has icon data: %q", apps[1].IconDataURI)
	}
}

func TestDecodeTrackerScanAppListRejectsMalformedData(t *testing.T) {
	for _, input := range []string{
		`not json`,
		`{}`,
		`{"apps":[{"bundleID":"com.example.app","name":"Example","iconData":"not-base64","iconFormat":"png"}]}`,
		`{"apps":[{"bundleID":"com.example.app","name":"Example","iconData":"` +
			base64.StdEncoding.EncodeToString([]byte("not png")) + `","iconFormat":"png"}]}`,
	} {
		if _, err := DecodeTrackerScanAppList([]byte(input)); err == nil {
			t.Errorf("DecodeTrackerScanAppList(%q) unexpectedly succeeded", input)
		}
	}
}

func TestListInstalledAppsCachesDecodedIcon(t *testing.T) {
	iconData := testPNG(t)
	response, err := json.Marshal(map[string]interface{}{
		"apps": []map[string]string{{
			"bundleID":   "com.example.cached",
			"name":       "Cached",
			"version":    "1.0",
			"iconData":   base64.StdEncoding.EncodeToString(iconData),
			"iconFormat": "png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sshScript := filepath.Join(t.TempDir(), "fake-ios-ssh.sh")
	script := "#!/bin/sh\nprintf '%s' '" + string(response) + "'\n"
	if err := os.WriteFile(sshScript, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	cachePath := filepath.Join(t.TempDir(), "icons")
	ready := make(chan struct{})
	close(ready)
	manager := NewManager(func(string, string) {}, Paths{
		SSHScriptPath:      sshScript,
		TrackerScanCommand: "am_scanner",
		AppIconCachePath:   cachePath,
	})
	manager.sshReady = ready
	manager.sshConnected = true

	apps, err := manager.ListInstalledApps(context.Background())
	if err != nil {
		t.Fatalf("ListInstalledApps() error = %v", err)
	}
	if len(apps) != 1 || apps[0].IconPath == "" {
		t.Fatalf("ListInstalledApps() returned unexpected apps: %+v", apps)
	}
	cachedIcon, err := os.ReadFile(apps[0].IconPath)
	if err != nil {
		t.Fatalf("read cached icon: %v", err)
	}
	if !bytes.Equal(cachedIcon, iconData) {
		t.Fatal("cached icon bytes differ from trackerscan icon")
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRunAppAnalysisOnInstalledAppSkipsInstallAndFrida(t *testing.T) {
	var stages []string
	manager := &Manager{
		helpers: &helpers.Manager{},
		logger:  func(string, string) {},
	}
	_, err := manager.RunAppAnalysis(context.Background(), AnalysisOptions{
		BundleID:            "com.example.app",
		AnalyzeInstalledApp: true,
		Progress: func(stage, _ string, _ int) {
			stages = append(stages, stage)
		},
	})
	if err != nil {
		t.Fatalf("RunAppAnalysis() error = %v", err)
	}
	for _, stage := range stages {
		if stage == "download" || stage == "install" || stage == "frida" {
			t.Errorf("installed-app analysis unexpectedly emitted %q stage", stage)
		}
	}
}

func TestRunAppAnalysisReturnsTrackerClassNames(t *testing.T) {
	response := `{"bundleID":"com.example.app","version":"2.4","classes":[{"name":"FirstClass","source":"memory"},{"name":"SecondClass","source":"memory"}],"frameworkNames":[],"plistTokens":[],"trackingDomains":[]}`
	sshScript := filepath.Join(t.TempDir(), "fake-ios-ssh.sh")
	script := "#!/bin/sh\nprintf '%s' '" + response + "'\n"
	if err := os.WriteFile(sshScript, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	close(ready)
	manager := NewManager(func(string, string) {}, Paths{
		Helpers:       &helpers.Manager{},
		SSHScriptPath: sshScript,
	})
	manager.sshReady = ready
	manager.sshConnected = true
	scanPath := filepath.Join(t.TempDir(), "scan.json")

	result, err := manager.RunAppAnalysis(context.Background(), AnalysisOptions{
		BundleID:            "com.example.app",
		AnalyzeInstalledApp: true,
		TrackerScanPath:     scanPath,
	})
	if err != nil {
		t.Fatalf("RunAppAnalysis() error = %v", err)
	}
	if result.Version != "2.4" || len(result.Classes) != 2 ||
		result.Classes[0] != "FirstClass" || result.Classes[1] != "SecondClass" {
		t.Fatalf("unexpected trackerscan results: %+v", result)
	}
	if result.TrackerWarning != nil {
		t.Fatalf("unexpected trackerscan warning: %v", result.TrackerWarning)
	}
	if _, err := os.Stat(scanPath); err != nil {
		t.Fatalf("trackerscan JSON output was not saved: %v", err)
	}
}
