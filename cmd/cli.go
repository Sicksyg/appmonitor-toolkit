package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"AppMonitor/android"
	"AppMonitor/appstores"
	"AppMonitor/helpers"
	"AppMonitor/internal/tools"
	"AppMonitor/ios"
	"AppMonitor/models"
	"AppMonitor/report"
)

// AppTarget represents an app entry loaded from CSV or command-line
type AppTarget struct {
	BundleID string
	Name     string
	Platform string // "ios", "android", or ""
}

// CLIRunner holds all managers and state for the CLI execution
type CLIRunner struct {
	ctx             context.Context
	settings        models.Settings
	settingsPath    string
	tmpPath         string
	tmpIpaPath      string
	outputPath      string
	reportPath      string
	iosClassLogPath string
	iosSSHScript    string
	iosSSHSetup     string
	trackerCommand  string
	dbPath          string
	helpersMgr      *helpers.Manager
	iosMgr          *ios.Manager
	androidMgr      *android.Manager
	appstoresMgr    *appstores.Manager
	toolPaths       helpers.ToolPaths
	libDir          string
	udid            string
	platform        string
	manualDownload  bool
	uninstallAfter  bool
	fridaEnabled    bool
	interactive     bool
	makeReport      bool
	verbose         bool
	logPath         string
	logFile         *os.File
	logMu           sync.Mutex
	reportOverride  string
	scanner         *bufio.Scanner
	// sleep waits between retries; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// cliOptions configures initCLIRunner.
type cliOptions struct {
	ConfigPath     string
	OutputPath     string
	UDID           string
	Platform       string
	ManualDownload bool
	UninstallAfter bool
	Frida          bool
	Interactive    bool
	NeedsIOS       bool
	DryRun         bool
	MakeReport     bool
	Verbose        bool
	Scanner        *bufio.Scanner
}

// initCLIRunner initializes workspace, tools, and managers using the same logic as App.startup.
// In dry-run mode no SSH tunnel is opened and no device is queried.
func initCLIRunner(ctx context.Context, opts cliOptions) (*CLIRunner, error) {
	platform := opts.Platform
	if platform == "" {
		platform = "ios"
	}
	scanner := opts.Scanner
	if scanner == nil {
		scanner = bufio.NewScanner(os.Stdin)
	}
	runner := &CLIRunner{
		ctx:            ctx,
		platform:       platform,
		manualDownload: opts.ManualDownload,
		uninstallAfter: opts.UninstallAfter,
		fridaEnabled:   opts.Frida,
		interactive:    opts.Interactive,
		makeReport:     opts.MakeReport,
		verbose:        opts.Verbose,
		scanner:        scanner,
		udid:           opts.UDID,
		sleep:          sleepContext,
	}
	customConfigPath, customOutputPath := opts.ConfigPath, opts.OutputPath

	// Setup workspace directories
	if err := runner.setupWorkspace(customConfigPath, customOutputPath); err != nil {
		return nil, fmt.Errorf("⚠️workspace setup failed: %w", err)
	}
	runner.openLogFile()

	// Load external tools (ipatool, idevice_id, ideviceinfo, ideviceinstaller) and their dylibs
	toolPaths, libDir, err := runner.loadExternalTools()
	if err != nil {
		runner.Log("Warning loading external tools: "+err.Error(), "CLI.init")
	}
	runner.toolPaths = toolPaths
	runner.libDir = libDir

	var projectRoot string
	if runner.fridaEnabled && opts.NeedsIOS && !opts.DryRun {
		projectRoot, err = tools.EnsureFridaProject("AppMonitor")
		if err != nil {
			return nil, fmt.Errorf("prepare Frida project: %w", err)
		}
	}

	// Initialize managers
	runner.helpersMgr = helpers.NewManager(runner.Log, ctx, toolPaths, helpers.Paths{
		TempPath: runner.tmpPath,
		LibPath:  libDir,
	})

	runner.iosMgr = ios.NewManager(runner.Log, ios.Paths{
		FridaRoot:          projectRoot,
		ClassLogPath:       runner.iosClassLogPath,
		Helpers:            runner.helpersMgr,
		SSHScriptPath:      runner.iosSSHScript,
		SSHSetupScriptPath: runner.iosSSHSetup,
		LDIDPath:           toolPaths.LDID,
		DittoPath:          os.Getenv("APPMONITOR_DITTO_PATH"),
		LibPath:            libDir,
		IProxyPath:         toolPaths.IProxy,
		SSHIdentityPath:    filepath.Join(filepath.Dir(runner.settingsPath), "ssh", "id_ed25519"),
		TrackerScanCommand: runner.trackerCommand,
	})
	if opts.NeedsIOS && !opts.DryRun {
		runner.iosMgr.StartSSH(ctx)
	}

	runner.androidMgr = android.NewManager(runner.Log, android.Paths{
		OutputPath: runner.outputPath,
		TempPath:   runner.tmpPath,
	})

	runner.appstoresMgr = appstores.NewManager(runner.Log)

	// Validate / detect iOS device if platform is iOS
	if opts.NeedsIOS && runner.udid == "" {
		detectedUDID := runner.helpersMgr.GetUDID()
		if detectedUDID != "" {
			runner.udid = detectedUDID
			runner.Log("Auto-detected iOS device UDID: "+runner.udid, "CLI.init")
		} else {
			runner.Log("No iOS device detected via idevice_id. (Will prompt if needed)", "CLI.init")
		}
	}

	return runner, nil
}

// setupWorkspace sets up settings and output directories matching app.go
func (r *CLIRunner) setupWorkspace(customConfigPath, customOutputPath string) error {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("⚠️get user config dir: %w", err)
	}

	settingsDir := filepath.Join(userConfigDir, "AppMonitor")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("⚠️create settings directory: %w", err)
	}

	r.settingsPath = filepath.Join(settingsDir, "settings.json")
	if customConfigPath != "" {
		r.settingsPath = customConfigPath
	}

	r.tmpPath = filepath.Join(settingsDir, "tmp")
	r.tmpIpaPath = filepath.Join(settingsDir, "ipas")
	r.outputPath = filepath.Join(settingsDir, "output")

	for _, p := range []string{r.tmpPath, r.tmpIpaPath, r.outputPath} {
		if err := os.MkdirAll(p, 0755); err != nil {
			return fmt.Errorf("⚠️create directory %s: %w", p, err)
		}
	}

	// Load or create settings
	if err := r.loadSettings(settingsDir); err != nil {
		return err
	}

	// Output paths
	if customOutputPath != "" {
		r.outputPath = customOutputPath
		r.reportPath = customOutputPath
	} else {
		homeDir, _ := os.UserHomeDir()
		if r.settings.Output.ReportSavePath == "" {
			r.settings.Output.ReportSavePath = filepath.Join(homeDir, "Documents", "AppMonitor Output")
		}
		if r.settings.Output.OutputPath != "" {
			r.outputPath = r.settings.Output.OutputPath
		}
		r.reportPath = r.settings.Output.ReportSavePath
	}

	r.iosClassLogPath = filepath.Join(r.reportPath, "ios", "classlogs")
	r.dbPath = filepath.Join(r.reportPath, "app_database.json")
	r.iosSSHScript, r.iosSSHSetup = resolveIOSSSHScript(r.settingsPath)
	r.trackerCommand = os.Getenv("APPMONITOR_TRACKERSCAN_COMMAND")
	if r.trackerCommand == "" {
		r.trackerCommand = "am_scanner"
	}

	for _, p := range []string{r.iosClassLogPath, r.outputPath} {
		if err := os.MkdirAll(p, 0755); err != nil {
			return fmt.Errorf("⚠️create output directory %s: %w", p, err)
		}
	}

	return nil
}

