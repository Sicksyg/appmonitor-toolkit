package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

// run dispatches to a subcommand. Without one, it behaves as `analyze`, which keeps
// every pre-subcommand invocation (for example `-csv apps.csv -a`) working.
func run(ctx context.Context, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "report":
			return runReportCommand(ctx, args[1:])
		case "uninstall":
			return runUninstallCommand(ctx, args[1:])
		case "analyze":
			return runAnalyzeCommand(ctx, args[1:])
		case "help", "-h", "-help", "--help":
			printUsage()
			return exitOK
		}
	}
	return runAnalyzeCommand(ctx, args)
}

func printUsage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `AppMonitor CLI — find the SDKs and permissions inside iOS/Android apps, one or many at a time.

USAGE
  %[1]s [command] [flags] [bundle-id ...]

COMMANDS
  analyze     Analyze apps and write a PDF report for each (default; the word is optional)
  report      Re-create PDF reports from results already in the database
  uninstall   Remove apps from the connected iPhone
  help        Show this message

  A command is the first word after the program name. Flags and bundle IDs come after it.
  Leave the command out and "analyze" runs, so "%[1]s com.example.app" works.

QUICK START
  %[1]s com.example.app                      Analyze one app
  %[1]s -i apps.csv                          Analyze every app in a list
  %[1]s -i apps.csv -a -export results.csv   Unattended run, save a results table
  %[1]s -dry-run -i apps.csv                 Check the list and setup without analyzing
  %[1]s report -all                          Rebuild every PDF from saved results
  %[1]s report -all -db other/app_database.json -out pdfs   Use a specific database
  %[1]s uninstall -i apps.csv                Remove the listed apps from the iPhone
  cat apps.txt | %[1]s -i - -p android       Read the list from stdin

  Bundle IDs are like com.example.app. Prefix one with ios: or android: to pick its platform.
  A list file can be .txt (one ID per line), .csv/.tsv (ID in the first column) or .json.

ANALYZE FLAGS
  What to analyze
    -i, -input, -c, -csv file   App list file, or '-' for stdin
    -b, -bundle id              A single bundle ID
    -p, -platform name          Default platform for IDs without a prefix: ios or android (default ios)
    -u, -udid id                iPhone UDID (auto-detected if omitted)

  Bulk runs
    -retries n                  Retries per app after a failure (default 2)
    -retry-delay duration       Wait before the first retry, doubled each time (default 5s)
    -export file                Save per-app results as .csv or .json
    -skip-done                  Skip apps already analyzed today (per the database)
    -skip-within duration       Skip apps analyzed within this period, e.g. 72h
    -dry-run                    Validate the list and setup, show the plan, then stop
    -a, -auto, -n, -no-interactive
                                Never prompt; needed for unattended runs

  Output
    -o, -output dir             Folder for the database and reports
    -f, -config file            Custom settings.json
    -no-report                  Skip the PDF report
    -v, -verbose                Also print diagnostic logs (always saved to <config>/logs/cli.log)

  Behavior
    -m, -manual-download        Open each iOS app in the App Store and wait for you to download it
    -x, -uninstall-after        Uninstall each iOS app once it has been analyzed
    --frida                     Also run Frida analysis after am_scanner (iOS only)

REPORT FLAGS  (%[1]s report [flags] [bundle-id ...])
    -all                        Every app in the database
    -b, -bundle id[,id...]      Only these apps (positional IDs also work)
    -all-versions               Every stored version, not just the latest
    -db file                    Use this app_database.json instead of the default database
    -out dir                    Write PDFs here instead of the default reports folder
    -j n                        PDFs rendered in parallel (default 4)
    -o, -output / -f, -config   Same as analyze

UNINSTALL FLAGS  (%[1]s uninstall [flags] [bundle-id ...])
  Removes the listed iOS apps from the iPhone. There is no confirmation prompt,
  so try -dry-run first. Apps that are not installed are skipped.
    -i, -csv file               App list file, or '-' for stdin
    -udid id                    iPhone UDID (auto-detected if omitted)
    -dry-run                    Show what would be removed without removing it
    -f, -config file            Same as analyze

EXIT STATUS
  0 everything succeeded, 1 at least one app failed, 2 invalid usage

Run "%[1]s <command> -h" to see only that command's flags.
`, name)
}

