package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	goruntime "runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"AppMonitor/android"
	"AppMonitor/appstores"
	"AppMonitor/helpers"
	"AppMonitor/internal/tools"
	"AppMonitor/ios"
	"AppMonitor/models"
	"AppMonitor/report"
)

// NewApp creates a new App application struct for the Wails framework
func NewApp() *App {
	app := &App{
		analysisDone: make(chan struct{}),
	}
	return app
}

/* --- Start of program -- */

// App struct
type App struct {
	ctx               context.Context
	appinfo           AppInfo
	analysisDone      chan struct{}
	reportMgr         *report.Manager
	appstoresMgr      *appstores.Manager
	helpersMgr        *helpers.Manager
	iosMgr            *ios.Manager
	androidMgr        *android.Manager
	settings          models.Settings
	settingsPath      string
	tmpPath           string
	tmpIpaPath        string
	logpath           string
	outputPath        string
	reportPath        string
	iosReportPath     string
	androidReportPath string
	iosClassLogPath   string
}

// AppInfo struct alias from models for app information and installation details
type AppInfo = models.AppInfo

// AnalysisStatus struct to hold analysis status
type AnalysisStatus struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
	Percent int    `json:"percent"`
}

// startup is called when the app starts. The context is saved so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Setup workspace and logging
	if err := a.SetupWorkspace(); err != nil {
		a.Log("Workspace setup failed: "+err.Error(), "App.startup")
		return
	}
	a.SetupLogging()

	// Load external tools (ipatool, idevice_id, ideviceinfo, ideviceinstaller) and their bundled dylibs
	toolPaths, libDir, err := a.LoadExternalTools()
	if err != nil {
		a.Log("Error loading external tools: "+err.Error(), "App.startup")
		return //Fail gracefully if tools cannot be loaded
	}
	a.Log(fmt.Sprintf("Resolved tool paths - ipatool=%s idevice_id=%s ideviceinfo=%s ideviceinstaller=%s", toolPaths.IPATool, toolPaths.IDeviceID, toolPaths.IDeviceInfo, toolPaths.IDeviceInstaller), "App.startup")

	// Ensure the Frida project is set up by calling EnsureFridaProject, which will create the project if it doesn't exist
	projectRoot, err := tools.EnsureFridaProject("AppMonitor")
	if err != nil {
		a.Log("Error preparing frida project: "+err.Error(), "App.startup")
		return
	}
	// Initialize helpers manager with the appropriate paths
	a.helpersMgr = helpers.NewManager(a.Log, ctx, toolPaths, helpers.Paths{
		TempPath: a.tmpPath,
		LibPath:  libDir,
	})

	// Initialize iOS manager with the appropriate paths
	a.iosMgr = ios.NewManager(a.Log, ios.Paths{
		FridaRoot:    projectRoot,
		ClassLogPath: a.iosClassLogPath,
	})
	// Initialize Android manager with the appropriate paths
	a.androidMgr = android.NewManager(a.Log, android.Paths{
		OutputPath: a.outputPath,
		TempPath:   a.tmpPath,
	})
	// Initialize report manager with the appropriate paths
	a.reportMgr = report.NewManager(a.Log, report.Paths{
		OutputPath: a.outputPath,
		TempPath:   a.tmpPath,
		ReportPath: a.reportPath,
	})
	// Initialize appstores manager, does not uses paths, but uses logging
	a.appstoresMgr = appstores.NewManager(a.Log)

	a.Log("Application started", "App.startup")

	// Get the device info and run continuously
	a.helpersMgr.GetInfo()
	go func() {
		for {
			a.helpersMgr.GetInfo()
			time.Sleep(10 * time.Second)
		}
	}()
}

func (a *App) OnShutdown(ctx context.Context) {
	a.Log("==================================================", "App.OnShutdown")
	a.Log("APPLICATION SHUTDOWN", "App.OnShutdown")
	a.Log("==================================================", "App.OnShutdown")
}