func resolveIOSSSHScript(settingsPath string) (string, string) {
	if configured := strings.TrimSpace(os.Getenv("APPMONITOR_IOS_SSH_SCRIPT")); configured != "" {
		setup := strings.TrimSpace(os.Getenv("APPMONITOR_IOS_SSH_SETUP_SCRIPT"))
		if setup == "" {
			setup = filepath.Join(filepath.Dir(configured), "ios-ssh-setup.sh")
		}
		return configured, setup
	}

	candidates := []string{
		filepath.Join(filepath.Dir(settingsPath), "scripts", "ios-ssh.sh"),
		filepath.Join("scripts", "ios-ssh.sh"),
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, "scripts", "ios-ssh.sh"),
			filepath.Join(executableDir, "..", "scripts", "ios-ssh.sh"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, filepath.Join(filepath.Dir(candidate), "ios-ssh-setup.sh")
		}
	}
	return "", ""
}

func (r *CLIRunner) trackerOutputPaths(bundleID string) (jsonPath, classNamesPath string, err error) {
	outputDir := filepath.Join(filepath.Dir(r.iosClassLogPath), "am_scanner")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("⚠️create am_scanner output directory: %w", err)
	}
	timestamp := time.Now().Format("20060102_150405.000000000")
	prefix := safeOutputName(bundleID)
	return filepath.Join(outputDir, prefix+"_am_scanner_"+timestamp+".json"),
		filepath.Join(r.iosClassLogPath, prefix+"_am_scanner_classes_"+timestamp+".txt"), nil
}

