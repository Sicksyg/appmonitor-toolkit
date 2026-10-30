package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"AppMonitor/helpers"
	"AppMonitor/ios"
	"AppMonitor/models"
)

const (
	statusOK      = "ok"
	statusFailed  = "failed"
	statusSkipped = "skipped"
	statusDone    = "up-to-date"

	maxRetryDelay = 2 * time.Minute
)

// batchConfig controls automatic retries.
type batchConfig struct {
	Retries    int
	RetryDelay time.Duration
	// SkipSince, when set, skips apps whose latest stored analysis is not older than this.
	SkipSince time.Time
	// Done maps bundle IDs to their latest stored analysis time.
	Done map[string]time.Time
}

// latestAnalysisTimes returns the newest analysis time stored for each bundle ID.
func latestAnalysisTimes(database models.AnalysisDatabase) map[string]time.Time {
	latest := make(map[string]time.Time, len(database))
	for id, history := range database {
		for _, record := range history.Versions {
			parsed, err := time.Parse(time.RFC3339, record.AnalysisDate)
			if err == nil && parsed.After(latest[id]) {
				latest[id] = parsed
			}
		}
	}
	return latest
}

// skipSinceTime converts -skip-done/-skip-within into a cutoff; zero means no skipping.
func skipSinceTime(now time.Time, today bool, within time.Duration) time.Time {
	switch {
	case within > 0:
		return now.Add(-within)
	case today:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	return time.Time{}
}

// appResult is the outcome of one target and the unit of the summary and export.
type appResult struct {
	BundleID     string  `json:"bundleId"`
	Name         string  `json:"name,omitempty"`
	Platform     string  `json:"platform"`
	Status       string  `json:"status"`
	Version      string  `json:"version,omitempty"`
	SDKs         int     `json:"sdkCount"`
	Permissions  int     `json:"permissionCount"`
	Attempts     int     `json:"attempts"`
	DurationSecs float64 `json:"durationSeconds"`
	Error        string  `json:"error,omitempty"`
	ReportPath   string  `json:"reportPath,omitempty"`
	AnalysisDate string  `json:"analysisDate,omitempty"`
	Note         string  `json:"note,omitempty"`
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isPermanentError reports errors that retrying cannot fix.
func isPermanentError(err error) bool {
	var unsupported *ios.UnsupportedMinimumOSVersionError
	return errors.Is(err, helpers.ErrAppleAuthRequired) ||
		errors.Is(err, helpers.ErrInstallCoordinatorConflict) ||
		errors.As(err, &unsupported)
}

// retryDelay doubles for each retry and is capped.
func retryDelay(base time.Duration, retry int) time.Duration {
	if base <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < retry; i++ {
		delay *= 2
		if delay >= maxRetryDelay {
			return maxRetryDelay
		}
	}
	return delay
}

// analyzeWithRetries runs one target, retrying transient failures with backoff.
func (r *CLIRunner) analyzeWithRetries(ctx context.Context, target AppTarget, cfg batchConfig) (analysisOutput, int, error) {
	sleep := r.sleep
	if sleep == nil {
		sleep = sleepContext
	}
	attempts := 0
	for {
		attempts++
		out, err := r.processSingleApp(target, target.Platform)
		if err == nil {
			return out, attempts, nil
		}
		if ctx.Err() != nil || isPermanentError(err) || attempts > cfg.Retries {
			return out, attempts, err
		}
		delay := retryDelay(cfg.RetryDelay, attempts)
		fmt.Printf("\n⚠️  Attempt %d/%d failed: %v\n🔄 Retrying in %s...\n", attempts, cfg.Retries+1, err, delay)
		if sleepErr := sleep(ctx, delay); sleepErr != nil {
			return out, attempts, err
		}
	}
}

// RunBatch processes targets sequentially (one device, one at a time) and always returns a
// result for every target, including those skipped after an interrupt or a quit.
func (r *CLIRunner) RunBatch(ctx context.Context, targets []AppTarget, cfg batchConfig) []appResult {
	results := make([]appResult, 0, len(targets))
	total := len(targets)
	quit := false

	for i, target := range targets {
		result := appResult{BundleID: target.BundleID, Name: target.Name, Platform: target.Platform}
		if quit || ctx.Err() != nil {
			result.Status = statusSkipped
			result.Note = "not run"
			results = append(results, result)
			continue
		}

		if when, ok := cfg.Done[target.BundleID]; ok && !cfg.SkipSince.IsZero() && !when.Before(cfg.SkipSince) {
			result.Status = statusDone
			result.Note = "already analyzed " + when.Local().Format("2006-01-02 15:04")
			fmt.Printf("\n[%d/%d] ⏭  %s: %s\n", i+1, total, target.BundleID, result.Note)
			results = append(results, result)
			continue
		}

		fmt.Println("\n--------------------------------------------------")
		fmt.Printf("[%d/%d] 🎯 %s (%s)\n", i+1, total, target.BundleID, strings.ToUpper(target.Platform))
		if target.Name != "" {
			fmt.Printf("       Name: %s\n", target.Name)
		}
		fmt.Println("--------------------------------------------------")

		started := time.Now()
		totalAttempts := 0
		var (
			out analysisOutput
			err error
		)
		for {
			var attempts int
			out, attempts, err = r.analyzeWithRetries(ctx, target, cfg)
			totalAttempts += attempts
			if err == nil || !r.interactive || ctx.Err() != nil {
				break
			}
			fmt.Printf("\n❌ Analysis failed for %s: %v\n", target.BundleID, err)
			choice := r.promptChoice("Options: [R]etry this app / [S]kip to next / [Q]uit batch: ", []string{"r", "s", "q"}, "s")
			if choice == "r" {
				continue
			}
			quit = choice == "q"
			break
		}

		result.Attempts = totalAttempts
		result.DurationSecs = time.Since(started).Round(100 * time.Millisecond).Seconds()
		result.Name = firstNonEmpty(out.Info.Name, result.Name)
		if err != nil {
			result.Status = statusFailed
			result.Error = strings.TrimSpace(strings.TrimPrefix(err.Error(), "⚠️"))
			fmt.Printf("\n❌ %s failed after %d attempt(s): %s\n", target.BundleID, totalAttempts, result.Error)
		} else {
			result.Status = statusOK
			result.Version = out.Info.Version
			result.SDKs = countSDKs(out.Info.SDKs)
			result.Permissions = len(out.Info.IosPermissions) + len(out.Info.AndroidPermissions)
			result.ReportPath = out.ReportPath
			result.AnalysisDate = out.Info.AnalysisDate
			fmt.Printf("\n✅ %s done in %s: %d SDK(s), %d permission(s)\n", target.BundleID,
				time.Duration(result.DurationSecs*float64(time.Second)), result.SDKs, result.Permissions)
			if out.ReportPath != "" {
				fmt.Printf("   📄 Report: %s\n", out.ReportPath)
			}
			if out.ReportErr != nil {
				result.Note = "report failed: " + out.ReportErr.Error()
				fmt.Printf("   ⚠️  %s\n", result.Note)
			}
		}
		results = append(results, result)
	}
	return results
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func countSDKs(sdks map[string][]string) int {
	total := 0
	for _, names := range sdks {
		total += len(names)
	}
	return total
}

// ------------------------------- summary & export ------------------------------- //

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func printSummaryTable(w io.Writer, results []appResult, elapsed time.Duration) {
	var ok, failed, skipped, upToDate int
	for _, r := range results {
		switch r.Status {
		case statusOK:
			ok++
		case statusFailed:
			failed++
		case statusDone:
			upToDate++
		default:
			skipped++
		}
	}

	fmt.Fprintln(w, "\n==================================================")
	fmt.Fprintln(w, "                 Batch Summary")
	fmt.Fprintln(w, "==================================================")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tBUNDLE ID\tPLATFORM\tSTATUS\tSDKS\tPERMS\tTRIES\tTIME\tNOTE")
	for i, r := range results {
		status := map[string]string{statusOK: "✅ ok", statusFailed: "❌ failed", statusSkipped: "⏭  skipped", statusDone: "⏭  up to date"}[r.Status]
		sdks, perms := "-", "-"
		if r.Status == statusOK {
			sdks, perms = strconv.Itoa(r.SDKs), strconv.Itoa(r.Permissions)
		}
		note := firstNonEmpty(r.Error, r.Note)
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", i+1, truncate(r.BundleID, 40),
			r.Platform, status, sdks, perms, r.Attempts,
			time.Duration(r.DurationSecs*float64(time.Second)).Round(time.Second), truncate(note, 60))
	}
	tw.Flush()
	fmt.Fprintf(w, "\nTotal: %d   ✅ %d   ❌ %d   ⏭  %d", len(results), ok, failed, skipped)
	if upToDate > 0 {
		fmt.Fprintf(w, "   ⏭  %d already analyzed", upToDate)
	}
	fmt.Fprintf(w, "   ⏱  %s\n", elapsed.Round(time.Second))
}

// exportFormat validates the -export path and returns "json" or "csv".
func exportFormat(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json", nil
	case ".csv":
		return "csv", nil
	}
	return "", fmt.Errorf("export file %q must end in .json or .csv", path)
}

func writeResultsFile(path string, results []appResult) error {
	format, err := exportFormat(path)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if format == "json" {
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if results == nil {
			results = []appResult{}
		}
		return encoder.Encode(results)
	}

	writer := csv.NewWriter(file)
	_ = writer.Write([]string{"bundle_id", "name", "platform", "status", "version", "sdk_count",
		"permission_count", "attempts", "duration_seconds", "error", "report_path", "analysis_date", "note"})
	for _, r := range results {
		_ = writer.Write([]string{r.BundleID, r.Name, r.Platform, r.Status, r.Version,
			strconv.Itoa(r.SDKs), strconv.Itoa(r.Permissions), strconv.Itoa(r.Attempts),
			strconv.FormatFloat(r.DurationSecs, 'f', 1, 64), r.Error, r.ReportPath, r.AnalysisDate, r.Note})
	}
	writer.Flush()
	return writer.Error()
}

// ------------------------------- targets & input ------------------------------- //

var bundleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// parseTargetArg parses "bundle.id" or "ios:bundle.id" / "android:bundle.id".
func parseTargetArg(arg, defaultPlatform string) AppTarget {
	arg = strings.TrimSpace(arg)
	target := AppTarget{BundleID: arg, Platform: defaultPlatform}
	if prefix, rest, ok := strings.Cut(arg, ":"); ok {
		switch strings.ToLower(prefix) {
		case "ios", "android":
			target.Platform = strings.ToLower(prefix)
			target.BundleID = strings.TrimSpace(rest)
		}
	}
	return target
}

// normalizeTargets applies default platforms, drops invalid entries and removes duplicates
// while keeping input order. Dropped entries are reported in problems.
func normalizeTargets(targets []AppTarget, defaultPlatform string) (valid []AppTarget, problems []string) {
	seen := make(map[string]bool, len(targets))
	for _, t := range targets {
		t = withPrefixParsed(t, defaultPlatform)
		switch {
		case !bundleIDPattern.MatchString(t.BundleID):
			problems = append(problems, fmt.Sprintf("skipping invalid bundle ID %q", t.BundleID))
			continue
		case t.Platform != "ios" && t.Platform != "android":
			problems = append(problems, fmt.Sprintf("skipping %s: unknown platform %q", t.BundleID, t.Platform))
			continue
		}
		key := t.Platform + ":" + t.BundleID
		if seen[key] {
			problems = append(problems, fmt.Sprintf("skipping duplicate %s (%s)", t.BundleID, t.Platform))
			continue
		}
		seen[key] = true
		valid = append(valid, t)
	}
	return valid, problems
}

func withPrefixParsed(t AppTarget, defaultPlatform string) AppTarget {
	platform := strings.ToLower(strings.TrimSpace(t.Platform))
	parsed := parseTargetArg(t.BundleID, platform)
	t.BundleID = parsed.BundleID
	t.Platform = parsed.Platform
	if t.Platform == "" {
		t.Platform = defaultPlatform
	}
	return t
}

func hasPlatform(targets []AppTarget, platform string) bool {
	for _, t := range targets {
		if t.Platform == platform {
			return true
		}
	}
	return false
}

// parseJSONAppList accepts ["id", ...], [{"bundleId": "...", "name": "...", "platform": "..."}, ...]
// or an object with those under an "apps" key.
func parseJSONAppList(data []byte) ([]AppTarget, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		var wrapper struct {
			Apps []json.RawMessage `json:"apps"`
		}
		if wrapErr := json.Unmarshal(data, &wrapper); wrapErr != nil || wrapper.Apps == nil {
			return nil, fmt.Errorf("JSON app list must be an array or an object with an \"apps\" array")
		}
		items = wrapper.Apps
	}

	var targets []AppTarget
	for i, raw := range items {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil {
			targets = append(targets, AppTarget{BundleID: strings.TrimSpace(id)})
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("JSON app list entry %d is neither a string nor an object", i+1)
		}
		get := func(keys ...string) string {
			for k, v := range obj {
				for _, want := range keys {
					if strings.EqualFold(k, want) {
						if s, ok := v.(string); ok {
							return strings.TrimSpace(s)
						}
					}
				}
			}
			return ""
		}
		targets = append(targets, AppTarget{
			BundleID: get("bundleId", "bundle_id", "bundle", "package", "id"),
			Name:     get("name", "title"),
			Platform: strings.ToLower(get("platform", "os")),
		})
	}
	return targets, nil
}