// emitStatus emits an analysis status event
func (a *App) emitStatus(stage, message string, percent int) {
	wruntime.EventsEmit(a.ctx, "analysisStatus", AnalysisStatus{
		Stage:   stage,
		Message: message,
		Percent: percent,
	})
}

// SetupLogging creates a fresh active log and retains the previous sessions
// as timestamped files in the archive directory.
func (a *App) SetupLogging() {
	if a.logpath != "" {
		return
	}
	// Use the workspace's Application Support directory for application logs.
	logDir := filepath.Join(filepath.Dir(a.settingsPath), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Error creating log directory: %v", err)
		return
	}

	logFilePath := filepath.Join(logDir, "app.log")
	archiveDir := filepath.Join(logDir, "archive")
	if _, err := os.Stat(logFilePath); err == nil {
		if err := os.MkdirAll(archiveDir, 0755); err != nil {
			log.Printf("Error creating log archive directory: %v", err)
			return
		}
		archiveName := fmt.Sprintf("app-%s.log", time.Now().Format("20060102_150405.000000000"))
		archivePath := filepath.Join(archiveDir, archiveName)
		if err := os.Rename(logFilePath, archivePath); err != nil {
			log.Printf("Error archiving previous log: %v", err)
			return
		}
	} else if !os.IsNotExist(err) {
		log.Printf("Error checking current log: %v", err)
		return
	}

	if err := pruneArchivedLogs(archiveDir, int(30)); err != nil { // Keep the last 30 logs
		log.Printf("Error pruning archived logs: %v", err)
	}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return
	}

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	a.Log("Logging initialized. Logs will be written to: "+logFilePath, "App.SetupLogging")
	a.logpath = logFilePath
}

func pruneArchivedLogs(archiveDir string, maxLogs int) error {
	entries, err := os.ReadDir(archiveDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var logs []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".log" && strings.HasPrefix(entry.Name(), "app-") {
			logs = append(logs, entry)
		}
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Name() > logs[j].Name()
	})

	if len(logs) <= maxLogs {
		return nil
	}

	for _, entry := range logs[maxLogs:] {
		if err := os.Remove(filepath.Join(archiveDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Log function to log messages to console and to a file
func (a *App) Log(message, source string) {
	log.Printf("[%s] %s", source, message)
	wruntime.LogInfof(a.ctx, "[%s] %s", source, message)
}

// LoadExternalTools ensures that the required external tools and their dylibs are available and returns the tool paths plus the extracted lib directory (Ipatool, idevice_id, ideviceinfo, and ideviceinstaller)
func (a *App) LoadExternalTools() (helpers.ToolPaths, string, error) {
	binDir, err := tools.EnsureBundledTools("AppMonitor")
	if err != nil {
		a.Log("Error ensuring external tools: "+err.Error(), "App.LoadExternalTools")
		return helpers.ToolPaths{}, "", err
	}
	a.Log("External tools loaded from: "+binDir, "App.LoadExternalTools")

	libDir, err := tools.EnsureBundledLibs("AppMonitor")
	if err != nil {
		a.Log("Error ensuring external libs: "+err.Error(), "App.LoadExternalTools")
		return helpers.ToolPaths{}, "", err
	}
	a.Log("External libs loaded from: "+libDir, "App.LoadExternalTools")

	return helpers.NewToolPaths(binDir), libDir, nil
}

func (a *App) SetupWorkspace() error {
	// Initialize private storage first, then load settings, and finally derive
	// the user-visible report directories from the loaded output settings.
	settingsDir, err := a.setupPrivateWorkspace()
	if err != nil {
		return err
	}
	if err := a.loadSettings(settingsDir); err != nil {
		return err
	}
	return a.setupOutputPaths()
}

// setupPrivateWorkspace prepares application-owned files that users do not
// normally browse, including settings, temporary files, IPAs, and internal data.
func (a *App) setupPrivateWorkspace() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}

	settingsDir := filepath.Join(userConfigDir, "AppMonitor")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return "", fmt.Errorf("create settings directory: %w", err)
	}

	a.settingsPath = filepath.Join(settingsDir, "settings.json")
	a.tmpPath = filepath.Join(settingsDir, "tmp")
	a.tmpIpaPath = filepath.Join(settingsDir, "ipas")
	a.outputPath = filepath.Join(settingsDir, "output")

	for name, path := range map[string]string{
		"temporary":       a.tmpPath,
		"IPA":             a.tmpIpaPath,
		"internal output": a.outputPath,
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			a.Log(fmt.Sprintf("Error creating %s directory at %s: %v", name, path, err), "App.setupPrivateWorkspace")
			return "", fmt.Errorf("create %s directory: %w", name, err)
		}
	}

	return settingsDir, nil
}