func safeOutputName(value string) string {
	var safe strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9', char == '-', char == '_', char == '.':
			safe.WriteRune(char)
		default:
			safe.WriteByte('_')
		}
	}
	name := strings.ReplaceAll(strings.Trim(safe.String(), "."), "..", "_")
	if name == "" {
		return "app"
	}
	return name
}

func writeClassNamesFile(path string, classNames []string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create class log directory: %w", err)
	}
	contents := strings.Join(classNames, "\n")
	if len(classNames) > 0 {
		contents += "\n"
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return "", fmt.Errorf("write class names: %w", err)
	}
	return path, nil
}

func mergeCLISDKs(primary, additional map[string][]string) map[string][]string {
	merged := make(map[string][]string, len(primary)+len(additional))
	for platform, sdks := range primary {
		merged[platform] = append([]string(nil), sdks...)
	}
	for platform, sdks := range additional {
		seen := make(map[string]struct{}, len(merged[platform]))
		for _, name := range merged[platform] {
			seen[name] = struct{}{}
		}
		for _, name := range sdks {
			if _, exists := seen[name]; !exists {
				merged[platform] = append(merged[platform], name)
				seen[name] = struct{}{}
			}
		}
	}
	return merged
}

func mergeCLIPermissions(primary, additional map[string]models.IosPermissionDetail) map[string]models.IosPermissionDetail {
	merged := make(map[string]models.IosPermissionDetail, len(primary)+len(additional))
	for name, permission := range primary {
		merged[name] = permission
	}
	for name, permission := range additional {
		merged[name] = permission
	}
	return merged
}

func mergeCLIMaps(primary, additional map[string]any) map[string]any {
	merged := make(map[string]any, len(primary)+len(additional))
	for name, value := range primary {
		merged[name] = value
	}
	for name, value := range additional {
		merged[name] = value
	}
	return merged
}

func (r *CLIRunner) loadSettings(settingsDir string) error {
	if _, err := os.Stat(r.settingsPath); os.IsNotExist(err) {
		defaultSettings := models.Settings{
			Auth: models.AuthSettings{
				AppleEmail:     "your-email-here",
				ApplePassword:  "your-app-specific-password-here",
				GoogleEmail:    "your-google-email-here",
				GooglePassword: "your-google-password-here",
			},
			Options: models.OptionsSettings{
				DownloadFromAppStore: true,
				InstallOnDevice:      true,
			},
			ExodusAPIKey:  models.ExodusAPIKey{Key: "your-exodus-api-key-here"},
			GoogleCookies: models.GoogleCookies{Cookies: "your-google-cookies-here"},
			Output:        models.OutputSetting{},
		}
		settingsBytes, _ := json.MarshalIndent(defaultSettings, "", "  ")
		_ = os.WriteFile(r.settingsPath, settingsBytes, 0644)
	}

	data, err := os.ReadFile(r.settingsPath)
	if err == nil {
		_ = json.Unmarshal(data, &r.settings)
	}
	return nil
}

func (r *CLIRunner) loadExternalTools() (helpers.ToolPaths, string, error) {
	binDir, err := tools.EnsureBundledTools("AppMonitor")
	if err != nil {
		return helpers.ToolPaths{}, "", err
	}
	libDir, err := tools.EnsureBundledLibs("AppMonitor")
	if err != nil {
		return helpers.ToolPaths{}, "", err
	}
	return helpers.NewToolPaths(binDir), libDir, nil
}

