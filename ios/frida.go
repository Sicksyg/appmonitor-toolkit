package ios

import (
	"AppMonitor/models"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frida/frida-go/frida"
)

// FridaData holds the active Frida device and session.
type FridaData struct {
	device  *frida.Device
	session *frida.Session
	script  *frida.Script
	pid     int
	spawned bool
}

const safariBundleID = "com.apple.mobilesafari"

// ResumeApp resumes a process suspended by Frida.
func (m *Manager) ResumeApp() error {
	if m.fridaData == nil {
		return fmt.Errorf("frida not initialized, call FridaSetup first")
	}
	if err := m.fridaData.device.Resume(m.fridaData.pid); err != nil {
		m.logger("Error resuming app: "+err.Error(), "Manager.ResumeApp")
		return fmt.Errorf("failed to resume app: %w", err)
	}
	m.logger("App resumed", "Manager.ResumeApp")
	return nil
}

func (m *Manager) checkFridaServer(device *frida.Device) error {
	// Check if frida server is running by enumerating processes on the device
	processes, err := device.EnumerateProcesses(frida.ScopeMinimal)
	if err != nil {
		m.logger("Error enumerating processes: "+err.Error(), "Manager.checkFridaServer")
		return fmt.Errorf("failed to enumerate processes: %w", err)
	}

	// check for the process: frida-server
	for _, proc := range processes {
		if proc.Name() == "frida-server" {
			m.logger("Frida server is running", "Manager.checkFridaServer")
			return nil
		}

	}
	return nil
}

// FridaSetup sets up frida for the given device UDID and app bundleID
func (m *Manager) FridaSetup(udid string, bundleID string) error {
	// This function sets up frida for the given bundleID

	m.logger(fmt.Sprintf("Setting up Frida for device UDID: %s and bundleID: %s", udid, bundleID), "Manager.FridaSetup")

	// Setup frida device manager
	mgr := frida.NewDeviceManager()

	// Enumerate devices
	devices, err := mgr.EnumerateDevices()
	if err != nil {
		m.logger("Error enumerating devices: "+err.Error(), "Manager.FridaSetup")
		return fmt.Errorf("failed to enumerate devices: %w", err)
	}
	m.logger(fmt.Sprintf("Found %d devices", len(devices)), "Manager.FridaSetup")
	_ = devices // device list currently unused

	// get device by UDID
	device, err := mgr.DeviceByID(udid)
	if err != nil {
		m.logger("Error getting device by ID: "+err.Error(), "Manager.FridaSetup")
		return fmt.Errorf("failed to get device by ID: %w", err)
	}
	m.logger(fmt.Sprintf("Using device: %s (%s)", device.Name(), device.ID()), "Manager.FridaSetup")

	// Spawn the app and get its PID from the bundle ID.
	pid, err := device.Spawn(bundleID, nil)
	if err != nil {
		m.logger("Error spawning app: "+err.Error(), "Manager.FridaSetup")
		m.checkFridaServer(device.(*frida.Device)) // Check if frida server is running and log processes for debugging
		return fmt.Errorf("failed to spawn app, make sure the frida server is running. If not, add the repo and install the frida-server in Sileo: %w", err)
	}
	m.logger(fmt.Sprintf("Spawned app with PID: %d", pid), "Manager.FridaSetup")

	// sleep for a 2 seconds to ensure the app is fully spawned before attaching
	time.Sleep(2 * time.Second)

	// Attach to app using the pid from above
	m.logger("Attaching to the app...", "Manager.FridaSetup")
	session, err := device.Attach(pid, nil)
	if err != nil {
		m.logger("Error attaching to app: "+err.Error(), "Manager.FridaSetup")
		return fmt.Errorf("failed to attach to app: %w", err)
	}

	// Surfaces the real cause (e.g. app self-killed on Frida detection) instead of a generic "session is gone" error later
	session.On("detached", func(reason frida.SessionDetachReason, crash *frida.Crash) {
		m.logger(fmt.Sprintf("Session detached: reason=%s crash=%v", reason, crash), "Manager.FridaSetup")
	})

	// Set frida data struct
	m.fridaData = &FridaData{
		device:  device.(*frida.Device),
		session: session,
		pid:     pid,
		spawned: true,
	}

	m.logger("Frida setup completed successfully", "Manager.FridaSetup")
	return nil
}