// loadSettings creates the initial settings file when needed, loads persisted
// values, and applies defaults for the private and user-visible output roots.
func (a *App) loadSettings(settingsDir string) error {
	if _, err := os.Stat(a.settingsPath); os.IsNotExist(err) {
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

		settingsBytes, err := json.MarshalIndent(defaultSettings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal default settings: %w", err)
		}
		if err := os.WriteFile(a.settingsPath, settingsBytes, 0644); err != nil {
			return fmt.Errorf("write default settings: %w", err)
		}
		a.Log("Created default settings file at: "+a.settingsPath, "App.SetupWorkspace")
	} else if err != nil {
		return fmt.Errorf("check settings file: %w", err)
	}

	settingsFile, err := os.ReadFile(a.settingsPath)
	if err != nil {
		return fmt.Errorf("read settings file: %w", err)
	}
	if err := json.Unmarshal(settingsFile, &a.settings); err != nil {
		return fmt.Errorf("parse settings file: %w", err)
	}
	a.Log("Settings loaded from: "+a.settingsPath, "App.SetupWorkspace")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home dir: %w", err)
	}
	if a.settings.Output.ReportSavePath == "" {
		a.settings.Output.ReportSavePath = filepath.Join(homeDir, "Documents", "AppMonitor Output")
	}
	if a.settings.Output.OutputPath == "" {
		a.settings.Output.OutputPath = filepath.Join(settingsDir, "output")
	}
	a.outputPath = a.settings.Output.OutputPath
	return nil
}

// setupOutputPaths derives and creates the user-visible report and iOS class
// log directories from the configured report root.
func (a *App) setupOutputPaths() error {
	a.reportPath = a.settings.Output.ReportSavePath
	a.iosReportPath = filepath.Join(a.reportPath, "ios", "reports")
	a.androidReportPath = filepath.Join(a.reportPath, "android", "reports")
	a.iosClassLogPath = filepath.Join(a.reportPath, "ios", "classlogs")

	for name, path := range map[string]string{
		"iOS reports":     a.iosReportPath,
		"Android reports": a.androidReportPath,
		"iOS class logs":  a.iosClassLogPath,
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("create %s directory: %w", name, err)
		}
	}
	return nil
}

// ------------------------- Frida analysis functions ----------------------- //

func (a *App) DownloadAndInstall(udid string, bundleID string) error {
	return a.helpersMgr.DownloadAndInstall(udid, bundleID, a.settings.Auth.AppleEmail, a.settings.Auth.ApplePassword, a.tmpPath)
}