// Log records diagnostics in the CLI log file. They only reach the terminal with -verbose,
// which keeps the progress output readable.
func (r *CLIRunner) Log(message, function string) {
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format("2006/01/02 15:04:05"), function, strings.TrimSpace(message))
	r.logMu.Lock()
	defer r.logMu.Unlock()
	if r.logFile != nil {
		_, _ = r.logFile.WriteString(line)
	}
	if r.verbose {
		_, _ = os.Stderr.WriteString(line)
	}
}

// openLogFile starts appending diagnostics to <config>/logs/cli.log.
func (r *CLIRunner) openLogFile() {
	dir := filepath.Join(filepath.Dir(r.settingsPath), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	r.logPath = filepath.Join(dir, "cli.log")
	if file, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		r.logFile = file
	}
}

// LoadAppListFromCSV loads a list of apps from a CSV file.
// Supports comma, semicolon, or tab separators, with or without headers.
// Handles columns for bundle ID / package name, optional app name, and optional platform.
func LoadAppListFromCSV(filePath string) ([]AppTarget, error) {
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("⚠️read app list: %w", err)
	}
	return ParseAppList(contentBytes)
}

// ParseAppList parses app list content: JSON (array of strings or objects) or delimited text.
func ParseAppList(contentBytes []byte) ([]AppTarget, error) {
	if trimmed := bytes.TrimSpace(contentBytes); len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		return parseJSONAppList(trimmed)
	}

	content := strings.TrimSpace(string(contentBytes))
	if content == "" {
		return nil, fmt.Errorf("⚠️app list is empty")
	}

	lines := strings.Split(content, "\n")

	// Detect delimiter from the first non-empty line
	delimiter := ','
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Count(trimmed, ";") > strings.Count(trimmed, ",") && strings.Count(trimmed, ";") > strings.Count(trimmed, "\t") {
			delimiter = ';'
		} else if strings.Count(trimmed, "\t") > strings.Count(trimmed, ",") {
			delimiter = '\t'
		}
		break
	}

	r := csv.NewReader(strings.NewReader(content))
	r.Comma = delimiter
	r.FieldsPerRecord = -1 // allow variable fields
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("⚠️parse csv: %w", err)
	}

	var targets []AppTarget
	bundleIdx := 0
	nameIdx := -1
	platformIdx := -1
	hasHeader := false

	if len(records) > 0 {
		firstRow := records[0]
		// Check if first row is header
		for i, col := range firstRow {
			colLower := strings.ToLower(strings.TrimSpace(col))
			switch colLower {
			case "bundle_id", "bundleid", "bundle", "package", "package_name", "app_id", "appid", "id":
				bundleIdx = i
				hasHeader = true
			case "name", "app_name", "track_name", "title", "application":
				nameIdx = i
				hasHeader = true
			case "platform", "os", "type":
				platformIdx = i
				hasHeader = true
			}
		}
	}

	startIndex := 0
	if hasHeader {
		startIndex = 1
	}

	for i := startIndex; i < len(records); i++ {
		row := records[i]
		if len(row) == 0 {
			continue
		}

		// Skip comments
		if strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}

		var bundleID, name, platform string

		if hasHeader {
			if bundleIdx < len(row) {
				bundleID = strings.TrimSpace(row[bundleIdx])
			}
			if nameIdx >= 0 && nameIdx < len(row) {
				name = strings.TrimSpace(row[nameIdx])
			}
			if platformIdx >= 0 && platformIdx < len(row) {
				platform = strings.ToLower(strings.TrimSpace(row[platformIdx]))
			}
		} else {
			// No header heuristic:
			// If 1 column: bundleID
			// If 2 columns: if col 0 has dots (com.foo.bar) => bundleID, col 1 => name. Else col 0 => name, col 1 => bundleID
			// If 3 columns: col 0 => bundleID, col 1 => name, col 2 => platform
			if len(row) == 1 {
				bundleID = strings.TrimSpace(row[0])
			} else if len(row) == 2 {
				c0 := strings.TrimSpace(row[0])
				c1 := strings.TrimSpace(row[1])
				if strings.Contains(c0, ".") || !strings.Contains(c1, ".") {
					bundleID = c0
					name = c1
				} else {
					name = c0
					bundleID = c1
				}
			} else if len(row) >= 3 {
				bundleID = strings.TrimSpace(row[0])
				name = strings.TrimSpace(row[1])
				platform = strings.ToLower(strings.TrimSpace(row[2]))
			}
		}

		// Clean quotes and spaces
		bundleID = strings.Trim(bundleID, `"' `)
		name = strings.Trim(name, `"' `)
		platform = strings.Trim(platform, `"' `)

		if bundleID != "" {
			targets = append(targets, AppTarget{
				BundleID: bundleID,
				Name:     name,
				Platform: platform,
			})
		}
	}

	return targets, nil
}

