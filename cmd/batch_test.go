package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"AppMonitor/helpers"
	"AppMonitor/ios"
	"AppMonitor/models"
)

func TestParseAppListFormats(t *testing.T) {
	cases := map[string]string{
		"txt":         "# apps\ncom.a.one\ncom.b.two\n",
		"csv":         "bundle_id,name,platform\ncom.a.one,One,ios\ncom.b.two,Two,android\n",
		"json strs":   `["com.a.one","com.b.two"]`,
		"json objs":   `[{"bundleId":"com.a.one","name":"One","platform":"iOS"},{"bundle_id":"com.b.two"}]`,
		"json nested": `{"apps":["com.a.one","com.b.two"]}`,
	}
	for name, content := range cases {
		targets, err := ParseAppList([]byte(content))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(targets) != 2 || targets[0].BundleID != "com.a.one" || targets[1].BundleID != "com.b.two" {
			t.Errorf("%s: unexpected targets %+v", name, targets)
		}
	}
	targets, _ := ParseAppList([]byte(cases["json objs"]))
	if targets[0].Platform != "ios" || targets[0].Name != "One" {
		t.Errorf("object fields not parsed: %+v", targets[0])
	}
	if _, err := ParseAppList([]byte(`{"nope":1}`)); err == nil {
		t.Error("expected error for JSON without apps")
	}
}

func TestNormalizeTargets(t *testing.T) {
	targets := []AppTarget{
		{BundleID: "com.a.one"},
		{BundleID: "ios:com.a.one"},
		{BundleID: "android:com.a.one"},
		{BundleID: "bad id!"},
		{BundleID: "com.b.two", Platform: "windows"},
		{BundleID: "com.c.three", Platform: "android"},
	}
	valid, problems := normalizeTargets(targets, "ios")
	if len(valid) != 3 {
		t.Fatalf("expected 3 valid targets, got %+v", valid)
	}
	if valid[1].Platform != "android" || valid[2].BundleID != "com.c.three" {
		t.Errorf("unexpected targets: %+v", valid)
	}
	if len(problems) != 3 {
		t.Errorf("expected 3 problems (duplicate, invalid, platform), got %v", problems)
	}
}

func TestRetryDelayBacksOffAndCaps(t *testing.T) {
	base := 5 * time.Second
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}
	for i, w := range want {
		if got := retryDelay(base, i+1); got != w {
			t.Errorf("retry %d: got %s want %s", i+1, got, w)
		}
	}
	if got := retryDelay(base, 30); got != maxRetryDelay {
		t.Errorf("delay not capped: %s", got)
	}
	if retryDelay(0, 3) != 0 {
		t.Error("zero base must stay zero")
	}
}

func TestIsPermanentError(t *testing.T) {
	permanent := []error{
		fmt.Errorf("wrapped: %w", helpers.ErrAppleAuthRequired),
		fmt.Errorf("wrapped: %w", helpers.ErrInstallCoordinatorConflict),
		fmt.Errorf("wrapped: %w", &ios.UnsupportedMinimumOSVersionError{}),
	}
	for _, err := range permanent {
		if !isPermanentError(err) {
			t.Errorf("%v should be permanent", err)
		}
	}
	if isPermanentError(errors.New("connection reset")) {
		t.Error("generic errors must be retryable")
	}
}

func TestSummaryTableAndExport(t *testing.T) {
	results := []appResult{
		{BundleID: "com.a.one", Platform: "ios", Status: statusOK, SDKs: 3, Permissions: 2, Attempts: 1, DurationSecs: 12.3, ReportPath: "/r/a.pdf"},
		{BundleID: "com.b.two", Platform: "android", Status: statusFailed, Attempts: 3, Error: "boom"},
		{BundleID: "com.c.three", Platform: "ios", Status: statusSkipped, Note: "not run"},
	}
	var out strings.Builder
	printSummaryTable(&out, results, 90*time.Second)
	for _, want := range []string{"com.a.one", "boom", "skipped", "Total: 3", "✅ 1", "❌ 1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary is missing %q:\n%s", want, out.String())
		}
	}

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sub", "results.json")
	if err := writeResultsFile(jsonPath, results); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(jsonPath)
	if !strings.Contains(string(data), `"sdkCount": 3`) {
		t.Errorf("unexpected JSON: %s", data)
	}
	csvPath := filepath.Join(dir, "results.csv")
	if err := writeResultsFile(csvPath, results); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(csvPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "bundle_id,") {
		t.Errorf("unexpected CSV: %s", data)
	}
	if err := writeResultsFile(filepath.Join(dir, "r.txt"), results); err == nil {
		t.Error("unknown export extension must fail")
	}
}