// ------------------------- Main iOS analysis flow ----------------------- //
func (a *App) StartIosAnalysis() {
	a.emitStatus("start", "Starting analysis", 0)
	a.Log("Starting analysis for: "+a.appinfo.BundleID, "App.StartAnalysis")

	// Reset AppInfo struct for fresh analysis
	a.appinfo.SDKs = make(map[string][]string)
	a.appinfo.IosPermissions = make(map[string]models.IosPermissionDetail)
	a.appinfo.ResultsPath = ""

	a.emitStatus("download", "Downloading and installing", 10)
	if err := a.DownloadAndInstall(a.appinfo.UDID, a.appinfo.BundleID); err != nil {
		message := fmt.Sprintf("Unable to download or install %s automatically: %v. Install the app directly from the App Store on the device, then retry the analysis.", a.appinfo.BundleID, err)
		a.Log(message, "App.StartIosAnalysis")
		a.emitStatus("error", message, 100)
		return
	}

	a.emitStatus("frida", "Running Frida analysis", 35)
	enrichedPermissions, sdks, err := a.iosMgr.RunCompleteAnalysis(a.appinfo.UDID, a.appinfo.BundleID)
	if err != nil {
		a.emitStatus("error", "Frida analysis failed: "+err.Error(), 100)
		a.Log("Error during frida analysis: "+err.Error(), "App.StartAnalysis")
		return
	}

	time.Sleep(time.Second * 2)

	a.appinfo.IosPermissions = enrichedPermissions
	a.appinfo.SDKs = sdks

	if err := a.iosMgr.Cleanup(); err != nil {
		a.emitStatus("cleanup", "Cleanup warning: "+err.Error(), 75)
		a.Log("Warning: Error during frida cleanup: "+err.Error(), "App.StartAnalysis")
	}

	a.emitStatus("report", "Generating report", 85)
	reportPath := filepath.Join(a.iosReportPath, fmt.Sprintf("%s_%d_report.pdf", a.appinfo.BundleID, time.Now().Unix()))
	reportInput := report.Input{
		ApplicationName:     a.appinfo.Name,
		ApplicationBundleID: a.appinfo.BundleID,
		AppStoreDescription: a.appinfo.Description,
		AppStoreIconPath:    a.appinfo.AppStoreIconPath,
		AppStoreURL:         a.appinfo.AppStoreURL,
		OutPath:             reportPath,
		SDKMap:              a.appinfo.SDKs,
		Permissions:         report.IosPermissionItems(a.appinfo.IosPermissions),
	}

	a.Log("Image path in report input: "+reportInput.AppStoreIconPath, "App.StartIosAnalysis")

	if err := a.reportMgr.MakeMarotoReport(reportInput); err != nil {
		a.emitStatus("error", "Report generation failed: "+err.Error(), 100)
		a.Log("Error generating PDF report: "+err.Error(), "App.StartAnalysis")
		return
	}

	a.appinfo.ResultsPath = reportPath
	a.emitStatus("done", "Analysis complete", 100)
	a.Log("Analysis complete! Report saved to: "+reportPath, "App.StartAnalysis")

	// push to database
	a.CreateAndPushToDatabase()
}

// ------------------------- Main Android analysis flow ----------------------- //
func (a *App) StartAndroidAnalysis() {
	a.emitStatus("start", "Starting Android analysis", 0)
	a.Log("Starting Android analysis for: "+a.appinfo.BundleID, "App.StartAndroidAnalysis")

	// get app data from Exodus API
	a.emitStatus("fetch", "Fetching app data from Exodus API", 20)

	// analyze the downloaded apk file using exodus api
	a.emitStatus("analyze", "Analyzing app with Exodus", 60)

	permissions, sdks, err := a.androidMgr.RunCompleteAnalysis(a.appinfo.BundleID, a.settings.ExodusAPIKey.Key)
	if err != nil {
		a.emitStatus("error", "Android analysis failed: "+err.Error(), 100)
		a.Log("Error during Android analysis: "+err.Error(), "App.StartAndroidAnalysis")
		return
	}

	a.appinfo.AndroidPermissions = permissions
	a.appinfo.SDKs = map[string][]string{"Android": make([]string, 0)}
	for _, sdk := range sdks {
		a.appinfo.SDKs["Android"] = append(a.appinfo.SDKs["Android"], sdk.Name)
	}

	time.Sleep(time.Second * 2)

	if err := a.iosMgr.Cleanup(); err != nil {
		a.emitStatus("cleanup", "Cleanup warning: "+err.Error(), 75)
		a.Log("Warning: Error during frida cleanup: "+err.Error(), "App.StartAnalysis")
	}

	a.emitStatus("report", "Generating report", 85)
	reportPath := filepath.Join(a.androidReportPath, fmt.Sprintf("%s_%d_report.pdf", a.appinfo.BundleID, time.Now().Unix()))
	reportInput := report.Input{
		ApplicationName:     a.appinfo.Name,
		ApplicationBundleID: a.appinfo.BundleID,
		AppStoreDescription: a.appinfo.Description,
		AppStoreIconPath:    a.appinfo.AppStoreIconPath,
		AppStoreURL:         a.appinfo.AppStoreURL,
		OutPath:             reportPath,
		SDKMap:              a.appinfo.SDKs,
		Permissions:         report.AndroidPermissionItems(a.appinfo.AndroidPermissions),
	}
	if err := a.reportMgr.MakeMarotoReport(reportInput); err != nil {
		a.emitStatus("error", "Report generation failed: "+err.Error(), 100)
		a.Log("Error generating PDF report: "+err.Error(), "App.StartAnalysis")
		return
	}

	a.appinfo.ResultsPath = reportPath

	a.emitStatus("done", "Analysis complete", 100)
	a.Log("Android analysis complete for: "+a.appinfo.BundleID, "App.StartAndroidAnalysis")
	a.CreateAndPushToDatabase()
}

