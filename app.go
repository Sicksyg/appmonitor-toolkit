package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	goruntime "runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"AppMonitor/analysis"
	"AppMonitor/android"
	"AppMonitor/helpers"
	"AppMonitor/itunes"
	"AppMonitor/models"
	"AppMonitor/report"
)

// NewApp creates a new App application struct for the Wails framework
func NewApp() *App {
	app := &App{
		analysisDone: make(chan struct{}),
	}
	app.reportMgr = report.NewManager(app.Log)
	app.itunesMgr = itunes.NewManager(app.Log)
	app.analysisMgr = analysis.NewManager(app.Log)
	app.androidMgr = android.NewManager(app.Log)
	return app
}

/* --- Start of program -- */

// App struct
type App struct {
	ctx          context.Context
	appinfo      AppInfo
	analysisDone chan struct{}
	reportMgr    *report.Manager
	itunesMgr    *itunes.Manager
	helpersMgr   *helpers.Manager
	analysisMgr  *analysis.Manager
	androidMgr   *android.Manager
	settings     models.Settings
	settingsPath string
}

// AppInfo struct to hold app information and installation details for the analysis
type AppInfo struct {
	Name             string
	BundleID         string
	InstallPath      string
	UDID             string
	ArtworkUrl       string
	SellerName       string
	ArtistViewUrl    string
	Description      string
	AppStoreURL      string
	AppStoreIconPath string
	InstalledApps    []helpers.InstalledApp
	ResultsPath      string
	SDKs             map[string][]string
	Permissions      map[string]models.PermissionDetail
}

// AnalysisStatus struct to hold analysis status
type AnalysisStatus struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
	Percent int    `json:"percent"`
}

// SetupWorkspace creates the tmp directory for analysis results if it doesn't exist
func (a *App) SetupWorkspace() {
	// Create tmp directory if it doesn't exist
	if _, err := os.Stat("tmp"); os.IsNotExist(err) {
		err := os.Mkdir("tmp", 0755)
		if err != nil {
			a.Log("Error creating tmp directory: "+err.Error(), "App.SetupWorkspace")
		} else {
			a.Log("Created tmp directory for analysis results", "App.SetupWorkspace")
		}
	} else {
		a.Log("Tmp directory already exists", "App.SetupWorkspace")
	}

	// Determine settings file path in the user config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		a.Log("Error getting user config dir: "+err.Error(), "App.SetupWorkspace")
		return
	}
	settingsDir := filepath.Join(configDir, "AppMonitor")
	if err = os.MkdirAll(settingsDir, 0755); err != nil {
		a.Log("Error creating settings directory: "+err.Error(), "App.SetupWorkspace")
		return
	}
	a.settingsPath = filepath.Join(settingsDir, "settings.json")

	// sets a default save path for reports based on the os and user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		a.Log("Error getting user home dir: "+err.Error(), "App.SetupWorkspace")
		return
	}
	defaultReportPath := filepath.Join(homeDir, "Documents", "AppMonitor_Reports")
	if a.settings.Report.SavePath == "" {
		a.settings.Report.SavePath = defaultReportPath
	}

	// Create default settings file if it does not exist yet
	if _, err = os.Stat(a.settingsPath); os.IsNotExist(err) {
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
			Report: models.ReportSettings{
				SavePath: "",
			},
			ExodusAPIKey: models.ExodusAPIKey{
				Key: "your-exodus-api-key-here",
			},
			GoogleCookies: models.GoogleCookies{
				Cookies: "your-google-cookies-here",
			},
		}
		settingsBytes, err := json.MarshalIndent(defaultSettings, "", "  ")
		if err != nil {
			a.Log("Error creating default settings JSON: "+err.Error(), "App.SetupWorkspace")
			return
		}
		if err = os.WriteFile(a.settingsPath, settingsBytes, 0644); err != nil {
			a.Log("Error writing default settings file: "+err.Error(), "App.SetupWorkspace")
			return
		}
		a.Log("Created default settings file at: "+a.settingsPath, "App.SetupWorkspace")
	}

	// Load settings from disk
	settingsFile, err := os.ReadFile(a.settingsPath)
	if err != nil {
		a.Log("Error reading settings file: "+err.Error(), "App.SetupWorkspace")
		return
	}
	if err = json.Unmarshal(settingsFile, &a.settings); err != nil {
		a.Log("Error parsing settings file: "+err.Error(), "App.SetupWorkspace")
		return
	}
	a.Log("Settings loaded from: "+a.settingsPath, "App.SetupWorkspace")
}