func TestSelectReportJobs(t *testing.T) {
	database := models.AnalysisDatabase{
		"com.a.one": {Name: "One", BundleID: "com.a.one", Versions: map[string]models.AnalysisRecord{
			"1.0": {AnalysisDate: "2025-01-01T00:00:00Z"},
			"2.0": {AnalysisDate: "2025-06-01T00:00:00Z"},
		}},
		"com.b.two": {Name: "Two", BundleID: "com.b.two", Versions: map[string]models.AnalysisRecord{"1": {}}},
	}
	jobs, missing := selectReportJobs(database, reportOptions{all: true})
	if len(jobs) != 2 || jobs[0].Version != "2.0" || len(missing) != 0 {
		t.Errorf("latest-only selection wrong: %+v %v", jobs, missing)
	}
	jobs, _ = selectReportJobs(database, reportOptions{all: true, allVersions: true})
	if len(jobs) != 3 {
		t.Errorf("expected all versions, got %d", len(jobs))
	}
	jobs, missing = selectReportJobs(database, reportOptions{bundles: stringList{"com.b.two", "com.x"}})
	if len(jobs) != 1 || len(missing) != 1 || missing[0] != "com.x" {
		t.Errorf("explicit selection wrong: %+v %v", jobs, missing)
	}
}

func TestAnalyzeWithRetries(t *testing.T) {
	calls := 0
	var slept []time.Duration
	runner := &CLIRunner{
		sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	// An unsupported platform fails every time without touching a device.
	target := AppTarget{BundleID: "com.a.one", Platform: "plan9"}
	_, attempts, err := runner.analyzeWithRetries(context.Background(), target, batchConfig{Retries: 2, RetryDelay: time.Second})
	calls = attempts
	if err == nil || calls != 3 {
		t.Fatalf("expected 3 attempts and an error, got %d, %v", calls, err)
	}
	if len(slept) != 2 || slept[0] != time.Second || slept[1] != 2*time.Second {
		t.Errorf("unexpected backoff: %v", slept)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, attempts, _ = runner.analyzeWithRetries(ctx, target, batchConfig{Retries: 5, RetryDelay: time.Second})
	if attempts != 1 {
		t.Errorf("cancelled context must not retry, got %d attempts", attempts)
	}
}

func TestRunBatchSkipsRemainingAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &CLIRunner{}
	results := runner.RunBatch(ctx, []AppTarget{{BundleID: "com.a.one", Platform: "ios"}, {BundleID: "com.b.two", Platform: "ios"}}, batchConfig{})
	if len(results) != 2 || results[0].Status != statusSkipped || results[1].Status != statusSkipped {
		t.Errorf("expected both skipped: %+v", results)
	}
}

func TestPreflightAndPlan(t *testing.T) {
	runner := &CLIRunner{}
	issues := runner.preflight([]AppTarget{{BundleID: "a.b", Platform: "android"}, {BundleID: "c.d", Platform: "ios"}})
	if !hasBlocker(issues) {
		t.Errorf("missing Exodus key and device should block: %+v", issues)
	}
	var out strings.Builder
	printPlan(&out, []AppTarget{{BundleID: "c.d", Platform: "ios"}, {BundleID: "a.b", Platform: "android"}},
		map[string]bool{"c.d": true}, analyzeOptions{retries: 2, retryDelay: time.Second})
	for _, want := range []string{"analyze installed app", "analyze via Exodus", "dry run"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("plan missing %q:\n%s", want, out.String())
		}
	}
}

func TestParseInterleavedFlags(t *testing.T) {
	var o analyzeOptions
	fs := newAnalyzeFlagSet(&o)
	positional, err := parseInterleaved(fs, []string{"com.a.one", "-p", "android", "android:com.b.two", "-a", "-retries", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if len(positional) != 2 || o.platform != "android" || !o.auto || o.retries != 5 {
		t.Errorf("unexpected parse: %v %+v", positional, o)
	}
}

func TestLoadUninstallListFromStdin(t *testing.T) {
	targets, err := loadUninstallList("-", strings.NewReader("bundle\ncom.a.app,Name\ncom.b.app\n"))
	if err != nil || len(targets) == 0 {
		t.Fatalf("targets=%v err=%v", targets, err)
	}
}

func TestPrintUninstallSummaryExit(t *testing.T) {
	if got := printUninstallSummary([]uninstallOutcome{{BundleID: "a", Status: "removed"}}); got != exitOK {
		t.Errorf("exit %d", got)
	}
	if got := printUninstallSummary([]uninstallOutcome{{BundleID: "a", Status: "failed", Err: errors.New("x")}}); got != exitFailed {
		t.Errorf("exit %d", got)
	}
}

func TestSkipSinceTime(t *testing.T) {
	now := time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC)
	if got := skipSinceTime(now, false, 0); !got.IsZero() {
		t.Errorf("no flags: %v", got)
	}
	if got := skipSinceTime(now, true, 0); !got.Equal(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("today: %v", got)
	}
	if got := skipSinceTime(now, false, 24*time.Hour); !got.Equal(now.Add(-24 * time.Hour)) {
		t.Errorf("within: %v", got)
	}
}

func TestRunBatchSkipsAlreadyAnalyzed(t *testing.T) {
	r := &CLIRunner{}
	now := time.Now()
	cfg := batchConfig{
		SkipSince: now.Add(-time.Hour),
		Done:      map[string]time.Time{"com.a.app": now.Add(-time.Minute)},
	}
	results := r.RunBatch(context.Background(), []AppTarget{{BundleID: "com.a.app", Platform: "ios"}}, cfg)
	if len(results) != 1 || results[0].Status != statusDone {
		t.Fatalf("results = %+v", results)
	}
}