// ------------------------------- preflight & dry-run ------------------------------- //

type preflightIssue struct {
	Level   string // "ok", "warn" or "error"
	Message string
}

func hasBlocker(issues []preflightIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}

// preflight checks the environment for the platforms present in targets.
func (r *CLIRunner) preflight(targets []AppTarget) []preflightIssue {
	var issues []preflightIssue
	add := func(level, format string, args ...any) {
		issues = append(issues, preflightIssue{Level: level, Message: fmt.Sprintf(format, args...)})
	}

	if hasPlatform(targets, "ios") {
		switch {
		case r.udid != "":
			add("ok", "iOS device: %s", r.udid)
		case r.interactive:
			add("warn", "no iOS device detected; you will be asked for a UDID")
		default:
			add("error", "no iOS device detected; connect an iPhone or pass -udid")
		}
		if r.iosSSHScript != "" {
			add("ok", "SSH helper: %s", r.iosSSHScript)
		} else {
			add("warn", "ios-ssh.sh not found next to the settings or binary; set APPMONITOR_IOS_SSH_SCRIPT")
		}
		if r.manualDownload || !r.settings.Options.DownloadFromAppStore {
			add("ok", "App Store downloads: manual or disabled (apps must be installed)")
		} else if r.helpersMgr != nil && r.helpersMgr.IsAppleIDAuthenticated() {
			add("ok", "ipatool: signed in (keychain session found)")
		} else {
			add("warn", "ipatool is not signed in; apps that are not installed will fail. Run `ipatool auth login` once")
		}
	}
	if hasPlatform(targets, "android") {
		if strings.TrimSpace(r.settings.ExodusAPIKey.Key) != "" {
			add("ok", "Exodus API key configured")
		} else {
			add("error", "Exodus API key missing; add it in settings.json")
		}
	}
	return issues
}

