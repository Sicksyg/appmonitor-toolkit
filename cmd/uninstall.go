package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type uninstallOptions struct {
	input  string
	udid   string
	config string
	dryRun bool
}

type uninstallOutcome struct {
	BundleID string
	Status   string // "removed", "not installed", "would remove", "failed"
	Err      error
}

func runUninstallCommand(ctx context.Context, args []string) int {
	var o uninstallOptions
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = printUsage
	fs.StringVar(&o.input, "csv", "", "App list (CSV, TSV, JSON or - for stdin)")
	fs.StringVar(&o.input, "i", "", "Shorthand for -csv")
	fs.StringVar(&o.udid, "udid", "", "iOS device UDID (auto-detected by default)")
	fs.StringVar(&o.config, "config", "", "Custom path to settings.json")
	fs.StringVar(&o.config, "f", "", "Shorthand for -config")
	fs.BoolVar(&o.dryRun, "dry-run", false, "List which apps would be removed without uninstalling")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	var targets []AppTarget
	for _, arg := range positional {
		targets = append(targets, parseTargetArg(arg, "ios"))
	}
	if input := strings.TrimSpace(o.input); input != "" {
		loaded, err := loadUninstallList(input, os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ read app list (%s): %v\n", input, err)
			return exitFailed
		}
		targets = append(targets, loaded...)
	}
	targets, problems := normalizeTargets(targets, "ios")
	for _, p := range problems {
		fmt.Printf("⚠️  %s\n", p)
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "❌ uninstall needs -csv <file> or at least one bundle ID")
		return exitUsage
	}

	runner, err := initCLIRunner(ctx, cliOptions{ConfigPath: o.config, UDID: o.udid, NeedsIOS: true, DryRun: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to initialize: %v\n", err)
		return exitFailed
	}
	if runner.logPath != "" {
		fmt.Printf("📝 Diagnostics: %s\n", runner.logPath)
	}
	if runner.udid == "" {
		fmt.Fprintln(os.Stderr, "❌ No iOS device detected. Connect it or pass -udid")
		return exitFailed
	}

	outcomes := runner.uninstallAll(ctx, targets, o.dryRun)
	return printUninstallSummary(outcomes)
}

func loadUninstallList(input string, stdin io.Reader) ([]AppTarget, error) {
	if input != "-" {
		return LoadAppListFromCSV(input)
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	return ParseAppList(data)
}

// uninstallAll removes each target that is installed; apps that are not installed are skipped.
func (r *CLIRunner) uninstallAll(ctx context.Context, targets []AppTarget, dryRun bool) []uninstallOutcome {
	installed := make(map[string]bool)
	for _, app := range r.helpersMgr.GetInstalledApps(r.udid) {
		installed[app.CFBundleIdentifier] = true
	}
	fmt.Printf("📱 Device %s: %d app(s) installed, %d requested\n", r.udid, len(installed), len(targets))

	var outcomes []uninstallOutcome
	for i, target := range targets {
		if ctx.Err() != nil {
			break
		}
		out := uninstallOutcome{BundleID: target.BundleID}
		switch {
		case !installed[target.BundleID]:
			out.Status = "not installed"
		case dryRun:
			out.Status = "would remove"
		default:
			if err := r.uninstallApp(r.udid, target.BundleID); err != nil {
				out.Status, out.Err = "failed", err
			} else {
				out.Status = "removed"
			}
		}
		fmt.Printf("[%d/%d] %s: %s\n", i+1, len(targets), target.BundleID, out.Status)
		outcomes = append(outcomes, out)
	}
	return outcomes
}

func printUninstallSummary(outcomes []uninstallOutcome) int {
	counts := map[string]int{}
	exit := exitOK
	for _, out := range outcomes {
		counts[out.Status]++
		if out.Err != nil {
			exit = exitFailed
			fmt.Printf("❌ %s: %v\n", out.BundleID, out.Err)
		}
	}
	fmt.Printf("\nSummary: %d removed, %d would be removed, %d not installed, %d failed\n",
		counts["removed"], counts["would remove"], counts["not installed"], counts["failed"])
	return exit
}