// --------------------------- Permission analysis functions ----------------------------- //
// AnalyseFridaPermissions analyses the app using frida to detect permissions used
func (m *Manager) AnalyseFridaPermissions(bundleID string) (map[string]string, error) {
	if m.fridaData == nil {
		return nil, fmt.Errorf("frida not initialized, call FridaSetup first")
	}

	// Path to frida project must be an absolute path on the local filesystem using path package
	// projectRoot, err := filepath.Abs(m.fridaRoot)
	// if err != nil {
	// 	m.logger("Error getting absolute path: "+err.Error(), "Manager.AnalyseFridaPermissions")
	// 	return nil, fmt.Errorf("failed to get absolute path: %w", err)
	// }

	comp := frida.NewCompiler()
	comp.On("diagnostics", func(diag string) {
		m.logger("Compiler diagnostics: "+diag, "Manager.AnalyseFridaPermissions")
	})

	bopts := frida.NewCompilerOptions()
	bopts.SetProjectRoot(m.fridaRoot)
	bopts.SetSourceMaps(frida.SourceMapsOmitted)
	bopts.SetJSCompression(frida.JSCompressionTerser)

	compiledScript, err := comp.Build("frida_permissions.js", bopts)
	if err != nil {
		m.logger("Error compiling script: "+err.Error(), "Manager.AnalyseFridaPermissions")
		return nil, fmt.Errorf("failed to compile script: %w", err)
	}

	// Create Frida script
	fridaScript, err := m.fridaData.session.CreateScript(compiledScript)
	if err != nil {
		m.logger("Error creating Permission script: "+err.Error(), "Manager.AnalyseFridaPermissions")
		return nil, fmt.Errorf("failed to create script: %w", err)
	}
	defer fridaScript.Clean()

	// Channel to receive permissions results
	permissionsChan := make(chan map[string]string, 1)

	// Set up message handler
	fridaScript.On("message", func(msg string) {
		if permissions := m.parsePermissionsResults(msg); permissions != nil {
			permissionsChan <- permissions
		}
	})

	// Load Frida script into the session
	if err := fridaScript.Load(); err != nil {
		m.logger("Error loading script: "+err.Error(), "Manager.AnalyseFridaPermissions")
		return nil, fmt.Errorf("failed to load script: %w", err)
	}

	// Wait for results with timeout
	select {
	case permissions := <-permissionsChan:
		m.logger(fmt.Sprintf("Found %d permissions", len(permissions)), "Manager.AnalyseFridaPermissions")
		return permissions, nil
	case <-time.After(5 * time.Second):
		m.logger("Timeout waiting for permissions results", "Manager.AnalyseFridaPermissions")
		return make(map[string]string), nil // Return empty map instead of error on timeout
	}
}