// --------------------------------------------------------------- //
//
// -- Helper functions for iTunes Search and installed apps -- //

// -- Frontend Wrappers -- //

func (a *App) LoadFromPhone() string {
	// Get UDID if not set
	if a.appinfo.UDID == "" {
		a.appinfo.UDID = a.helpersMgr.GetUDID()
	}
	// Get installed apps directly from helpers
	programs := a.helpersMgr.GetInstalledApps(a.appinfo.UDID)
	a.appinfo.InstalledApps = programs
	// Search iTunes for each installed app bundleID to get more info and store results in a list
	var results []map[string]interface{}
	for _, program := range programs {
		itunesResult := a.appstoresMgr.ItunesSearchBundle(program.CFBundleIdentifier)
		if itunesResult != "" {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(itunesResult), &result); err == nil {
				results = append(results, result)
			}
		}
	}
	// Convert results to JSON string to return to frontend
	jsonBytes, _ := json.Marshal(results)
	return string(jsonBytes)
}

// Authentication function for Apple ID credentials. This is a wrapper around the helpers.Manager.AuthenticateAppleID method.
func (a *App) AuthenticateAppleID() {
	a.helpersMgr.AuthenticateAppleID(a.settings.Auth.AppleEmail, a.settings.Auth.ApplePassword)
}

func (a *App) Search(bundleID string) string {
	return a.appstoresMgr.ItunesSearchBundle(bundleID)
}

// SearchWild performs a search on iTunes with the given term and returns the results to the frontend
func (a *App) ItunesSearchWild(term string) string {
	return a.appstoresMgr.ItunesSearchWild(term)
}

// -- Helper functions for Google Play Store search -- //

// SearchGooglePlay searches the Google Play Store for the given term and returns a list of matching apps
func (a *App) SearchGooglePlay(term string) string {
	return a.appstoresMgr.GooglePlaySearch(term)
}

// SelectItem is called when the user selects an app from the search results. It sets the selected app's details in the AppInfo struct for use in the analysis.
func (a *App) SelectItem(trackName string, trackId int, bundleId string, artworkUrl string, sellerName string, artistViewUrl string, description string) {
	a.Log(fmt.Sprintf("Selected item - trackName: %s, trackId: %d, bundleId: %s", trackName, trackId, bundleId), "App.SelectItem")
	// set the AppStruct to the selected item
	a.appinfo.Name = trackName
	a.appinfo.BundleID = bundleId
	a.appinfo.ArtworkUrl = artworkUrl
	a.appinfo.SellerName = sellerName
	a.appinfo.ArtistViewUrl = artistViewUrl
	a.appinfo.Description = description

	a.appinfo.AppStoreIconPath = a.helpersMgr.DownloadAndSaveAppIcon(artworkUrl, bundleId)

	// a.Log(fmt.Sprintf("App info updated:\n  Name: %s\n  BundleID: %s\n  ArtworkUrl: %s\n  SellerName: %s\n  ArtistViewUrl: %s\n  Description: %s", a.appinfo.Name, a.appinfo.BundleID, a.appinfo.ArtworkUrl, a.appinfo.SellerName, a.appinfo.ArtistViewUrl, a.appinfo.Description), "App.SelectItem")
}

