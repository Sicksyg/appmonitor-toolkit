package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	ctx               context.Context
	settings          models.Settings
	settingsPath      string
	tmpPath           string
	tmpIpaPath        string
	outputPath        string
	reportPath        string
	iosReportPath     string
	androidReportPath string
	iosClassLogPath   string
	dbPath            string
	helpersMgr        *helpers.Manager
	iosMgr            *ios.Manager
	androidMgr        *android.Manager
	reportMgr         *report.Manager
	appstoresMgr      *appstores.Manager
	udid              string
	platform          string
	interactive       bool
	scanner           *bufio.Scanner
}

func main() {
	csvPathFlag := flag.String("csv", "", "Path to CSV file containing app list")
	bundleFlag := flag.String("bundle", "", "Single app bundle ID to analyze")
	platformFlag := flag.String("platform", "ios", "Target platform: 'ios' or 'android'")
	udidFlag := flag.String("udid", "", "iOS Device UDID (optional, auto-detected if omitted)")
	outputPathFlag := flag.String("output", "", "Custom base output directory for reports and database")
	configPathFlag := flag.String("config", "", "Custom path to settings.json")
	autoFlag := flag.Bool("auto", false, "Non-interactive mode (automatically proceed on success)")
	noInteractiveFlag := flag.Bool("no-interactive", false, "Alias for -auto (non-interactive mode)")
	flag.Parse()

	isAuto := *autoFlag || *noInteractiveFlag

	fmt.Println("==================================================")
	fmt.Println("             AppMonitor CLI Runner                ")
	fmt.Println("==================================================")

	runner, err := initCLIRunner(*configPathFlag, *outputPathFlag, *udidFlag, strings.ToLower(*platformFlag), !isAuto)
	if err != nil {
		log.Fatalf("❌ Failed to initialize CLI runner: %v", err)
	}

	// Determine targets: either single bundle ID, CSV file, or prompt user
	var targets []AppTarget
	csvPath := *csvPathFlag
	bundleID := *bundleFlag

	if bundleID != "" {
		targets = append(targets, AppTarget{
			BundleID: strings.TrimSpace(bundleID),
			Platform: runner.platform,
		})
	} else if csvPath != "" {
		loaded, err := LoadAppListFromCSV(csvPath)
		if err != nil {
			log.Fatalf("❌ Failed to read CSV file (%s): %v", csvPath, err)
		}
		targets = loaded
	} else {
		// Interactive prompt if neither was provided
		fmt.Print("\nEnter CSV file path or single bundle ID to analyze: ")
		if runner.scanner.Scan() {
			input := strings.TrimSpace(runner.scanner.Text())
			if strings.HasSuffix(strings.ToLower(input), ".csv") || fileExists(input) {
				loaded, err := LoadAppListFromCSV(input)
				if err != nil {
					log.Fatalf("❌ Failed to read CSV file: %v", err)
				}
				targets = loaded
			} else if input != "" {
				targets = append(targets, AppTarget{
					BundleID: input,
					Platform: runner.platform,
				})
			} else {
				fmt.Println("No input provided. Exiting.")
				return
			}
		}
	}

	if len(targets) == 0 {
		fmt.Println("⚠️  No app targets found to process.")
		return
	}

	fmt.Printf("\n📋 Loaded %d app(s) to process.\n", len(targets))
	runner.RunBatch(targets)
}

