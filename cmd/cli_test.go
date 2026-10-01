package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"AppMonitor/models"
)

func TestLoadAppListFromCSVWithHeader(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "apps.csv")
	contents := "bundle_id,name,platform\ncom.example.ios,Example iOS,ios\ncom.example.android,Example Android,android\n"
	if err := os.WriteFile(csvPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	targets, err := LoadAppListFromCSV(csvPath)
	if err != nil {
		t.Fatalf("LoadAppListFromCSV returned an error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0] != (AppTarget{BundleID: "com.example.ios", Name: "Example iOS", Platform: "ios"}) {
		t.Errorf("unexpected first target: %+v", targets[0])
	}
	if targets[1] != (AppTarget{BundleID: "com.example.android", Name: "Example Android", Platform: "android"}) {
		t.Errorf("unexpected second target: %+v", targets[1])
	}
}

func TestLoadAppListFromCSVWithoutHeader(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "apps.csv")
	contents := "com.example.app,Example App\n"
	if err := os.WriteFile(csvPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	targets, err := LoadAppListFromCSV(csvPath)
	if err != nil {
		t.Fatalf("LoadAppListFromCSV returned an error: %v", err)
	}
	if len(targets) != 1 || targets[0].BundleID != "com.example.app" || targets[0].Name != "Example App" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestPushToDatabasePersistsAppInfo(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "app_database.json")
	runner := &CLIRunner{dbPath: dbPath}
	info := models.AppInfo{
		BundleID:         "com.example.app",
		Name:             "Example App",
		Version:          "1.12.1",
		AppStoreURL:      "https://apps.apple.com/app/example",
		MinimumOSVersion: "16.0",
		TrackerScanPath:  "/tmp/example-trackerscan.json",
		SDKs:             map[string][]string{"iOS": {"ExampleSDK"}},
	}

	if err := runner.pushToDatabase(info); err != nil {
		t.Fatalf("pushToDatabase returned an error: %v", err)
	}

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var database models.AnalysisDatabase
	if err := json.Unmarshal(data, &database); err != nil {
		t.Fatalf("database is not valid JSON: %v", err)
	}
	appHistory, ok := database[info.BundleID]
	if !ok {
		t.Fatalf("database does not contain %q", info.BundleID)
	}
	stored, ok := appHistory.Versions[info.Version]
	if !ok {
		t.Fatalf("database does not contain version %s", info.Version)
	}
	if appHistory.Name != info.Name || appHistory.AppStoreURL != info.AppStoreURL ||
		stored.SDKs["iOS"][0] != "ExampleSDK" ||
		stored.MinimumOSVersion != info.MinimumOSVersion ||
		stored.TrackerScanPath != info.TrackerScanPath {
		t.Errorf("stored app analysis does not match: %+v", appHistory)
	}
	if _, err := time.Parse(time.RFC3339, stored.AnalysisDate); err != nil {
		t.Errorf("analysis date is not RFC 3339: %q", stored.AnalysisDate)
	}

	info.SDKs = map[string][]string{"iOS": {"UpdatedSDK"}}
	if err := runner.pushToDatabase(info); err != nil {
		t.Fatalf("same-version pushToDatabase returned an error: %v", err)
	}
	data, err = os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &database); err != nil {
		t.Fatalf("database is not valid JSON: %v", err)
	}
	stored = database[info.BundleID].Versions[info.Version]
	if stored.SDKs["iOS"][0] != "UpdatedSDK" {
		t.Errorf("same-version analysis was not replaced: %+v", stored)
	}

	info.Version = "1.12.2"
	if err := runner.pushToDatabase(info); err != nil {
		t.Fatalf("second pushToDatabase returned an error: %v", err)
	}
	data, err = os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &database); err != nil {
		t.Fatalf("database is not valid JSON: %v", err)
	}
	appHistory = database[info.BundleID]
	if len(appHistory.Versions) != 2 {
		t.Errorf("expected records grouped by two versions, got: %+v", appHistory)
	}

	info.Version = "1.12.0"
	if err := runner.pushToDatabase(info); err != nil {
		t.Fatalf("out-of-order pushToDatabase returned an error: %v", err)
	}
	data, err = os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &database); err != nil {
		t.Fatalf("database is not valid JSON: %v", err)
	}
	appHistory = database[info.BundleID]
	if len(appHistory.Versions) != 3 {
		t.Errorf("expected records grouped by three versions, got: %+v", appHistory)
	}
}

func TestWaitForManualDownloadRequiresInteractiveMode(t *testing.T) {
	runner := &CLIRunner{
		interactive: false,
		scanner:     bufio.NewScanner(strings.NewReader("\n")),
	}

	err := runner.waitForManualDownload("https://example.com/app", "com.example.app", "device")
	if err == nil {
		t.Fatal("expected non-interactive manual download to fail")
	}
	if !strings.Contains(err.Error(), "requires interactive mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForManualDownloadRequiresStoreURL(t *testing.T) {
	runner := &CLIRunner{interactive: true}

	err := runner.waitForManualDownload("", "com.example.app", "device")
	if err == nil {
		t.Fatal("expected missing App Store URL to fail")
	}
	if !strings.Contains(err.Error(), "no App Store URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteClassNamesFileWritesOneNamePerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "classes.txt")
	gotPath, err := writeClassNamesFile(path, []string{"FirstClass", "SecondClass"})
	if err != nil {
		t.Fatalf("writeClassNamesFile returned an error: %v", err)
	}
	if gotPath != path {
		t.Fatalf("writeClassNamesFile path = %q, want %q", gotPath, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "FirstClass\nSecondClass\n" {
		t.Fatalf("unexpected class file contents %q", contents)
	}
}

func TestTrackerOutputPathsUsesSafeBundleID(t *testing.T) {
	classDir := filepath.Join(t.TempDir(), "ios", "classlogs")
	runner := &CLIRunner{iosClassLogPath: classDir}

	jsonPath, classesPath, err := runner.trackerOutputPaths("../../com.example/app")
	if err != nil {
		t.Fatalf("trackerOutputPaths returned an error: %v", err)
	}
	if filepath.Dir(classesPath) != classDir {
		t.Fatalf("class names path escaped class log directory: %q", classesPath)
	}
	if filepath.Dir(jsonPath) != filepath.Join(filepath.Dir(classDir), "trackerscan") {
		t.Fatalf("unexpected JSON path: %q", jsonPath)
	}
	if strings.Contains(filepath.Base(classesPath), "/") || strings.Contains(filepath.Base(classesPath), "..") {
		t.Fatalf("bundle ID was not safely encoded in output filename: %q", classesPath)
	}
}

func TestResolveIOSSSHScriptUsesConfiguredPath(t *testing.T) {
	script := filepath.Join(t.TempDir(), "ssh-wrapper")
	setup := filepath.Join(t.TempDir(), "ssh-setup")
	t.Setenv("APPMONITOR_IOS_SSH_SCRIPT", script)
	t.Setenv("APPMONITOR_IOS_SSH_SETUP_SCRIPT", setup)

	gotScript, gotSetup := resolveIOSSSHScript(filepath.Join(t.TempDir(), "settings.json"))
	if gotScript != script || gotSetup != setup {
		t.Fatalf("resolveIOSSSHScript() = (%q, %q), want (%q, %q)", gotScript, gotSetup, script, setup)
	}
}
