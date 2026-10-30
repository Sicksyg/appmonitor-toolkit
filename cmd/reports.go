package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"AppMonitor/ios"
	"AppMonitor/models"
)

type reportOptions struct {
	all         bool
	allVersions bool
	bundles     stringList
	out         string
	db          string
	output      string
	config      string
	jobs        int
}

// stringList is a repeatable, comma-separated flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

// reportJob is one stored analysis to render again.
type reportJob struct {
	BundleID string
	Version  string
	History  models.AppHistory
	Record   models.AnalysisRecord
}

type reportOutcome struct {
	Job  reportJob
	Path string
	Err  error
	Note string
}

func runReportCommand(ctx context.Context, args []string) int {
	var o reportOptions
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = printUsage
	fs.BoolVar(&o.all, "all", false, "Regenerate reports for every app in the database")
	fs.BoolVar(&o.allVersions, "all-versions", false, "Every stored version instead of the latest")
	fs.Var(&o.bundles, "bundle", "Bundle ID(s) to include (repeatable, comma-separated)")
	fs.Var(&o.bundles, "b", "Shorthand for -bundle")
	fs.StringVar(&o.out, "out", "", "Write PDFs to this directory")
	fs.StringVar(&o.db, "db", "", "Read this app_database.json instead of the default database")
	fs.StringVar(&o.output, "output", "", "Custom output directory containing app_database.json")
	fs.StringVar(&o.output, "o", "", "Shorthand for -output")
	fs.StringVar(&o.config, "config", "", "Custom path to settings.json")
	fs.StringVar(&o.config, "f", "", "Shorthand for -config")
	fs.IntVar(&o.jobs, "j", 4, "Reports generated in parallel")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	o.bundles = append(o.bundles, positional...)
	if !o.all && len(o.bundles) == 0 {
		fmt.Fprintln(os.Stderr, "❌ report needs -all or at least one bundle ID")
		return exitUsage
	}
	if o.jobs < 1 {
		o.jobs = 1
	}

	runner, err := initCLIRunner(ctx, cliOptions{
		ConfigPath: o.config,
		OutputPath: o.output,
		DryRun:     true,
		MakeReport: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to initialize: %v\n", err)
		return exitFailed
	}
	if o.out != "" {
		runner.reportOverride = o.out
	}
	if o.db != "" {
		if _, err := os.Stat(o.db); err != nil {
			fmt.Fprintf(os.Stderr, "❌ database %s: %v\n", o.db, err)
			return exitFailed
		}
		runner.dbPath = o.db
	}

	database, err := runner.loadDatabase()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return exitFailed
	}
	jobs, missing := selectReportJobs(database, o)
	for _, id := range missing {
		fmt.Printf("⚠️  %s is not in the database\n", id)
	}
	if len(jobs) == 0 {
		fmt.Println("⚠️  Nothing to regenerate.")
		return exitFailed
	}
	fmt.Printf("📄 Regenerating %d report(s) from %s\n", len(jobs), runner.dbPath)

	outcomes := runner.regenerateReports(ctx, jobs, o.jobs)

	failed := 0
	updates := make(map[string]map[string]string)
	for _, out := range outcomes {
		label := out.Job.BundleID + " " + out.Job.Version
		if out.Err != nil {
			failed++
			fmt.Printf("❌ %s: %v\n", label, out.Err)
			continue
		}
		fmt.Printf("✅ %s → %s\n", label, out.Path)
		if out.Note != "" {
			fmt.Printf("   ⚠️  %s\n", out.Note)
		}
		if updates[out.Job.BundleID] == nil {
			updates[out.Job.BundleID] = make(map[string]string)
		}
		updates[out.Job.BundleID][out.Job.Version] = out.Path
	}
	if len(updates) > 0 {
		if err := runner.updateResultsPaths(updates); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Reports written but the database could not be updated: %v\n", err)
			return exitFailed
		}
	}
	fmt.Printf("\nTotal: %d   ✅ %d   ❌ %d\n", len(outcomes), len(outcomes)-failed, failed)
	if failed > 0 || ctx.Err() != nil {
		return exitFailed
	}
	return exitOK
}

func (r *CLIRunner) loadDatabase() (models.AnalysisDatabase, error) {
	data, err := os.ReadFile(r.dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no database at %s; run an analysis first", r.dbPath)
		}
		return nil, fmt.Errorf("read database: %w", err)
	}
	database, err := models.DecodeAnalysisDatabase(data)
	if err != nil {
		return nil, fmt.Errorf("parse database: %w", err)
	}
	return database, nil
}

