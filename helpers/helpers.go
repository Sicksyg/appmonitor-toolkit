package helpers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// InstalledApp represents an app installed on the device
type InstalledApp struct {
	CFBundleIdentifier  string
	CFBundleDisplayName string
}

// DeviceInfo holds device information
type DeviceInfo struct {
	Name    string
	UDID    string
	Model   string
	Version string
}

// ToolPaths holds absolute paths to external binaries extracted by EnsureTools.
type ToolPaths struct {
	IPATool          string
	IDeviceID        string
	IDeviceInfo      string
	IDeviceInstaller string
}

type Paths struct {
	TempPath string
	LibPath  string
}

// NewToolPaths builds tool paths from the extracted bin directory.
func NewToolPaths(binDir string) ToolPaths {
	return ToolPaths{
		IPATool:          filepath.Join(binDir, "ipatool"),
		IDeviceID:        filepath.Join(binDir, "idevice_id"),
		IDeviceInfo:      filepath.Join(binDir, "ideviceinfo"),
		IDeviceInstaller: filepath.Join(binDir, "ideviceinstaller"),
	}
}

// Manager handles device-related operations
type Manager struct {
	logger  func(message, function string)
	ctx     context.Context
	tools   ToolPaths
	tmpPath string
	libPath string
}

// NewManager creates a new helpers Manager
func NewManager(logger func(message, function string), ctx context.Context, tools ToolPaths, paths Paths) *Manager {
	return &Manager{
		logger:  logger,
		ctx:     ctx,
		tools:   tools,
		tmpPath: paths.TempPath,
		libPath: paths.LibPath,
	}
}

// withBundledLibEnv sets DYLD_LIBRARY_PATH/DYLD_FALLBACK_LIBRARY_PATH so the bundled
// binaries can find their extracted dylibs regardless of their baked-in rpath.
func (m *Manager) withBundledLibEnv(cmd *exec.Cmd) *exec.Cmd {
	if m.libPath == "" {
		return cmd
	}
	cmd.Env = append(os.Environ(),
		"DYLD_LIBRARY_PATH="+m.libPath,
		"DYLD_FALLBACK_LIBRARY_PATH="+m.libPath,
	)
	return cmd
}

// GetInfo retrieves device information using ideviceinfo
func (m *Manager) GetInfo() *DeviceInfo {
	// Get info from connected iPhone using ideviceinfo
	ideviceInfoCMD := m.withBundledLibEnv(exec.Command(m.tools.IDeviceInfo, "-s"))
	output, err := ideviceInfoCMD.Output()
	if err != nil {
		m.logger("Error getting device info: "+err.Error(), "helpers.Manager.GetInfo")
		// Emit disconnected state
		info := map[string]string{
			"DeviceName": "",
			"Model":      "",
			"OSVersion":  "",
			"Udid":       "",
			"Connected":  "false",
		}
		runtime.EventsEmit(m.ctx, "deviceInfo", info)
		return &DeviceInfo{}
	}

	// Parse output to get model, os, name, udid
	var model, osv, name, udid string
	lines := string(output)

	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				switch key {
				case "ProductType":
					model = value
				case "HumanReadableProductVersionString":
					osv = value
				case "DeviceName":
					name = value
				case "UniqueDeviceID":
					udid = value
				}
			}
		}
	}

	// Emit connected state to frontend
	info := map[string]string{
		"DeviceName": name,
		"Model":      model,
		"OSVersion":  osv,
		"Udid":       udid,
		"Connected":  "true",
	}
	runtime.EventsEmit(m.ctx, "deviceInfo", info)

	return &DeviceInfo{
		Name:    name,
		UDID:    udid,
		Model:   model,
		Version: osv,
	}
}

// GetUDID retrieves the UDID of the connected device
func (m *Manager) GetUDID() string {
	// Create object to run and capture output from idevice_id
	ideviceCMD := m.withBundledLibEnv(exec.Command(m.tools.IDeviceID))
	output, err := ideviceCMD.Output()
	if err != nil {
		m.logger("Error getting UDID: "+err.Error(), "helpers.Manager.GetUDID")
		return ""
	}

	// Convert output to string and return
	udid := strings.TrimSpace(strings.Replace(string(output), " (USB)", "", -1))
	m.logger("UDID retrieved: "+udid, "helpers.Manager.GetUDID")

	return udid
}