// analyzeOptions holds every flag of the analyze command.
type analyzeOptions struct {
	input          string
	bundle         string
	platform       string
	udid           string
	output         string
	config         string
	export         string
	manualDownload bool
	uninstallAfter bool
	auto           bool
	noInteractive  bool
	frida          bool
	dryRun         bool
	verbose        bool
	noReport       bool
	retries        int
	retryDelay     time.Duration
	skipDone       bool
	skipWithin     time.Duration
}

func newAnalyzeFlagSet(o *analyzeOptions) *flag.FlagSet {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = printUsage

	str := func(p *string, def, usage string, names ...string) {
		for _, name := range names {
			fs.StringVar(p, name, def, usage)
		}
	}
	boolean := func(p *bool, usage string, names ...string) {
		for _, name := range names {
			fs.BoolVar(p, name, false, usage)
		}
	}

	str(&o.input, "", "App list file (.txt, .csv, .json) or '-' for stdin", "input", "i", "csv", "c")
	str(&o.bundle, "", "Single bundle ID to analyze", "bundle", "b")
	str(&o.platform, "ios", "Default platform: 'ios' or 'android'", "platform", "p")
	str(&o.udid, "", "iOS device UDID (auto-detected if omitted)", "udid", "u")
	str(&o.output, "", "Custom output directory", "output", "o")
	str(&o.config, "", "Custom path to settings.json", "config", "f")
	str(&o.export, "", "Write run results to a .csv or .json file", "export")
	boolean(&o.manualDownload, "Open each iOS app in the App Store and wait for manual download", "manual-download", "m")
	boolean(&o.uninstallAfter, "Uninstall each analyzed iOS app after processing", "uninstall-after", "x")
	boolean(&o.auto, "Non-interactive mode", "auto", "a")
	boolean(&o.noInteractive, "Alias for -auto", "no-interactive", "n")
	boolean(&o.frida, "Also run Frida analysis after am_scanner (iOS only)", "frida")
	boolean(&o.dryRun, "Validate and show the plan without analyzing", "dry-run")
	boolean(&o.verbose, "Print diagnostic logs to the terminal (always saved to the log file)", "verbose", "v")
	boolean(&o.noReport, "Skip PDF report generation", "no-report")
	boolean(&o.skipDone, "Skip apps already analyzed today", "skip-done")
	fs.DurationVar(&o.skipWithin, "skip-within", 0, "Skip apps analyzed within this period, e.g. 72h (implies -skip-done)")
	fs.IntVar(&o.retries, "retries", 2, "Automatic retries per app after a failure")
	fs.DurationVar(&o.retryDelay, "retry-delay", 5*time.Second, "Delay before the first retry; doubles each time")
	return fs
}