// selectReportJobs picks the stored analyses to render, in a stable order.
func selectReportJobs(database models.AnalysisDatabase, o reportOptions) (jobs []reportJob, missing []string) {
	ids := []string(o.bundles)
	if o.all {
		ids = sortedKeys(database)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		history, ok := database[id]
		if !ok || len(history.Versions) == 0 {
			missing = append(missing, id)
			continue
		}
		versions := sortedVersions(history)
		if !o.allVersions {
			versions = versions[:1]
		}
		for _, version := range versions {
			jobs = append(jobs, reportJob{BundleID: id, Version: version, History: history, Record: history.Versions[version]})
		}
	}
	return jobs, missing
}

// sortedVersions orders versions newest first by analysis date, then by name.
func sortedVersions(history models.AppHistory) []string {
	versions := sortedKeys(history.Versions)
	sort.SliceStable(versions, func(i, j int) bool {
		a, b := history.Versions[versions[i]].AnalysisDate, history.Versions[versions[j]].AnalysisDate
		if a != b {
			return a > b
		}
		return versions[i] > versions[j]
	})
	return versions
}

func (r *CLIRunner) regenerateReports(ctx context.Context, jobs []reportJob, workers int) []reportOutcome {
	outcomes := make([]reportOutcome, len(jobs))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, job reportJob) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				outcomes[i] = reportOutcome{Job: job, Err: ctx.Err()}
				return
			}
			outcomes[i] = r.regenerateReport(job)
		}(i, job)
	}
	wg.Wait()
	return outcomes
}

func (r *CLIRunner) regenerateReport(job reportJob) reportOutcome {
	outcome := reportOutcome{Job: job}
	record := job.Record
	info := models.AppInfo{
		Name:               job.History.Name,
		BundleID:           job.BundleID,
		ArtworkUrl:         job.History.ArtworkURL,
		Description:        job.History.Description,
		AppStoreURL:        job.History.AppStoreURL,
		Version:            job.Version,
		MinimumOSVersion:   record.MinimumOSVersion,
		TrackerScanPath:    record.TrackerScanPath,
		AnalysisDate:       record.AnalysisDate,
		SDKs:               record.SDKs,
		IosPermissions:     record.IosPermissions,
		BundleInfo:         record.BundleInfo,
		AndroidPermissions: record.AndroidPermissions,
	}
	if info.Name == "" {
		info.Name = job.BundleID
	}

	platform := "android"
	if record.TrackerScanPath != "" || len(record.IosPermissions) > 0 {
		platform = "ios"
	}

	if platform == "ios" && record.TrackerScanPath != "" {
		details, err := r.sdkDetailsFromScan(record.TrackerScanPath)
		if err != nil {
			outcome.Note = fmt.Sprintf("SDK details unavailable (%v); report lists SDK names only", err)
		} else {
			info.SDKDetails = details
		}
	}
	if info.ArtworkUrl != "" && r.helpersMgr != nil {
		info.AppStoreIconPath = r.helpersMgr.DownloadAndSaveAppIcon(info.ArtworkUrl, job.BundleID)
	}

	path, err := r.writeReport(&info, platform, info.SDKDetails)
	outcome.Path, outcome.Err = path, err
	return outcome
}

func (r *CLIRunner) sdkDetailsFromScan(path string) (map[string]models.IosSDKDetail, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dump, err := ios.DecodeTrackerScanDump(data)
	if err != nil {
		return nil, err
	}
	return r.iosMgr.DetectSDKDetailsFromDump(dump), nil
}

// updateResultsPaths records new report paths in the database with a single write.
func (r *CLIRunner) updateResultsPaths(updates map[string]map[string]string) error {
	database, err := r.loadDatabase()
	if err != nil {
		return err
	}
	for bundleID, versions := range updates {
		history, ok := database[bundleID]
		if !ok {
			continue
		}
		for version, path := range versions {
			record, ok := history.Versions[version]
			if !ok {
				continue
			}
			record.ResultsPath = path
			history.Versions[version] = record
		}
		database[bundleID] = history
	}
	encoded, err := json.MarshalIndent(database, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.dbPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.dbPath, encoded, 0o644)
}