// GetInstalledApps retrieves list of installed apps from the device
func (m *Manager) GetInstalledApps(udid string) []InstalledApp {
	// If udid not provided, get it
	if udid == "" {
		udid = m.GetUDID()
	}

	// Create object to run and capture output from ideviceinstaller "ideviceinstaller", "-u", udid, "list", "--user"
	ideviceListCMD := m.withBundledLibEnv(exec.Command(m.tools.IDeviceInstaller, "-u", udid, "list", "--user"))

	// Set up pipes to capture stdout
	ideviceOut, _ := ideviceListCMD.StdoutPipe()
	ideviceListCMD.Start()

	outputBytes, _ := io.ReadAll(ideviceOut)
	ideviceListCMD.Wait()

	// Parse the output into a list of programs - output=(dk.plo.MinLaege, "3.8.2", "Min læge")
	var programs []InstalledApp
	lines := strings.Split(string(outputBytes), "\n")
	for i, line := range lines {
		// Skip the first line with the headers
		if i == 0 {
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		if len(parts) == 3 {
			program := InstalledApp{
				CFBundleIdentifier:  strings.TrimSpace(parts[0]),
				CFBundleDisplayName: strings.Trim(strings.TrimSpace(parts[2]), "\""),
			}
			programs = append(programs, program)
		}
	}
	m.logger(fmt.Sprintf("Found %d installed apps", len(programs)), "helpers.Manager.GetInstalledApps")

	return programs
}

// Helper function to Authenticate with Apple ID using ipatool
func (m *Manager) AuthenticateAppleID(email string, password string) error {
	m.logger("Authenticating Apple ID: "+email, "helpers.Manager.AuthenticateAppleID")

	ipatoolAuthCMD := m.withBundledLibEnv(exec.Command(m.tools.IPATool, "auth", "login", "--email", email, "--password", password))
	outputBytes, err := ipatoolAuthCMD.CombinedOutput()

	m.logger("Authentication output: "+string(outputBytes), "helpers.Manager.AuthenticateAppleID")
	if err != nil {
		m.logger("Authentication error: "+err.Error(), "helpers.Manager.AuthenticateAppleID")
	}
	return err
}

// Helper function to download an IPA from the App Store using ipatool
func (m *Manager) DownloadApp(bundleID string, email string, password string, pathToTmpDir string) error {
	m.logger("Downloading app with bundleID: "+bundleID, "helpers.Manager.DownloadApp")

	downloadPath := filepath.Join(pathToTmpDir, bundleID+".ipa")

	// authenticate with Apple ID before downloading
	if err := m.AuthenticateAppleID(email, password); err != nil {
		return fmt.Errorf("authenticate Apple ID: %w", err)
	}

	ipatoolCMD := m.withBundledLibEnv(exec.Command(m.tools.IPATool, "download", "--bundle-identifier", bundleID, "--output", downloadPath, "--purchase", "--verbose"))
	outputBytes, err := ipatoolCMD.CombinedOutput()
	m.logger("Download output: "+string(outputBytes), "helpers.Manager.DownloadApp")
	if err != nil {
		m.logger("Download failed: "+err.Error(), "helpers.Manager.DownloadApp")
		return fmt.Errorf("download IPA: %w", err)
	}
	m.logger("Downloaded IPA to: "+downloadPath, "helpers.Manager.DownloadApp")
	return nil
}

// Helper function to install an IPA on the connected iPhone
func (m *Manager) InstallApp(udid, pathToIpaFile string) error {
	m.logger("Installing App: "+pathToIpaFile+" on UDID: "+udid, "helpers.Manager.InstallApp")

	// get udid from App struct and if not set, use the passed udid
	if udid == "" {
		udid = m.GetUDID()
	}

	ideviceInstallCMD := m.withBundledLibEnv(exec.Command(m.tools.IDeviceInstaller, "-u", udid, "-w", "install", pathToIpaFile))
	outputBytes, err := ideviceInstallCMD.CombinedOutput()

	// Emit installation output to frontend
	runtime.EventsEmit(m.ctx, "installationOutput", string(outputBytes))

	m.logger("Installation output: "+string(outputBytes), "helpers.Manager.InstallApp")
	if err != nil {
		m.logger("Installation failed: "+err.Error(), "helpers.Manager.InstallApp")
		return fmt.Errorf("install IPA: %w", err)
	}
	return nil
}

// Calls DownloadApp and InstallApp in sequence, checking if the app is already installed
func (m *Manager) DownloadAndInstall(udid, bundleID string, email, password string, pathToTmpDir string) error {
	// Check if app is already installed
	installedApps := m.GetInstalledApps(udid)
	for _, app := range installedApps {
		if app.CFBundleIdentifier == bundleID {
			m.logger("App "+bundleID+" is already installed on device "+udid, "helpers.Manager.DownloadAndInstall")
			return nil
		}
	}
	// Download and install the app
	if err := m.DownloadApp(bundleID, email, password, pathToTmpDir); err != nil {
		return err
	}
	pathToIpaFile := filepath.Join(pathToTmpDir, bundleID+".ipa")
	if err := m.InstallApp(udid, pathToIpaFile); err != nil {
		return err
	}
	return nil
}

func (m *Manager) DownloadAndSaveAppIcon(url string, bundleID string) string {
	// Download the app icon from the provided URL and save it to a temporary location
	m.logger(fmt.Sprintf("Downloading app icon from URL: %s", url), "helpers.Manager.DownloadAndSaveAppIcon")
	resp, err := http.Get(url)
	if err != nil {
		m.logger(fmt.Sprintf("failed to download app icon: %v", err), "helpers.Manager.DownloadAndSaveAppIcon")
		return ""
	}
	defer resp.Body.Close()

	// append suffix to the icon file based on the URL extension
	suffix := ""
	switch {
	case strings.HasSuffix(url, ".png"):
		suffix += ".png"
	case strings.HasSuffix(url, ".jpg"), strings.HasSuffix(url, ".jpeg"):
		suffix += ".jpg"
	default:
		suffix += ".png" // Default to PNG if no extension found
	}

	iconPath := filepath.Join(m.tmpPath, fmt.Sprintf("%s_icon%s", bundleID, suffix))
	outFile, err := os.Create(iconPath)
	if err != nil {
		m.logger(fmt.Sprintf("failed to create icon file: %v", err), "helpers.Manager.DownloadAndSaveAppIcon")
		return ""
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("failed to save app icon: %v", err), "helpers.Manager.DownloadAndSaveAppIcon")
	}

	m.logger(fmt.Sprintf("App icon saved to: %s", iconPath), "helpers.Manager.DownloadAndSaveAppIcon")

	return iconPath
}