// initCLIRunner initializes workspace, tools, and managers using the same logic as App.startup
func initCLIRunner(customConfigPath, customOutputPath, udidOverride, platform string, interactive bool) (*CLIRunner, error) {
	ctx := context.Background()
	runner := &CLIRunner{
		ctx:         ctx,
		platform:    platform,
		interactive: interactive,
		scanner:     bufio.NewScanner(os.Stdin),
		udid:        udidOverride,
	}

	if runner.platform == "" {
		runner.platform = "ios"
	}

	// Setup workspace directories
	if err := runner.setupWorkspace(customConfigPath, customOutputPath); err != nil {
		return nil, fmt.Errorf("workspace setup failed: %w", err)
	}

	// Load external tools (ipatool, idevice_id, ideviceinfo, ideviceinstaller) and their dylibs
	toolPaths, libDir, err := runner.loadExternalTools()
	if err != nil {
		log.Printf("⚠️  Warning loading external tools: %v", err)
	}

	// Prepare Frida project
	projectRoot, err := tools.EnsureFridaProject("AppMonitor")
	if err != nil {
		log.Printf("⚠️  Warning preparing Frida project: %v", err)
	}

	// Initialize managers
	runner.helpersMgr = helpers.NewManager(runner.Log, ctx, toolPaths, helpers.Paths{
		TempPath: runner.tmpPath,
		LibPath:  libDir,
	})

	runner.iosMgr = ios.NewManager(runner.Log, ios.Paths{
		FridaRoot:    projectRoot,
		ClassLogPath: runner.iosClassLogPath,
	})

	runner.androidMgr = android.NewManager(runner.Log, android.Paths{
		OutputPath: runner.outputPath,
		TempPath:   runner.tmpPath,
	})

	runner.reportMgr = report.NewManager(runner.Log, report.Paths{
		OutputPath: runner.outputPath,
		TempPath:   runner.tmpPath,
		ReportPath: runner.reportPath,
	})

	runner.appstoresMgr = appstores.NewManager(runner.Log)

	// Validate / detect iOS device if platform is iOS
	if runner.platform == "ios" && runner.udid == "" {
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
		return fmt.Errorf("get user config dir: %w", err)
	}

	settingsDir := filepath.Join(userConfigDir, "AppMonitor")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
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
			return fmt.Errorf("create directory %s: %w", p, err)
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

	r.iosReportPath = filepath.Join(r.reportPath, "ios", "reports")
	r.androidReportPath = filepath.Join(r.reportPath, "android", "reports")
	r.iosClassLogPath = filepath.Join(r.reportPath, "ios", "classlogs")
	r.dbPath = filepath.Join(r.outputPath, "app_database.json")

	for _, p := range []string{r.iosReportPath, r.androidReportPath, r.iosClassLogPath, r.outputPath} {
		if err := os.MkdirAll(p, 0755); err != nil {
			return fmt.Errorf("create output directory %s: %w", p, err)
		}
	}

	return nil
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

func (r *CLIRunner) Log(message, function string) {
	log.Printf("[%s] %s", function, message)
}

// LoadAppListFromCSV loads a list of apps from a CSV file.
// Supports comma, semicolon, or tab separators, with or without headers.
// Handles columns for bundle ID / package name, optional app name, and optional platform.
func LoadAppListFromCSV(filePath string) ([]AppTarget, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open csv file: %w", err)
	}
	defer file.Close()

	// Read content to determine delimiter and structure
	contentBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read csv file: %w", err)
	}

	content := strings.TrimSpace(string(contentBytes))
	if content == "" {
		return nil, fmt.Errorf("csv file is empty")
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("csv file is empty")
	}

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
		return nil, fmt.Errorf("parse csv: %w", err)
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

// RunBatch processes a list of apps sequentially with failsafe retries
func (r *CLIRunner) RunBatch(targets []AppTarget) {
	total := len(targets)
	successCount := 0
	skippedCount := 0

	for i, target := range targets {
		appPlatform := target.Platform
		if appPlatform == "" {
			appPlatform = r.platform
		}

		fmt.Println("\n--------------------------------------------------")
		fmt.Printf("[%d/%d] 🎯 App: %s (Platform: %s)\n", i+1, total, target.BundleID, strings.ToUpper(appPlatform))
		if target.Name != "" {
			fmt.Printf("       Name: %s\n", target.Name)
		}
		fmt.Println("--------------------------------------------------")

		// Failsafe execution loop for current app
		for {
			info, err := r.processSingleApp(target, appPlatform)
			if err != nil {
				fmt.Printf("\n❌ Analysis failed for %s: %v\n", target.BundleID, err)
				if !r.interactive {
					log.Printf("Non-interactive mode: skipping %s after failure", target.BundleID)
					skippedCount++
					break
				}

				choice := r.promptChoice("Options: [R]etry this app / [S]kip to next / [Q]uit batch: ", []string{"r", "s", "q"}, "r")
				if choice == "r" {
					fmt.Println("🔄 Retrying analysis...")
					continue
				} else if choice == "s" {
					fmt.Printf("⏭️  Skipping %s\n", target.BundleID)
					skippedCount++
					break
				} else if choice == "q" {
					fmt.Println("🛑 Batch processing aborted by user.")
					r.printSummary(total, successCount, skippedCount)
					return
				}
			} else {
				// Analysis succeeded, report generated and saved to DB
				successCount++
				fmt.Printf("\n✅ Analysis complete for %s!\n", target.BundleID)
				fmt.Printf("   📊 Detected SDKs: %d\n", len(info.SDKs))
				if appPlatform == "ios" {
					fmt.Printf("   🔐 Detected Permissions: %d\n", len(info.IosPermissions))
				} else {
					fmt.Printf("   🔐 Detected Permissions: %d\n", len(info.AndroidPermissions))
				}
				fmt.Printf("   📄 PDF Report: %s\n", info.ResultsPath)
				fmt.Printf("   💾 Database updated: %s\n", r.dbPath)

				if !r.interactive {
					break
				}

				// User satisfaction loop
				satisfied := false
				for !satisfied {
					choice := r.promptChoice("\nActions: [C]ontinue (default) / [R]etry analysis / [V]iew report / [Q]uit batch: ", []string{"c", "r", "v", "q"}, "c")
					switch choice {
					case "c":
						satisfied = true
					case "r":
						fmt.Println("🔄 Re-running analysis for app...")
						// decrement successCount since we're re-running
						successCount--
						satisfied = false
						break
					case "v":
						r.openReportFile(info.ResultsPath)
					case "q":
						fmt.Println("🛑 Batch processing terminated by user.")
						r.printSummary(total, successCount, skippedCount)
						return
					}
					if choice == "r" {
						break
					}
				}

				if satisfied {
					break
				}
			}
		}
	}

	r.printSummary(total, successCount, skippedCount)
}