// parsePermissionsResults parses the Frida message and extracts permissions
func (m *Manager) parsePermissionsResults(results string) map[string]string {
	if results == "" {
		return nil
	}

	// Parse json to get the payload
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(results), &msg); err != nil {
		m.logger("Error parsing permissions results JSON: "+err.Error(), "Manager.parsePermissionsResults")
		return nil
	}

	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		m.logger("Error: payload is not a map", "Manager.parsePermissionsResults")
		return nil
	}

	// Convert to map[string]string
	permissions := make(map[string]string)
	for permission, description := range payload {
		if strVal, ok := description.(string); ok {
			permissions[permission] = strVal
		}
	}

	return permissions
}
func (m *Manager) AnalyseFridaStatic(bundleID string) ([]string, error) {
	if m.fridaData == nil {
		return nil, fmt.Errorf("frida not initialized, call FridaSetup first")
	}

	// Path to frida project must be an absolute path on the local filesystem using path package
	// projectRoot, err := filepath.Abs("./frida")
	// if err != nil {
	// 	m.logger("Error getting absolute path: "+err.Error(), "Manager.AnalyseFridaStatic")
	// 	return nil, fmt.Errorf("failed to get absolute path: %w", err)
	// }

	comp := frida.NewCompiler()
	comp.On("diagnostics", func(diag string) {
		m.logger("Compiler diagnostics: "+diag, "Manager.AnalyseFridaStatic")
	})

	bopts := frida.NewCompilerOptions()
	bopts.SetProjectRoot(m.fridaRoot)
	bopts.SetSourceMaps(frida.SourceMapsOmitted)
	bopts.SetJSCompression(frida.JSCompressionTerser)

	compiledScript, err := comp.Build("find_all_classes.ts", bopts)
	if err != nil {
		m.logger("Error compiling static script: "+err.Error(), "Manager.AnalyseFridaStatic")
		return nil, fmt.Errorf("failed to compile script: %w", err)
	}

	// Create Frida script
	fridaScript, err := m.fridaData.session.CreateScript(compiledScript)
	if err != nil {
		m.logger("Error creating static script: "+err.Error(), "Manager.AnalyseFridaStatic")
		return nil, fmt.Errorf("failed to create script: %w", err)
	}
	defer fridaScript.Clean()

	// Channel to receive class list results
	classListChan := make(chan []string, 1)

	// Set up message handler
	fridaScript.On("message", func(msg string) {
		if classList := m.parseStaticResults(msg); classList != nil {
			classListChan <- classList
		}
	})

	// Load Frida script into the session
	if err := fridaScript.Load(); err != nil {
		m.logger("Error loading script: "+err.Error(), "Manager.AnalyseFridaStatic")
		return nil, fmt.Errorf("failed to load script: %w", err)
	}

	// Wait for results with timeout
	select {
	case classList := <-classListChan:
		if logPath, saveErr := m.saveClassListLog(bundleID, classList); saveErr != nil {
			m.logger("Warning: failed to save class log: "+saveErr.Error(), "Manager.AnalyseFridaStatic")
		} else {
			m.logger("Class log saved to: "+logPath, "Manager.AnalyseFridaStatic")
		}
		m.logger(fmt.Sprintf("Found %d classes", len(classList)), "Manager.AnalyseFridaStatic")
		return classList, nil
	case <-time.After(10 * time.Second):
		m.logger("Timeout waiting for static analysis results", "Manager.AnalyseFridaStatic")
		return nil, fmt.Errorf("timeout waiting for results")
	}
}

// parseStaticResults parses the Frida message and extracts the class list
func (m *Manager) parseStaticResults(results string) []string {
	if results == "" {
		return nil
	}

	// Parse the results in json to extract the payload. This is Frida logic https://frida.re/docs/messages/
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(results), &msg); err != nil {
		m.logger("Error parsing analysis results JSON: "+err.Error(), "Manager.parseStaticResults")
		return nil
	}

	payload, ok := msg["payload"].([]interface{})
	if !ok {
		m.logger("Error: payload is not an array", "Manager.parseStaticResults")
		return nil
	}

	// Convert to string slice
	classList := make([]string, 0, len(payload))
	for _, item := range payload {
		if className, ok := item.(string); ok {
			classList = append(classList, className)
		}
	}

	return classList
}
func (m *Manager) saveClassListLog(bundleID string, classList []string) (string, error) {
	if len(classList) == 0 {
		return "", nil
	}

	classLogDir := m.classLogPath
	if err := os.MkdirAll(classLogDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create class log directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("%s_classlog_%s.txt", bundleID, timestamp)
	filePath := filepath.Join(classLogDir, fileName)
	contents := strings.Join(classList, "\n") + "\n"

	if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
		return "", fmt.Errorf("failed to write class log file: %w", err)
	}

	return filePath, nil
}