// startup is called when the app starts. The context is saved so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.helpersMgr = helpers.NewManager(a.Log, ctx)
	a.analysisMgr = analysis.NewManager(a.Log)
	a.androidMgr = android.NewManager(a.Log)

	// setup tmp directory for analysis results
	a.SetupWorkspace()
	a.Log("Application started", "App.startup")

	// Get the device info and run continuously <------ fix to emit from here instead of helpers
	a.helpersMgr.GetInfo()
	go func() {
		for {
			a.helpersMgr.GetInfo()
			time.Sleep(10 * time.Second)
		}
	}()
}

// Simple console logging method
func (a *App) Log(message, source string) {
	log.Printf("%s: %s", source, message)
}

// emitStatus emits an analysis status event
func (a *App) emitStatus(stage, message string, percent int) {
	wruntime.EventsEmit(a.ctx, "analysisStatus", AnalysisStatus{
		Stage:   stage,
		Message: message,
		Percent: percent,
	})
}

// ------------------------- Frida analysis functions ----------------------- //

func (a *App) DownloadAndInstall(udid string, bundleID string) {
	a.helpersMgr.DownloadAndInstall(udid, bundleID, a.settings.Auth.AppleEmail, a.settings.Auth.ApplePassword)
}

// ------------------------- Main iOS analysis flow ----------------------- //

// Main function to start analysis
func (a *App) StartAnalysis() {
	a.emitStatus("start", "Starting analysis", 0)
	a.Log("Starting analysis for: "+a.appinfo.BundleID, "App.StartAnalysis")

	// Reset AppInfo struct for fresh analysis
	a.appinfo.SDKs = make(map[string][]string)
	a.appinfo.Permissions = make(map[string]models.PermissionDetail)
	a.appinfo.ResultsPath = ""

	a.emitStatus("download", "Downloading and installing", 10)
	a.DownloadAndInstall(a.appinfo.UDID, a.appinfo.BundleID)

	a.emitStatus("frida", "Running Frida analysis", 35)
	enrichedPermissions, sdks, err := a.analysisMgr.RunCompleteAnalysis(a.appinfo.UDID, a.appinfo.BundleID)
	if err != nil {
		a.emitStatus("error", "Frida analysis failed: "+err.Error(), 100)
		a.Log("Error during frida analysis: "+err.Error(), "App.StartAnalysis")
		return
	}

	time.Sleep(time.Second * 2)

	a.appinfo.Permissions = enrichedPermissions
	a.appinfo.SDKs = sdks

	if err := a.analysisMgr.Cleanup(); err != nil {
		a.emitStatus("cleanup", "Cleanup warning: "+err.Error(), 75)
		a.Log("Warning: Error during frida cleanup: "+err.Error(), "App.StartAnalysis")
	}

	a.emitStatus("report", "Generating report", 85)
	reportPath := filepath.Join("tmp", fmt.Sprintf("%s_report.pdf", a.appinfo.BundleID))
	if err := a.reportMgr.MakeMarotoReport(a.appinfo.Name, a.appinfo.BundleID, a.appinfo.Description, a.appinfo.AppStoreIcon, a.appinfo.AppStoreURL, reportPath, a.appinfo.SDKs, a.appinfo.Permissions); err != nil {
		a.emitStatus("error", "Report generation failed: "+err.Error(), 100)
		a.Log("Error generating PDF report: "+err.Error(), "App.StartAnalysis")
		return
	}

	a.appinfo.ResultsPath = reportPath
	a.emitStatus("done", "Analysis complete", 100)
	a.Log("Analysis complete! Report saved to: "+reportPath, "App.StartAnalysis")
}