// analysisOutput is what one app analysis produces.
type analysisOutput struct {
	Info       models.AppInfo
	ReportPath string
	// ReportErr is set when the analysis succeeded but the PDF could not be written.
	ReportErr error
}

// processSingleApp runs the analysis, writes the PDF report and pushes results to the database.
func (r *CLIRunner) processSingleApp(target AppTarget, platform string) (analysisOutput, error) {
	var (
		info models.AppInfo
		err  error
	)
	switch platform {
	case "ios":
		info, err = r.processIOSApp(target)
	case "android":
		info, err = r.processAndroidApp(target)
	default:
		return analysisOutput{}, fmt.Errorf("⚠️unsupported platform: %s", platform)
	}
	out := analysisOutput{Info: info}
	if err != nil {
		return out, err
	}

	info.AnalysisDate = time.Now().UTC().Format(time.RFC3339)
	if path, reportErr := r.writeReport(&info, platform, info.SDKDetails); reportErr != nil {
		out.ReportErr = reportErr
		r.Log(reportErr.Error(), "CLI.processSingleApp")
	} else {
		out.ReportPath = path
	}
	if err := r.pushToDatabase(info); err != nil {
		r.Log("Failed to push to database: "+err.Error(), "CLI.processSingleApp")
		return out, fmt.Errorf("⚠️database save failed: %w", err)
	}
	out.Info = info
	return out, nil
}

// writeReport renders the PDF for a finished analysis and records its path on info.
func (r *CLIRunner) writeReport(info *models.AppInfo, platform string, details map[string]models.IosSDKDetail) (string, error) {
	if !r.makeReport {
		return "", nil
	}
	dir := filepath.Join(r.reportPath, platform, "reports")
	if r.reportOverride != "" {
		dir = r.reportOverride
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create report directory: %w", err)
	}
	out := filepath.Join(dir, fmt.Sprintf("%s_%s_%d_report.pdf", safeOutputName(info.BundleID), safeOutputName(info.Version), time.Now().Unix()))
	input := report.Input{
		ApplicationName:     info.Name,
		ApplicationBundleID: info.BundleID,
		AnalysisDate:        info.AnalysisDate,
		AppStoreDescription: info.Description,
		AppStoreIconPath:    info.AppStoreIconPath,
		AppStoreURL:         info.AppStoreURL,
		OutPath:             out,
		SDKMap:              info.SDKs,
		SDKDetails:          details,
	}
	if platform == "ios" {
		input.Permissions = report.IosPermissionItems(info.IosPermissions)
	} else {
		input.Permissions = report.AndroidPermissionItems(info.AndroidPermissions)
	}
	mgr := report.NewManager(r.Log, report.Paths{OutputPath: r.outputPath, TempPath: r.tmpPath, ReportPath: dir})
	if err := mgr.MakeMarotoReport(input); err != nil {
		return "", fmt.Errorf("generate PDF report: %w", err)
	}
	info.ResultsPath = out
	return out, nil
}