// processSingleApp runs the analysis, generates the report, and pushes to database
func (r *CLIRunner) processSingleApp(target AppTarget, platform string) (models.AppInfo, error) {
	if platform == "ios" {
		return r.processIOSApp(target)
	} else if platform == "android" {
		return r.processAndroidApp(target)
	}
	return models.AppInfo{}, fmt.Errorf("unsupported platform: %s", platform)
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
				return models.AppInfo{}, fmt.Errorf("no iOS device UDID available. Connect device or specify -udid")
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
			appInfo.AppStoreURL = res.ArtistViewURL
		}
	}
	if appInfo.Name == "" {
		appInfo.Name = bundleID
	}

	// Download app icon if artwork URL found
	if appInfo.ArtworkUrl != "" {
		appInfo.AppStoreIconPath = r.helpersMgr.DownloadAndSaveAppIcon(appInfo.ArtworkUrl, bundleID)
	}

	// 2. Check installation and auto-install if configured
	fmt.Printf("📱 Checking if %s is installed on device (%s)...\n", bundleID, udid)
	installedApps := r.helpersMgr.GetInstalledApps(udid)
	isInstalled := false
	for _, app := range installedApps {
		if app.CFBundleIdentifier == bundleID {
			isInstalled = true
			break
		}
	}

	if !isInstalled {
		if r.settings.Options.DownloadFromAppStore && r.settings.Auth.AppleEmail != "" && !strings.Contains(r.settings.Auth.AppleEmail, "your-email") {
			fmt.Printf("📥 App not installed. Attempting download and install via ipatool...\n")
			if err := r.helpersMgr.DownloadAndInstall(udid, bundleID, r.settings.Auth.AppleEmail, r.settings.Auth.ApplePassword, r.tmpPath); err != nil {
				fmt.Printf("⚠️  Automatic download/install failed: %v\n", err)
				fmt.Println("👉 Please install the app on the device manually, then retry.")
			}
		} else {
			fmt.Printf("ℹ️  App %s not detected on device. Make sure it is installed.\n", bundleID)
		}
	}

	// 3. Run Frida analysis
	fmt.Printf("🔬 Running Frida analysis on %s...\n", bundleID)
	enrichedPermissions, sdks, err := r.iosMgr.RunCompleteAnalysis(udid, bundleID)
	if err != nil {
		_ = r.iosMgr.Cleanup()
		return appInfo, fmt.Errorf("frida analysis failed: %w", err)
	}

	appInfo.IosPermissions = enrichedPermissions
	appInfo.SDKs = sdks

	// Cleanup Frida
	if err := r.iosMgr.Cleanup(); err != nil {
		r.Log("Warning during cleanup: "+err.Error(), "CLI.processIOSApp")
	}

	// 4. Generate PDF Report
	fmt.Println("📝 Generating PDF report...")
	timestamp := time.Now().Unix()
	reportFileName := fmt.Sprintf("%s_%d_report.pdf", bundleID, timestamp)
	reportPath := filepath.Join(r.iosReportPath, reportFileName)

	reportInput := report.Input{
		ApplicationName:     appInfo.Name,
		ApplicationBundleID: appInfo.BundleID,
		AppStoreDescription: appInfo.Description,
		AppStoreIconPath:    appInfo.AppStoreIconPath,
		AppStoreURL:         appInfo.AppStoreURL,
		OutPath:             reportPath,
		SDKMap:              appInfo.SDKs,
		Permissions:         report.IosPermissionItems(appInfo.IosPermissions),
	}

	if err := r.reportMgr.MakeMarotoReport(reportInput); err != nil {
		return appInfo, fmt.Errorf("generate pdf report: %w", err)
	}

	appInfo.ResultsPath = reportPath

	// 5. Push to Database
	if err := r.pushToDatabase(appInfo); err != nil {
		r.Log("Failed to push to database: "+err.Error(), "CLI.processIOSApp")
		return appInfo, fmt.Errorf("database save failed: %w", err)
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
		return appInfo, fmt.Errorf("android analysis failed: %w", err)
	}

	appInfo.AndroidPermissions = permissions
	appInfo.SDKs = map[string][]string{"Android": make([]string, 0)}
	for _, sdk := range sdks {
		appInfo.SDKs["Android"] = append(appInfo.SDKs["Android"], sdk.Name)
	}

	// 3. Generate PDF Report
	fmt.Println("📝 Generating PDF report...")
	timestamp := time.Now().Unix()
	reportFileName := fmt.Sprintf("%s_%d_report.pdf", bundleID, timestamp)
	reportPath := filepath.Join(r.androidReportPath, reportFileName)

	reportInput := report.Input{
		ApplicationName:     appInfo.Name,
		ApplicationBundleID: appInfo.BundleID,
		AppStoreDescription: appInfo.Description,
		AppStoreIconPath:    appInfo.AppStoreIconPath,
		AppStoreURL:         appInfo.AppStoreURL,
		OutPath:             reportPath,
		SDKMap:              appInfo.SDKs,
		Permissions:         report.AndroidPermissionItems(appInfo.AndroidPermissions),
	}

	if err := r.reportMgr.MakeMarotoReport(reportInput); err != nil {
		return appInfo, fmt.Errorf("generate pdf report: %w", err)
	}

	appInfo.ResultsPath = reportPath

	// 4. Push to Database
	if err := r.pushToDatabase(appInfo); err != nil {
		r.Log("Failed to push to database: "+err.Error(), "CLI.processAndroidApp")
		return appInfo, fmt.Errorf("database save failed: %w", err)
	}

	return appInfo, nil
}

