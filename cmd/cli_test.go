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
		BundleID:    "com.example.app",
		Name:        "Example App",
		Version:     "1.12.1",
		AppStoreURL: "https://apps.apple.com/app/example",
		SDKs:        map[string][]string{"iOS": {"ExampleSDK"}},
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
	history, ok := database[info.BundleID]
	if !ok {
		t.Fatalf("database does not contain %q", info.BundleID)
	}
	if len(history) != 1 {
		t.Fatalf("expected one analysis record, got %d", len(history))
	}
	stored := history[0]
	if stored.Name != info.Name || stored.AppStoreURL != info.AppStoreURL || stored.Version != info.Version {
		t.Errorf("stored app info does not match: %+v", stored)
	}
	if _, err := time.Parse(time.RFC3339, stored.AnalysisDate); err != nil {
		t.Errorf("analysis date is not RFC 3339: %q", stored.AnalysisDate)
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
	history = database[info.BundleID]
	if len(history) != 2 || history[1].Version != "1.12.2" {
		t.Errorf("expected two versioned analysis records, got: %+v", history)
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