func (r *CLIRunner) processIOSApp(target AppTarget) (models.AppInfo, error) {
	bundleID := target.BundleID
	r.Log("Starting iOS analysis for: "+bundleID, "CLI.processIOSApp")

	// Ensure UDID
	udid := r.udid
	if udid == "" {
		udid = r.helpersMgr.GetUDID()
		if udid == "" {
			if r.interactive {
				fmt.Print("⚠️  No iOS device auto-detected. Enter device UDID: ")
				if r.scanner.Scan() {
					udid = strings.TrimSpace(r.scanner.Text())
				}
			}
			if udid == "" {
				return models.AppInfo{}, fmt.Errorf("⚠️no iOS device UDID available. Connect device or specify -udid")
			}
		}
		r.udid = udid
	}

	appInfo := models.AppInfo{
		BundleID:       bundleID,
		Name:           target.Name,
		UDID:           udid,
		SDKs:           make(map[string][]string),
		IosPermissions: make(map[string]models.IosPermissionDetail),
	}

	// 1. Fetch metadata from iTunes Search
	fmt.Printf("🔍 Fetching App Store metadata for %s...\n", bundleID)
	itunesResult := r.appstoresMgr.ItunesSearchBundle(bundleID)
	if itunesResult != "" {
		var resp appstores.StoreSearchResponse
		if err := json.Unmarshal([]byte(itunesResult), &resp); err == nil && len(resp.Results) > 0 {
			res := resp.Results[0]
			if appInfo.Name == "" {
				appInfo.Name = res.TrackName
			}
			appInfo.ArtworkUrl = res.ArtworkURL
			appInfo.SellerName = res.SellerName
			appInfo.ArtistViewUrl = res.ArtistViewURL
			appInfo.Description = res.Description
			appInfo.AppStoreURL = res.TrackViewUrl
			if appInfo.AppStoreURL == "" {
				appInfo.AppStoreURL = res.ArtistViewURL
			}
		}
	}
	if appInfo.Name == "" {
		appInfo.Name = bundleID
	}

	// Download app icon if artwork URL found
	if appInfo.ArtworkUrl != "" {
		appInfo.AppStoreIconPath = r.helpersMgr.DownloadAndSaveAppIcon(appInfo.ArtworkUrl, bundleID)
	}

	// 2. Install the app (download, patch for the device OS, install) unless it is already present.
	fmt.Printf("📱 Checking if %s is installed on device (%s)...\n", bundleID, udid)
	analyzeInstalled := r.isAppInstalled(udid, bundleID)
	switch {
	case analyzeInstalled:
		fmt.Printf("✅ %s is already installed on the device, skipping download step.\n", bundleID)
	case r.manualDownload:
		if err := r.waitForManualDownload(appInfo.AppStoreURL, bundleID, udid); err != nil {
			return appInfo, err
		}
		analyzeInstalled = true
	case !r.settings.Options.DownloadFromAppStore:
		return appInfo, fmt.Errorf("⚠️%s is not installed and App Store downloads are disabled in settings; install it manually or use -manual-download", bundleID)
	}
	if r.uninstallAfter {
		defer func() {
			if err := r.uninstallApp(udid, bundleID); err != nil {
				r.Log("Failed to uninstall "+bundleID+": "+err.Error(), "CLI.processIOSApp")
			}
		}()
	}

	if err := os.MkdirAll(r.tmpIpaPath, 0o755); err != nil {
		return appInfo, fmt.Errorf("⚠️create IPA directory: %w", err)
	}
	trackerJSONPath, classNamesPath, err := r.trackerOutputPaths(bundleID)
	if err != nil {
		return appInfo, err
	}
	var minimumOSVersion string
	if response := itunesResult; response != "" {
		if version, versionErr := ios.MinimumOSVersionFromStoreResponse(response); versionErr != nil {
			r.Log("Unable to read MinimumOSVersion from App Store lookup: "+versionErr.Error(), "CLI.processIOSApp")
		} else {
			minimumOSVersion = version
		}
	}
	fmt.Printf("🔬 Analysing %s...\n", bundleID)
	trackerResult, err := r.iosMgr.RunAppAnalysis(r.ctx, ios.AnalysisOptions{
		UDID:                udid,
		BundleID:            bundleID,
		AnalyzeInstalledApp: analyzeInstalled,
		AppleEmail:          r.settings.Auth.AppleEmail,
		ApplePassword:       r.settings.Auth.ApplePassword,
		IPADirectory:        r.tmpIpaPath,
		MinimumOSVersion:    minimumOSVersion,
		TrackerScanPath:     trackerJSONPath,
		Progress: func(_, message string, percent int) {
			fmt.Printf("   [%3d%%] %s\n", percent, message)
		},
	})
	if err != nil {
		return appInfo, fmt.Errorf("⚠️analysis failed: %w", err)
	}
	if trackerResult.TrackerScanPath != "" {
		appInfo.TrackerScanPath = trackerResult.TrackerScanPath
		appInfo.MinimumOSVersion = trackerResult.MinimumOSVersion
		appInfo.SDKDetails = trackerResult.SDKDetails
		appInfo.Version = trackerResult.Version
		appInfo.IosPermissions = trackerResult.Permissions
		appInfo.SDKs = trackerResult.SDKs
		appInfo.BundleInfo = trackerResult.BundleInfo
		classLogPath, saveErr := writeClassNamesFile(classNamesPath, trackerResult.Classes)
		if saveErr != nil {
			return appInfo, fmt.Errorf("⚠️save am_scanner class names: %w", saveErr)
		}
		fmt.Printf("📝 Saved %d am_scanner class names to %s\n", len(trackerResult.Classes), classLogPath)
	}
	if trackerResult.TrackerWarning != nil {
		fmt.Printf("⚠️  am_scanner failed: %v\n", trackerResult.TrackerWarning)
		r.Log("am_scanner failed: "+trackerResult.TrackerWarning.Error(), "CLI.processIOSApp")
		if !r.fridaEnabled {
			return appInfo, fmt.Errorf("⚠️am_scanner analysis failed: %w", trackerResult.TrackerWarning)
		}
	} else if trackerResult.TrackerScanPath == "" {
		return appInfo, fmt.Errorf("⚠️am_scanner did not produce a scan result")
	}

	if r.fridaEnabled {
		fmt.Printf("🔬 Running optional Frida analysis on %s...\n", bundleID)
		fridaPermissions, fridaSDKs, fridaBundleInfo, fridaErr := r.iosMgr.RunCompleteAnalysis(udid, bundleID)
		if fridaErr != nil {
			_ = r.iosMgr.Cleanup()
			return appInfo, fmt.Errorf("⚠️frida analysis failed: %w", fridaErr)
		}
		appInfo.IosPermissions = mergeCLIPermissions(appInfo.IosPermissions, fridaPermissions)
		appInfo.SDKs = mergeCLISDKs(appInfo.SDKs, fridaSDKs)
		appInfo.BundleInfo = mergeCLIMaps(appInfo.BundleInfo, fridaBundleInfo)
		if appInfo.Version == "" {
			if version, ok := fridaBundleInfo["shortVersion"].(string); ok {
				appInfo.Version = version
			}
		}
		if err := r.iosMgr.Cleanup(); err != nil {
			r.Log("Warning during cleanup: "+err.Error(), "CLI.processIOSApp")
		}
	}

	if appInfo.Version == "" {
		appInfo.Version = "unknown"
	}

	return appInfo, nil
}

