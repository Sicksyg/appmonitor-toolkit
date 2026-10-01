package ios

import (
	"AppMonitor/assets"
	"AppMonitor/helpers"
	"AppMonitor/models"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SDKSignature holds SDK signature information.
type SDKSignature struct {
	Regex       string `json:"regex"`
	DomainRegex string `json:"domain_regex"`
	Name        string `json:"name"`
	Comment     string `json:"comment"`
	Detail      string `json:"detail"`
	Website     string `json:"website"`
	Link        string `json:"link"`
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
	logger             func(message, function string)
	fridaData          *FridaData
	fridaRoot          string
	classLogPath       string
	helpers            *helpers.Manager
	sshScriptPath      string
	sshSetupScriptPath string
	trackerCommand     string
	ldidPath           string
	dittoPath          string
	libPath            string
	iproxyPath         string
	sshIdentityPath    string
	appIconCachePath   string
	sshControlPath     string
	sshMaster          *exec.Cmd
	sshMasterDone      chan error
	sshReady           chan struct{}
	sshConnected       bool
	sshMutex           sync.Mutex
}

type Paths struct {
	FridaRoot          string
	ClassLogPath       string
	Helpers            *helpers.Manager
	SSHScriptPath      string
	SSHSetupScriptPath string
	LDIDPath           string
	DittoPath          string
	LibPath            string
	IProxyPath         string
	SSHIdentityPath    string
	TrackerScanCommand string
	AppIconCachePath   string
}

func NewManager(logger func(message, function string), paths Paths) *Manager {
	return &Manager{
		logger:             logger,
		fridaRoot:          paths.FridaRoot,
		classLogPath:       paths.ClassLogPath,
		helpers:            paths.Helpers,
		sshScriptPath:      paths.SSHScriptPath,
		sshSetupScriptPath: paths.SSHSetupScriptPath,
		trackerCommand:     paths.TrackerScanCommand,
		ldidPath:           paths.LDIDPath,
		dittoPath:          paths.DittoPath,
		libPath:            paths.LibPath,
		iproxyPath:         paths.IProxyPath,
		sshIdentityPath:    paths.SSHIdentityPath,
		appIconCachePath:   paths.AppIconCachePath,
		sshReady:           make(chan struct{}),
	}
}

func (m *Manager) LoadPermissionsSignatures() []ApplePermissionSignature {
	// Load permissions signatures from the embedded assets filesystem
	// Get permissions signatures from https://github.com/Sicksyg/iOS_ProtectedResources/blob/main/ios_ProtectedResources.json
	// Return slice of PermissionSignature structs

	m.logger("Loading permissions signatures", "Manager.LoadPermissionsSignatures")
	fileData, err := assets.ReadFile("ios_permissions.json")
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

func (m *Manager) AnalyseDetectPermissions(appPermissions map[string]string) (map[string]models.IosPermissionDetail, error) {
	// Load permissions signatures from Apple
	applePermissionSignatures := m.LoadPermissionsSignatures()

	// Create a map for quick lookup: plkey -> ApplePermissionSignature
	appleSigMap := make(map[string]ApplePermissionSignature)
	for _, sig := range applePermissionSignatures {
		appleSigMap[sig.PlistKey] = sig
	}

	// Build enriched permissions map by cross-referencing with Apple signatures
	enrichedPermissions := make(map[string]models.IosPermissionDetail)

	for permKey, developerDesc := range appPermissions {
		detail := models.IosPermissionDetail{
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

// --------------------------- SDK signature matching ----------------------------- //

func (m *Manager) LoadSDKSignatures() []SDKSignature {
	// Get SDK signatures from https://github.com/Sicksyg/iOS-SDK-Signatures/blob/main/ios_signatures.json

	m.logger("Loading SDK signatures", "Manager.detectSDKs")
	fileData, err := assets.ReadFile("ios_signatures.json")
	if err != nil {
		m.logger("Error reading SDK signatures file: "+err.Error(), "Manager.LoadSDKSignatures")
		return []SDKSignature{}
	}

	// Unmarshal JSON data into slice of Signature structs
	var sdkSignatures []SDKSignature
	err = json.Unmarshal(fileData, &sdkSignatures)
	if err != nil {
		m.logger("Error unmarshaling SDK signatures: "+err.Error(), "Manager.LoadSDKSignatures")
		return []SDKSignature{}
	}

	return sdkSignatures
}

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
			m.logger("Error compiling regex: "+err.Error(), "Manager.AnalyseDetectSDKs")
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
		Detail    string
		Website   string
		Link      string
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
							Detail:    sig.Signature.Detail,
							Website:   sig.Signature.Website,
							Link:      sig.Signature.Link,
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

type trackerScanClass struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// TrackerScanDump represents the raw evidence returned by `trackerscan --dump`.
type TrackerScanDump struct {
	BundleID         string             `json:"bundleID"`
	Version          string             `json:"version"`
	RuntimeError     string             `json:"runtimeError,omitempty"`
	Classes          []trackerScanClass `json:"classes"`
	FrameworkNames   []string           `json:"frameworkNames"`
	PlistTokens      []string           `json:"plistTokens"`
	TrackingDomains  []string           `json:"trackingDomains"`
	Permissions      map[string]string  `json:"permissions"`
	BundleInfo       map[string]any     `json:"bundleInfo"`
	PrivacyManifests int                `json:"privacyManifests"`
	PrivacyTracking  bool               `json:"privacyTracking"`
}

// AnalysisOptions contains the selected app and the local inputs needed to
// install the latest version or analyze the installed copy.
type AnalysisOptions struct {
	UDID                string
	BundleID            string
	AnalyzeInstalledApp bool
	AppleEmail          string
	ApplePassword       string
	IPADirectory        string
	MinimumOSVersion    string
	TrackerScanPath     string
	Progress            func(stage, message string, percent int)
}

// AnalysisResult contains the combined iOS analysis results.
type AnalysisResult struct {
	MinimumOSVersion string
	Version          string
	TrackerScanPath  string
	TrackerWarning   error
	Classes          []string
	Permissions      map[string]models.IosPermissionDetail
	SDKs             map[string][]string
	BundleInfo       map[string]any
}

// UnsupportedMinimumOSVersionError reports apps whose declared OS requirement
// is outside the supported install-patching range.
type UnsupportedMinimumOSVersionError struct {
	Version string
}

func (e *UnsupportedMinimumOSVersionError) Error() string {
	return fmt.Sprintf("unsupported app: minimum iOS version %s is above iOS 18.x", e.Version)
}

// MinimumOSVersionFromStoreResponse extracts the iTunes lookup value from its
// JSON response, allowing selection flows to retain the value before download.
func MinimumOSVersionFromStoreResponse(response string) (string, error) {
	var result struct {
		Results []struct {
			MinimumOSVersion string `json:"minimumOsVersion"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return "", fmt.Errorf("decode App Store lookup: %w", err)
	}
	if len(result.Results) == 0 {
		return "", nil
	}
	return strings.TrimSpace(result.Results[0].MinimumOSVersion), nil
}

// StartSSH establishes the persistent SSH connection used for trackerscan.
// Connection failures are logged and remain non-fatal to the iOS analysis flow.
func (m *Manager) StartSSH(ctx context.Context) {
	if m.sshScriptPath == "" {
		m.logger("iOS SSH script path is not configured", "Manager.StartSSH")
		close(m.sshReady)
		return
	}
	go m.establishSSH(ctx)
}

func (m *Manager) establishSSH(ctx context.Context) {
	defer close(m.sshReady)
	m.sshControlPath = filepath.Join(os.TempDir(), fmt.Sprintf("am-ios-ssh-%d-%d.sock", os.Getuid(), os.Getpid()))
	if err := os.Remove(m.sshControlPath); err != nil && !os.IsNotExist(err) {
		m.logger("Unable to remove stale iOS SSH control socket: "+err.Error(), "Manager.StartSSH")
		return
	}

	keyExists := false
	if m.sshIdentityPath != "" {
		if _, err := os.Stat(m.sshIdentityPath); err == nil {
			keyExists = true
		} else if !os.IsNotExist(err) {
			m.logger("Unable to inspect AppMonitor SSH identity: "+err.Error(), "Manager.StartSSH")
			return
		}
	}
	if !keyExists {
		if err := m.openSSHSetupTerminal(ctx); err != nil {
			m.logger("Unable to start one-time SSH key setup: "+err.Error(), "Manager.StartSSH")
			return
		}
		if err := m.waitForSSHAuthorization(ctx); err != nil {
			m.logger("AppMonitor SSH key setup did not complete: "+err.Error(), "Manager.StartSSH")
			return
		}
	} else {
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := m.probeSSHIdentity(probeCtx)
		cancel()
		if err != nil {
			if !strings.Contains(err.Error(), "Permission denied") &&
				!strings.Contains(err.Error(), "Host key verification failed") &&
				!strings.Contains(err.Error(), "Load key") &&
				!strings.Contains(err.Error(), "agent refused operation") {
				m.logger("Unable to check AppMonitor SSH key authentication: "+err.Error(), "Manager.StartSSH")
				return
			}
			if err := m.openSSHSetupTerminal(ctx); err != nil {
				m.logger("Unable to start one-time SSH key setup: "+err.Error(), "Manager.StartSSH")
				return
			}
			if err := m.waitForSSHAuthorization(ctx); err != nil {
				m.logger("AppMonitor SSH key setup did not complete: "+err.Error(), "Manager.StartSSH")
				return
			}
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := m.startPersistentSSH(ctx, connectCtx); err != nil {
		m.logger("Unable to establish persistent iOS SSH connection: "+err.Error(), "Manager.StartSSH")
	}
}

func (m *Manager) probeSSHIdentity(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, m.sshScriptPath, "true")
	cmd.Env = m.sshEnvironment(false, "")
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
}

func (m *Manager) openSSHSetupTerminal(ctx context.Context) error {
	if m.sshSetupScriptPath == "" {
		return fmt.Errorf("SSH setup script path is not configured")
	}
	osascript, err := exec.LookPath("/usr/bin/osascript")
	if err != nil {
		return fmt.Errorf("AppleScript is unavailable: %w", err)
	}
	command := "APPMONITOR_IOS_SSH_IDENTITY_FILE=" + shellQuote(m.sshIdentityPath) +
		" APPMONITOR_IOS_SSH_SCRIPT=" + shellQuote(m.sshScriptPath) +
		" APPMONITOR_IPROXY_PATH=" + shellQuote(m.iproxyPath) +
		" /bin/bash " + shellQuote(m.sshSetupScriptPath)
	appleScript := "tell application \"Terminal\" to do script \"" +
		strings.ReplaceAll(strings.ReplaceAll(command, `\`, `\\`), `"`, `\"`) + "\"\ntell application \"Terminal\" to activate"
	cmd := exec.CommandContext(ctx, osascript, "-e", appleScript)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("open Terminal for one-time SSH setup: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) waitForSSHAuthorization(ctx context.Context) error {
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := m.probeSSHIdentity(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for SSH authorization in Terminal")
		case <-ticker.C:
		}
	}
}

func (m *Manager) startPersistentSSH(parentCtx, connectCtx context.Context) error {
	master := exec.CommandContext(parentCtx, m.sshScriptPath)
	master.Env = m.sshEnvironment(true, "")
	var masterOutput bytes.Buffer
	master.Stderr = &masterOutput
	if err := master.Start(); err != nil {
		return fmt.Errorf("start SSH control master: %w", err)
	}

	done := make(chan error, 1)
	m.sshMutex.Lock()
	m.sshMaster = master
	m.sshMasterDone = done
	m.sshMutex.Unlock()
	go func() {
		err := master.Wait()
		m.sshMutex.Lock()
		if m.sshMaster == master {
			m.sshMaster = nil
			m.sshConnected = false
		}
		m.sshMutex.Unlock()
		done <- err
	}()

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		checkCtx, checkCancel := context.WithTimeout(connectCtx, 2*time.Second)
		check := exec.CommandContext(checkCtx, m.sshScriptPath)
		check.Env = m.sshEnvironment(false, "check")
		output, checkErr := check.CombinedOutput()
		checkCancel()
		if checkErr == nil {
			m.sshMutex.Lock()
			connected := m.sshMaster == master
			if connected {
				m.sshConnected = true
			}
			m.sshMutex.Unlock()
			if connected {
				m.logger("Persistent iOS SSH connection established", "Manager.StartSSH")
				return nil
			}
			processErr := <-done
			return fmt.Errorf("SSH control master exited during setup: %v: %s", processErr, strings.TrimSpace(masterOutput.String()))
		}
		select {
		case processErr := <-done:
			detail := strings.TrimSpace(masterOutput.String())
			if detail == "" {
				detail = strings.TrimSpace(string(output))
			}
			return fmt.Errorf("SSH control master exited before setup completed: %v: %s", processErr, detail)
		case <-connectCtx.Done():
			if err := master.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				m.logger("Unable to stop failed iOS SSH connection attempt: "+err.Error(), "Manager.StartSSH")
			}
			return fmt.Errorf("wait for SSH control master: %w: %s", connectCtx.Err(), strings.TrimSpace(masterOutput.String()))
		case <-ticker.C:
		}
	}
}

// StopSSH closes the app-owned SSH ControlMaster at shutdown.
func (m *Manager) StopSSH() {
	m.sshMutex.Lock()
	master := m.sshMaster
	done := m.sshMasterDone
	m.sshMutex.Unlock()
	if master == nil {
		if m.sshControlPath != "" {
			if err := os.Remove(m.sshControlPath); err != nil && !os.IsNotExist(err) {
				m.logger("Unable to remove stale iOS SSH control socket: "+err.Error(), "Manager.StopSSH")
			}
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop := exec.CommandContext(ctx, m.sshScriptPath)
	stop.Env = m.sshEnvironment(false, "exit")
	if output, err := stop.CombinedOutput(); err != nil {
		m.logger(fmt.Sprintf("Unable to close persistent iOS SSH connection: %v: %s", err, strings.TrimSpace(string(output))), "Manager.StopSSH")
	}
	select {
	case err := <-done:
		if err != nil {
			m.logger("Persistent iOS SSH connection stopped: "+err.Error(), "Manager.StopSSH")
		}
	case <-ctx.Done():
		if err := master.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			m.logger("Unable to terminate persistent iOS SSH process: "+err.Error(), "Manager.StopSSH")
		}
	}
	if err := os.Remove(m.sshControlPath); err != nil && !os.IsNotExist(err) {
		m.logger("Unable to remove iOS SSH control socket: "+err.Error(), "Manager.StopSSH")
	}
}

func (m *Manager) sshEnvironment(master bool, controlAction string) []string {
	env := append(os.Environ(), "APPMONITOR_IOS_SSH_CONTROL_PATH="+m.sshControlPath)
	if m.iproxyPath != "" {
		env = append(env, "APPMONITOR_IPROXY_PATH="+m.iproxyPath)
	}
	if m.sshIdentityPath != "" {
		env = append(env, "APPMONITOR_IOS_SSH_IDENTITY_FILE="+m.sshIdentityPath)
	}
	env = append(env, "APPMONITOR_IOS_SSH_BATCH=1")
	if master {
		env = append(env, "APPMONITOR_IOS_SSH_MASTER=1")
	}
	if controlAction != "" {
		env = append(env, "APPMONITOR_IOS_SSH_CONTROL_ACTION="+controlAction)
	}
	return env
}

// RunAppAnalysis installs the latest app when requested, runs trackerscan, and
// optionally follows with the retained Frida analysis.
func (m *Manager) RunAppAnalysis(ctx context.Context, options AnalysisOptions) (AnalysisResult, error) {
	if m.helpers == nil {
		return AnalysisResult{}, fmt.Errorf("iOS helper manager is not configured")
	}
	if options.BundleID == "" {
		return AnalysisResult{}, fmt.Errorf("iOS bundle ID is required")
	}
	progress := func(stage, message string, percent int) {
		if options.Progress != nil {
			options.Progress(stage, message, percent)
		}
	}
	minimumOSVersion := strings.TrimSpace(options.MinimumOSVersion)
	if !options.AnalyzeInstalledApp {
		progress("download", "Downloading app", 10)
		if err := m.helpers.DownloadApp(options.BundleID, options.AppleEmail, options.ApplePassword, options.IPADirectory); err != nil {
			return AnalysisResult{}, fmt.Errorf("unable to download %s: %w", options.BundleID, err)
		}

		ipaPath := filepath.Join(options.IPADirectory, options.BundleID+".ipa")
		if minimumOSVersion == "" {
			return AnalysisResult{}, fmt.Errorf("unable to determine the app's MinimumOSVersion from the App Store lookup")
		}

		needsPatch, unsupported, err := MinimumOSVersionPolicy(minimumOSVersion)
		if err != nil {
			return AnalysisResult{}, err
		}
		if unsupported {
			return AnalysisResult{}, &UnsupportedMinimumOSVersionError{Version: minimumOSVersion}
		}

		progress("install", "Preparing app for installation", 25)
		installIPAPath := ipaPath
		var prepared IPAResult
		if needsPatch {
			prepared, err = PrepareIPA(ctx, ipaPath, ipaOutputPath(options.IPADirectory, options.BundleID, "-patched"), "16.0", false, m.ipaTools())
			if err != nil {
				return AnalysisResult{}, fmt.Errorf("prepare %s for installation: %w", options.BundleID, err)
			}
			installIPAPath = prepared.OutputPath
			if prepared.Changed {
				m.logger("Prepared compatible IPA at "+prepared.OutputPath, "Manager.RunAppAnalysis")
			}
			if prepared.HasIncompatibleExtensions {
				m.logger("IPA contains extension points outside the compatibility allowlist", "Manager.RunAppAnalysis")
			}
		}
		progress("install", "Installing latest app", 25)
		if err := m.helpers.InstallApp(options.UDID, installIPAPath); err != nil {
			if !needsPatch || !prepared.HasIncompatibleExtensions {
				return AnalysisResult{}, fmt.Errorf("install %s: %w", options.BundleID, err)
			}
			m.logger("Initial install failed; retrying once with incompatible extensions pruned: "+err.Error(), "Manager.RunAppAnalysis")
			progress("install_retry", "Retrying installation without incompatible extensions", 30)
			pruned, pruneErr := PrepareIPA(ctx, ipaPath, ipaOutputPath(options.IPADirectory, options.BundleID, "-pruned"), "16.0", true, m.ipaTools())
			if pruneErr != nil {
				return AnalysisResult{}, fmt.Errorf("install %s failed (%v), and pruning extensions failed: %w", options.BundleID, err, pruneErr)
			}
			if !pruned.Changed {
				return AnalysisResult{}, fmt.Errorf("install %s failed (%v), but no incompatible extensions could be pruned", options.BundleID, err)
			}
			m.logger("Prepared pruned IPA at "+pruned.OutputPath, "Manager.RunAppAnalysis")
			if retryErr := m.helpers.InstallApp(options.UDID, pruned.OutputPath); retryErr != nil {
				return AnalysisResult{}, fmt.Errorf("install %s after pruning extensions (initial attempt: %v): %w", options.BundleID, err, retryErr)
			}
		}
	}

	if !options.AnalyzeInstalledApp {
		defer func() {
			if err := m.Cleanup(); err != nil {
				m.logger("Frida cleanup warning: "+err.Error(), "Manager.RunAppAnalysis")
			}
		}()
	}
	result := AnalysisResult{MinimumOSVersion: minimumOSVersion}
	progress("trackerscan", "Running on-device trackerscan", 35)
	m.logger("Starting on-device trackerscan for "+options.BundleID, "Manager.RunAppAnalysis")
	dump, scanErr := m.runTrackerScan(ctx, options.BundleID, options.TrackerScanPath)
	if scanErr != nil {
		result.TrackerWarning = scanErr
		m.logger("Trackerscan warning: "+scanErr.Error(), "Manager.RunAppAnalysis")
		progress("trackerscan_warning", "Trackerscan warning: "+scanErr.Error(), 40)
	} else {
		result.TrackerScanPath = options.TrackerScanPath
		m.logger("Trackerscan returned JSON for "+options.BundleID+"; saved to "+options.TrackerScanPath, "Manager.RunAppAnalysis")
		if dump.RuntimeError != "" {
			result.TrackerWarning = fmt.Errorf("runtime scan was incomplete: %s", dump.RuntimeError)
			m.logger("Trackerscan warning: "+result.TrackerWarning.Error(), "Manager.RunAppAnalysis")
			progress("trackerscan_warning", "Trackerscan warning: "+result.TrackerWarning.Error(), 40)
		}
	}

	if scanErr == nil {
		result.Version = dump.Version
		result.Classes = make([]string, 0, len(dump.Classes))
		for _, class := range dump.Classes {
			if class.Name != "" {
				result.Classes = append(result.Classes, class.Name)
			}
		}
		result.SDKs = mergeSDKMaps(result.SDKs, m.detectTrackerSDKs(dump))
		result.Permissions = mergePermissions(result.Permissions, m.enrichTrackerPermissions(dump.Permissions))
		result.BundleInfo = mergeBundleInfo(result.BundleInfo, dump.BundleInfo)
	}
	if options.AnalyzeInstalledApp {
		progress("analysis_complete", "On-device analysis complete", 75)
		return result, nil
	}
	progress("frida", "Running Frida analysis", 45)
	fridaPermissions, fridaSDKs, fridaBundleInfo, err := m.RunCompleteAnalysis(options.UDID, options.BundleID)
	if err != nil {
		return result, fmt.Errorf("Frida analysis failed: %w", err)
	}
	result.Permissions = mergePermissions(result.Permissions, fridaPermissions)
	result.SDKs = mergeSDKMaps(result.SDKs, fridaSDKs)
	result.BundleInfo = mergeBundleInfo(result.BundleInfo, fridaBundleInfo)
	return result, nil
}

func (m *Manager) runTrackerScan(ctx context.Context, bundleID, outputPath string) (TrackerScanDump, error) {
	if !validBundleID(bundleID) {
		return TrackerScanDump{}, fmt.Errorf("invalid iOS bundle ID %q", bundleID)
	}
	if err := m.waitForSSH(ctx); err != nil {
		return TrackerScanDump{}, err
	}

	command := m.trackerCommand
	if command == "" {
		command = "am_scanner"
	}
	remoteCommand := trackerScanRemoteCommand(command, bundleID)
	scanCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(scanCtx, m.sshScriptPath, remoteCommand)
	cmd.Env = m.sshEnvironment(false, "")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return TrackerScanDump{}, fmt.Errorf("run trackerscan over SSH: %w: %s", err, detail)
		}
		return TrackerScanDump{}, fmt.Errorf("run trackerscan over SSH: %w", err)
	}
	if outputPath != "" {
		if err := os.WriteFile(outputPath, stdout, 0644); err != nil {
			return TrackerScanDump{}, fmt.Errorf("save trackerscan JSON: %w", err)
		}
	}
	dump, err := DecodeTrackerScanDump(stdout)
	if err != nil {
		return TrackerScanDump{}, err
	}
	if dump.BundleID != bundleID {
		return TrackerScanDump{}, fmt.Errorf("trackerscan returned bundle ID %q, expected %q", dump.BundleID, bundleID)
	}
	return dump, nil
}

// OpenURL opens a URL on the connected iOS device using the SSH helper.
func (m *Manager) OpenURL(ctx context.Context, targetURL string) error {
	if strings.TrimSpace(targetURL) == "" {
		return fmt.Errorf("URL is required")
	}
	if m.sshScriptPath == "" {
		return fmt.Errorf("iOS SSH script path is not configured")
	}
	if err := m.waitForSSH(ctx); err != nil {
		return err
	}
	remoteCommand := `export PATH="/var/jb/usr/local/bin:/var/jb/usr/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"; exec uiopen ` +
		shellQuote(targetURL)
	cmd := exec.CommandContext(ctx, m.sshScriptPath, remoteCommand)
	cmd.Env = m.sshEnvironment(false, "")
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("open URL on iOS device over SSH: %w: %s", err, detail)
		}
		return fmt.Errorf("open URL on iOS device over SSH: %w", err)
	}
	return nil
}

func (m *Manager) waitForSSH(ctx context.Context) error {
	if m.sshReady != nil {
		select {
		case <-m.sshReady:
		case <-ctx.Done():
			return fmt.Errorf("wait for persistent iOS SSH connection: %w", ctx.Err())
		}
	}
	m.sshMutex.Lock()
	connected := m.sshConnected
	m.sshMutex.Unlock()
	if !connected {
		return fmt.Errorf("persistent iOS SSH connection is unavailable")
	}
	return nil
}

// ListInstalledApps retrieves metadata and icon data from trackerscan over the
// persistent iOS SSH connection, then caches decoded icons for reports.
func (m *Manager) ListInstalledApps(ctx context.Context) ([]helpers.InstalledApp, error) {
	if m.sshScriptPath == "" {
		return nil, fmt.Errorf("iOS SSH script path is not configured")
	}
	if err := m.waitForSSH(ctx); err != nil {
		return nil, err
	}
	command := m.trackerCommand
	if command == "" {
		command = "am_scanner"
	}
	remoteCommand := trackerScanListRemoteCommand(command)
	scanCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(scanCtx, m.sshScriptPath, remoteCommand)
	cmd.Env = m.sshEnvironment(false, "")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("list installed apps with trackerscan over SSH: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("list installed apps with trackerscan over SSH: %w", err)
	}
	apps, err := DecodeTrackerScanAppList(stdout)
	if err != nil {
		return nil, err
	}
	for index := range apps {
		if apps[index].IconDataURI == "" {
			continue
		}
		iconBytes, mimeType, err := parseIconDataURI(apps[index].IconDataURI)
		if err != nil {
			return nil, fmt.Errorf("decode icon for %s: %w", apps[index].CFBundleIdentifier, err)
		}
		if m.appIconCachePath == "" {
			return nil, fmt.Errorf("iOS app icon cache path is not configured")
		}
		if err := os.MkdirAll(m.appIconCachePath, 0755); err != nil {
			return nil, fmt.Errorf("create iOS app icon cache: %w", err)
		}
		extension := map[string]string{
			"image/png":  ".png",
			"image/jpeg": ".jpg",
			"image/gif":  ".gif",
			"image/webp": ".webp",
		}[mimeType]
		cacheKey := sha256.Sum256([]byte(apps[index].CFBundleIdentifier + "\x00" + apps[index].Version))
		iconPath := filepath.Join(m.appIconCachePath, fmt.Sprintf("%x%s", cacheKey, extension))
		if err := os.WriteFile(iconPath, iconBytes, 0644); err != nil {
			return nil, fmt.Errorf("cache icon for %s: %w", apps[index].CFBundleIdentifier, err)
		}
		apps[index].IconPath = iconPath
	}
	return apps, nil
}

func trackerScanRemoteCommand(command, bundleID string) string {
	return `export PATH="/var/jb/usr/local/bin:/var/jb/usr/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"; exec ` +
		shellQuote(command) + " --dump " + shellQuote(bundleID)
}

func trackerScanListRemoteCommand(command string) string {
	return `export PATH="/var/jb/usr/local/bin:/var/jb/usr/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"; exec ` +
		shellQuote(command) + " --list --json"
}

// DecodeTrackerScanAppList validates trackerscan's --list --json output and
// converts icon bytes into data URLs suitable for the Wails frontend.
func DecodeTrackerScanAppList(data []byte) ([]helpers.InstalledApp, error) {
	var response struct {
		Apps []struct {
			BundleID   string `json:"bundleID"`
			Name       string `json:"name"`
			Version    string `json:"version"`
			IconData   string `json:"iconData"`
			IconFormat string `json:"iconFormat"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode trackerscan app list JSON: %w", err)
	}
	if response.Apps == nil {
		return nil, fmt.Errorf("trackerscan app list JSON is missing apps")
	}
	apps := make([]helpers.InstalledApp, 0, len(response.Apps))
	for index, app := range response.Apps {
		if !validBundleID(app.BundleID) || strings.TrimSpace(app.Name) == "" {
			return nil, fmt.Errorf("trackerscan app list entry %d has invalid bundleID or name", index)
		}
		installed := helpers.InstalledApp{
			CFBundleIdentifier:  app.BundleID,
			CFBundleDisplayName: app.Name,
			Version:             app.Version,
		}
		if app.IconData != "" {
			iconData, err := base64.StdEncoding.DecodeString(app.IconData)
			if err != nil {
				return nil, fmt.Errorf("decode base64 icon for %s: %w", app.BundleID, err)
			}
			mimeType, _, err := validateTrackerIcon(iconData, app.IconFormat)
			if err != nil {
				return nil, fmt.Errorf("validate icon for %s: %w", app.BundleID, err)
			}
			installed.IconDataURI = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(iconData)
		}
		apps = append(apps, installed)
	}
	return apps, nil
}

func validateTrackerIcon(data []byte, format string) (mimeType, extension string, err error) {
	switch strings.ToLower(format) {
	case "png":
		if len(data) < 8 || !bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
			return "", "", fmt.Errorf("iconFormat is png but image signature is invalid")
		}
		return "image/png", "png", nil
	case "jpeg", "jpg":
		if len(data) < 3 || !bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
			return "", "", fmt.Errorf("iconFormat is jpeg but image signature is invalid")
		}
		return "image/jpeg", "jpg", nil
	case "gif":
		if len(data) < 6 || (!bytes.Equal(data[:6], []byte("GIF87a")) && !bytes.Equal(data[:6], []byte("GIF89a"))) {
			return "", "", fmt.Errorf("iconFormat is gif but image signature is invalid")
		}
		return "image/gif", "gif", nil
	case "webp":
		if len(data) < 12 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
			return "", "", fmt.Errorf("iconFormat is webp but image signature is invalid")
		}
		return "image/webp", "webp", nil
	default:
		return "", "", fmt.Errorf("unsupported icon format %q", format)
	}
}

func parseIconDataURI(uri string) ([]byte, string, error) {
	const prefix = "data:"
	if !strings.HasPrefix(uri, prefix) {
		return nil, "", fmt.Errorf("icon data URL is malformed")
	}
	metadata, encoded, ok := strings.Cut(strings.TrimPrefix(uri, prefix), ",")
	if !ok || !strings.HasSuffix(metadata, ";base64") {
		return nil, "", fmt.Errorf("icon data URL is not base64 encoded")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("decode icon data URL: %w", err)
	}
	return data, strings.TrimSuffix(metadata, ";base64"), nil
}

func (m *Manager) detectTrackerSDKs(dump TrackerScanDump) map[string][]string {
	evidence := make(map[string]struct{}, len(dump.Classes)+len(dump.FrameworkNames))
	for _, class := range dump.Classes {
		if class.Name != "" {
			evidence[class.Name] = struct{}{}
		}
	}
	for _, framework := range dump.FrameworkNames {
		if framework != "" {
			evidence[framework] = struct{}{}
		}
	}
	names := make([]string, 0, len(evidence))
	for name := range evidence {
		names = append(names, name)
	}
	sort.Strings(names)
	return m.AnalyseDetectSDKs(names)
}

func (m *Manager) enrichTrackerPermissions(raw map[string]string) map[string]models.IosPermissionDetail {
	if len(raw) == 0 {
		return nil
	}
	permissions, err := m.AnalyseDetectPermissions(raw)
	if err != nil {
		m.logger("Unable to enrich trackerscan permissions: "+err.Error(), "Manager.RunAppAnalysis")
		return nil
	}
	return permissions
}

func mergeSDKMaps(primary, additional map[string][]string) map[string][]string {
	merged := make(map[string][]string, len(primary)+len(additional))
	for sdk, classes := range primary {
		merged[sdk] = append([]string(nil), classes...)
	}
	for sdk, classes := range additional {
		seen := make(map[string]struct{}, len(merged[sdk]))
		for _, class := range merged[sdk] {
			seen[class] = struct{}{}
		}
		for _, class := range classes {
			if _, exists := seen[class]; exists {
				continue
			}
			merged[sdk] = append(merged[sdk], class)
			seen[class] = struct{}{}
		}
		sort.Strings(merged[sdk])
	}
	return merged
}

func mergePermissions(primary, additional map[string]models.IosPermissionDetail) map[string]models.IosPermissionDetail {
	merged := make(map[string]models.IosPermissionDetail, len(primary)+len(additional))
	for key, value := range primary {
		merged[key] = value
	}
	for key, value := range additional {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}
	return merged
}

func mergeBundleInfo(primary, additional map[string]any) map[string]any {
	merged := make(map[string]any, len(primary)+len(additional))
	for key, value := range primary {
		merged[key] = value
	}
	for key, value := range additional {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}
	return merged
}

func validBundleID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// MinimumOSVersionPolicy classifies an app's declared minimum iOS version.
func MinimumOSVersionPolicy(version string) (needsPatch, unsupported bool, err error) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return false, false, fmt.Errorf("invalid MinimumOSVersion %q", version)
	}
	values := make([]int, 3)
	for index, part := range parts {
		if part == "" {
			return false, false, fmt.Errorf("invalid MinimumOSVersion %q", version)
		}
		values[index], err = strconv.Atoi(part)
		if err != nil || values[index] < 0 {
			return false, false, fmt.Errorf("invalid MinimumOSVersion %q", version)
		}
	}
	unsupported = values[0] > 18
	needsPatch = !unsupported && values[0] >= 16
	return needsPatch, unsupported, nil
}

// DecodeTrackerScanDump decodes and validates a trackerscan JSON dump.
func DecodeTrackerScanDump(data []byte) (TrackerScanDump, error) {
	var dump TrackerScanDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return TrackerScanDump{}, fmt.Errorf("decode trackerscan JSON: %w", err)
	}
	if dump.BundleID == "" {
		return TrackerScanDump{}, fmt.Errorf("trackerscan JSON is missing bundleID")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return TrackerScanDump{}, fmt.Errorf("decode trackerscan JSON fields: %w", err)
	}
	for _, field := range []string{"classes", "frameworkNames", "plistTokens"} {
		if _, exists := fields[field]; !exists {
			return TrackerScanDump{}, fmt.Errorf("trackerscan JSON is missing %s", field)
		}
	}
	return dump, nil
}