// LoadSDKSignatures loads the SDK signatures from the JSON file or fetches from GitHub if not present
func (m *Manager) GetBundleInformation() (map[string]any, error) {
	if m.fridaData == nil {
		return nil, fmt.Errorf("frida not initialized, call FridaSetup first")
	}

	comp := frida.NewCompiler()
	comp.On("diagnostics", func(diag string) {
		m.logger("Compiler diagnostics: "+diag, "Manager.GetBundleInformation")
	})

	bopts := frida.NewCompilerOptions()
	bopts.SetProjectRoot(m.fridaRoot)
	bopts.SetSourceMaps(frida.SourceMapsOmitted)
	bopts.SetJSCompression(frida.JSCompressionTerser)

	compiledScript, err := comp.Build("frida_get_bundledata.js", bopts)
	if err != nil {
		m.logger("Error compiling script: "+err.Error(), "Manager.GetBundleInformation")
		return nil, fmt.Errorf("failed to compile script: %w", err)
	}

	fridaScript, err := m.fridaData.session.CreateScript(compiledScript)
	if err != nil {
		m.logger("Error creating bundle information script: "+err.Error(), "Manager.GetBundleInformation")
		return nil, fmt.Errorf("failed to create script: %w", err)
	}
	defer fridaScript.Clean()

	bundleInfoChan := make(chan map[string]any, 1)
	fridaScript.On("message", func(msg string) {
		if bundleInfo := m.parseBundleInformation(msg); bundleInfo != nil {
			bundleInfoChan <- bundleInfo
		}
	})

	if err := fridaScript.Load(); err != nil {
		m.logger("Error loading bundle information script: "+err.Error(), "Manager.GetBundleInformation")
		return nil, fmt.Errorf("failed to load script: %w", err)
	}

	select {
	case bundleInfo := <-bundleInfoChan:
		m.logger(fmt.Sprintf("Found %d bundle information fields", len(bundleInfo)), "Manager.GetBundleInformation")
		return bundleInfo, nil
	case <-time.After(5 * time.Second):
		m.logger("Timeout waiting for bundle information", "Manager.GetBundleInformation")
		return make(map[string]any), nil
	}
}
func (m *Manager) parseBundleInformation(results string) map[string]any {
	if results == "" {
		return nil
	}

	var msg struct {
		Type    string         `json:"type"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(results), &msg); err != nil {
		m.logger("Error parsing bundle information JSON: "+err.Error(), "Manager.parseBundleInformation")
		return nil
	}
	if msg.Type != "send" || msg.Payload == nil {
		return nil
	}

	return msg.Payload
}

// RunCompleteAnalysis runs the full Frida analysis workflow: setup, permissions, bundle data, static analysis, and SDK detection.
func (m *Manager) RunCompleteAnalysis(udid, bundleID string) (map[string]models.IosPermissionDetail, map[string][]string, map[string]any, error) {
	// Step 1: Setup Frida
	if err := m.FridaSetup(udid, bundleID); err != nil {
		return nil, nil, nil, fmt.Errorf("frida setup failed: %w", err)
	}

	time.Sleep(time.Second * 1) // brief pause to ensure app is fully started

	// Step 2: Run static analysis to get class list (works while suspended)
	classList, err := m.AnalyseFridaStatic(bundleID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("static analysis failed: %w", err)
	}

	// Step 3: Resume immediately so the OS launch watchdog doesn't kill the
	// still-suspended process; permission hooks also require the app to run.
	if err := m.ResumeApp(); err != nil {
		return nil, nil, nil, fmt.Errorf("resume failed: %w", err)
	}

	// Step 4: Analyze permissions (raw from Frida)
	rawPermissions, err := m.AnalyseFridaPermissions(bundleID)
	if err != nil {
		m.logger("Permissions analysis failed: "+err.Error(), "Manager.RunCompleteAnalysis")
		// Continue with other analyses even if permissions fail
		rawPermissions = make(map[string]string)
	}

	// Step 5: Collect Info.plist metadata while the instrumented app is running.
	bundleInfo, err := m.GetBundleInformation()
	if err != nil {
		m.logger("Bundle information analysis failed: "+err.Error(), "Manager.RunCompleteAnalysis")
		bundleInfo = make(map[string]any)
	}

	// Step 6: Detect SDKs from class list (CPU-only, safe to run after resume)
	sdks := m.AnalyseDetectSDKs(classList)

	// Step 7: Enrich permissions with Apple signature data
	enrichedPermissions, err := m.AnalyseDetectPermissions(rawPermissions)
	if err != nil {
		m.logger("Permission enrichment failed: "+err.Error(), "Manager.RunCompleteAnalysis")
		// Continue even if permission enrichment fails
		enrichedPermissions = make(map[string]models.IosPermissionDetail)
	}

	m.logger("Complete analysis finished successfully", "Manager.RunCompleteAnalysis")
	return enrichedPermissions, sdks, bundleInfo, nil
}

// AnalyseDetectSDKs detects SDKs in the given class list using signature matching
func (m *Manager) Cleanup() error {
	if m.fridaData == nil {
		return nil
	}

	var errs []error

	// Detach session
	if m.fridaData.session != nil {
		if err := m.fridaData.session.Detach(); err != nil {
			m.logger("Error detaching session: "+err.Error(), "Manager.Cleanup")
			errs = append(errs, err)
		}
		m.fridaData.session.Clean()
	}

	// Kill only processes spawned by this manager.
	if m.fridaData.spawned && m.fridaData.device != nil && m.fridaData.pid > 0 {
		if err := m.fridaData.device.Kill(m.fridaData.pid); err != nil {
			m.logger("Error killing app: "+err.Error(), "Manager.Cleanup")
			errs = append(errs, err)
		}
	}

	m.fridaData = nil

	if len(errs) > 0 {
		return fmt.Errorf("cleanup encountered %d error(s)", len(errs))
	}
	return nil
}
func (m *Manager) OpenAppInAppStore(udid string, appStoreURL string) error {
	// Function to open the appstore on ios device using frida.
	// "trackViewUrl": "https://apps.apple.com/dk/app/mobilbank-middelfartsparekasse/id1466762662?uo=4"
	if strings.TrimSpace(udid) == "" {
		m.logger("No device UDID supplied; FridaSetup requires an explicit UDID", "Manager.OpenAppInAppStore")
		return fmt.Errorf("no device UDID supplied")
	}

	// Step 1: Setup Frida
	if err := m.FridaSetup(udid, safariBundleID); err != nil {
		m.logger("Frida setup failed: "+err.Error(), "Manager.OpenAppInAppStore")
		return fmt.Errorf("frida setup: %w", err)
	}
	defer func() {
		if err := m.Cleanup(); err != nil {
			m.logger("Frida cleanup failed: "+err.Error(), "Manager.OpenAppInAppStore")
		}
	}()

	// Frida spawns apps suspended. Resume Safari before loading the script and
	// calling the RPC so UIApplication can execute the URL open request.
	if err := m.ResumeApp(); err != nil {
		m.logger("Safari resume failed: "+err.Error(), "Manager.OpenAppInAppStore")
		return fmt.Errorf("resume Safari: %w", err)
	}

	comp := frida.NewCompiler()
	comp.On("diagnostics", func(diag string) {
		m.logger("Compiler diagnostics: "+diag, "Manager.OpenAppInAppStore")
	})

	bopts := frida.NewCompilerOptions()
	bopts.SetProjectRoot(m.fridaRoot)
	bopts.SetSourceMaps(frida.SourceMapsOmitted)
	bopts.SetJSCompression(frida.JSCompressionTerser)

	compiledScript, err := comp.Build("frida_open_appstore.js", bopts)
	if err != nil {
		m.logger("Error compiling script: "+err.Error(), "Manager.OpenAppInAppStore")
		return fmt.Errorf("compile App Store script: %w", err)
	}

	// Create and load the script before invoking its RPC exports.
	fridaScript, err := m.fridaData.session.CreateScript(compiledScript)
	if err != nil {
		m.logger("Error creating App Store script: "+err.Error(), "Manager.OpenAppInAppStore")
		return fmt.Errorf("create App Store script: %w", err)
	}
	defer fridaScript.Clean()

	if err := fridaScript.Load(); err != nil {
		m.logger("Error loading App Store script: "+err.Error(), "Manager.OpenAppInAppStore")
		return fmt.Errorf("load App Store script: %w", err)
	}

	rpcContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	activation := fridaScript.ExportsCallWithContext(rpcContext, "openurl", appStoreURL)
	opened, ok := activation.(bool)
	if !ok || !opened {
		m.logger(fmt.Sprintf("App Store RPC failed: %v", activation), "Manager.OpenAppInAppStore")
		return fmt.Errorf("App Store RPC did not open URL")
	}

	m.logger(fmt.Sprintf("App Store RPC result: %v", activation), "Manager.OpenAppInAppStore")
	return nil
}

// --------------------------- iOS installation and TrackerScan -----------------------------