func (a *App) OpenUrl(url string) {
	wruntime.BrowserOpenURL(a.ctx, url)
}

func (a *App) SetReportSavePath() string {
	a.Log("Opening directory dialog", "App.SetReportSavePath")
	dirPath, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Select a directory",
		DefaultDirectory: "./",
	})
	if err != nil {
		a.Log("Failed to open directory dialog: "+err.Error(), "App.SetReportSavePath")
		return ""
	}

	a.Log("Selected report directory: "+dirPath, "App.SetReportSavePath")
	return dirPath
}

// GetSettings returns the current settings to the frontend.
func (a *App) GetSettings() models.Settings {
	return a.settings
}

// SaveSettings persists updated settings to disk and refreshes the in-memory copy.
func (a *App) SaveSettings(settings models.Settings) error {
	settingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(a.settingsPath, settingsBytes, 0644); err != nil {
		a.Log("Error writing settings file: "+err.Error(), "App.SaveSettings")
		return err
	}
	a.settings = settings
	a.Log("Settings saved to: "+a.settingsPath, "App.SaveSettings")
	return nil
}

func (a *App) OpenSettingsDir() {
	// This function opens the settings directory in the default file manager
	a.Log("Opening settings directory", "App.OpenSettingsDir")

	settingsDir := filepath.Dir(a.settingsPath)

	switch goruntime.GOOS {
	case "darwin":
		cmd := exec.Command("open", settingsDir)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening path in Finder: "+err.Error(), "App.OpenPathInFileManager")
		}
	case "windows":
		cmd := exec.Command("explorer", settingsDir)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening path in Explorer: "+err.Error(), "App.OpenPathInFileManager")
		}
	case "linux":
		cmd := exec.Command("xdg-open", settingsDir)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening path in file manager: "+err.Error(), "App.OpenPathInFileManager")
		}
	default:
		a.Log("Unsupported OS for opening file manager", "App.OpenPathInFileManager")
	}
}

func (a *App) OpenOutputDir() {
	// This function opens the output directory in the default file manager
	a.Log("Opening output directory", "App.OpenOutputDir")

	switch goruntime.GOOS {
	case "darwin":
		cmd := exec.Command("open", a.reportPath)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening path in Finder: "+err.Error(), "App.OpenPathInFileManager")
		}
	case "windows":
		cmd := exec.Command("explorer", a.reportPath)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening path in Explorer: "+err.Error(), "App.OpenPathInFileManager")
		}
	case "linux":
		cmd := exec.Command("xdg-open", a.reportPath)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening path in file manager: "+err.Error(), "App.OpenPathInFileManager")
		}
	default:
		a.Log("Unsupported OS for opening file manager", "App.OpenPathInFileManager")
	}
}

func (a *App) OpenReportFileInDefaultApp() {
	// This function opens a file in the default application based on the OS
	filePath := a.appinfo.ResultsPath
	if filePath == "" {
		a.Log("No file path specified to open", "App.OpenFileInDefaultApp")
		return
	}
	switch goruntime.GOOS {
	case "darwin":
		cmd := exec.Command("open", filePath)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening file: "+err.Error(), "helpers.OpenFileInDefaultApp")
		}
	case "windows":
		cmd := exec.Command("explorer", filePath)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening file: "+err.Error(), "helpers.OpenFileInDefaultApp")
		}
	case "linux":
		cmd := exec.Command("xdg-open", filePath)
		if err := cmd.Run(); err != nil {
			a.Log("Error opening file: "+err.Error(), "helpers.OpenFileInDefaultApp")
		}
	default:
		a.Log("Unsupported OS for opening files", "helpers.OpenFileInDefaultApp")
	}
}