func (r *CLIRunner) processAndroidApp(target AppTarget) (models.AppInfo, error) {
	bundleID := target.BundleID
	r.Log("Starting Android analysis for: "+bundleID, "CLI.processAndroidApp")

	appInfo := models.AppInfo{
		BundleID:           bundleID,
		Name:               target.Name,
		SDKs:               make(map[string][]string),
		AndroidPermissions: make(map[string]models.AndroidPermissionDetail),
	}

	// 1. Fetch metadata from Google Play Store
	fmt.Printf("🔍 Fetching Google Play metadata for %s...\n", bundleID)
	details := r.appstoresMgr.GooglePlayDetails(bundleID)
	if details.TrackName != "" {
		if appInfo.Name == "" {
			appInfo.Name = details.TrackName
		}
		appInfo.ArtworkUrl = details.ArtworkURL
		appInfo.SellerName = details.SellerName
		appInfo.ArtistViewUrl = details.ArtistViewURL
		appInfo.Description = details.Description
		appInfo.AppStoreURL = details.ArtistViewURL
	}
	if appInfo.Name == "" {
		appInfo.Name = bundleID
	}

	// Download app icon if artwork URL found
	if appInfo.ArtworkUrl != "" {
		appInfo.AppStoreIconPath = r.helpersMgr.DownloadAndSaveAppIcon(appInfo.ArtworkUrl, bundleID)
	}

	// 2. Run Exodus Analysis
	fmt.Printf("🔬 Analyzing app via Exodus API (%s)...\n", bundleID)
	apiKey := r.settings.ExodusAPIKey.Key
	permissions, sdks, err := r.androidMgr.RunCompleteAnalysis(bundleID, apiKey)
	if err != nil {
		return appInfo, fmt.Errorf("⚠️android analysis failed: %w", err)
	}

	appInfo.AndroidPermissions = permissions
	appInfo.SDKs = map[string][]string{"Android": make([]string, 0)}
	for _, sdk := range sdks {
		appInfo.SDKs["Android"] = append(appInfo.SDKs["Android"], sdk.Name)
	}

	return appInfo, nil
}