// parseInterleaved parses flags that may appear before, between or after positional arguments.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func runAnalyzeCommand(ctx context.Context, args []string) int {
	var o analyzeOptions
	fs := newAnalyzeFlagSet(&o)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	platform := strings.ToLower(strings.TrimSpace(o.platform))
	if platform == "" {
		platform = "ios"
	}
	if platform != "ios" && platform != "android" {
		fmt.Fprintf(os.Stderr, "❌ Unsupported platform %q: use 'ios' or 'android'\n", o.platform)
		return exitUsage
	}
	if o.retries < 0 {
		fmt.Fprintln(os.Stderr, "❌ -retries must be zero or greater")
		return exitUsage
	}

	if o.export != "" {
		if _, err := exportFormat(o.export); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			return exitUsage
		}
	}

	interactive := !(o.auto || o.noInteractive)
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("==================================================")
	fmt.Println("             AppMonitor CLI Runner                ")
	fmt.Println("==================================================")

	targets, usedStdin, err := collectTargets(o, positional, platform, scanner, interactive, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return exitFailed
	}
	if usedStdin {
		// stdin carried the app list, so there is nothing left to answer prompts with.
		interactive = false
	}

	targets, problems := normalizeTargets(targets, platform)
	for _, problem := range problems {
		fmt.Printf("⚠️  %s\n", problem)
	}
	if len(targets) == 0 {
		fmt.Println("⚠️  No app targets found to process.")
		return exitUsage
	}
	fmt.Printf("\n📋 Loaded %d app(s) to process.\n", len(targets))

	runner, err := initCLIRunner(ctx, cliOptions{
		ConfigPath:     o.config,
		OutputPath:     o.output,
		UDID:           o.udid,
		Platform:       platform,
		ManualDownload: o.manualDownload,
		UninstallAfter: o.uninstallAfter,
		Frida:          o.frida,
		Interactive:    interactive,
		NeedsIOS:       hasPlatform(targets, "ios"),
		DryRun:         o.dryRun,
		MakeReport:     !o.noReport,
		Verbose:        o.verbose,
		Scanner:        scanner,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to initialize CLI runner: %v\n", err)
		return exitFailed
	}
	if runner.iosMgr != nil {
		defer runner.iosMgr.StopSSH()
	}
	if runner.logPath != "" {
		fmt.Printf("📝 Diagnostics: %s (use -v to print them)\n", runner.logPath)
	}

	issues := runner.preflight(targets)
	printPreflight(os.Stdout, issues)
	if o.dryRun {
		printPlan(os.Stdout, targets, runner.iosInstalledSet(), o)
		if hasBlocker(issues) {
			return exitFailed
		}
		return exitOK
	}
	if hasBlocker(issues) {
		fmt.Println("\n❌ Fix the problems above and run again.")
		return exitFailed
	}

	started := time.Now()
	cfg := batchConfig{Retries: o.retries, RetryDelay: o.retryDelay}
	if cfg.SkipSince = skipSinceTime(time.Now(), o.skipDone, o.skipWithin); !cfg.SkipSince.IsZero() {
		if database, err := runner.loadDatabase(); err == nil {
			cfg.Done = latestAnalysisTimes(database)
		}
	}
	results := runner.RunBatch(ctx, targets, cfg)
	printSummaryTable(os.Stdout, results, time.Since(started))

	if o.export != "" {
		if err := writeResultsFile(o.export, results); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Export failed: %v\n", err)
			return exitFailed
		}
		fmt.Printf("📤 Results exported to %s\n", o.export)
	}
	if ctx.Err() != nil {
		fmt.Println("🛑 Interrupted; remaining apps were skipped.")
		return exitFailed
	}
	for _, result := range results {
		if result.Status != statusOK && result.Status != statusDone {
			return exitFailed
		}
	}
	return exitOK
}

// collectTargets merges positional IDs, -b and the input file. When nothing was given and
// the session is interactive, it prompts for a file path or bundle ID as before.
func collectTargets(o analyzeOptions, positional []string, platform string, scanner *bufio.Scanner, interactive bool, stdin io.Reader) (targets []AppTarget, usedStdin bool, err error) {
	for _, arg := range positional {
		targets = append(targets, parseTargetArg(arg, platform))
	}
	if bundle := strings.TrimSpace(o.bundle); bundle != "" {
		targets = append(targets, parseTargetArg(bundle, platform))
	}

	if input := strings.TrimSpace(o.input); input != "" {
		var loaded []AppTarget
		if input == "-" {
			data, readErr := io.ReadAll(stdin)
			if readErr != nil {
				return nil, false, fmt.Errorf("read app list from stdin: %w", readErr)
			}
			usedStdin = true
			loaded, err = ParseAppList(data)
		} else {
			loaded, err = LoadAppListFromCSV(input)
		}
		if err != nil {
			return nil, usedStdin, fmt.Errorf("read app list (%s): %w", input, err)
		}
		targets = append(targets, loaded...)
	}

	if len(targets) == 0 && interactive {
		fmt.Print("\nEnter an app list file path or a single bundle ID to analyze: ")
		if scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			switch {
			case input == "":
			case fileExists(input) || looksLikeListFile(input):
				loaded, loadErr := LoadAppListFromCSV(input)
				if loadErr != nil {
					return nil, false, fmt.Errorf("read app list: %w", loadErr)
				}
				targets = loaded
			default:
				targets = append(targets, parseTargetArg(input, platform))
			}
		}
	}
	return targets, usedStdin, nil
}

func looksLikeListFile(path string) bool {
	switch strings.ToLower(filepathExt(path)) {
	case ".csv", ".txt", ".json":
		return true
	}
	return false
}

func filepathExt(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 && !strings.ContainsAny(path[i:], `/\`) {
		return path[i:]
	}
	return ""
}