func (a *App) LoadAppList() string {
	// This function loads a list of apps from a CSV file and returns the content as an ordered list
	a.Log("Loading app list", "helpers.Manager.LoadAppList")
	filePath, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Select a file",
		DefaultDirectory: a.outputPath,
		Filters: []wruntime.FileFilter{
			{
				DisplayName: "CSV Files",
				Pattern:     "*.csv",
			},
		},
	})
	if err != nil {
		a.Log("Failed to open file dialog: "+err.Error(), "App.LoadAppList")
		return ""
	}
	a.Log("Selected app list file: "+filePath, "App.LoadAppList")

	return filePath
}

func (a *App) ClearTmpDir() {
	// This function clears the temporary directory used by the application
	a.Log("Clearing temporary directory: "+a.tmpPath, "App.ClearTmpDir")

	entries, err := os.ReadDir(a.tmpPath)
	if err != nil {
		a.Log("Error reading temporary directory: "+err.Error(), "App.ClearTmpDir")
		return
	}
	for _, entry := range entries {
		err := os.RemoveAll(filepath.Join(a.tmpPath, entry.Name()))
		if err != nil {
			a.Log("Error removing temporary file: "+err.Error(), "App.ClearTmpDir")
		}
	}
}

func (a *App) OpenAppInAppStore() {
	// This function opens the app's App Store URL on the device using Frida.
	if a.appinfo.AppStoreURL == "" {
		a.Log("No App Store URL available for the selected app", "App.OpenAppInAppStore")
		return
	}
	a.iosMgr.OpenAppInAppStore(a.appinfo.UDID, a.appinfo.BundleID, a.appinfo.AppStoreURL)
}

func (a *App) CreateAndPushToDatabase() {
	// Creaste a database json file with results from analysis (AppInfo struct) if it doesn't exist
	dbPath := filepath.Join(a.outputPath, "app_database.json")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			a.Log("Error creating database directory: "+err.Error(), "App.CreateAndPushToDatabase")
			return
		}
		initialData := make(map[string]AppInfo)
		initialBytes, err := json.MarshalIndent(initialData, "", "  ")
		if err != nil {
			a.Log("Error creating initial database JSON: "+err.Error(), "App.CreateAndPushToDatabase")
			return
		}
		if err = os.WriteFile(dbPath, initialBytes, 0644); err != nil {
			a.Log("Error writing initial database file: "+err.Error(), "App.CreateAndPushToDatabase")
			return
		}
		a.Log("Created new database file at: "+dbPath, "App.CreateAndPushToDatabase")
	}

	// Load existing database
	dbFile, err := os.ReadFile(dbPath)
	if err != nil {
		a.Log("Error reading database file: "+err.Error(), "App.CreateAndPushToDatabase")
		return
	}

	var database map[string]AppInfo
	if err = json.Unmarshal(dbFile, &database); err != nil {
		a.Log("Error parsing database file: "+err.Error(), "App.CreateAndPushToDatabase")
		return
	}

	// Add current app info to database
	database[a.appinfo.BundleID] = a.appinfo

	// Save updated database to disk
	dbBytes, err := json.MarshalIndent(database, "", "  ")
	if err != nil {
		a.Log("Error encoding database JSON: "+err.Error(), "App.CreateAndPushToDatabase")
		return
	}
	if err = os.WriteFile(dbPath, dbBytes, 0644); err != nil {
		a.Log("Error writing database file: "+err.Error(), "App.CreateAndPushToDatabase")
		return
	}
	a.Log("App info added to database for bundle ID: "+a.appinfo.BundleID, "App.CreateAndPushToDatabase")
}