// ------------------------- Main Android analysis flow ----------------------- //
func (a *App) StartAndroidAnalysis() {
	a.emitStatus("start", "Starting Android analysis", 0)
	a.Log("Starting Android analysis for: "+a.appinfo.BundleID, "App.StartAndroidAnalysis")

	// get app data from Exodus API
	a.emitStatus("fetch", "Fetching app data from Exodus API", 20)
	a.androidMgr.GetAppDataFromExodus(a.appinfo.BundleID, a.settings.ExodusAPIKey.Key)

	// analyze the downloaded apk file using exodus api
	a.emitStatus("analyze", "Analyzing app with Exodus", 60)
	a.androidMgr.GetAppDataFromExodus(a.appinfo.BundleID, a.settings.ExodusAPIKey.Key)

	a.emitStatus("done", "Analysis complete", 100)
	a.Log("Android analysis complete for: "+a.appinfo.BundleID, "App.StartAndroidAnalysis")
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
		itunesResult := a.itunesMgr.ItunesSearchBundle(program.CFBundleIdentifier)
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

func (a *App) Search(bundleID string) string {
	return a.itunesMgr.ItunesSearchBundle(bundleID)
}

func (a *App) SearchWild(term string) string {
	return a.itunesMgr.ItunesSearchWild(term)
}

// -- Helper functions for Google Play Store search -- //

// SearchGooglePlay searches the Google Play Store for the given term and returns a list of matching apps
func (a *App) SearchGooglePlay(term string) string {
	return a.androidMgr.GooglePlaySearch(term)
}

func (a *App) SelectItem(trackName string, trackId int, bundleId string, artworkUrl string, sellerName string, artistViewUrl string, description string) {
	a.Log(fmt.Sprintf("Selected item - trackName: %s, trackId: %d, bundleId: %s", trackName, trackId, bundleId), "App.SelectItem")
	// set the AppStruct to the selected item
	a.appinfo.Name = trackName
	a.appinfo.BundleID = bundleId
	a.appinfo.ArtworkUrl = artworkUrl
	a.appinfo.SellerName = sellerName
	a.appinfo.ArtistViewUrl = artistViewUrl
	a.appinfo.Description = description

	a.helpersMgr.DownloadAndSaveAppIcon(artworkUrl, bundleId)

	a.Log(fmt.Sprintf("App info updated - Name: %s, BundleID: %s, ArtworkUrl: %s, SellerName: %s, ArtistViewUrl: %s, Description: %s", a.appinfo.Name, a.appinfo.BundleID, a.appinfo.ArtworkUrl, a.appinfo.SellerName, a.appinfo.ArtistViewUrl, a.appinfo.Description), "App.SelectItem")
}

// OpenFile opens a file dialog
func (a *App) LoadIpaFile() string {
	// This function opens a file dialog to select an IPA file for install and analysis
	a.Log("Opening file dialog", "App.LoadIpaFile")
	filePath, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Select a file",
		DefaultDirectory: "./Test_files/",
		Filters: []wruntime.FileFilter{
			{
				DisplayName: "IPA Files",
				Pattern:     "*.ipa",
			},
		},
	})
	if err != nil {
		fmt.Println("Failed to open file dialog:", err)
		return ""
	}

	// set the AppStruct
	a.appinfo.InstallPath = filePath

	return filePath
}

func (a *App) LoadAppList() string {
	// This function loads a list of apps from a CSV file and returns the content as an ordered list
	a.Log("Loading app list", "App.LoadAppList")
	filePath, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Select a file",
		DefaultDirectory: "./Test_files/",
		Filters: []wruntime.FileFilter{
			{
				DisplayName: "CSV Files",
				Pattern:     "*.csv",
			},
		},
	})
	if err != nil {
		fmt.Println("Failed to open file dialog:", err)
		return ""
	}
	fmt.Println("Selected file:", filePath)

	a.appinfo.InstallPath = filePath

	return filePath
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
		fmt.Println("Failed to open directory dialog:", err)
		return ""
	}

	fmt.Println("Selected directory:", dirPath)
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