func printPreflight(w io.Writer, issues []preflightIssue) {
	if len(issues) == 0 {
		return
	}
	fmt.Fprintln(w, "\n🩺 Preflight")
	icons := map[string]string{"ok": "✅", "warn": "⚠️ ", "error": "❌"}
	for _, issue := range issues {
		fmt.Fprintf(w, "   %s %s\n", icons[issue.Level], issue.Message)
	}
}

// iosInstalledSet lists bundle IDs installed on the device, or nil when it cannot be queried.
func (r *CLIRunner) iosInstalledSet() map[string]bool {
	if r.helpersMgr == nil || r.udid == "" {
		return nil
	}
	installed := make(map[string]bool)
	for _, app := range r.helpersMgr.GetInstalledApps(r.udid) {
		installed[app.CFBundleIdentifier] = true
	}
	return installed
}

func printPlan(w io.Writer, targets []AppTarget, installed map[string]bool, o analyzeOptions) {
	fmt.Fprintf(w, "\n🗒  Plan (%d app(s), dry run: nothing will be downloaded, installed or analyzed)\n", len(targets))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tBUNDLE ID\tPLATFORM\tNAME\tACTION")
	for i, t := range targets {
		action := "analyze via Exodus"
		if t.Platform == "ios" {
			switch {
			case installed == nil:
				action = "analyze (install state unknown)"
			case installed[t.BundleID]:
				action = "analyze installed app"
			case o.manualDownload:
				action = "wait for manual install, then analyze"
			default:
				action = "download, patch, install, analyze"
			}
			if o.uninstallAfter {
				action += ", uninstall"
			}
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", i+1, t.BundleID, t.Platform, truncate(t.Name, 30), action)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nRetries: %d (first delay %s)   Reports: %v   Export: %s\n",
		o.retries, o.retryDelay, !o.noReport, firstNonEmpty(o.export, "-"))
}

// sortedKeys returns map keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
