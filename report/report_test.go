package report

import (
	"os"
	"path/filepath"
	"testing"

	"AppMonitor/models"
)

func TestMakeMarotoReport_WithMockData_WritesPDF(t *testing.T) {
	t.Helper()

	sdkMap := map[string][]string{
		"Firebase Analytics": {
			"FIRAnalytics",
			"FIRInstallations",
		},
		"Adjust": {
			"ADJActivityHandler",
			"ADJSessionParameters",
		},
	}

	permissionMap := map[string]models.PermissionDetail{
		"NSCameraUsageDescription": {
			CommonName:           "Camera",
			AppleDescription:     "Accesses camera hardware for image capture.",
			DeveloperDescription: "Used for profile photo and document scanning.",
			Category:             "Sensitive",
			PlistKey:             "NSCameraUsageDescription",
		},
		"NSLocationWhenInUseUsageDescription": {
			CommonName:           "Location (When In Use)",
			AppleDescription:     "Accesses user location while app is active.",
			DeveloperDescription: "Used to provide nearby content and localization.",
			Category:             "Tracking",
			PlistKey:             "NSLocationWhenInUseUsageDescription",
		},
	}

	outDir := filepath.Join("..", "tmp", "report_test_output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}

	outPath := filepath.Join(outDir, "mock_analysis_report.pdf")
	rm := NewManager(func(_, _ string) {})

	if err := rm.MakeMarotoReport("Mock App", "com.example.mockapp", outPath, sdkMap, permissionMap); err != nil {
		t.Fatalf("MakeMarotoReport failed: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated report: %v", err)
	}

	if len(content) == 0 {
		t.Fatal("generated report is empty")
	}

	if len(content) < 4 || string(content[:4]) != "%PDF" {
		t.Fatalf("generated file is not a PDF: %s", outPath)
	}

	t.Logf("Mock report generated: %s", outPath)
}

func TestMakeMarotoReport_WithEmptyData_WritesPDF(t *testing.T) {
	t.Helper()

	outDir := filepath.Join("..", "tmp", "report_test_output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}

	outPath := filepath.Join(outDir, "mock_empty_report.pdf")
	rm := NewManager(func(_, _ string) {})

	if err := rm.MakeMarotoReport("Mock Empty App", "com.example.empty", outPath, map[string][]string{}, map[string]models.PermissionDetail{}); err != nil {
		t.Fatalf("MakeMarotoReport failed for empty data: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat generated report: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("generated empty-data report has zero bytes")
	}

	t.Logf("Mock empty-data report generated: %s", outPath)
}