// pushToDatabase saves the app info into the shared app_database.json
func (r *CLIRunner) pushToDatabase(info models.AppInfo) error {
	dbPath := r.dbPath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("⚠️create database directory: %w", err)
	}

	data, err := os.ReadFile(dbPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("⚠️read database: %w", err)
	}
	database, err := models.DecodeAnalysisDatabase(data)
	if err != nil {
		return fmt.Errorf("⚠️parse database: %w", err)
	}

	if info.AnalysisDate == "" {
		info.AnalysisDate = time.Now().UTC().Format(time.RFC3339)
	}
	database.AddAnalysis(info)

	encoded, err := json.MarshalIndent(database, "", "  ")
	if err != nil {
		return fmt.Errorf("⚠️marshal database: %w", err)
	}

	if err := os.WriteFile(dbPath, encoded, 0644); err != nil {
		return fmt.Errorf("⚠️write database file: %w", err)
	}

	r.Log("App info pushed to database at: "+dbPath, "CLI.pushToDatabase")
	return nil
}

func (r *CLIRunner) runBundledCommand(label, executable string, args ...string) error {
	command := exec.Command(executable, args...)
	command.Env = append(os.Environ(),
		"DYLD_LIBRARY_PATH="+r.libDir,
		"DYLD_FALLBACK_LIBRARY_PATH="+r.libDir,
	)

	output, err := command.CombinedOutput()
	if len(output) > 0 {
		fmt.Printf("[%s]\n%s", label, output)
	}
	if err != nil {
		return fmt.Errorf("⚠️%s: %w", label, err)
	}
	return nil
}

func (r *CLIRunner) uninstallApp(udid, bundleID string) error {
	if r.toolPaths.IDeviceInstaller == "" {
		return fmt.Errorf("⚠️bundled ideviceinstaller is not available")
	}
	fmt.Printf("🗑️  Uninstalling %s...\n", bundleID)
	return r.runBundledCommand("ideviceinstaller uninstall", r.toolPaths.IDeviceInstaller, "-u", udid, "uninstall", bundleID)
}

func (r *CLIRunner) isAppInstalled(udid, bundleID string) bool {
	for _, app := range r.helpersMgr.GetInstalledApps(udid) {
		if app.CFBundleIdentifier == bundleID {
			return true
		}
	}
	return false
}

func (r *CLIRunner) waitForManualDownload(storeURL, bundleID, udid string) error {
	if strings.TrimSpace(storeURL) == "" {
		return fmt.Errorf("⚠️no App Store URL available for %s", bundleID)
	}
	if !r.interactive {
		return fmt.Errorf("⚠️manual download for %s requires interactive mode; omit -auto or -no-interactive", bundleID)
	}

	const maxOpenAttempts = 3
	var openErr error
	for attempt := 1; attempt <= maxOpenAttempts; attempt++ {
		fmt.Printf("🛍️  Opening %s in the iPhone App Store (attempt %d/%d)...\n", bundleID, attempt, maxOpenAttempts)
		ctx := r.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		openErr = r.iosMgr.OpenURL(ctx, storeURL)
		if openErr == nil {
			break
		}
		r.Log(fmt.Sprintf("App Store launch attempt %d failed: %v", attempt, openErr), "CLI.waitForManualDownload")
		if attempt < maxOpenAttempts {
			time.Sleep(time.Second)
		}
	}
	if openErr != nil {
		return fmt.Errorf("⚠️open App Store after %d attempts: %w", maxOpenAttempts, openErr)
	}

	fmt.Println("\n 💾 Please download and install the app on the iPhone, then press Enter to continue.")
	if !r.scanner.Scan() {
		return fmt.Errorf("⚠️waiting for manual download was interrupted")
	}
	if !r.isAppInstalled(udid, bundleID) {
		return fmt.Errorf("⚠️app %s is not installed after manual download", bundleID)
	}
	return nil
}

func (r *CLIRunner) promptChoice(prompt string, validOptions []string, defaultOption string) string {
	for {
		fmt.Print(prompt)
		if !r.scanner.Scan() {
			return defaultOption
		}
		input := strings.ToLower(strings.TrimSpace(r.scanner.Text()))
		if input == "" {
			return defaultOption
		}
		for _, opt := range validOptions {
			if input == opt {
				return input
			}
		}
		fmt.Printf("Invalid choice. Please enter one of %v.\n", validOptions)
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