// pushToDatabase saves the app info into the shared app_database.json
func (r *CLIRunner) pushToDatabase(info models.AppInfo) error {
	dbPath := r.dbPath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	database := make(map[string]models.AppInfo)
	if data, err := os.ReadFile(dbPath); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &database)
	}

	database[info.BundleID] = info

	encoded, err := json.MarshalIndent(database, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal database: %w", err)
	}

	if err := os.WriteFile(dbPath, encoded, 0644); err != nil {
		return fmt.Errorf("write database file: %w", err)
	}

	r.Log("App info pushed to database at: "+dbPath, "CLI.pushToDatabase")
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

func (r *CLIRunner) openReportFile(filePath string) {
	if filePath == "" {
		fmt.Println("No report file path to open.")
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "windows":
		cmd = exec.Command("explorer", filePath)
	default:
		cmd = exec.Command("xdg-open", filePath)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to open report file: %v\n", err)
	} else {
		fmt.Printf("Opened report in default viewer: %s\n", filePath)
	}
}

func (r *CLIRunner) printSummary(total, success, skipped int) {
	fmt.Println("\n==================================================")
	fmt.Println("               Batch Summary                      ")
	fmt.Println("==================================================")
	fmt.Printf("Total Apps Processed: %d\n", total)
	fmt.Printf("Successfully Analyzed: %d\n", success)
	fmt.Printf("Skipped / Failed:     %d\n", skipped)
	fmt.Printf("Database File:        %s\n", r.dbPath)
	fmt.Println("==================================================")
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