// OpenFile opens a file dialog
func (m *Manager) LoadIpaFile() string {
	// This function opens a file dialog to select an IPA file for install and analysis
	m.logger("Opening file dialog", "helpers.Manager.LoadIpaFile")
	filePath, err := runtime.OpenFileDialog(m.ctx, runtime.OpenDialogOptions{
		Title:            "Select a file",
		DefaultDirectory: m.tmpPath,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "IPA Files",
				Pattern:     "*.ipa",
			},
		},
	})
	if err != nil {
		m.logger(fmt.Sprintf("failed to open IPA file dialog: %v", err), "helpers.Manager.LoadIpaFile")
		return ""
	}

	return filePath
}

func (m *Manager) LoadAppList() string {
	// This function loads a list of apps from a CSV file and returns the content as an ordered list
	m.logger("Loading app list", "helpers.Manager.LoadAppList")
	filePath, err := runtime.OpenFileDialog(m.ctx, runtime.OpenDialogOptions{
		Title:            "Select a file",
		DefaultDirectory: m.tmpPath,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "CSV Files",
				Pattern:     "*.csv",
			},
		},
	})
	if err != nil {
		m.logger(fmt.Sprintf("failed to open app list dialog: %v", err), "helpers.Manager.LoadAppList")
		return ""
	}
	m.logger("Selected app list file: "+filePath, "helpers.Manager.LoadAppList")

	return filePath
}
