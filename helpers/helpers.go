package helpers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

// Manager handles device-related operations
type Manager struct {
	logger func(message, function string)
	ctx    context.Context
}

// NewManager creates a new helpers Manager
func NewManager(logger func(message, function string), ctx context.Context) *Manager {
	return &Manager{
		logger: logger,
		ctx:    ctx,
	}
}

// GetInfo retrieves device information using ideviceinfo
func (m *Manager) GetInfo() *DeviceInfo {
	// Get info from connected iPhone using ideviceinfo
	ideviceInfoCMD := exec.Command("ideviceinfo", "-s")
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
	ideviceCMD := exec.Command("idevice_id")
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

	// Create object to run and capture output from ideviceinstaller
	ideviceListCMD := exec.Command("ideviceinstaller", "-u", udid, "list", "--user")

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

// Helper function to download an IPA from the App Store using ipatool
func (m *Manager) DownloadApp(bundleID string, email string, password string) {
	m.logger("Downloading app with bundleID: "+bundleID, "helpers.Manager.DownloadApp")

	installPath := "tmp/" + bundleID + ".ipa"

	ipatoolAuthCMD := exec.Command("ipatool", "auth", "login", "--email", email, "--password", password)
	ipatoolAuthCMD.Start()
	ipatoolAuthCMD.Wait()

	// Create object to run and capture output from ipatool.
	ipatoolCMD := exec.Command("ipatool", "download", "--bundle-identifier", bundleID, "--output", installPath, "--purchase", "--verbose")
	// Set up pipes to capture stdout and stderr
	ipatoolOut, _ := ipatoolCMD.StdoutPipe()
	ipatoolCMD.Start()

	outputBytes, _ := io.ReadAll(ipatoolOut)

	// Log download output
	m.logger("Downloaded IPA to: "+installPath, "helpers.Manager.DownloadApp")

	ipatoolCMD.Wait()
	m.logger("Download output: "+string(outputBytes), "helpers.Manager.DownloadApp")
}

// Helper function to install an IPA on the connected iPhone
func (m *Manager) InstallApp(udid, installPath string) {
	m.logger("Installing App: "+installPath+" on UDID: "+udid, "helpers.Manager.InstallApp")

	// get udid from App struct and if not set, use the passed udid
	if udid == "" {
		udid = m.GetUDID()
	}

	//Create object to run and capture output from ideviceinstaller.
	ideviceInstallCMD := exec.Command("ideviceinstaller", "-u", udid, "-w", "install", installPath)

	// Set up pipes to capture stdout and stderr
	ideviceIn, _ := ideviceInstallCMD.StdinPipe()
	ideviceOut, _ := ideviceInstallCMD.StdoutPipe()
	//ideviceErr, _ := ideviceInstallCMD.StderrPipe()
	ideviceInstallCMD.Start()

	ideviceIn.Close()
	outputBytes, _ := io.ReadAll(ideviceOut)

	// Emit installation output to frontend
	runtime.EventsEmit(m.ctx, "installationOutput", string(outputBytes))

	ideviceInstallCMD.Wait()
	m.logger("Installation output: "+string(outputBytes), "helpers.Manager.InstallApp")
	//errBytes, _ := io.ReadAll(ideviceErr)

	//return string(outputBytes)
}

func (m *Manager) DownloadAndInstall(udid, bundleID string, email, password string) {
	// Check if app is already installed
	installedApps := m.GetInstalledApps(udid)
	for _, app := range installedApps {
		if app.CFBundleIdentifier == bundleID {
			m.logger("App "+bundleID+" is already installed on device "+udid, "helpers.Manager.DownloadAndInstall")
			return
		}
	}

	// Download and install the app
	m.DownloadApp(bundleID, email, password)
	installPath := "tmp/" + bundleID + ".ipa"
	m.InstallApp(udid, installPath)
}

func (m *Manager) DownloadAndSaveAppIcon(url string, bundleID string) (string, error) {
	// Download the app icon from the provided URL and save it to a temporary location
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download app icon: %v", err)
	}
	defer resp.Body.Close()

	iconPath := fmt.Sprintf("tmp/%s_icon.png", bundleID)
	outFile, err := os.Create(iconPath)
	if err != nil {
		return "", fmt.Errorf("failed to create icon file: %v", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save app icon: %v", err)
	}

	return iconPath, nil
}
