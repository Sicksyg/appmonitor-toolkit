package analysis

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/frida/frida-go/frida"

	"AppMonitor/models"
)

// FridaData struct to hold frida related data to share across methods
type FridaData struct {
	device  *frida.Device
	session *frida.Session
	script  *frida.Script
	pid     int
}

// Signature struct to hold SDK signature information
type SDKSignature struct {
	Regex       string `json:"regex"`
	DomainRegex string `json:"domain_regex"`
	Name        string `json:"name"`
	Comment     string `json:"comment"`
	ID          int    `json:"id"`
}

// ApplePermissionSignature struct to hold permission signature information
type ApplePermissionSignature struct {
	PlistKey       string `json:"plkey"`
	CommonName     string `json:"commonName"`
	IosDescription string `json:"description"`
	Category       string `json:"category"`
}

// Manager struct to handle analysis operations
type Manager struct {
	logger    func(message, function string)
	fridaData *FridaData
}

// NewManager creates a new analysis Manager
func NewManager(logger func(message, function string)) *Manager {
	return &Manager{
		logger: logger,
	}
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

	// Spawn app and get the pid from the bundleID
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

	// Set frida data struct
	m.fridaData = &FridaData{
		device:  device.(*frida.Device),
		session: session,
		pid:     pid,
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
	projectRoot, err := filepath.Abs("./frida")
	if err != nil {
		m.logger("Error getting absolute path: "+err.Error(), "Manager.AnalyseFridaPermissions")
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	comp := frida.NewCompiler()
	comp.On("diagnostics", func(diag string) {
		m.logger("Compiler diagnostics: "+diag, "Manager.AnalyseFridaPermissions")
	})

	bopts := frida.NewCompilerOptions()
	bopts.SetProjectRoot(projectRoot)
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
		m.logger("Error creating script: "+err.Error(), "Manager.AnalyseFridaPermissions")
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

	// Resume app from suspended state
	if err := m.fridaData.device.Resume(m.fridaData.pid); err != nil {
		m.logger("Error resuming app: "+err.Error(), "Manager.AnalyseFridaPermissions")
		return nil, fmt.Errorf("failed to resume app: %w", err)
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

func (m *Manager) LoadPermissionsSignatures() []ApplePermissionSignature {
	// Load permissions signatures from JSON file or other source
	// Get permissions signatures from https://github.com/Sicksyg/iOS_ProtectedResources/blob/main/ios_ProtectedResources.json
	// Return slice of PermissionSignature structs

	m.logger("Loading permissions signatures", "Manager.LoadPermissionsSignatures")
	// Check if signatures file exists, if not fetch from GitHub
	sigPath := filepath.Join("tmp", "ios_permissions.json")
	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		m.logger("Permission file not found, fetching from GitHub", "Manager.LoadPermissionsSignatures")
		url := "https://raw.githubusercontent.com/Sicksyg/iOS_ProtectedResources/main/ios_ProtectedResources.json"
		resp, err := http.Get(url)
		if err != nil {
			m.logger("Error fetching permissions signatures from GitHub: "+err.Error(), "Manager.LoadPermissionsSignatures")
			return []ApplePermissionSignature{}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			m.logger("Error reading GitHub response: "+err.Error(), "Manager.LoadPermissionsSignatures")
			return []ApplePermissionSignature{}
		}
		err = os.WriteFile(sigPath, body, 0644)
		if err != nil {
			m.logger("Error writing permissions signatures to file: "+err.Error(), "Manager.LoadPermissionsSignatures")
			return []ApplePermissionSignature{}
		}
		m.logger("Permissions signatures downloaded and saved", "Manager.LoadPermissionsSignatures")
	}

	// Open the JSON file
	signatures, err := os.Open(filepath.Join("tmp", "ios_permissions.json"))
	if err != nil {
		m.logger("Error opening permissions signatures file: "+err.Error(), "Manager.LoadPermissionsSignatures")
		return []ApplePermissionSignature{}
	}
	defer signatures.Close()

	// Read file contents
	fileData, err := io.ReadAll(signatures)
	if err != nil {
		m.logger("Error reading permissions signatures file: "+err.Error(), "Manager.LoadPermissionsSignatures")
		return []ApplePermissionSignature{}
	}

	// Unmarshal JSON data into a map with "permissions" key
	var jsonData map[string]map[string]ApplePermissionSignature
	err = json.Unmarshal(fileData, &jsonData)
	if err != nil {
		m.logger("Error unmarshaling permissions signatures: "+err.Error(), "Manager.LoadPermissionsSignatures")
		return []ApplePermissionSignature{}
	}

	// Extract permissions from the nested structure
	var applePermissionSignatures []ApplePermissionSignature
	if permissions, ok := jsonData["permissions"]; ok {
		for _, perm := range permissions {
			applePermissionSignatures = append(applePermissionSignatures, perm)
		}
	}

	return applePermissionSignatures
}

func (m *Manager) AnalyseDetectPermissions(appPermissions map[string]string) (map[string]models.PermissionDetail, error) {
	// Load permissions signatures from Apple
	applePermissionSignatures := m.LoadPermissionsSignatures()

	// Create a map for quick lookup: plkey -> ApplePermissionSignature
	appleSigMap := make(map[string]ApplePermissionSignature)
	for _, sig := range applePermissionSignatures {
		appleSigMap[sig.PlistKey] = sig
	}

	// Build enriched permissions map by cross-referencing with Apple signatures
	enrichedPermissions := make(map[string]models.PermissionDetail)

	for permKey, developerDesc := range appPermissions {
		detail := models.PermissionDetail{
			DeveloperDescription: developerDesc,
		}

		// Look up the permission in Apple's signature database
		if appleInfo, found := appleSigMap[permKey]; found {
			detail.CommonName = appleInfo.CommonName
			detail.AppleDescription = appleInfo.IosDescription
			detail.Category = appleInfo.Category
			detail.PlistKey = permKey
		} else {
			// If not found in Apple's database, use the plist key as common name
			detail.CommonName = permKey
			detail.AppleDescription = "Unknown permission"
			m.logger(fmt.Sprintf("Permission %s not found in Apple signatures", permKey), "Manager.AnalyseDetectPermissions")
		}

		enrichedPermissions[permKey] = detail
	}

	m.logger(fmt.Sprintf("Detected and enriched %d permissions", len(enrichedPermissions)), "Manager.AnalyseDetectPermissions")
	return enrichedPermissions, nil
}

// --------------------------- StaticSDK analysis functions ----------------------------- //
// AnalyseFridaStatic analyses the app using frida to get the list of classes

func (m *Manager) AnalyseFridaStatic(bundleID string) ([]string, error) {
	if m.fridaData == nil {
		return nil, fmt.Errorf("frida not initialized, call FridaSetup first")
	}

	// Path to frida project must be an absolute path on the local filesystem using path package
	projectRoot, err := filepath.Abs("./frida")
	if err != nil {
		m.logger("Error getting absolute path: "+err.Error(), "Manager.AnalyseFridaStatic")
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	comp := frida.NewCompiler()
	comp.On("diagnostics", func(diag string) {
		m.logger("Compiler diagnostics: "+diag, "Manager.AnalyseFridaStatic")
	})

	bopts := frida.NewCompilerOptions()
	bopts.SetProjectRoot(projectRoot)
	bopts.SetSourceMaps(frida.SourceMapsOmitted)
	bopts.SetJSCompression(frida.JSCompressionTerser)

	compiledScript, err := comp.Build("find_all_classes.ts", bopts)
	if err != nil {
		m.logger("Error compiling script: "+err.Error(), "Manager.AnalyseFridaStatic")
		return nil, fmt.Errorf("failed to compile script: %w", err)
	}

	// Create Frida script
	fridaScript, err := m.fridaData.session.CreateScript(compiledScript)
	if err != nil {
		m.logger("Error creating script: "+err.Error(), "Manager.AnalyseFridaStatic")
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

// LoadSDKSignatures loads the SDK signatures from the JSON file or fetches from GitHub if not present
func (m *Manager) LoadSDKSignatures() []SDKSignature {
	// Get SDK signatures from https://github.com/Sicksyg/iOS-SDK-Signatures/blob/main/ios_signatures.json

	m.logger("Loading SDK signatures", "Manager.detectSDKs")
	// Check if signatures file exists, if not fetch from GitHub
	sigPath := filepath.Join("tmp", "ios_signatures.json")
	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		m.logger("SDK signatures file not found, fetching from GitHub", "Manager.LoadSDKSignatures")
		url := "https://raw.githubusercontent.com/Sicksyg/iOS-SDK-Signatures/main/ios_signatures.json"
		resp, err := http.Get(url)
		if err != nil {
			m.logger("Error fetching SDK signatures from GitHub: "+err.Error(), "Manager.LoadSDKSignatures")
			return []SDKSignature{}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			m.logger("Error reading GitHub response: "+err.Error(), "Manager.LoadSDKSignatures")
			return []SDKSignature{}
		}
		err = os.WriteFile(sigPath, body, 0644)
		if err != nil {
			m.logger("Error writing SDK signatures to file: "+err.Error(), "Manager.LoadSDKSignatures")
			return []SDKSignature{}
		}
		m.logger("SDK signatures downloaded and saved", "Manager.LoadSDKSignatures")
	}

	// Open the JSON file
	signatures, err := os.Open(filepath.Join("tmp", "ios_signatures.json"))
	if err != nil {
		m.logger("Error opening SDK signatures file: "+err.Error(), "Manager.LoadSDKSignatures")
		return []SDKSignature{}
	}
	defer signatures.Close()

	// Read file contents
	fileData, err := io.ReadAll(signatures)
	if err != nil {
		m.logger("Error reading SDK signatures file: "+err.Error(), "Manager.LoadSDKSignatures")
		return []SDKSignature{}
	}

	// Unmarshal JSON data into slice of Signature structs
	var sdkSignatures []SDKSignature
	err = json.Unmarshal(fileData, &sdkSignatures)
	if err != nil {
		fmt.Println("Error unmarshaling SDK signatures: " + err.Error())
		return []SDKSignature{}
	}

	return sdkSignatures
}

// RunCompleteAnalysis runs the full Frida analysis workflow: setup, permissions, static analysis, and SDK detection
func (m *Manager) RunCompleteAnalysis(udid, bundleID string) (map[string]models.PermissionDetail, map[string][]string, error) {
	// Step 1: Setup Frida
	if err := m.FridaSetup(udid, bundleID); err != nil {
		return nil, nil, fmt.Errorf("frida setup failed: %w", err)
	}

	time.Sleep(time.Second * 1) // brief pause to ensure app is fully started

	// Step 2: Analyze permissions (raw from Frida)
	rawPermissions, err := m.AnalyseFridaPermissions(bundleID)
	if err != nil {
		m.logger("Permissions analysis failed: "+err.Error(), "Manager.RunCompleteAnalysis")
		// Continue with other analyses even if permissions fail
		rawPermissions = make(map[string]string)
	}

	time.Sleep(time.Second * 1) // brief pause to ensure app is fully started

	// Step 3: Run static analysis to get class list
	classList, err := m.AnalyseFridaStatic(bundleID)
	if err != nil {
		return nil, nil, fmt.Errorf("static analysis failed: %w", err)
	}

	time.Sleep(time.Second * 1) // brief pause to ensure app is fully started

	// Step 4: Detect SDKs from class list
	sdks := m.AnalyseDetectSDKs(classList)

	time.Sleep(time.Second * 1) // brief pause to ensure app is fully started

	// Step 5: Enrich permissions with Apple signature data
	enrichedPermissions, err := m.AnalyseDetectPermissions(rawPermissions)
	if err != nil {
		m.logger("Permission enrichment failed: "+err.Error(), "Manager.RunCompleteAnalysis")
		// Continue even if permission enrichment fails
		enrichedPermissions = make(map[string]models.PermissionDetail)
	}

	m.logger("Complete analysis finished successfully", "Manager.RunCompleteAnalysis")
	return enrichedPermissions, sdks, nil
}

// AnalyseDetectSDKs detects SDKs in the given class list using signature matching
func (m *Manager) AnalyseDetectSDKs(classlist []string) map[string][]string {
	// fmt.Printf("Number of classes in analysis results: %d\n", len(classList))
	// fmt.Printf("This is a class list snippet: %v\n", classList[len(classList)-5:])

	// Load signatures
	sdkSignatures := m.LoadSDKSignatures()

	// Compile regex signatures from loaded signatures
	compiledSignatures := []struct {
		Signature SDKSignature
		Regex     *regexp.Regexp
	}{}

	for _, sig := range sdkSignatures {
		regex, err := regexp.Compile(sig.Regex)
		if err != nil {
			fmt.Println("Error compiling regex: " + err.Error())
			continue
		}
		compiledSignatures = append(compiledSignatures, struct {
			Signature SDKSignature
			Regex     *regexp.Regexp
		}{
			Signature: sig,
			Regex:     regex,
		})
	}

	// Detect SDKs in class list using goroutines
	type Detection struct {
		SDKName   string
		ClassName string
	}

	// Set up worker pool and channels for concurrent processing, Limit number of workers to avoid overwhelming the system
	numWorkers := 4
	detectionsChan := make(chan Detection, numWorkers)
	sigChan := make(chan struct {
		Signature SDKSignature
		Regex     *regexp.Regexp
	}, len(compiledSignatures))

	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sig := range sigChan {
				for _, className := range classlist {
					if sig.Regex.MatchString(className) {
						detectionsChan <- Detection{
							SDKName:   sig.Signature.Name,
							ClassName: className,
						}
					}
				}
			}
		}()
	}

	// Send signatures to workers
	go func() {
		for _, compSig := range compiledSignatures {
			sigChan <- compSig
		}
		close(sigChan)
	}()

	// Close channel when workers finish
	go func() {
		wg.Wait()
		close(detectionsChan)
	}()

	// Collect all detections
	detectionMap := make(map[string][]string) // SDK name -> list of matching classes
	for detection := range detectionsChan {
		detectionMap[detection.SDKName] = append(detectionMap[detection.SDKName], detection.ClassName)
	}

	// Sort class names per SDK (optional for stable output)
	for sdk := range detectionMap {
		sort.Strings(detectionMap[sdk])
	}

	// Extract and sort unique SDK names
	uniqueSDKs := make([]string, 0, len(detectionMap))
	for sdk := range detectionMap {
		uniqueSDKs = append(uniqueSDKs, sdk)
	}
	sort.Strings(uniqueSDKs)

	// Print detected SDKs with all matches
	fmt.Println("\n=== Detected SDKs in app ===")
	for _, sdk := range uniqueSDKs {
		fmt.Printf("- %s\n", sdk)
		for _, className := range detectionMap[sdk] {
			fmt.Printf("  └─ %s\n", className)
		}
	}

	return detectionMap
}

// Cleanup properly cleans up Frida resources
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

	// Kill the app process
	if m.fridaData.device != nil && m.fridaData.pid > 0 {
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
